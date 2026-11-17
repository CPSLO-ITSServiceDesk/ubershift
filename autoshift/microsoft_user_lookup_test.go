package autoshift

import (
	"sync"
	"testing"
	"time"
)

func TestFetchMicrosoftUsersLooksUpDistinctUsersInParallel(t *testing.T) {
	var mu sync.Mutex
	active, maxActive := 0, 0
	lookup := func(_ string, id string) (MicrosoftUser, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()
		return MicrosoftUser{ID: id, DisplayName: id + " Name"}, nil
	}

	users, err := fetchMicrosoftUsersWithLookup("token", []string{"ava", "kai", "ava"}, lookup)
	if err != nil {
		t.Fatalf("fetchMicrosoftUsersWithLookup() returned error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("looked up %d users, want 2 distinct users", len(users))
	}
	if maxActive < 2 {
		t.Errorf("lookups ran sequentially; maximum concurrent lookups = %d", maxActive)
	}
}
