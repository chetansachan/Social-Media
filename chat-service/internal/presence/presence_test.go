package presence

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T, presenceTTL, typingTTL time.Duration) (*Store, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	return NewStore(redis.NewClient(&redis.Options{Addr: server.Addr()}), presenceTTL, typingTTL), server
}

func TestPresenceTouchAndOnline(t *testing.T) {
	store, _ := newTestStore(t, time.Minute, time.Minute)
	viewer := uuid.New()
	other := uuid.New()

	if err := store.Touch(context.Background(), viewer); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	presence, err := store.Online(context.Background(), []uuid.UUID{viewer, other})
	if err != nil {
		t.Fatalf("Online: %v", err)
	}
	if _, ok := presence[viewer.String()]; !ok {
		t.Fatal("an online user should be present in the map")
	}
	if _, ok := presence[other.String()]; ok {
		t.Fatal("a user with no heartbeat should be absent, not online")
	}
}

func TestPresenceExpires(t *testing.T) {
	store, server := newTestStore(t, 2*time.Second, time.Minute)
	viewer := uuid.New()

	if err := store.Touch(context.Background(), viewer); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	// The TTL is the mechanism: a client that stops heartbeating stops being
	// online without the service ever learning about the disconnect.
	server.FastForward(3 * time.Second)

	presence, err := store.Online(context.Background(), []uuid.UUID{viewer})
	if err != nil {
		t.Fatalf("Online after expiry: %v", err)
	}
	if len(presence) != 0 {
		t.Fatalf("presence = %v, want empty after the TTL lapsed", presence)
	}
}

func TestTypingSetAndClear(t *testing.T) {
	store, _ := newTestStore(t, time.Minute, 30*time.Second)
	alice, bob := uuid.New(), uuid.New()
	conversation := uuid.New()

	typing, err := store.StartTyping(context.Background(), conversation, alice)
	if err != nil {
		t.Fatalf("StartTyping: %v", err)
	}
	// The sender is excluded from their own typing list.
	for _, member := range typing {
		if member == alice {
			t.Fatal("the typist should not appear in their own list")
		}
	}

	bobTyping, err := store.StartTyping(context.Background(), conversation, bob)
	if err != nil {
		t.Fatalf("StartTyping for bob: %v", err)
	}
	if len(bobTyping) != 1 || bobTyping[0] != alice {
		t.Fatalf("bob sees %v, want [alice]", bobTyping)
	}

	if err := store.StopTyping(context.Background(), conversation, alice); err != nil {
		t.Fatalf("StopTyping: %v", err)
	}
	remaining, err := store.TypingMembers(context.Background(), conversation, bob)
	if err != nil {
		t.Fatalf("TypingMembers: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining = %v, want nobody left after alice stopped", remaining)
	}
}

func TestTypingExpiresOnItsOwn(t *testing.T) {
	store, _ := newTestStore(t, time.Minute, 3*time.Second)
	alice, bob := uuid.New(), uuid.New()
	conversation := uuid.New()

	// The expiry deadline lives in the member's score, so a stopped typist
	// lapses even if they never send a stop signal. The fake clock stands in
	// for the seconds passing between two keystroke bursts.
	clock := time.Now().UTC()
	store.withNow(func() time.Time { return clock })

	if _, err := store.StartTyping(context.Background(), conversation, alice); err != nil {
		t.Fatalf("StartTyping: %v", err)
	}

	members, err := store.TypingMembers(context.Background(), conversation, bob)
	if err != nil {
		t.Fatalf("TypingMembers before expiry: %v", err)
	}
	if len(members) != 1 || members[0] != alice {
		t.Fatalf("members = %v, want [alice] while the flag is fresh", members)
	}

	clock = clock.Add(4 * time.Second)
	members, err = store.TypingMembers(context.Background(), conversation, bob)
	if err != nil {
		t.Fatalf("TypingMembers after expiry: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("members = %v, want the flag to lapse on its own", members)
	}
}

func TestClearSenderTypingIdempotent(t *testing.T) {
	store, _ := newTestStore(t, time.Minute, time.Minute)
	alice := uuid.New()
	conversation := uuid.New()

	if _, err := store.StartTyping(context.Background(), conversation, alice); err != nil {
		t.Fatalf("StartTyping: %v", err)
	}
	store.ClearSenderTyping(context.Background(), conversation, alice)
	// Sending a message clears the flag again; a second clear must not error.
	store.ClearSenderTyping(context.Background(), conversation, alice)

	members, err := store.TypingMembers(context.Background(), conversation, alice)
	if err != nil {
		t.Fatalf("TypingMembers: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("members = %v, want the sender's flag gone", members)
	}
}
