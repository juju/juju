// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"time"
)

// Signal is the signature of a function used to generate a
// hook signal.
type TimedSignal func(now, lastSignal time.Time, interval time.Duration) <-chan time.Time
