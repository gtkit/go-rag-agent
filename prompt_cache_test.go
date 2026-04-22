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

func TestInMemoryPromptCacheMaintenance(t *testing.T) {
	t.Parallel()

	cache := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{
		MaxEntries: 16,
		TTL:        time.Minute,
	})
	cache.Set(context.Background(), "a", []Message{{Role: RoleUser, Content: "a"}})
	cache.Set(context.Background(), "b", []Message{{Role: RoleUser, Content: "b"}})

	maint, ok := cache.(PromptCacheMaintenance)
	if !ok {
		t.Fatal("cache does not implement PromptCacheMaintenance")
	}
	stats, err := maint.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Entries != 2 {
		t.Fatalf("Stats().Entries = %d, want 2", stats.Entries)
	}
	if err := maint.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := cache.Get(context.Background(), "a"); ok {
		t.Fatal("Get(a) hit after Delete, want miss")
	}
	if err := maint.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, ok := cache.Get(context.Background(), "b"); ok {
		t.Fatal("Get(b) hit after Clear, want miss")
	}
}

func TestInMemoryPromptCacheStatsSkipsExpiredEntries(t *testing.T) {
	t.Parallel()

	cache := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{
		MaxEntries: 16,
		TTL:        time.Minute,
	}).(*inMemoryPromptCache)
	now := time.Now()
	cache.now = func() time.Time { return now }
	cache.Set(context.Background(), "expired", []Message{{Role: RoleUser, Content: "old"}})
	cache.Set(context.Background(), "active", []Message{{Role: RoleUser, Content: "new"}})
	cache.mu.Lock()
	cache.items["expired"].Value.(*promptCacheEntry).expiresAt = now.Add(-time.Second)
	cache.items["active"].Value.(*promptCacheEntry).expiresAt = now.Add(time.Minute)
	cache.mu.Unlock()

	maint, ok := any(cache).(PromptCacheMaintenance)
	if !ok {
		t.Fatal("cache does not implement PromptCacheMaintenance")
	}
	stats, err := maint.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Entries != 1 {
		t.Fatalf("Stats().Entries = %d, want 1", stats.Entries)
	}
}
