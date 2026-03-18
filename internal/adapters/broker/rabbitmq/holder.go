package rabbitmq

import (
	"sync"

	amqp091 "github.com/rabbitmq/amqp091-go"
)

// ChannelHolder holds the current AMQP connection and channel so they can be
// replaced on reconnect without changing publisher references.
type ChannelHolder struct {
	mu   sync.RWMutex
	conn *amqp091.Connection
	ch   *amqp091.Channel
}

// NewChannelHolder returns a new holder with no connection (publishers will fail until Set is called).
func NewChannelHolder() *ChannelHolder {
	return &ChannelHolder{}
}

// GetChannel returns the current channel, or nil if not set.
func (h *ChannelHolder) GetChannel() *amqp091.Channel {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ch
}

// GetConn returns the current connection, or nil if not set.
func (h *ChannelHolder) GetConn() *amqp091.Connection {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conn
}

// Set sets the current connection and channel (e.g. after reconnect).
// Pass nil to clear after connection is closed.
func (h *ChannelHolder) Set(conn *amqp091.Connection, ch *amqp091.Channel) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conn = conn
	h.ch = ch
}
