// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

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

	// DefaultNetworkOperationRetryDelay is the default time
	// to wait between operation retries.
	DefaultNetworkOperationRetryDelay = 30 * time.Second

	// DefaultNetworkOperationAttempts is the default number
	// of attempts before giving up.
	DefaultNetworkOperationAttempts = 10
)

// NetworkOperationWithDefaultRetries calls the supplied function and if it returns a
// network error which is temporary, will retry a number of times before giving up.
// A default attempt strategy is used.
func NetworkOperationWitDefaultRetries(networkOp func() error, description string) func() error {
	attempt := utils.AttemptStrategy{
		Delay: DefaultNetworkOperationRetryDelay,
		Min:   DefaultNetworkOperationAttempts,
	}
	return NetworkOperationWithRetries(attempt, networkOp, description)
}

// NetworkOperationWithRetries calls the supplied function and if it returns a
// network error which is temporary, will retry a number of times before giving up.
func NetworkOperationWithRetries(strategy utils.AttemptStrategy, networkOp func() error, description string) func() error {
	return func() error {
		for a := strategy.Start(); a.Next(); {
			err := networkOp()
			if !a.HasNext() || err == nil {
				return errors.Trace(err)
			}
			if networkErr, ok := errors.Cause(err).(net.Error); !ok || !networkErr.Temporary() {
				return errors.Trace(err)
			}
			logger.Debugf("%q error, will retry: %v", description, err)
		}
		panic("unreachable")
	}
}
