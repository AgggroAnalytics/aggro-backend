package temporalworkflows

import (
	"context"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

// WorkflowFailureMessage returns a human-readable failure from the workflow close history (if any).
func WorkflowFailureMessage(ctx context.Context, c client.Client, workflowID, runID string) string {
	if c == nil {
		return ""
	}
	iter := c.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_CLOSE_EVENT)
	for iter.HasNext() {
		ev, err := iter.Next()
		if err != nil || ev == nil {
			return ""
		}
		if ev.GetEventType() == enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED {
			attr := ev.GetWorkflowExecutionFailedEventAttributes()
			if attr != nil && attr.Failure != nil {
				msg := attr.Failure.GetMessage()
				if msg != "" {
					return msg
				}
				return "workflow failed (no message)"
			}
		}
	}
	return ""
}
