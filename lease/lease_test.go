// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"fmt"
	"sync"
	"testing"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	coretesting "github.com/juju/juju/testing"
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
	if tok.Id == "error" {
		return errors.New("error")
	}
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

type leaseSuite struct {
	coretesting.BaseSuite
}

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
func (s *leaseSuite) TestCopyOfLeaseTokensIsolated(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)

	toksA := mgr.CopyOfLeaseTokens()
	toksB := mgr.CopyOfLeaseTokens()

	// The tokens are equivalent...
	c.Assert(toksA, gc.HasLen, 1)
	c.Check(toksA, gc.DeepEquals, toksB)

	//...but isolated.
	toksA[0].Id = "I'm a bad, bad programmer. Why would I do this?"
	c.Check(toksA[0], gc.Not(gc.Equals), toksB[0])

	//...and the cache remains intact.
	err = mgr.ReleaseLease(testNamespace, testId)
	c.Check(err, jc.ErrorIsNil)
}

func (s *leaseSuite) TestCopyOfLeaseTokensRaces(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)

	// Fill a channel with several concurrently-acquired copies...
	var wg sync.WaitGroup
	const count = 10
	results := make(chan []Token, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			results <- mgr.CopyOfLeaseTokens()
			wg.Done()
		}()
	}
	wg.Wait()

	// ...then extract all those copies for checking below...
	var allResults [][]Token
	for i := 0; i < count; i++ {
		select {
		case result := <-results:
			allResults = append(allResults, result)
		default:
			c.Fatalf("not enough results received")
		}
	}

	// ...and verify that they're all the same.
	for i := 1; i < count; i++ {
		c.Check(allResults[0], jc.DeepEquals, allResults[i])
	}
}

func (s *leaseSuite) TestClaimLeaseSuccess(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()

	ownerId, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ownerId, gc.Equals, testId)

	toks := mgr.CopyOfLeaseTokens()
	c.Assert(toks, gc.HasLen, 1)
	c.Assert(toks[0].Namespace, gc.Equals, testNamespace)
	c.Assert(toks[0].Id, gc.Equals, testId)
}

func (s *leaseSuite) TestClaimLeaseError(c *gc.C) {
	stop := make(chan struct{})
	go func() {
		err := WorkerLoop(&stubLeasePersistor{})(stop)
		c.Assert(err, gc.NotNil)
	}()
	mgr := Manager()

	_, err := mgr.ClaimLease(testNamespace, "error", testDuration)
	c.Assert(errors.Cause(err), gc.Equals, LeaseManagerErr)
}

func (s *leaseSuite) TestClaimLeaseRaces(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()

	// Run several concurrent requests for different ids in the same namespace.
	var wg sync.WaitGroup
	const count = 10
	owners := make(chan string, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			id := fmt.Sprintf("unit/%d", i)
			ownerId, err := mgr.ClaimLease(testNamespace, id, testDuration)
			c.Logf(ownerId)
			if err != nil {
				c.Check(err, gc.Equals, LeaseClaimDeniedErr)
			}
			owners <- ownerId
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Consolidate all the results, and check they agree.
	allOwners := set.NewStrings()
	for i := 0; i < count; i++ {
		select {
		case ownerId := <-owners:
			allOwners.Add(ownerId)
		default:
			c.Fatalf("not enough ownerIds received")
		}
	}
	c.Check(allOwners.Size(), gc.Equals, 1)
	c.Check(allOwners.Contains(""), jc.IsFalse)
}

func (s *leaseSuite) TestReleaseLease(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)

	err = mgr.ReleaseLease(testNamespace, testId)
	c.Assert(err, jc.ErrorIsNil)

	toks := mgr.CopyOfLeaseTokens()
	c.Assert(toks, gc.HasLen, 0)
}

type stubLeasePersistorRemoveError struct {
	stubLeasePersistor
}

func (p *stubLeasePersistorRemoveError) RemoveToken(id string) error {
	return errors.New("error")
}

func (s *leaseSuite) TestReleaseLeaseError(c *gc.C) {
	stop := make(chan struct{})
	go func() {
		err := WorkerLoop(&stubLeasePersistorRemoveError{})(stop)
		c.Assert(err, gc.NotNil)
	}()
	mgr := Manager()
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)

	err = mgr.ReleaseLease(testNamespace, testId)
	c.Assert(errors.Cause(err), gc.Equals, LeaseManagerErr)
}

func (s *leaseSuite) TestReleaseLeaseNotOwned(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)

	err = mgr.ReleaseLease(testNamespace, "1234")
	// No error returned (we log it).
	c.Assert(err, jc.ErrorIsNil)
	// But cache unaffected.
	toks := mgr.CopyOfLeaseTokens()
	c.Assert(toks, gc.HasLen, 1)
	c.Assert(toks[0].Namespace, gc.Equals, testNamespace)
	c.Assert(toks[0].Id, gc.Equals, testId)
}

func (s *leaseSuite) TestReleaseLeaseRaces(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()

	// Add several leases in different namespaces.
	const count = 10
	var namespaces []string
	for i := 0; i < count; i++ {
		namespace := fmt.Sprintf("namespace-%d", i)
		namespaces = append(namespaces, namespace)
		_, err := mgr.ClaimLease(namespace, testId, testDuration)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Release them all.
	var wg sync.WaitGroup
	for _, namespace := range namespaces {
		wg.Add(1)
		go func(namespace string) {
			err := mgr.ReleaseLease(namespace, testId)
			c.Check(err, jc.ErrorIsNil)
			wg.Done()
		}(namespace)
	}
	wg.Wait()

	// Check the cache agrees they're all released.
	toks := mgr.CopyOfLeaseTokens()
	c.Assert(toks, gc.HasLen, 0)
}

func (s *leaseSuite) TestRetrieveLease(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)

	tok := mgr.RetrieveLease(testNamespace)
	c.Check(tok.Id, gc.Equals, testId)
	c.Check(tok.Namespace, gc.Equals, testNamespace)
}

func (s *leaseSuite) TestRetrieveLeaseWithBadNamespaceFails(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()

	mgr := Manager()

	tok := mgr.RetrieveLease(testNamespace)
	c.Assert(tok, gc.DeepEquals, Token{})
}

func (s *leaseSuite) TestReleaseLeaseNotification(c *gc.C) {
	stop := make(chan struct{})
	go WorkerLoop(&stubLeasePersistor{})(stop)
	defer func() { stop <- struct{}{} }()
	mgr := Manager()
	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Assert(err, jc.ErrorIsNil)

	// Listen for the lease to be released.
	subscription := mgr.LeaseReleasedNotifier(testNamespace)
	receivedSignal := make(chan struct{})
	go func() {
		<-subscription
		receivedSignal <- struct{}{}
	}()

	// Release it
	err = mgr.ReleaseLease(testNamespace, testId)
	c.Assert(err, jc.ErrorIsNil)

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

	// Grab a lease.
	_, err := mgr.ClaimLease(testNamespace, testId, leaseDuration)
	c.Assert(err, jc.ErrorIsNil)
	leaseClaimedTime := time.Now()

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

	_, err := mgr.ClaimLease(testNamespace, testId, testDuration)
	c.Check(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)

	numRemoveCalls := 0
	persistor.RemoveTokenFn = func(id string) error {
		numRemoveCalls++
		c.Check(id, gc.Equals, testNamespace)
		return nil
	}

	// Release the lease, and the persistor should be called.
	mgr.ReleaseLease(testNamespace, testId)

	c.Check(numRemoveCalls, gc.Equals, 1)
}

func (s *leaseSuite) TestManagerDepersistsAllTokensOnStart(c *gc.C) {

	persistor := &stubLeasePersistor{}

	numCalls := 0
	testToks := []Token{
		{testNamespace, testId, time.Now().Add(testDuration)},
		{testNamespace + "2", "a" + testId, time.Now().Add(testDuration)},
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
