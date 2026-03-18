// Package temporalworkflows reads Temporal visibility for field processing workflows.
package temporalworkflows

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"github.com/google/uuid"
)

// WorkflowTypeFieldProcessing must match Python @workflow.defn class name.
const WorkflowTypeFieldProcessing = "FieldProcessingWorkflow"

// FieldWorkflowID is the canonical workflow id when starting from run_field_workflow / API.
func FieldWorkflowID(fieldID uuid.UUID) string {
	return fmt.Sprintf("field-%s", fieldID.String())
}

// FieldRunSummary is a lightweight status for one execution (no history payload).
type FieldRunSummary struct {
	WorkflowID   string   `json:"workflow_id"`
	RunID        string   `json:"run_id"`
	Status       string   `json:"status"`
	Stage        string   `json:"stage"`
	StageLabel   string   `json:"stage_label"`
	ActiveStages []string `json:"active_stages,omitempty"`
	StartedAt    *string  `json:"started_at,omitempty"`
	ClosedAt     *string  `json:"closed_at,omitempty"`
}

var stageLabels = map[string]string{
	"cut_tiles":                  "Cutting field into tiles",
	"geo_metrics":                "Satellite / GEE metrics",
	"ml_analytics":               "ML analytics per tile",
	"observed_pmtiles_build":     "Building observed PMTiles",
	"observed_pmtiles_upload":    "Uploading observed PMTiles",
	"predicted_pmtiles_build":    "Building predicted PMTiles",
	"predicted_pmtiles_upload":   "Uploading predicted PMTiles",
	"pmtiles_urls":               "Building PMTiles URLs",
	"finalize_db":                "Saving results to database",
	"completed":                  "Completed",
	"failed":                     "Failed",
	"terminated":                 "Terminated",
	"canceled":                   "Canceled",
	"timed_out":                  "Timed out",
	"continued_as_new":           "Continued as new",
	"running":                    "Running",
	"parallel":                   "Several steps in parallel",
	"between_steps":              "Transitioning between steps",
	"other":                      "In progress",
}

func activityNameToStage(name string) string {
	switch name {
	case "cut_into_tiles":
		return "cut_tiles"
	case "request_tile_gee_metrics":
		return "geo_metrics"
	case "run_ml_analytics_batch":
		return "ml_analytics"
	case "build_observed_pmtiles_activity":
		return "observed_pmtiles_build"
	case "upload_observed_pmtiles_activity":
		return "observed_pmtiles_upload"
	case "build_predicted_pmtiles_activity":
		return "predicted_pmtiles_build"
	case "upload_predicted_pmtiles_activity":
		return "predicted_pmtiles_upload"
	case "build_pmtiles_urls_activity":
		return "pmtiles_urls"
	case "finalize_field_processing_db":
		return "finalize_db"
	default:
		return "other"
	}
}

func inferStageFromPending(pending []*workflowpb.PendingActivityInfo) (stage, label string, active []string) {
	if len(pending) == 0 {
		return "between_steps", stageLabels["between_steps"], nil
	}
	seen := make(map[string]struct{})
	for _, p := range pending {
		if p == nil || p.ActivityType == nil {
			continue
		}
		code := activityNameToStage(p.ActivityType.Name)
		if _, ok := seen[code]; !ok {
			seen[code] = struct{}{}
			active = append(active, code)
		}
	}
	sort.Strings(active)
	if len(active) == 0 {
		return "running", stageLabels["running"], nil
	}
	if len(active) == 1 {
		c := active[0]
		l := stageLabels[c]
		if l == "" {
			l = c
		}
		return c, l, active
	}
	return "parallel", stageLabels["parallel"], active
}

// Lister uses Temporal visibility API (same namespace as workers).
type Lister struct {
	Client    client.Client
	Namespace string
}

// ListFieldProcessingRuns returns executions for workflow id `field-{fieldId}` and type FieldProcessingWorkflow.
func (l *Lister) ListFieldProcessingRuns(ctx context.Context, fieldID uuid.UUID) ([]FieldRunSummary, error) {
	if l.Client == nil {
		return nil, fmt.Errorf("temporal client not configured")
	}
	wid := FieldWorkflowID(fieldID)
	q := fmt.Sprintf(`WorkflowType = "%s" AND WorkflowId = "%s"`, WorkflowTypeFieldProcessing, wid)

	var executions []*workflowpb.WorkflowExecutionInfo
	var token []byte
	for {
		resp, err := l.Client.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
			Namespace:     l.Namespace,
			PageSize:      50,
			Query:         q,
			NextPageToken: token,
		})
		if err != nil {
			return nil, err
		}
		executions = append(executions, resp.Executions...)
		token = resp.NextPageToken
		if len(token) == 0 {
			break
		}
	}

	sort.Slice(executions, func(i, j int) bool {
		return executions[i].GetStartTime().AsTime().After(executions[j].GetStartTime().AsTime())
	})

	out := make([]FieldRunSummary, 0, len(executions))
	for _, ex := range executions {
		if ex == nil || ex.Execution == nil {
			continue
		}
		sum := FieldRunSummary{
			WorkflowID: ex.Execution.WorkflowId,
			RunID:      ex.Execution.RunId,
			Status:     strings.TrimPrefix(ex.Status.String(), "WORKFLOW_EXECUTION_STATUS_"),
		}
		if ex.StartTime != nil {
			t := ex.StartTime.AsTime().UTC().Format(time.RFC3339)
			sum.StartedAt = &t
		}
		if ex.CloseTime != nil {
			t := ex.CloseTime.AsTime().UTC().Format(time.RFC3339)
			sum.ClosedAt = &t
		}

		switch ex.Status {
		case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
			sum.Stage = "completed"
			sum.StageLabel = stageLabels["completed"]
		case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
			sum.Stage = "failed"
			sum.StageLabel = stageLabels["failed"]
		case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
			sum.Stage = "terminated"
			sum.StageLabel = stageLabels["terminated"]
		case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
			sum.Stage = "canceled"
			sum.StageLabel = stageLabels["canceled"]
		case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
			sum.Stage = "timed_out"
			sum.StageLabel = stageLabels["timed_out"]
		case enumspb.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
			sum.Stage = "continued_as_new"
			sum.StageLabel = stageLabels["continued_as_new"]
		case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
			desc, err := l.Client.DescribeWorkflowExecution(ctx, ex.Execution.WorkflowId, ex.Execution.RunId)
			if err != nil {
				sum.Stage = "running"
				sum.StageLabel = stageLabels["running"]
			} else {
				sum.Stage, sum.StageLabel, sum.ActiveStages = inferStageFromPending(desc.PendingActivities)
				if sum.Stage == "other" && len(sum.ActiveStages) == 1 {
					sum.StageLabel = stageLabels["other"]
				}
				if sum.Stage == "running" && len(sum.ActiveStages) == 0 {
					sum.StageLabel = stageLabels["running"]
				}
			}
		default:
			sum.Stage = strings.ToLower(sum.Status)
			sum.StageLabel = sum.Status
		}
		out = append(out, sum)
	}
	return out, nil
}
