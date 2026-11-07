package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WSHandler upgrades authenticated clients and runs the ping/pong loop. The
// same handler works for web, iOS, and Android clients.
type WSHandler struct {
	hub            *Hub
	upgrader       websocket.Upgrader
	jwtSecret      string
	allowDevUserID bool
	logger         *log.Logger
}

func NewWSHandler(hub *Hub, jwtSecret string, allowDevUserID bool, allowedOrigins []string, logger *log.Logger) *WSHandler {
	if logger == nil {
		logger = log.Default()
	}
	allowed := make(map[string]struct{}, len(allowedOrigins))
	allowAll := false
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			allowAll = true
		}
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if allowAll {
				return true
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			_, ok := allowed[origin]
			return ok
		},
	}

	return &WSHandler{
		hub:            hub,
		upgrader:       upgrader,
		jwtSecret:      jwtSecret,
		allowDevUserID: allowDevUserID,
		logger:         logger,
	}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, err := h.authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Printf("ws: upgrade failed for %s: %v", userID, err)
		return
	}

	client := newWSClient(userID, conn, h.hub, h.logger)
	h.hub.Register(client)
	client.enqueue([]byte(`{"event":"connected"}`))
	h.logger.Printf("ws: user %s connected from %s", userID, client.remoteAddr)

	go client.writePump()
	client.readPump()
}

func (h *WSHandler) authenticate(r *http.Request) (string, error) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		header := r.Header.Get("Authorization")
		if strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		}
	}

	if h.jwtSecret == "" {
		// Development fallback: the gateway trusts ?user_id=. This is gated by
		// ALLOW_DEV_USER_ID and is disabled by default in production.
		if !h.allowDevUserID {
			return "", errors.New("JWT_SECRET is not configured")
		}
		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
		if _, err := uuid.Parse(userID); err != nil {
			return "", errors.New("provide a valid user_id in development mode")
		}
		return userID, nil
	}

	claims, err := verifyHS256(token, h.jwtSecret)
	if err != nil {
		return "", err
	}

	userID := ""
	if value, ok := claims["user_id"].(string); ok {
		userID = value
	}
	if userID == "" {
		if value, ok := claims["sub"].(string); ok {
			userID = value
		}
	}
	if _, err := uuid.Parse(userID); err != nil {
		return "", errors.New("token does not contain a valid user id")
	}
	return userID, nil
}

func (c *wsClient) readPump() {
	defer func() {
		c.hub.Unregister(c)
		c.close()
		c.logger.Printf("ws: user %s disconnected", c.userID)
	}()

	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var frame struct {
			Action string `json:"action"`
		}
		if json.Unmarshal(payload, &frame) == nil && frame.Action == "ping" {
			c.enqueue([]byte(`{"event":"pong"}`))
		}
	}
}

func (c *wsClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case payload := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// verifyHS256 validates the compact JWT format without pulling in a JWT
// dependency. Only HS256 tokens are accepted.
func verifyHS256(token, secret string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}

	headerJSON, err := decodeJWTPart(parts[0])
	if err != nil {
		return nil, errors.New("malformed token header")
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil || header.Alg != "HS256" {
		return nil, errors.New("unsupported token algorithm")
	}

	signature, err := decodeJWTPart(parts[2])
	if err != nil {
		return nil, errors.New("malformed token signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, errors.New("invalid token signature")
	}

	payloadJSON, err := decodeJWTPart(parts[1])
	if err != nil {
		return nil, errors.New("malformed token payload")
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, errors.New("malformed token claims")
	}

	if exp, ok := claims["exp"].(float64); ok && time.Now().Unix() >= int64(exp) {
		return nil, errors.New("token expired")
	}
	return claims, nil
}

// decodeJWTPart accepts a JWT segment in either base64url form: unpadded, which
// the spec requires, or padded, which some implementations emit. An empty
// segment is rejected outright, because decoding it would succeed and defer the
// failure to a confusing JSON parse later.
func decodeJWTPart(value string) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("empty token segment")
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}
