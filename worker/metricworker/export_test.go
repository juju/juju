// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"github.com/juju/testing"
)

// PatchNotificationChannel sets the notify channel which can be used
// in tests to know that a particular worker has called its work function.
func PatchNotificationChannel(n chan string) func() {
	return testing.PatchValue(&notify, n)
}
