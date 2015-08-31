// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envworkermanager

import (
	"time"

	"github.com/juju/testing"
)

// PatchNotify sets the notify func which can be used in tests to know that a
// particular worker has called its work function.
func PatchNotify(n func()) func() {
	return testing.PatchValue(&notify, n)
}

func PatchRIPTime(t time.Duration) func() {
	return testing.PatchValue(&ripTime, t)
}
