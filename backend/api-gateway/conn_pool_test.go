package main

import (
	"context"
	"sync"
	"testing"

	"google.golang.org/grpc"
)

type recordingConn struct {
	id    int
	mu    *sync.Mutex
	calls *[]int
}

func (c recordingConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	*c.calls = append(*c.calls, c.id)
	return nil
}

func (c recordingConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	*c.calls = append(*c.calls, c.id)
	return nil, nil
}

func (recordingConn) Close() error { return nil }

func TestConnPoolRoundRobinsCalls(t *testing.T) {
	var mu sync.Mutex
	var calls []int
	pool := newConnPool(
		recordingConn{id: 0, mu: &mu, calls: &calls},
		recordingConn{id: 1, mu: &mu, calls: &calls},
		recordingConn{id: 2, mu: &mu, calls: &calls},
	)

	for i := 0; i < 6; i++ {
		if err := pool.Invoke(context.Background(), "/test.Service/Method", nil, nil); err != nil {
			t.Fatalf("Invoke: %v", err)
		}
	}

	want := []int{1, 2, 0, 1, 2, 0}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}
