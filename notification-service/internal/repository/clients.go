package repository

import (
	"context"
	"sync"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	userpb "github.com/chetansachan/socialflow/proto"
)

// ActorDirectory resolves actor IDs into display profiles for inbox rendering.
// Failures are intentionally soft: a missing user must never hide a
// notification, so implementations return the IDs that could be resolved.
type ActorDirectory interface {
	GetActors(ctx context.Context, ids []string) map[string]models.Actor
}

type grpcActorDirectory struct {
	client userpb.UserServiceClient
}

func NewActorDirectory(client userpb.UserServiceClient) ActorDirectory {
	return &grpcActorDirectory{client: client}
}

// GetActors hydrates up to 100 unique IDs with bounded concurrency. The User
// Service exposes only GetUserById today, so this is effectively a batch RPC
// implemented with a worker pool.
func (d *grpcActorDirectory) GetActors(ctx context.Context, ids []string) map[string]models.Actor {
	result := make(map[string]models.Actor, len(ids))
	if d.client == nil || len(ids) == 0 {
		return result
	}

	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
		if len(unique) == 100 {
			break
		}
	}

	const workers = 8
	jobs := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for id := range jobs {
				if ctx.Err() != nil {
					return
				}
				response, err := d.client.GetUserById(ctx, &userpb.GetUserByIdRequest{Id: id})
				if err != nil || response == nil || response.GetUser() == nil {
					continue
				}
				user := response.GetUser()
				actor := models.Actor{
					ID:          user.GetId(),
					Username:    user.GetUsername(),
					DisplayName: user.GetName(),
					Email:       user.GetEmail(),
				}
				mu.Lock()
				result[id] = actor
				mu.Unlock()
			}
		}()
	}

	for _, id := range unique {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return result
		case jobs <- id:
		}
	}
	close(jobs)
	wg.Wait()

	return result
}
