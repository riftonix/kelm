package kelm

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

type CountdownResult int

const (
	ExpiredState CountdownResult = iota
	CancelledState
	InvalidTTLState
)

func CreateCountdown(ctx context.Context, envName string, ttlSeconds int, scenario string) CountdownResult {
	if ttlSeconds <= 0 {
		logrus.Debugf("Env '%s' TTL expired for scenario %s!", envName, scenario)
		return InvalidTTLState
	}
	timer := time.NewTimer(time.Duration(ttlSeconds) * time.Second)
	defer timer.Stop() // Delayed timer cleanup

	select {
	case <-ctx.Done():
		// Timer canceled
		logrus.Debugf("Env '%s' TTL countdown cancelled for scenario %s.", envName, scenario)
		return CancelledState
	case <-timer.C:
		// Env expired
		logrus.Debugf("Env '%s' TTL expired after %d seconds for scenario %s!", envName, ttlSeconds, scenario)
		return ExpiredState
	}
}
