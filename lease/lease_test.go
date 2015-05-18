// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"testing"
	"time"

	coretesting "github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) { gc.TestingT(t) }

const (
	testNamespace = "leadership-stub-service"
	testId        = "stub-unit/0"
	testDuration  = 30 * time.Hour
)

var (
	_ = gc.Suite(&leaseSuite{})
)

type stubLeasePersistor struct {
	WriteTokenFn      func(string, Token) error
	RemoveTokenFn     func(string) error
	PersistedTokensFn func() ([]Token, error)
}

func (p *stubLeasePersistor) WriteToken(id string, tok Token) error {
	if p.WriteTokenFn != nil {
		return p.WriteTokenFn(id, tok)
	}
	return nil
}

func (p *stubLeasePersistor) RemoveToken(id string) error {
	if p.RemoveTokenFn != nil {
		return p.RemoveTokenFn(id)
	}
	return nil
}

func (p *stubLeasePersistor) PersistedTokens() ([]Token, error) {
	if p.PersistedTokensFn != nil {
		return p.PersistedTokensFn()
	}
	return nil, nil
}

type leaseSuite struct{}

func (s *leaseSuite) TestSingleton(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()

	copyA := Manager()
	copyB := Manager()

	c.Assert(copyA, gc.NotNil)
	c.Assert(copyA, gc.Equals, copyB)
}

// TestTokenListIsolation ensures that the copy of the lease tokens we
// get is truly a copy and thus isolated from all other code.
func (s *leaseSuite) TestTokenListIsolation(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()

	mgr.ClaimLease(testNamespace, testId, testDuration)
	toksA := mgr.CopyOfLeaseTokens()
	toksB := mgr.CopyOfLeaseTokens()

	// The tokens are equivalent...
	c.Assert(toksA, gc.HasLen, 1)
	c.Check(toksA, gc.DeepEquals, toksB)

	//...but isolated.
	toksA[0].Id = "I'm a bad, bad programmer. Why would I do this?"
	c.Check(toksA[0], gc.Not(gc.Equals), toksB[0])

	//...and the cache remains in tact.
	err := mgr.ReleaseLease(testNamespace, testId)
	c.Check(err, gc.IsNil)
}

func (s *leaseSuite) TestClaimLease(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()
	ownerId, err := mgr.ClaimLease(testNamespace, testId, testDuration)

	c.Assert(err, gc.IsNil)
	c.Assert(ownerId, gc.Equals, testId)

	toks := mgr.CopyOfLeaseTokens()
	c.Assert(toks, gc.HasLen, 1)
	c.Assert(toks[0].Namespace, gc.Equals, testNamespace)
	c.Assert(toks[0].Id, gc.Equals, testId)
}

func (s *leaseSuite) TestReleaseLease(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()

	ownerId, err := mgr.ClaimLease(testNamespace, testId, 30*time.Hour)
	c.Assert(err, gc.IsNil)
	c.Assert(ownerId, gc.Equals, testId)

	err = mgr.ReleaseLease(testNamespace, testId)
	c.Assert(err, gc.IsNil)

	toks := mgr.CopyOfLeaseTokens()
	c.Assert(toks, gc.HasLen, 0)
}

func (s *leaseSuite) TestReleaseLeaseNotification(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()

	// Grab a lease.
	_, err := mgr.ClaimLease(testNamespace, testId, 30*time.Hour)
	c.Assert(err, gc.IsNil)

	// Listen for it to be released.
	subscription := mgr.LeaseReleasedNotifier(testNamespace)
	receivedSignal := make(chan struct{})
	go func() {
		<-subscription
		receivedSignal <- struct{}{}
	}()

	// Release it
	err = mgr.ReleaseLease(testNamespace, testId)
	c.Assert(err, gc.IsNil)

	select {
	case <-receivedSignal:
	case <-time.After(coretesting.LongWait):
		c.Errorf("Failed to unblock after release. Waited for %s", coretesting.LongWait)
	}
}

func (s *leaseSuite) TestLeaseExpiration(c *gc.C) {

	// WARNING: This code may be load-sensitive. Unfortunately it must
	// deal with ellapsed time since this is the nature of the code
	// it is testing. For that reason, we try a few times to see if we
	// can get a successful run.

	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()

	const (
		leaseDuration      = 500 * time.Millisecond
		acceptableOverhead = 50 * time.Millisecond
	)

	if leaseDuration+acceptableOverhead > coretesting.LongWait {
		panic("This test will always fail.")
	}

	// Listen for releases before sending the claim to avoid the
	// overhead which may affect our timing measurements.
	mgr := Manager()
	subscription := mgr.LeaseReleasedNotifier(testNamespace)
	receivedSignal := make(chan struct{})

	var leaseClaimedTime time.Time
	go func() {

		<-subscription
		leaseReleasedTime := time.Now()

		// Ensure we didn't release too early or too late.
		switch elapsed := leaseReleasedTime.Sub(leaseClaimedTime); {
		default:
			receivedSignal <- struct{}{}
		case elapsed > leaseDuration+acceptableOverhead:
			fallthrough
		case elapsed < leaseDuration-acceptableOverhead:
			c.Errorf(
				"Expected the lease to be released in %s, but it was released in %s",
				leaseDuration,
				elapsed,
			)
		}
	}()

	// Grab a lease.
	_, err := mgr.ClaimLease(testNamespace, testId, leaseDuration)
	leaseClaimedTime = time.Now()
	c.Assert(err, gc.IsNil)

	// Wait for the all-clear, or a time-out.
	select {
	case <-receivedSignal:
	case <-time.After(coretesting.LongWait):
		c.Errorf("Failed to unblock after release. Waited for %s", coretesting.LongWait)
	}
}

func (s *leaseSuite) TestManagerPeresistsOnClaims(c *gc.C) {

	persistor := &stubLeasePersistor{}

	stop := make(chan struct{})
	go WorkerLoop(persistor)(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()

	numWriteCalls := 0
	persistor.WriteTokenFn = func(id string, tok Token) error {
		numWriteCalls++

		c.Assert(tok, gc.NotNil)
		c.Check(tok.Namespace, gc.Equals, testNamespace)
		c.Check(tok.Id, gc.Equals, testId)
		c.Check(id, gc.Equals, testNamespace)

		return nil
	}

	mgr.ClaimLease(testNamespace, testId, testDuration)

	c.Check(numWriteCalls, gc.Equals, 1)
}

func (s *leaseSuite) TestManagerRemovesOnRelease(c *gc.C) {

	persistor := &stubLeasePersistor{}

	stop := make(chan struct{})
	go WorkerLoop(persistor)(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()

	// Grab a lease.
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, gc.IsNil)

	numRemoveCalls := 0
	persistor.RemoveTokenFn = func(id string) error {
		numRemoveCalls++
		c.Check(id, gc.Equals, testNamespace)
		return nil
	}

	// Release the lease, and the peresitor should be called.
	mgr.ReleaseLease(testNamespace, testId)

	c.Check(numRemoveCalls, gc.Equals, 1)
}

func (s *leaseSuite) TestManagerDepersistsAllTokensOnStart(c *gc.C) {

	persistor := &stubLeasePersistor{}

	numCalls := 0
	testToks := []Token{
		Token{testNamespace, testId, time.Now().Add(testDuration)},
		Token{testNamespace + "2", "a" + testId, time.Now().Add(testDuration)},
	}
	persistor.PersistedTokensFn = func() ([]Token, error) {

		numCalls++
		return testToks, nil
	}

	stop := make(chan struct{})
	go WorkerLoop(persistor)(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()

	// NOTE: This call will naturally block until the worker loop is
	// sucessfully pumping. Place all checks below here.
	heldToks := mgr.CopyOfLeaseTokens()

	c.Assert(numCalls, gc.Equals, 1)

	for _, heldTok := range heldToks {
		found := false
		for _, testTok := range testToks {
			found, _ = gc.DeepEquals.Check([]interface{}{testTok, heldTok}, []string{})
			if found {
				break
			}
		}
		if !found {
			c.Log("The manager is not managing the expected token list.\nNOTE: Test is coded so that order does not matter.")
			c.Assert(heldToks, gc.DeepEquals, testToks)
		}
	}
}
