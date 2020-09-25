// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"sync"
)

type Probe struct {
	hasStartedLock sync.RWMutex
	hasStarted     bool
}

func (p *Probe) HasStarted() bool {
	p.hasStartedLock.RLock()
	defer p.hasStartedLock.RUnlock()
	return p.hasStarted
}

func (p *Probe) SetHasStarted() {
	p.hasStartedLock.Lock()
	defer p.hasStartedLock.Unlock()
	p.hasStarted = true
}
