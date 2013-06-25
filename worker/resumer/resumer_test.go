// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/resumer"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type ResumerSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&ResumerSuite{})

func (s *ResumerSuite) TestRunStopWithState(c *C) {
	// Test with state ensures that state fulfills the
	// TransactionResumer interface.
	rr := resumer.NewResumer(s.State)

	c.Assert(rr.Stop(), IsNil)
}

func (s *ResumerSuite) TestResumerCalls(c *C) {
	// Shorter interval and mock help to count
	// the resumer calls in a given timespan.
	resumer.SetInterval(10 * time.Millisecond)
	defer resumer.RestoreInterval()

	tr := &transactionResumerMock{0}
	rr := resumer.NewResumer(tr)
	defer func() { c.Assert(rr.Stop(), IsNil) }()

	time.Sleep(55 * time.Millisecond)
	c.Assert(tr.counter, Equals, 5)
}

// transactionResumerMock is used to check the
// calls of ResumeTransactions().
type transactionResumerMock struct {
	counter int
}

func (t *transactionResumerMock) ResumeTransactions() error {
	t.counter++
	return nil
}
