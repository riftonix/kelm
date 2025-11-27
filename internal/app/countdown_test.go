package kelm

import (
	"context"
	"testing"
	"time"
)

func TestCreateCountdown(t *testing.T) {
	// 1. Invalid TTL (<= 0)
	ctx := context.Background()
	env := Env{Name: "env1", Namespaces: []string{"ns1", "ns2"}}
	if result := CreateCountdown(ctx, env, 0, "sc1", nil); result != InvalidTTLState {
		t.Errorf("Expected InvalidTTLState for ttlSeconds=0, got %v", result)
	}
	if result := CreateCountdown(ctx, env, -5, "sc2", nil); result != InvalidTTLState {
		t.Errorf("Expected InvalidTTLState for ttlSeconds=-5, got %v", result)
	}

	// 2. Cancelled context before timer fires
	ctx2, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	env2 := Env{Name: "env2", Namespaces: []string{"ns3"}}
	result2 := CreateCountdown(ctx2, env2, 1, "sc3", nil)
	if result2 != CancelledState {
		t.Errorf("Expected CancelledState when context cancelled, got %v", result2)
	}

	// 3. Timer expires normally
	ctx3 := context.Background()
	start := time.Now()
	env3 := Env{Name: "env3", Namespaces: []string{"ns4", "ns5"}}
	var deleted []string
	result3 := CreateCountdown(ctx3, env3, 1, "removal", func(namespaces []string) {
		deleted = append(deleted, namespaces...)
	})
	elapsed := time.Since(start)
	if result3 != ExpiredState {
		t.Errorf("Expected ExpiredState for normal timer expiry, got %v", result3)
	}
	if elapsed < time.Second || elapsed > 2*time.Second {
		t.Errorf("Expected function to wait ~1s, elapsed: %v", elapsed)
	}
	if len(deleted) != 2 || deleted[0] != "ns4" || deleted[1] != "ns5" {
		t.Errorf("Expected callback to be called with namespaces, got %v", deleted)
	}
}
