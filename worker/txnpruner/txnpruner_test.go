// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnpruner_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/txnpruner"
)

type TxnPrunerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TxnPrunerSuite{})

func (s *TxnPrunerSuite) TestPrunes(c *gc.C) {
	fakePruner := newFakeTransactionPruner()
	testClock := testing.NewClock(time.Now())
	interval := time.Minute
	p := txnpruner.New(fakePruner, interval, testClock)
	defer p.Kill()

	select {
	case <-testClock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to stat")
	}
	c.Logf("pruner running and waiting: %s (%s)", testClock.Now(), time.Now())
	// Show that we prune every minute
	for i := 0; i < 5; i++ {
		testClock.Advance(interval)
		c.Logf("loop %d: %s (%s)", i, testClock.Now(), time.Now())
		select {
		case <-fakePruner.pruneCh:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for pruning to happen")
		}
		// Now we need to wait for the txn pruner to call clock.After again
		// before we advance the clock, or it will be waiting for the wrong time.
		select {
		case <-testClock.Alarms():
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for worker to loop around")
		}
	}
}

func (s *TxnPrunerSuite) TestStops(c *gc.C) {
	success := make(chan bool)
	check := func() {
		p := txnpruner.New(newFakeTransactionPruner(), time.Minute, clock.WallClock)
		p.Kill()
		c.Check(p.Wait(), jc.ErrorIsNil)
		success <- true
	}
	go check()

	select {
	case <-success:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to stop")
	}
}

func newFakeTransactionPruner() *fakeTransactionPruner {
	return &fakeTransactionPruner{
		pruneCh: make(chan bool),
	}
}

type fakeTransactionPruner struct {
	pruneCh chan bool
}

// MaybePruneTransactions implements the txnpruner.TransactionPruner
// interface.
func (p *fakeTransactionPruner) MaybePruneTransactions() error {
	p.pruneCh <- true
	return nil
}
