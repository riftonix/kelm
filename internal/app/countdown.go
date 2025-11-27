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

type DeleteNamespacesCallback func(namespaces []string)

func CreateCountdown(
	ctx context.Context,
	env Env,
	ttlSeconds int,
	scenario string,
	deleteNamespaces DeleteNamespacesCallback,
) CountdownResult {
	if ttlSeconds <= 0 {
		logrus.Debugf("Env '%s' TTL expired for scenario %s!", env.Name, scenario)
		return InvalidTTLState
	}
	timer := time.NewTimer(time.Duration(ttlSeconds) * time.Second)
	defer timer.Stop() // Delayed timer cleanup

	select {
	case <-ctx.Done():
		// Timer canceled
		logrus.Debugf("Env '%s' TTL countdown cancelled for scenario %s.", env.Name, scenario)
		return CancelledState
	case <-timer.C:
		// Env expired
		logrus.Debugf("Env '%s' TTL expired after %d seconds for scenario %s!", env.Name, ttlSeconds, scenario)
		if scenario == "removal" && deleteNamespaces != nil {
			logrus.Infof("Force deleting namespaces for env '%s': %v", env.Name, env.Namespaces)
			deleteNamespaces(env.Namespaces)
		}
		return ExpiredState
	}
}
