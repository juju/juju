// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnpruner_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/txnpruner"
)

type TxnPrunerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&TxnPrunerSuite{})

func (s *TxnPrunerSuite) TestPrunes(c *gc.C) {
	fakePruner := newFakeTransactionPruner()
	interval := 10 * time.Millisecond
	p := txnpruner.New(fakePruner, interval)
	defer p.Kill()

	var t0 time.Time
	for i := 0; i < 5; i++ {
		select {
		case <-fakePruner.pruneCh:
			t1 := time.Now()
			if i > 0 {
				// Check that pruning runs at the expected interval
				// (but not the first time around as we don't know
				// when the worker actually started).
				td := t1.Sub(t0)
				c.Assert(td >= interval, jc.IsTrue, gc.Commentf("td=%s", td))
			}
			t0 = t1
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for pruning to happen")
		}
	}
}

func (s *TxnPrunerSuite) TestStops(c *gc.C) {
	success := make(chan bool)
	check := func() {
		p := txnpruner.New(newFakeTransactionPruner(), time.Minute)
		p.Kill()
		c.Assert(p.Wait(), jc.ErrorIsNil)
		success <- true
	}
	go check()

	select {
	case <-success:
	case <-time.After(testing.LongWait):
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
