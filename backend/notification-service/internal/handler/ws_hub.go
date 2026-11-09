package handler

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// ConnectionMetrics tracks live WebSocket connections. observability.Metrics
// satisfies it; the interface keeps this package independent of the platform
// module.
type ConnectionMetrics interface {
	WebsocketConnected()
	WebsocketDisconnected()
}

// Hub tracks live WebSocket clients per user and bridges Redis Pub/Sub frames
// to every connection that user has open. It is safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*wsClient]struct{}
	subs    map[string]context.CancelFunc
	ctx     context.Context
	redis   *redis.Client
	logger  *log.Logger
	metrics ConnectionMetrics
}

func NewHub(ctx context.Context, redisClient *redis.Client, logger *log.Logger) *Hub {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Hub{
		clients: make(map[string]map[*wsClient]struct{}),
		subs:    make(map[string]context.CancelFunc),
		ctx:     ctx,
		redis:   redisClient,
		logger:  logger,
	}
}

// SetMetrics wires the live-connection gauge into the hub. It is safe to call
// with nil to disable it.
func (h *Hub) SetMetrics(metrics ConnectionMetrics) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.metrics = metrics
}

// Register adds a connection and starts the Redis subscription on the user's
// first connection. Multiple devices for one user share one subscription.
func (h *Hub) Register(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	connections, ok := h.clients[client.userID]
	if !ok {
		connections = make(map[*wsClient]struct{})
		h.clients[client.userID] = connections
	}
	connections[client] = struct{}{}
	if h.metrics != nil {
		h.metrics.WebsocketConnected()
	}

	if _, subscribed := h.subs[client.userID]; subscribed || h.redis == nil {
		return
	}

	subCtx, cancel := context.WithCancel(h.ctx)
	h.subs[client.userID] = cancel
	go h.subscribe(subCtx, client.userID)
}

func (h *Hub) Unregister(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	connections, ok := h.clients[client.userID]
	if !ok {
		return
	}
	if _, registered := connections[client]; !registered {
		return
	}
	delete(connections, client)
	if h.metrics != nil {
		h.metrics.WebsocketDisconnected()
	}

	if len(connections) > 0 {
		return
	}
	delete(h.clients, client.userID)

	if cancel, ok := h.subs[client.userID]; ok {
		cancel()
		delete(h.subs, client.userID)
	}
}

func (h *Hub) subscribe(ctx context.Context, userID string) {
	channel := "notify:" + userID
	pubsub := h.redis.Subscribe(ctx, channel)
	defer pubsub.Close()

	messages := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			h.broadcast(userID, []byte(message.Payload))
		}
	}
}

func (h *Hub) broadcast(userID string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients[userID] {
		if !client.enqueue(payload) {
			h.logger.Printf("ws: dropping frame for slow client %s (%s)", userID, client.remoteAddr)
		}
	}
}

// Close cancels every subscription and closes every live connection. The HTTP
// server cannot close hijacked WebSocket connections itself, so this is what
// makes SIGTERM actually drain them instead of waiting for a read deadline.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for userID, cancel := range h.subs {
		cancel()
		delete(h.subs, userID)
	}
	for userID, connections := range h.clients {
		for client := range connections {
			client.close()
			if h.metrics != nil {
				h.metrics.WebsocketDisconnected()
			}
			delete(connections, client)
		}
		delete(h.clients, userID)
	}
}

type wsClient struct {
	userID     string
	remoteAddr string
	conn       *websocket.Conn
	send       chan []byte
	hub        *Hub
	logger     *log.Logger
}

func newWSClient(userID string, conn *websocket.Conn, hub *Hub, logger *log.Logger) *wsClient {
	return &wsClient{
		userID:     userID,
		remoteAddr: conn.RemoteAddr().String(),
		conn:       conn,
		send:       make(chan []byte, 32),
		hub:        hub,
		logger:     logger,
	}
}

func (c *wsClient) enqueue(payload []byte) bool {
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

func (c *wsClient) close() {
	_ = c.conn.Close()
}
