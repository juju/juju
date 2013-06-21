// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"time"
)

func SetInterval(i time.Duration) {
	interval = i
}

func RestoreInterval() {
	interval = defaultInterval
}
