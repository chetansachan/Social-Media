package presence

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// DefaultPresenceTTL is how long one heartbeat keeps a user online. A
	// client that stops heartbeating goes offline on its own, so a crashed
	// socket cannot leave a ghost online.
	DefaultPresenceTTL = 60 * time.Second
	// DefaultTypingTTL is how long a typing signal survives. It is longer than
	// a keystroke gap and shorter than a coffee break, which keeps a stopped
	// typist from looking busy forever.
	DefaultTypingTTL = 6 * time.Second
)

// Store keeps the online bitmap and typing keys in Redis. Both are deliberately
// ephemeral: they describe the last few seconds, so there is nothing to recover
// from a durable store and nothing to reconcile on restart.
type Store struct {
	client *redis.Client

	presenceTTL time.Duration
	typingTTL   time.Duration

	// now is swappable so tests can exercise expiry without sleeping.
	now func() time.Time
}

func NewStore(client *redis.Client, presenceTTL, typingTTL time.Duration) *Store {
	if presenceTTL <= 0 {
		presenceTTL = DefaultPresenceTTL
	}
	if typingTTL <= 0 {
		typingTTL = DefaultTypingTTL
	}
	return &Store{client: client, presenceTTL: presenceTTL, typingTTL: typingTTL, now: time.Now}
}

// withNow swaps the clock. Only used by tests.
func (s *Store) withNow(now func() time.Time) {
	s.now = now
}

func (s *Store) presenceKey(userID uuid.UUID) string {
	return "chat:presence:" + userID.String()
}

// typingMembersKey holds the set of currently-typing members, scored by the
// instant their signal expires.
func (s *Store) typingMembersKey(conversationID uuid.UUID) string {
	return "chat:typing:" + conversationID.String()
}

// Touch records one heartbeat with its timestamp. Any RPC a connected client
// makes counts, so presence costs no extra round trips.
func (s *Store) Touch(ctx context.Context, userID uuid.UUID) error {
	if err := s.client.Set(ctx, s.presenceKey(userID), strconv.FormatInt(s.now().UnixMilli(), 10), s.presenceTTL).Err(); err != nil {
		return fmt.Errorf("failed to record presence: %w", err)
	}
	return nil
}

// Online returns the last heartbeat timestamp in unix milliseconds for each
// requested user. Offline users are absent from the map, so a caller sees an
// empty entry instead of guessing.
func (s *Store) Online(ctx context.Context, userIDs []uuid.UUID) (map[string]int64, error) {
	presence := make(map[string]int64, len(userIDs))
	if len(userIDs) == 0 {
		return presence, nil
	}

	keys := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		keys = append(keys, s.presenceKey(userID))
	}

	// One MGET for the whole batch: presence is read on every conversation
	// render, and a loop of GETs would be one round trip per participant.
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read presence: %w", err)
	}

	for i, id := range userIDs {
		value, ok := values[i].(string)
		if !ok || value == "" {
			continue
		}
		seenAt, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			continue
		}
		presence[id.String()] = seenAt
	}
	return presence, nil
}

// StartTyping marks a member as typing and returns everybody else typing in the
// same thread, so a caller renders the whole set from one response.
func (s *Store) StartTyping(ctx context.Context, conversationID, userID uuid.UUID) ([]uuid.UUID, error) {
	key := s.typingMembersKey(conversationID)
	if err := s.client.ZAdd(ctx, key, redis.Z{
		Score:  float64(s.now().Add(s.typingTTL).UnixMilli()),
		Member: userID.String(),
	}).Err(); err != nil {
		return nil, fmt.Errorf("failed to record typing: %w", err)
	}

	// Pruning on read is what lets a stopped typist lapse without a cleanup job
	// and without trusting the client to send a stop signal.
	if err := s.client.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", s.now().UnixMilli())).Err(); err != nil {
		return nil, fmt.Errorf("failed to prune typing flags: %w", err)
	}

	return s.TypingMembers(ctx, conversationID, userID)
}

// StopTyping clears one member's flag.
func (s *Store) StopTyping(ctx context.Context, conversationID, userID uuid.UUID) error {
	if err := s.client.ZRem(ctx, s.typingMembersKey(conversationID), userID.String()).Err(); err != nil {
		return fmt.Errorf("failed to clear typing: %w", err)
	}
	return nil
}

// TypingMembers lists who else is typing in a thread, excluding the viewer.
func (s *Store) TypingMembers(ctx context.Context, conversationID, viewerID uuid.UUID) ([]uuid.UUID, error) {
	key := s.typingMembersKey(conversationID)
	if err := s.client.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", s.now().UnixMilli())).Err(); err != nil {
		return nil, fmt.Errorf("failed to prune typing flags: %w", err)
	}

	members, err := s.client.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read typing members: %w", err)
	}

	users := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member == viewerID.String() {
			continue
		}
		userID, err := uuid.Parse(member)
		if err != nil {
			continue
		}
		users = append(users, userID)
	}
	return users, nil
}

// ClearSenderTyping drops a sender's flag when their message lands: sending is
// the strongest possible "I am done typing" signal.
func (s *Store) ClearSenderTyping(ctx context.Context, conversationID, senderID uuid.UUID) {
	_ = s.StopTyping(ctx, conversationID, senderID)
}
