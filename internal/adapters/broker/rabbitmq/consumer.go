package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/rabbitmq/amqp091-go"
)

// ReplyHandler is called for each message from the backend reply queue.
// Body is the JSON payload; the handler may dispatch by checking for "tiles" vs "timeseries".
type ReplyHandler func(ctx context.Context, body []byte) error

// ConsumeReplies declares the reply queue and consumes messages, calling handler for each.
// Run in a goroutine; returns when the channel is closed or context is done.
func ConsumeReplies(ctx context.Context, conn *amqp091.Connection, replyQueue string, handler ReplyHandler) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(replyQueue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", replyQueue, err)
	}

	deliveries, err := ch.Consume(replyQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", replyQueue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return nil
			}
			if err := handler(ctx, d.Body); err != nil {
				slog.Error("reply handler failed", "err", err, "body_preview", string(d.Body)[:min(200, len(d.Body))])
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IsTileReply returns true if the JSON body looks like a tile-worker reply (has "tiles" key).
func IsTileReply(body []byte) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(body, &m) == nil && m["tiles"] != nil
}

// IsGeoReply returns true if the JSON body looks like a geo-worker reply (has "timeseries" key).
func IsGeoReply(body []byte) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(body, &m) == nil && m["timeseries"] != nil
}
