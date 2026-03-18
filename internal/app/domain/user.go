package domain

import "github.com/google/uuid"

type User struct {
	ID        uuid.UUID
	Username  string
	Firstname string
	LastName  string
	Email     string
}

type UserRole string

const (
	UserRoleAdmin   UserRole = "admin"
	UserRoleManager UserRole = "manager"
	UserRoleFarmer  UserRole = "farmer"
	UserRoleViewer  UserRole = "viewer"
)
