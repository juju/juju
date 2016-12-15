// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import "time"

func truncateDBTime(t time.Time) time.Time {
	// MongoDB only stores timestamps with ms precision.
	return t.Truncate(time.Millisecond)
}
