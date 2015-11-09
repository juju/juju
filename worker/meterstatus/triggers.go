// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"time"

	"github.com/juju/utils/clock"
)

type TriggerCreator func(WorkerState, string, time.Time, clock.Clock, time.Duration, time.Duration) (<-chan time.Time, <-chan time.Time)

// GetTriggers returns the signal channels for state transitions based on the current state.
// It controls the transitions of the inactive meter status worker.
//
// In a simple case, the transitions are trivial:
//
// D------------------A----------------------R--------------------->
//
// D - disconnect time
// A - amber status triggered
// R - red status triggered
//
// The problem arises from the fact that the lifetime of the worker can
// be interrupted, possibly with significant portions of the duration missing.
func GetTriggers(
	wst WorkerState,
	status string,
	disconnectedAt time.Time,
	clk clock.Clock,
	amberGracePeriod time.Duration,
	redGracePeriod time.Duration) (<-chan time.Time, <-chan time.Time) {

	now := clk.Now()

	if wst == Done {
		return nil, nil
	}

	if wst <= WaitingAmber && status == "RED" {
		// If the current status is already RED, we don't want to deescalate.
		wst = WaitingRed
		//	} else if wst <= WaitingAmber && now.Sub(disconnectedAt) >= amberGracePeriod {
		// If we missed the transition to amber, activate it.
		//		wst = WaitingRed
	} else if wst < Done && now.Sub(disconnectedAt) >= redGracePeriod {
		// If we missed the transition to amber and it's time to transition to RED, go straight to RED.
		wst = WaitingRed
	}

	if wst == WaitingRed {
		redSignal := clk.After(redGracePeriod - now.Sub(disconnectedAt))
		return nil, redSignal
	}
	if wst == WaitingAmber || wst == Uninitialized {
		amberSignal := clk.After(amberGracePeriod - now.Sub(disconnectedAt))
		redSignal := clk.After(redGracePeriod - now.Sub(disconnectedAt))
		return amberSignal, redSignal
	}
	return nil, nil
}
