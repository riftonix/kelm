package kelm

import (
	"context"
	"testing"
	"time"
)

func TestCreateCountdown(t *testing.T) {
	// 1. Invalid TTL (<= 0)
	ctx := context.Background()
	if result := CreateCountdown(ctx, "env1", 0, "sc1"); result != InvalidTTLState {
		t.Errorf("Expected InvalidTTLState for ttlSeconds=0, got %v", result)
	}
	if result := CreateCountdown(ctx, "env1", -5, "sc2"); result != InvalidTTLState {
		t.Errorf("Expected InvalidTTLState for ttlSeconds=-5, got %v", result)
	}

	// 2. Cancelled context before timer fires
	ctx2, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	result2 := CreateCountdown(ctx2, "env2", 1, "sc3")
	if result2 != CancelledState {
		t.Errorf("Expected CancelledState when context cancelled, got %v", result2)
	}

	// 3. Timer expires normally
	ctx3 := context.Background()
	start := time.Now()
	result3 := CreateCountdown(ctx3, "env3", 1, "sc4")
	elapsed := time.Since(start)
	if result3 != ExpiredState {
		t.Errorf("Expected ExpiredState for normal timer expiry, got %v", result3)
	}
	if elapsed < time.Second || elapsed > 2*time.Second {
		t.Errorf("Expected function to wait ~1s, elapsed: %v", elapsed)
	}
}
