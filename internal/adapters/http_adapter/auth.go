package httpadapter

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type contextKey string

const SubjectContextKey contextKey = "subject"
const AuthClaimsContextKey contextKey = "auth_claims"

var oauth2InitOnce sync.Once

// AuthClaims are Keycloak JWT claims used for ensure-user and handlers.
type AuthClaims struct {
	Sub               string // Keycloak user ID (UUID string)
	PreferredUsername string
	Email             string
	GivenName         string
	FamilyName        string
}

// AuthMiddleware validates Keycloak JWT and sets subject + full claims in context. If issuer is empty, no auth.
// Optional jwksURI: when set (e.g. in-cluster Keycloak URL), verifier uses it to fetch keys so token issuer can
// differ (e.g. token has iss=http://localhost:8080/realms/aggro, keys from keycloak.infra.svc).
func AuthMiddleware(issuer, jwksURI string, next http.Handler) http.Handler {
	if issuer == "" {
		return next
	}
	var (
		mu       sync.Mutex
		verifier *oidc.IDTokenVerifier
	)
	oauth2InitOnce.Do(func() { _ = oauth2.NewClient(context.Background(), nil) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let CORS middleware answer preflight; if CORS is misconfigured, avoid 401 on OPTIONS.
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// S3 proxy is public (PMTiles served to map renderer without auth)
		if strings.HasPrefix(r.URL.Path, "/s3/") {
			next.ServeHTTP(w, r)
			return
		}
		// Health checks and OpenAPI spec (codegen / docs) are unauthenticated
		if r.URL.Path == "/healthz" || r.URL.Path == "/openapi.yaml" {
			next.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		v := verifier
		mu.Unlock()
		if v == nil {
			oidcConfig := &oidc.Config{SkipClientIDCheck: true}
			if jwksURI != "" {
				keySet := oidc.NewRemoteKeySet(r.Context(), jwksURI)
				v = oidc.NewVerifier(issuer, keySet, oidcConfig)
			} else {
				provider, err := oidc.NewProvider(r.Context(), issuer)
				if err != nil {
					slog.Warn("oidc provider (will retry on next request)", "issuer", issuer, "err", err)
					http.Error(w, `{"error":"auth not ready"}`, http.StatusServiceUnavailable)
					return
				}
				v = provider.Verifier(oidcConfig)
			}
			mu.Lock()
			verifier = v
			mu.Unlock()
		}
		token := bearerToken(r)
		if token == "" {
			http.Error(w, `{"error":"missing or invalid authorization"}`, http.StatusUnauthorized)
			return
		}
		idToken, err := v.Verify(r.Context(), token)
		if err != nil {
			slog.Warn("token verification failed", "err", err)
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		var claims struct {
			Sub               string `json:"sub"`
			PreferredUsername string `json:"preferred_username"`
			Email             string `json:"email"`
			GivenName         string `json:"given_name"`
			FamilyName        string `json:"family_name"`
		}
		if err := idToken.Claims(&claims); err != nil {
			http.Error(w, `{"error":"invalid claims"}`, http.StatusUnauthorized)
			return
		}
		authClaims := AuthClaims{
			Sub:               claims.Sub,
			PreferredUsername: claims.PreferredUsername,
			Email:             claims.Email,
			GivenName:         claims.GivenName,
			FamilyName:        claims.FamilyName,
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, SubjectContextKey, claims.Sub)
		ctx = context.WithValue(ctx, AuthClaimsContextKey, authClaims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func shortUserSuffix(id uuid.UUID) string {
	s := strings.ReplaceAll(id.String(), "-", "")
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// EnsureUserMiddleware creates the user in the backend DB on first authenticated request (after Keycloak login/register).
func EnsureUserMiddleware(userRepo ports.UserRepository, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := AuthClaimsFromContext(r.Context())
		if claims == nil || claims.Sub == "" {
			next.ServeHTTP(w, r)
			return
		}
		userID, err := uuid.Parse(claims.Sub)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		u, err := userRepo.GetByID(r.Context(), userID)
		if err != nil {
			slog.Error("ensure user get", "sub", claims.Sub, "err", err)
			http.Error(w, `{"error":"user lookup failed"}`, http.StatusInternalServerError)
			return
		}
		if u != nil {
			next.ServeHTTP(w, r)
			return
		}
		username := claims.PreferredUsername
		if username == "" {
			username = claims.Email
		}
		if username == "" {
			username = claims.Sub
		}
		if existingByUsername, err := userRepo.GetByUsername(r.Context(), username); err != nil {
			slog.Error("ensure user get by username", "sub", claims.Sub, "username", username, "err", err)
			http.Error(w, `{"error":"user lookup failed"}`, http.StatusInternalServerError)
			return
		} else if existingByUsername != nil && existingByUsername.ID != userID {
			username = username + "-" + shortUserSuffix(userID)
		}
		givenName, familyName := claims.GivenName, claims.FamilyName
		if givenName == "" {
			givenName = username
		}
		if familyName == "" {
			familyName = " "
		}
		newUser := &domain.User{
			ID:        userID,
			Username:  username,
			Firstname: givenName,
			LastName:  familyName,
			Email:     claims.Email,
		}
		if newUser.Email == "" {
			newUser.Email = claims.PreferredUsername + "@keycloak"
		}
		if existingByEmail, err := userRepo.GetByEmail(r.Context(), newUser.Email); err != nil {
			slog.Error("ensure user get by email", "sub", claims.Sub, "email", newUser.Email, "err", err)
			http.Error(w, `{"error":"user lookup failed"}`, http.StatusInternalServerError)
			return
		} else if existingByEmail != nil && existingByEmail.ID != userID {
			newUser.Email = userID.String() + "@keycloak.local"
		}
		if err := userRepo.Upsert(r.Context(), newUser); err != nil {
			slog.Error("ensure user upsert", "sub", claims.Sub, "err", err)
			http.Error(w, `{"error":"could not provision local user"}`, http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	s := r.Header.Get("Authorization")
	if s == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(s, prefix) {
		return ""
	}
	return strings.TrimSpace(s[len(prefix):])
}

// SubjectFromContext returns the JWT subject (user id) from request context, or "".
func SubjectFromContext(ctx context.Context) string {
	v, _ := ctx.Value(SubjectContextKey).(string)
	return v
}

// AuthClaimsFromContext returns the full auth claims from request context, or nil.
func AuthClaimsFromContext(ctx context.Context) *AuthClaims {
	v, _ := ctx.Value(AuthClaimsContextKey).(AuthClaims)
	if v.Sub == "" {
		return nil
	}
	return &v
}
