package server

import (
	"database/sql"
	"net/http"
	"sync"

	"order_processor/internal/store"
)

type orderEventNotifications struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]string
}

func (n *orderEventNotifications) publish(account string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for wake, subscribed := range n.subscribers {
		if account == subscribed {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
	}
}

func (n *orderEventNotifications) stream(db *sql.DB, auth *authentication) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, err := orderEventCursor(r)
		if err != nil {
			send(w, 400, map[string]string{"error": err.Error()})
			return
		}
		account := r.PathValue("id")
		wake := make(chan struct{}, 1)
		n.mu.Lock()
		if n.subscribers == nil {
			n.subscribers = make(map[chan struct{}]string)
		}
		n.subscribers[wake] = account
		n.mu.Unlock()
		defer func() { n.mu.Lock(); delete(n.subscribers, wake); n.mu.Unlock() }()
		// Subscribe before reading the cursor so commits during catch-up also wake the stream.
		wake <- struct{}{}
		serveAccountStream(w, r, auth, wake, func(_ struct{}, write func(any) error) error {
			for {
				events, cursor, err := store.OrderEvents(db, account, after)
				if err != nil {
					return err
				}
				if after != nil && len(events) == 0 {
					return nil
				}
				if err := write(map[string]any{"cursor": cursor, "events": events}); err != nil {
					return err
				}
				after = &cursor
				if len(events) == 0 {
					return nil
				}
			}
		})
	}
}
