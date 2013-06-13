// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"fmt"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/tomb"
	"time"
)

// interval sets how often the resuming is called.
const interval = time.Minute

// Resumer is responsible for a periodical resuming of pending transactions.
type Resumer struct {
	tomb tomb.Tomb
	st   *state.State
}

// NewResumer ...
func NewResumer(st *state.State) *Resumer {
	rr := &Resumer{st: st}
	go func() {
		defer rr.tomb.Done()
		rr.tomb.Kill(rr.loop())
	}()
	return rr
}

func (rr *Resumer) String() string {
	return fmt.Sprintf("resumer")
}

func (rr *Resumer) Kill() {
	rr.tomb.Kill(nil)
}

func (rr *Resumer) Stop() error {
	rr.tomb.Kill(nil)
	return rr.tomb.Wait()
}

func (rr *Resumer) Wait() error {
	return rr.tomb.Wait()
}

func (rr *Resumer) loop() error {
	for {
		select {
		case <-rr.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(interval):
			if err := rr.st.ResumeTransactions(); err != nil {
				log.Errorf("worker/resumer: cannot resume transactions: %v", err)
			}
		}
	}
	panic("unreachable")
}
