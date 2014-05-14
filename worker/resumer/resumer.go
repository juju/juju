// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"fmt"
	"time"

	"github.com/juju/loggo"
	"launchpad.net/tomb"
)

var logger = loggo.GetLogger("juju.worker.resumer")

// defaultInterval is the standard value for the interval setting.
const defaultInterval = time.Minute

// interval sets how often the resuming is called.
var interval = defaultInterval

// TransactionResumer defines the interface for types capable to
// resume transactions.
type TransactionResumer interface {
	// ResumeTransactions resumes all pending transactions.
	ResumeTransactions() error
}

// Resumer is responsible for a periodical resuming of pending transactions.
type Resumer struct {
	tomb tomb.Tomb
	tr   TransactionResumer
}

// NewResumer periodically resumes pending transactions.
func NewResumer(tr TransactionResumer) *Resumer {
	rr := &Resumer{tr: tr}
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
			if err := rr.tr.ResumeTransactions(); err != nil {
				logger.Errorf("cannot resume transactions: %v", err)
			}
		}
	}
}
