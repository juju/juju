// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unittime

import (
	"time"
)

// PatchTime replaces the call to time.Now in the timer with the time specified
func PatchTime(t time.Time) {
	timeNow = func() time.Time {
		return t
	}
}
