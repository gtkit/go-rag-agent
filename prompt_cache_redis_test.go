package ragagent

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisPromptCacheRoundTrip(t *testing.T) {
	server := miniredis.RunT(t)
	t.Cleanup(server.Close)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cache := NewRedisPromptCache(client, RedisPromptCacheConfig{
		KeyPrefix: "prompt",
		TTL:       time.Minute,
	})
	want := []Message{{Role: RoleUser, Content: "hello"}}
	cache.Set(context.Background(), "key", want)

	got, ok := cache.Get(context.Background(), "key")
	if !ok {
		t.Fatal("Get() miss, want hit")
	}
	if len(got) != 1 || got[0].Content != "hello" {
		t.Fatalf("Get() = %+v, want hello", got)
	}
}

func TestTwoLevelPromptCacheWarmsLocalFromRemote(t *testing.T) {
	server := miniredis.RunT(t)
	t.Cleanup(server.Close)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	local := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{
		MaxEntries: 16,
		TTL:        time.Minute,
	})
	remote := NewRedisPromptCache(client, RedisPromptCacheConfig{
		KeyPrefix: "prompt",
		TTL:       time.Minute,
	})
	twoLevel := NewTwoLevelPromptCache(local, remote)

	remote.Set(context.Background(), "key", []Message{{Role: RoleAssistant, Content: "remote"}})
	got, ok := twoLevel.Get(context.Background(), "key")
	if !ok {
		t.Fatal("TwoLevel.Get() miss, want hit")
	}
	if len(got) != 1 || got[0].Content != "remote" {
		t.Fatalf("TwoLevel.Get() = %+v, want remote", got)
	}
	warmed, ok := local.Get(context.Background(), "key")
	if !ok || len(warmed) != 1 || warmed[0].Content != "remote" {
		t.Fatalf("local.Get() = %+v, want warmed remote", warmed)
	}
}

func TestRedisPromptCacheMaintenanceReturnsError(t *testing.T) {
	server := miniredis.RunT(t)
	t.Cleanup(server.Close)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cache := NewRedisPromptCache(client, RedisPromptCacheConfig{
		KeyPrefix: "prompt",
		TTL:       time.Minute,
	})
	maint, ok := cache.(PromptCacheMaintenance)
	if !ok {
		t.Fatal("cache does not implement PromptCacheMaintenance")
	}

	server.Close()
	if err := maint.Clear(context.Background()); err == nil {
		t.Fatal("Clear() error = nil, want backend error")
	}
	if _, err := maint.Stats(context.Background()); err == nil {
		t.Fatal("Stats() error = nil, want backend error")
	}
}

func TestTwoLevelPromptCacheStatsUsesRemoteWhenAvailable(t *testing.T) {
	server := miniredis.RunT(t)
	t.Cleanup(server.Close)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	local := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{
		MaxEntries: 16,
		TTL:        time.Minute,
	})
	remote := NewRedisPromptCache(client, RedisPromptCacheConfig{
		KeyPrefix: "prompt",
		TTL:       time.Minute,
	})
	twoLevel := NewTwoLevelPromptCache(local, remote)

	local.Set(context.Background(), "local-only", []Message{{Role: RoleUser, Content: "local"}})
	remote.Set(context.Background(), "shared-a", []Message{{Role: RoleUser, Content: "a"}})
	remote.Set(context.Background(), "shared-b", []Message{{Role: RoleUser, Content: "b"}})

	maint, ok := twoLevel.(PromptCacheMaintenance)
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
}
