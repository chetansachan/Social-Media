package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Both channels for one viewer are relayed over a single connection, so a
// browser client pays one TCP stream instead of two and ordering between the
// inbox and the mailbox is preserved.
const (
	streamChannelNotification = "notify:"
	streamChannelChat         = "chat:"

	// A comment every 15s keeps proxies from reaping an idle connection, and
	// gives the handler a chance to notice that the client is gone.
	streamHeartbeatInterval = 15 * time.Second
)

// streamEvent is the JSON envelope written to the client. It exists because
// `EventSource` dispatches on the field name, not on an application-level
// discriminator, so the two backend frame shapes are normalised here.
type streamEvent struct {
	Stream     string          `json:"stream"`
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data,omitempty"`
	UnixMillis int64           `json:"receivedAt,omitempty"`
}

// realtimeStreamHandler turns the gateway's Redis Pub/Sub relays into
// Server-Sent Events.
//
// The GraphQL WebSocket transport is deliberately not used by the web client:
// it carries the access token in a query parameter or an init payload, both of
// which are awkward to hold in an httpOnly cookie without exposing it to
// JavaScript. SSE authenticates on the Authorization header, which this
// handler's own middleware supplies from the same cookie the GraphQL proxy
// reads.
func realtimeStreamHandler(redisClient *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.FromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if redisClient == nil {
			http.Error(w, "realtime is not configured", http.StatusServiceUnavailable)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// The subscription channels are built from the claim, so a token whose
		// subject is not a UUID is rejected here rather than being subscribed
		// to a channel nobody ever publishes to.
		viewerID := strings.TrimSpace(principal.UserID)
		if _, err := uuid.Parse(viewerID); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		headers := w.Header()
		headers.Set("Content-Type", "text/event-stream")
		headers.Set("Cache-Control", "no-cache, no-transform")
		headers.Set("Connection", "keep-alive")
		// Nginx buffers by default; without this the frames arrive in bursts.
		headers.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// The subscription is confirmed before the handler returns, so a frame
		// published the instant the client connects cannot race past it. The
		// request context is cancelled when the client disconnects.
		pubsub := redisClient.Subscribe(r.Context(), streamChannelNotification+viewerID, streamChannelChat+viewerID)
		defer pubsub.Close()
		if _, err := pubsub.Receive(r.Context()); err != nil {
			return
		}

		writeStreamComment(w, flusher, "connected")

		messages := pubsub.Channel()
		heartbeat := time.NewTicker(streamHeartbeatInterval)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
			 return
			case <-heartbeat.C:
				// A bare comment keeps the connection warm without waking the
				// client's message handler.
				writeStreamComment(w, flusher, "keep-alive")
			case message, ok := <-messages:
				if !ok {
					return
				}
				stream, eventType, payload, err := classifyFrame(message.Channel, viewerID, message.Payload)
				if err != nil || stream == "" {
					// One malformed frame must not tear down a healthy stream;
					// the next one carries the same state.
					continue
				}
				if writeErr := writeStreamEvent(w, flusher, stream, eventType, payload); writeErr != nil {
					return
				}
			}
		}
	}
}

// classifyFrame resolves one Redis frame into the stream it belongs to and the
// `data` payload the browser needs. Both channels are per-user, so a frame for
// another user can only mean the subscriber set was broadened; it is dropped
// rather than relayed.
//
// A frame that cannot be decoded returns an error: the caller drops that one
// frame and keeps the connection, because the next frame carries the same
// state.
func classifyFrame(channel, viewerID, payload string) (stream, eventType string, data json.RawMessage, err error) {
	switch {
	case strings.HasPrefix(channel, streamChannelNotification):
		if strings.TrimPrefix(channel, streamChannelNotification) != viewerID {
			// Another user's frame: not an error, just not ours to relay.
			return "", "", nil, nil
		}
		// notification-service emits exactly one event, so the frame's own
		// discriminator is redundant here.
		data, _, err = decodeFramePayload(payload)
		if err != nil {
			return "", "", nil, err
		}
		return "notifications", "notification.received", data, nil

	case strings.HasPrefix(channel, streamChannelChat):
		if strings.TrimPrefix(channel, streamChannelChat) != viewerID {
			return "", "", nil, nil
		}
		data, eventType, err = decodeFramePayload(payload)
		if err != nil {
			return "", "", nil, err
		}
		return "chat", eventType, data, nil

	default:
		return "", "", nil, nil
	}
}

type rawFrame struct {
	Event string          `json:"event"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data"`
}

// decodeFramePayload normalises the two backend envelopes onto a `data`
// payload: notification-service writes `{"event":..,"data":{..}}` while
// chat-service writes `{"type":..,"data":{..}}`.
func decodeFramePayload(payload string) (json.RawMessage, string, error) {
	var frame rawFrame
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		return nil, "", fmt.Errorf("decode frame: %w", err)
	}
	data := frame.Data
	if len(data) == 0 {
		data = json.RawMessage(payload)
	}
	return data, firstNonEmpty(frame.Type, frame.Event), nil
}

func writeStreamEvent(
	w http.ResponseWriter,
	flusher http.Flusher,
	stream, eventType string,
	data json.RawMessage,
) error {
	event := streamEvent{Stream: stream, Type: eventType, Data: data, UnixMillis: time.Now().UnixMilli()}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", stream, encoded); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeStreamComment(w http.ResponseWriter, flusher http.Flusher, comment string) {
	// A write error here means the client is gone; the caller notices on its
	// next frame and stops.
	_, _ = fmt.Fprintf(w, ":%s\n\n", comment)
	flusher.Flush()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}