package presence

import (
	"sync"
	"time"
)

type Info struct {
	UserID   string
	Username string
	Resource string // e.g. "owner/repo" or "owner/repo/issues/42"
	LastSeen time.Time
}

type Tracker struct {
	mu       sync.Mutex
	active   map[string]map[string]*Info // resource -> userID -> info
	ttl      time.Duration
	stopCh   chan struct{}
}

func NewTracker() *Tracker {
	t := &Tracker{
		active: make(map[string]map[string]*Info),
		ttl:    60 * time.Second,
		stopCh: make(chan struct{}),
	}
	go t.cleanupLoop()
	return t
}

func (t *Tracker) Stop() {
	close(t.stopCh)
}

func (t *Tracker) Heartbeat(userID, username, resource string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	users, ok := t.active[resource]
	if !ok {
		users = make(map[string]*Info)
		t.active[resource] = users
	}
	users[userID] = &Info{
		UserID:   userID,
		Username: username,
		Resource: resource,
		LastSeen: time.Now(),
	}
}

func (t *Tracker) GetActiveViewers(resource string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	users, ok := t.active[resource]
	if !ok {
		return nil
	}

	now := time.Now()
	var result []string
	for _, info := range users {
		if now.Sub(info.LastSeen) <= t.ttl {
			result = append(result, info.Username)
		}
	}
	return result
}

func (t *Tracker) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.cleanup()
		case <-t.stopCh:
			return
		}
	}
}

func (t *Tracker) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for resource, users := range t.active {
		for userID, info := range users {
			if now.Sub(info.LastSeen) > t.ttl {
				delete(users, userID)
			}
		}
		if len(users) == 0 {
			delete(t.active, resource)
		}
	}
}
