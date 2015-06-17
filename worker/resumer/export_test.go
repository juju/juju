// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"sync"
	"time"
)

var mu sync.Mutex

func SetInterval(i time.Duration) {
	mu.Lock()
	defer mu.Unlock()

	interval = i
}

func RestoreInterval() {
	mu.Lock()
	defer mu.Unlock()

	interval = defaultInterval
}
