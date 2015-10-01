// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager

import "github.com/juju/testing"

// PatchNotificationChannel sets the notify channel which can be used
// in tests.
func PatchNotificationChannel(n chan string) func() {
	return testing.PatchValue(&notify, n)
}
