package presence

import (
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

const defaultMaxExpiry = 24 * time.Hour

type Subscription struct {
	CallID        string
	WatcherFrom   string // From tag on SUBSCRIBE
	LocalToTag    string // To tag on 200 OK
	Watcher       string
	Watched       string
	Event         string
	Expires       time.Time
	NotifyContact sip.Uri
	NotifyDest    string
	CSeq          uint32
}

type SubscriptionStore struct {
	mu        sync.RWMutex
	byKey     map[string]*Subscription
	byWatched map[string][]*Subscription
}

func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{
		byKey:     make(map[string]*Subscription),
		byWatched: make(map[string][]*Subscription),
	}
}

func subKey(callID, fromTag string) string {
	return callID + "|" + fromTag
}

func (s *SubscriptionStore) Upsert(sub *Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := subKey(sub.CallID, sub.WatcherFrom)
	if old, ok := s.byKey[key]; ok {
		s.removeLocked(old)
	}
	s.byKey[key] = sub
	s.byWatched[sub.Watched] = append(s.byWatched[sub.Watched], sub)
}

func (s *SubscriptionStore) Remove(callID, fromTag string) *Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := subKey(callID, fromTag)
	sub, ok := s.byKey[key]
	if !ok {
		return nil
	}
	s.removeLocked(sub)
	delete(s.byKey, key)
	return sub
}

func (s *SubscriptionStore) removeLocked(sub *Subscription) {
	list := s.byWatched[sub.Watched]
	out := list[:0]
	for _, item := range list {
		if item != sub {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		delete(s.byWatched, sub.Watched)
	} else {
		s.byWatched[sub.Watched] = out
	}
}

func (s *SubscriptionStore) ForWatched(ext string) []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	list := s.byWatched[ext]
	out := make([]*Subscription, 0, len(list))
	for _, sub := range list {
		if sub.Expires.After(now) {
			out = append(out, sub)
		}
	}
	return out
}

func (s *SubscriptionStore) PruneExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	removed := 0
	for key, sub := range s.byKey {
		if sub.Expires.After(now) {
			continue
		}
		s.removeLocked(sub)
		delete(s.byKey, key)
		removed++
	}
	return removed
}
