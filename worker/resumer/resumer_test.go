// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	"errors"
	"sync"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/resumer"
)

type ResumerSuite struct {
	coretesting.BaseSuite

	mockState *transactionResumerMock
}

var _ = gc.Suite(&ResumerSuite{})

// Ensure *state.State implements TransactionResumer
var _ resumer.TransactionResumer = (*state.State)(nil)

func (s *ResumerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.mockState = &transactionResumerMock{
		Stub: &testing.Stub{},
	}
}

func (s *ResumerSuite) TestRunStopWithMockState(c *gc.C) {
	rr := resumer.NewResumer(s.mockState)
	c.Assert(rr.Stop(), gc.IsNil)
}

func (s *ResumerSuite) TestResumerCalls(c *gc.C) {
	// Shorter interval and mock help to count
	// the resumer calls in a given timespan.
	testInterval := coretesting.ShortWait
	resumer.SetInterval(testInterval)
	defer resumer.RestoreInterval()

	rr := resumer.NewResumer(s.mockState)
	defer func() { c.Assert(rr.Stop(), gc.IsNil) }()

	time.Sleep(10 * testInterval)

	s.mockState.CheckTimestamps(c, testInterval)
}

func (s *ResumerSuite) TestResumeTransactionsFailure(c *gc.C) {
	// Force the first call to ResumeTransactions() to fail, the
	// remaining returning no error.
	s.mockState.SetErrors(errors.New("boom!"))

	// Shorter interval and mock help to count
	// the resumer calls in a given timespan.
	testInterval := coretesting.ShortWait
	resumer.SetInterval(testInterval)
	defer resumer.RestoreInterval()

	rr := resumer.NewResumer(s.mockState)
	defer func() { c.Assert(rr.Stop(), gc.IsNil) }()

	// For 4 intervals between 2 and 3 calls should be made.
	time.Sleep(4 * testInterval)
	s.mockState.CheckNumCallsBetween(c, 2, 3)
}

// transactionResumerMock is used to check the
// calls of ResumeTransactions().
type transactionResumerMock struct {
	*testing.Stub

	mu         sync.Mutex
	timestamps []time.Time
}

func (tr *transactionResumerMock) ResumeTransactions() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.timestamps = append(tr.timestamps, time.Now())
	tr.MethodCall(tr, "ResumeTransactions")
	return tr.NextErr()
}

func (tr *transactionResumerMock) CheckNumCallsBetween(c *gc.C, minCalls, maxCalls int) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	// To combat test flakyness (see bug #1462412) we're expecting up
	// to maxCalls, but at least minCalls.
	calls := tr.Stub.Calls()
	c.Assert(len(calls), jc.GreaterThan, minCalls-1)
	c.Assert(len(calls), jc.LessThan, maxCalls+1)
	for _, call := range calls {
		c.Check(call.FuncName, gc.Equals, "ResumeTransactions")
	}
}

func (tr *transactionResumerMock) CheckTimestamps(c *gc.C, testInterval time.Duration) {
	// Check that a number of calls has happened with a time
	// difference somewhere between the interval and twice the
	// interval. A more precise time behavior cannot be
	// specified due to the load during the test.
	tr.mu.Lock()
	defer tr.mu.Unlock()

	longestInterval := 4 * testInterval
	c.Assert(len(tr.timestamps) > 0, jc.IsTrue)
	for i := 1; i < len(tr.timestamps); i++ {
		diff := tr.timestamps[i].Sub(tr.timestamps[i-1])

		c.Assert(diff >= testInterval, jc.IsTrue)
		c.Assert(diff <= longestInterval, jc.IsTrue)
		tr.Stub.CheckCall(c, i-1, "ResumeTransactions")
	}
}

var _ resumer.TransactionResumer = (*transactionResumerMock)(nil)
