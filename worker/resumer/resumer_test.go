// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

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

var _ = gc.Suite(&ResumerSuite{})

func (s *ResumerSuite) TestRunStopWithState(c *gc.C) {
	// Test with state ensures that state fulfills the
	// TransactionResumer interface.
	rr := resumer.NewResumer(s.State)

	c.Assert(rr.Stop(), gc.IsNil)
}

func (s *ResumerSuite) TestResumerCalls(c *gc.C) {
	// Shorter interval and mock help to count
	// the resumer calls in a given timespan.
	testInterval := 10 * time.Millisecond
	resumer.SetInterval(testInterval)
	defer resumer.RestoreInterval()

	tr := &transactionResumerMock{[]time.Time{}}
	rr := resumer.NewResumer(tr)
	defer func() { c.Assert(rr.Stop(), gc.IsNil) }()

	time.Sleep(10 * testInterval)

	// Check that a numner of calls has happened with a time
	// difference somewhere between the interval and twice the
	// interval. A more precise time behavior cannot be
	// specified due to the load during the test.
	c.Assert(len(tr.timestamps) > 0, gc.Equals, true)
	for i := 1; i < len(tr.timestamps); i++ {
		diff := tr.timestamps[i].Sub(tr.timestamps[i-1])

		c.Assert(diff >= testInterval, gc.Equals, true)
		c.Assert(diff <= 2*testInterval, gc.Equals, true)
	}
}

// transactionResumerMock is used to check the
// calls of ResumeTransactions().
type transactionResumerMock struct {
	timestamps []time.Time
}

func (tr *transactionResumerMock) ResumeTransactions() error {
	tr.timestamps = append(tr.timestamps, time.Now())
	return nil
}
