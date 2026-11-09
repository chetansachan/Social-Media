package graph

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return server, client
}

func TestNotificationReceivedRelaysThisUsersFrames(t *testing.T) {
	server, client := newTestRedis(t)
	resolver := &subscriptionResolver{&Resolver{Redis: client}}

	ctx, cancel := context.WithCancel(viewerContext("u1"))
	defer cancel()

	items, err := resolver.NotificationReceived(ctx)
	if err != nil {
		t.Fatalf("NotificationReceived: %v", err)
	}

	// A notification for somebody else must never reach this stream.
	server.Publish(realtimeChannel("u2"), `{"event":"notification_received","data":{"id":"other"}}`)
	server.Publish(realtimeChannel("u1"), `{"event":"notification_received","data":{"id":"n1","type":"post_like","title":"New like","action_url":"/posts/p1","total_actors_count":1,"created_at":"2026-01-01T00:00:00Z"}}`)

	select {
	case item, ok := <-items:
		if !ok {
			t.Fatal("subscription channel closed unexpectedly")
		}
		if item.ID != "n1" {
			t.Fatalf("received %q, want n1", item.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the notification frame")
	}
}

func TestNotificationReceivedClosesWhenClientDisconnects(t *testing.T) {
	_, client := newTestRedis(t)
	resolver := &subscriptionResolver{&Resolver{Redis: client}}

	ctx, cancel := context.WithCancel(viewerContext("u1"))
	items, err := resolver.NotificationReceived(ctx)
	if err != nil {
		t.Fatalf("NotificationReceived: %v", err)
	}

	cancel()
	select {
	case _, ok := <-items:
		if ok {
			t.Fatal("expected no frame before the channel closes")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription channel was not closed after disconnect")
	}
}

func TestNotificationReceivedRequiresAuthentication(t *testing.T) {
	_, client := newTestRedis(t)
	resolver := &subscriptionResolver{&Resolver{Redis: client}}

	if _, err := resolver.NotificationReceived(context.Background()); err == nil {
		t.Fatal("expected anonymous subscriptions to fail")
	}
}

func TestNotificationReceivedWithoutRedis(t *testing.T) {
	resolver := &subscriptionResolver{&Resolver{}}

	ctx := auth.WithPrincipal(context.Background(), auth.Principal{UserID: "u1"})
	if _, err := resolver.NotificationReceived(ctx); err == nil {
		t.Fatal("expected a configuration error without Redis")
	}
}
