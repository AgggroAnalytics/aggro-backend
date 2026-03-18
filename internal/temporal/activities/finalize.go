package activities

import (
	"context"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/usecase/workflowfinalize"
)

// Finalize runs DB persistence for field processing (Temporal activity implementation on backend).
type Finalize struct {
	Deps workflowfinalize.Deps
}

// FinalizeFieldProcessingDB activity name: finalize_field_processing_db (called from FieldProcessingWorkflow on backend task queue).
func (f *Finalize) FinalizeFieldProcessingDB(ctx context.Context, req workflowfinalize.FieldProcessingCompleteRequest) (*workflowfinalize.Result, error) {
	return workflowfinalize.Apply(ctx, f.Deps, &req)
}
