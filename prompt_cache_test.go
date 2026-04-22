package ragagent

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryPromptCacheEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()

	cache := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{
		MaxEntries: 2,
	})

	cache.Set(context.Background(), "a", []Message{{Role: RoleUser, Content: "a"}})
	cache.Set(context.Background(), "b", []Message{{Role: RoleUser, Content: "b"}})
	if _, ok := cache.Get(context.Background(), "a"); !ok {
		t.Fatal("Get(a) miss, want hit")
	}
	cache.Set(context.Background(), "c", []Message{{Role: RoleUser, Content: "c"}})

	if _, ok := cache.Get(context.Background(), "b"); ok {
		t.Fatal("Get(b) hit, want evicted")
	}
	if _, ok := cache.Get(context.Background(), "a"); !ok {
		t.Fatal("Get(a) miss after eviction, want retained")
	}
	if _, ok := cache.Get(context.Background(), "c"); !ok {
		t.Fatal("Get(c) miss after set, want retained")
	}
}

func TestInMemoryPromptCacheExpiresEntries(t *testing.T) {
	t.Parallel()

	cache := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{
		MaxEntries: 16,
		TTL:        20 * time.Millisecond,
	})

	cache.Set(context.Background(), "a", []Message{{Role: RoleUser, Content: "a"}})
	time.Sleep(30 * time.Millisecond)

	if _, ok := cache.Get(context.Background(), "a"); ok {
		t.Fatal("Get(a) hit, want expired")
	}
}
