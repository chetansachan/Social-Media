package repository

import (
	"strings"
	"testing"
)

/* ---- Unread counts -------------------------------------------------------
   The badge has to count messages a peer sent that the viewer has not seen. It
   must never count the viewer's own messages: sending a message advances no
   read receipt, so without a sender filter an author's own messages inflate
   their own badge -- exactly the "5 unread" shown on a thread whose only
   unread entry was the viewer's own "dh".

   The three query shapes are named constants, so the invariant is asserted
   once here instead of being re-derived from cache in the client. */

// senderFilters returns every `m.sender_id <> $N` binding in a query.
func senderFilters(query string) []string {
	var bound []string
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "AND m.sender_id <> $") {
			continue
		}
		bound = append(bound, strings.TrimPrefix(line, "AND m.sender_id <> "))
	}
	return bound
}

func TestUnreadQueriesFilterOutTheViewersOwnMessages(t *testing.T) {
	queries := []struct {
		name          string
		query         string
		viewerBinding string
	}{
		{"UnreadCountFor", queryUnreadCountForConversation, "$2"},
		{"unreadCountsFor", queryUnreadCountsForMany, "$1"},
		{"TotalUnread", queryTotalUnread, "$1"},
	}

	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			filters := senderFilters(q.query)
			if len(filters) != 1 {
				t.Fatalf(
					"expected exactly one sender filter, got %d:\n%s",
					len(filters), q.query,
				)
			}

			// The filter must reuse the viewer the call site already binds for
			// its `p.user_id` join, never introduce a new placeholder.
			if filters[0] != q.viewerBinding {
				t.Fatalf(
					"sender filter bound to %s, want the viewer binding %s:\n%s",
					filters[0], q.viewerBinding, q.query,
				)
			}

			// Both of these must survive: dropping the soft-delete filter counts
			// messages nobody can see, and dropping the read watermark counts
			// messages the viewer has already seen.
			if !strings.Contains(q.query, "m.deleted_at IS NULL") {
				t.Fatalf("lost the soft-delete filter:\n%s", q.query)
			}
			if !strings.Contains(q.query, "m.created_at > p.last_read_at") {
				t.Fatalf("lost the read watermark:\n%s", q.query)
			}
		})
	}
}

// A sender filter pointing at a placeholder nothing binds fails at prepare
// time; one pointed at the peer would read that peer's unread state. Each query
// names its viewer binding explicitly, so the call site and the SQL cannot drift.
func TestSenderFilterPointsAtTheViewerArgument(t *testing.T) {
	// UnreadCountFor(ctx, conversationID, viewerID): viewer is the 2nd arg.
	if !strings.Contains(queryUnreadCountForConversation, "m.sender_id <> $2") {
		t.Fatal("UnreadCountFor must exclude the viewer bound as $2")
	}
	// unreadCountsFor(ctx, viewerID, conversationIDs): viewer is the 1st arg.
	if !strings.Contains(queryUnreadCountsForMany, "m.sender_id <> $1") {
		t.Fatal("unreadCountsFor must exclude the viewer bound as $1")
	}
	// TotalUnread(ctx, viewerID, excluded): viewer is the 1st arg.
	if !strings.Contains(queryTotalUnread, "m.sender_id <> $1") {
		t.Fatal("TotalUnread must exclude the viewer bound as $1")
	}
}
