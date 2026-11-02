package handler

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// Hub tracks live WebSocket clients per user and bridges Redis Pub/Sub frames
// to every connection that user has open. It is safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*wsClient]struct{}
	subs    map[string]context.CancelFunc
	ctx     context.Context
	redis   *redis.Client
	logger  *log.Logger
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
	delete(connections, client)

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

// Close cancels every subscription. Connections themselves are closed by the
// HTTP server shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for userID, cancel := range h.subs {
		cancel()
		delete(h.subs, userID)
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
