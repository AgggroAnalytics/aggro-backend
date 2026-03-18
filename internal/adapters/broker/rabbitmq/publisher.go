package rabbitmq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/outbox"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

// DeclareTopology declares exchanges and the reply queue so that publish
// calls don't fail when workers haven't started yet.
func DeclareTopology(ch *amqp091.Channel, exchanges []string, replyQueue string) error {
	for _, ex := range exchanges {
		if err := ch.ExchangeDeclare(ex, "direct", true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", ex, err)
		}
		slog.Info("declared exchange", "exchange", ex)
	}
	if replyQueue != "" {
		if _, err := ch.QueueDeclare(replyQueue, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare reply queue %s: %w", replyQueue, err)
		}
		slog.Info("declared reply queue", "queue", replyQueue)
	}
	return nil
}

type RabbitPublisher struct {
	holder     *ChannelHolder
	exchange   string
	routingKey string
}

func NewRabbitPublisher(holder *ChannelHolder, exchange string, routingKey string) *RabbitPublisher {
	return &RabbitPublisher{
		holder:     holder,
		exchange:   exchange,
		routingKey: routingKey,
	}
}

// ListenReturns starts a goroutine that logs mandatory messages returned by RabbitMQ
// (e.g. when no queue is bound to the exchange/routing-key).
func ListenReturns(ch *amqp091.Channel) {
	ret := ch.NotifyReturn(make(chan amqp091.Return, 16))
	go func() {
		for r := range ret {
			slog.Warn("message returned by broker",
				"exchange", r.Exchange,
				"routingKey", r.RoutingKey,
				"replyCode", r.ReplyCode,
				"replyText", r.ReplyText,
			)
		}
	}()
}

func encodeOutboxEvent(e *outbox.OutboxEvent) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := enc.Encode(e)

	if err != nil {
		return nil, errors.New("could not encode outbox event")
	}

	return buf.Bytes(), nil
}

func (r *RabbitPublisher) Publish(ctx context.Context, message outbox.OutboxEvent) error {
	encoded, err := encodeOutboxEvent(&message)
	if err != nil {
		return fmt.Errorf("Could not publish outbox event %w", err)
	}

	msg := amqp091.Publishing{
		Headers:      amqp091.Table{},
		ContentType:  "application/json",
		DeliveryMode: 2,
		Body:         encoded,
		MessageId:    uuid.NewString(),
	}

	ch := r.holder.GetChannel()
	if ch == nil {
		return fmt.Errorf("rabbit channel not ready")
	}
	// mandatory=true; immediate=false (RabbitMQ 3.x does not support immediate)
	return ch.Publish(r.exchange, r.routingKey, true, false, msg)
}

// PublishJSON publishes a raw JSON body to the publisher's exchange and routing key.
// replyTo and correlationID are optional (e.g. for request-reply).
func (r *RabbitPublisher) PublishJSON(ctx context.Context, body []byte, replyTo, correlationID string) error {
	ch := r.holder.GetChannel()
	if ch == nil {
		return fmt.Errorf("rabbit channel not ready")
	}
	msg := amqp091.Publishing{
		Headers:       amqp091.Table{},
		ContentType:   "application/json",
		DeliveryMode:  2,
		Body:          body,
		ReplyTo:       replyTo,
		CorrelationId: correlationID,
		MessageId:     uuid.NewString(),
	}
	// mandatory=true; immediate=false (RabbitMQ 3.x does not support immediate)
	return ch.Publish(r.exchange, r.routingKey, true, false, msg)
}

// PublishTileRequest implements ports.TilesPublisher: publish tile-job to tile-worker.
func (r *RabbitPublisher) PublishTileRequest(ctx context.Context, body []byte, replyTo, correlationID string) error {
	return r.PublishJSON(ctx, body, replyTo, correlationID)
}

// PublishGeo publishes to geo-worker exchange (implements consumer.GeoPublisher).
func (r *RabbitPublisher) PublishGeo(ctx context.Context, body []byte, replyTo, correlationID string) error {
	return r.PublishJSON(ctx, body, replyTo, correlationID)
}

// PublishPmtilesBuildJob publishes a build job to pmtiles-worker (implements ports.PmtilesBuildPublisher).
func (r *RabbitPublisher) PublishPmtilesBuildJob(ctx context.Context, body []byte) error {
	return r.PublishJSON(ctx, body, "", "")
}
