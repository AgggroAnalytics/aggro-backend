package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	amqp091 "github.com/rabbitmq/amqp091-go"
)

// PmtilesBuildPublisherAdapter publishes build jobs to the pmtiles-worker queue.
type PmtilesBuildPublisherAdapter struct {
	*RabbitPublisher
}

// NewPmtilesBuildPublisherAdapter returns an adapter that implements ports.PmtilesBuildPublisher.
func NewPmtilesBuildPublisherAdapter(pub *RabbitPublisher) *PmtilesBuildPublisherAdapter {
	return &PmtilesBuildPublisherAdapter{RabbitPublisher: pub}
}

// PublishBuildJob implements ports.PmtilesBuildPublisher.
// Uses a deterministic message_id so pmtiles worker inbox deduplicates identical requests.
func (p *PmtilesBuildPublisherAdapter) PublishBuildJob(ctx context.Context, fieldID uuid.UUID, analysisKind string, analysisDate time.Time, module string) error {
	dateStr := analysisDate.Format("2006-01-02")
	payload := map[string]interface{}{
		"field_id":      fieldID.String(),
		"analysis_kind": analysisKind,
		"analysis_date": dateStr,
		"module":        module,
	}
	body, _ := json.Marshal(payload)
	stableID := fmt.Sprintf("pmtiles-%s-%s-%s-%s", fieldID, analysisKind, dateStr, module)
	msg := amqp091.Publishing{
		Headers:      amqp091.Table{},
		ContentType:  "application/json",
		DeliveryMode: 2,
		Body:         body,
		MessageId:    stableID,
	}
	ch := p.holder.GetChannel()
	if ch == nil {
		return fmt.Errorf("rabbit channel not ready")
	}
	return ch.Publish(p.exchange, p.routingKey, true, false, msg)
}

var _ ports.PmtilesBuildPublisher = (*PmtilesBuildPublisherAdapter)(nil)
