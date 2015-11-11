// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

var (
	// The defaults below are best suited to retries associated
	// with disk I/O timeouts, eg database operations.
	// Use the NetworkOperationWithRetries() variant to explicitly
	// use retry values better suited to different scenarios.

	// defaultNetworkOperationRetryDelay is the default time
	// to wait between operation retries.
	defaultNetworkOperationRetryDelay = 30 * time.Second

	// defaultNetworkOperationAttempts is the default number
	// of attempts before giving up.
	defaultNetworkOperationAttempts = 10
)

// networkOperationWithDefaultRetries calls the supplied function and if it returns a
// network error which is temporary, will retry a number of times before giving up.
// A default attempt strategy is used.
func networkOperationWitDefaultRetries(networkOp func() error, description string) func() error {
	attempt := utils.AttemptStrategy{
		Delay: defaultNetworkOperationRetryDelay,
		Min:   defaultNetworkOperationAttempts,
	}
	return networkOperationWithRetries(attempt, networkOp, description)
}

// networkOperationWithRetries calls the supplied function and if it returns a
// network error which is temporary, will retry a number of times before giving up.
func networkOperationWithRetries(strategy utils.AttemptStrategy, networkOp func() error, description string) func() error {
	return func() error {
		for a := strategy.Start(); ; {
			a.Next()
			err := networkOp()
			if !a.HasNext() || err == nil {
				return errors.Trace(err)
			}
			if networkErr, ok := errors.Cause(err).(net.Error); !ok || !networkErr.Temporary() {
				return errors.Trace(err)
			}
			logger.Debugf("%q error, will retry: %v", description, err)
		}
	}
}
