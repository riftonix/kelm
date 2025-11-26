package timer

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestGetDuration(t *testing.T) {
	currentTime := time.Now().UTC()

	t.Run("positive_ttl", func(t *testing.T) {
		creationTime := currentTime.Add(-1 * time.Hour)
		ttl := "2h"
		factor := 1.0
		duration, err := GetDuration(creationTime, ttl, factor)
		if err != nil {
			t.Errorf("GetDuration returned error: %v", err)
		}
		// 2h - 1h = 1h; 1h +- 1min  (ttl * factor - creationHours)
		expectedDuration := time.Hour
		if duration < expectedDuration-time.Second || duration > expectedDuration+time.Second {
			t.Errorf("Expected duration around %v, got %v", expectedDuration, duration)
		}
	})

	t.Run("ttl_expired", func(t *testing.T) {
		creationTime := currentTime.Add(-3 * time.Hour)
		ttl := "2h"
		factor := 1.0
		duration, err := GetDuration(creationTime, ttl, factor)
		if err != nil {
			t.Errorf("GetDuration returned error: %v", err)
		}
		// Expecting 0, besause of 2h<3h
		if duration != 0 {
			t.Errorf("Expected duration 0, got %v", duration)
		}
	})

	t.Run("factor_greater_than_one", func(t *testing.T) {
		creationTime := currentTime.Add(-1 * time.Hour)
		ttl := "1h"
		factor := 2.0
		duration, err := GetDuration(creationTime, ttl, factor)
		if err != nil {
			t.Errorf("GetDuration returned error: %v", err)
		}

		// 1h * 2.0 - 1h = 1h; 1h +- 1min
		expectedDuration := time.Hour
		if duration < expectedDuration-time.Second || duration > expectedDuration+time.Second {
			t.Errorf("Expected duration around %v, got %v", expectedDuration, duration)
		}
	})

	t.Run("factor_less_than_one", func(t *testing.T) {
		creationTime := currentTime.Add(-30 * time.Minute)
		ttl := "2h"
		factor := 0.5
		duration, err := GetDuration(creationTime, ttl, factor)
		if err != nil {
			t.Errorf("GetDuration returned error: %v", err)
		}

		// 2h * 0.5 - 30m = 30m
		expectedDuration := 30 * time.Minute
		if duration < expectedDuration-time.Second || duration > expectedDuration+time.Second {
			t.Errorf("Expected duration around %v, got %v", expectedDuration, duration)
		}
	})

	t.Run("invalid_ttl_format", func(t *testing.T) {
		creationTime := currentTime
		ttl := "invalid" // Incorrect ttl format
		factor := 1.0
		duration, err := GetDuration(creationTime, ttl, factor)
		if err == nil {
			t.Error("Expected error for invalid TTL format, but got none")
		}
		if duration != 0 {
			t.Errorf("Expected duration 0 when error occurs, got %v", duration)
		}
	})
}

func TestGetEntityAge(t *testing.T) {
	currentTime := time.Now().UTC()
	t.Run("just_created", func(t *testing.T) {
		creationTime := currentTime
		age := GetEntityAge(creationTime)
		if age != 0 {
			t.Errorf("Expected age close to 0, got %v", age)
		}
	})

	t.Run("one_hour_ago", func(t *testing.T) {
		creationTime := currentTime.Add(-1 * time.Hour)
		age := GetEntityAge(creationTime)
		expectedAge := time.Hour
		logrus.Warning(age)
		if age != expectedAge {
			t.Errorf("Expected age around %v, got %v", expectedAge, age)
		}
	})

	t.Run("thirty_minutes_ago", func(t *testing.T) {
		creationTime := currentTime.Add(-30 * time.Minute)
		age := GetEntityAge(creationTime)
		expectedAge := 30 * time.Minute
		if age != expectedAge {
			t.Errorf("Expected age around %v, got %v", expectedAge, age)
		}
	})

	t.Run("future_time", func(t *testing.T) {
		creationTime := currentTime.Add(10 * time.Minute)
		age := GetEntityAge(creationTime)
		if age > 0 {
			t.Errorf("Expected negative or zero age for future creation time, got %v", age)
		}
	})
}
func TestParseTime(t *testing.T) {
	tests := []struct {
		input       string
		expectError bool
	}{
		// Valid RFC3339
		{"2023-11-25T14:04:31Z", false},
		{"2023-11-25T17:04:31+03:00", false},
		{"2023-11-25T14:04:31.123Z", false},
		// Only date (invalid for RFC3339 time)
		{"2023-11-25", true},
		// Empty string
		{"", true},
		// Invalid format
		{"not-a-date", true},
		// Valid with fractional seconds and timezone
		{"2023-11-25T14:04:31.123456789+00:00", false},
	}

	for _, testCase := range tests {
		t.Run(testCase.input, func(t *testing.T) {
			_, err := ParseTime(testCase.input)
			if testCase.expectError && err == nil {
				t.Errorf("Expected error for input %q, got nil", testCase.input)
			}
			if !testCase.expectError && err != nil {
				t.Errorf("Did not expect error for input %q, got %v", testCase.input, err)
			}
		})
	}
}

func TestGetMaxTime(t *testing.T) {
	currentTime := time.Now()
	tests := []struct {
		name         string
		t1           time.Time
		t2           time.Time
		expectedTime time.Time
	}{
		{
			name:         "t1 after t2",
			t1:           currentTime.Add(1 * time.Hour),
			t2:           currentTime,
			expectedTime: currentTime.Add(1 * time.Hour),
		},
		{
			name:         "t2 after t1",
			t1:           currentTime,
			t2:           currentTime.Add(1 * time.Hour),
			expectedTime: currentTime.Add(1 * time.Hour),
		},
		{
			name:         "t1 equals t2",
			t1:           currentTime,
			t2:           currentTime,
			expectedTime: currentTime,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := GetMaxTime(testCase.t1, testCase.t2)
			if !result.Equal(testCase.expectedTime) {
				t.Errorf("GetMaxTime() = %v, but %v expected", result, testCase.expectedTime)
			}
		})
	}
}

func TestGetMaxDuration(t *testing.T) {
	tests := []struct {
		name           string
		a              string
		b              string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "a greater than b",
			a:              "2h",
			b:              "1h",
			expectedResult: "2h",
		},
		{
			name:           "b greater than a",
			a:              "30m",
			b:              "1h",
			expectedResult: "1h",
		},
		{
			name:           "equal durations",
			a:              "45m",
			b:              "45m",
			expectedResult: "45m",
		},
		{
			name:        "invalid a",
			a:           "bad",
			b:           "1h",
			expectError: true,
		},
		{
			name:        "invalid b",
			a:           "1h",
			b:           "bad",
			expectError: true,
		},
		{
			name:        "both invalid",
			a:           "bad",
			b:           "bad",
			expectError: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := GetMaxDuration(testCase.a, testCase.b)
			if testCase.expectError {
				if err == nil {
					t.Errorf("Expected error for input (%q, %q), got nil", testCase.a, testCase.b)
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect error for input (%q, %q), got %v", testCase.a, testCase.b, err)
				}
				if result != testCase.expectedResult {
					t.Errorf("GetMaxDuration(%q, %q) = %q, want %q", testCase.a, testCase.b, result, testCase.expectedResult)
				}
			}
		})
	}
}

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
