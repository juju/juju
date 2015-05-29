// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	"errors"
	"sync"
	"time"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/resumer"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
	testInterval := 10 * time.Millisecond
	resumer.SetInterval(testInterval)
	defer resumer.RestoreInterval()

	rr := resumer.NewResumer(s.mockState)
	defer func() { c.Assert(rr.Stop(), gc.IsNil) }()

	time.Sleep(10 * testInterval)

	// Check that a number of calls has happened with a time
	// difference somewhere between the interval and twice the
	// interval. A more precise time behavior cannot be
	// specified due to the load during the test.
	s.mockState.mu.Lock()
	defer s.mockState.mu.Unlock()
	c.Assert(len(s.mockState.timestamps) > 0, jc.IsTrue)
	for i := 1; i < len(s.mockState.timestamps); i++ {
		diff := s.mockState.timestamps[i].Sub(s.mockState.timestamps[i-1])

		c.Assert(diff >= testInterval, jc.IsTrue)
		c.Assert(diff <= 4*testInterval, jc.IsTrue)
		s.mockState.CheckCall(c, i-1, "ResumeTransactions")
	}
}

func (s *ResumerSuite) TestResumeTransactionsFailure(c *gc.C) {
	// Force the first call to ResumeTransactions() to fail, the
	// remaining returning no error.
	s.mockState.SetErrors(errors.New("boom!"))

	// Shorter interval and mock help to count
	// the resumer calls in a given timespan.
	testInterval := 10 * time.Millisecond
	resumer.SetInterval(testInterval)
	defer resumer.RestoreInterval()

	rr := resumer.NewResumer(s.mockState)
	defer func() { c.Assert(rr.Stop(), gc.IsNil) }()

	// For 4 intervals at least 3 calls should be made.
	time.Sleep(4 * testInterval)
	s.mockState.CheckCallNames(c,
		"ResumeTransactions",
		"ResumeTransactions",
		"ResumeTransactions",
	)
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
	tr.timestamps = append(tr.timestamps, time.Now())
	tr.MethodCall(tr, "ResumeTransactions")
	err := tr.NextErr()
	tr.mu.Unlock()
	return err
}

var _ resumer.TransactionResumer = (*transactionResumerMock)(nil)
