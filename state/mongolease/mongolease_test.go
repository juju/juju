// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongolease

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/mongo"
)

var _ = gc.Suite(&mongoLeaseSuite{})

type mongoLeaseSuite struct {
	testing.IsolationSuite
	testing.MgoSuite
	db         *mgo.Database
	leaseStore *mongoLeaseStore
	// errorLog loggo.Logger
}

const (
	leasesC   = "testleaseholders"
	expiriesC = "testleaseexpiries"
)

func (s *mongoLeaseSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.IsolationSuite.SetUpSuite(c)
}

func (s *mongoLeaseSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *mongoLeaseSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.IsolationSuite.SetUpTest(c)
	s.db = s.Session.DB("juju")
}

func (s *mongoLeaseSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

// TestMongo exposes database operations. It uses a real database -- we can't mock
// mongo out, we need to check it really actually works -- but it's good to
// have the runner accessible for adversarial transaction tests.
type TestMongo struct {
	database *mgo.Database
	runner   jujutxn.Runner
	txnErr   error
}

// NewTestMongo returns a *TestMongo backed by the supplied database.
func NewTestMongo(database *mgo.Database) *TestMongo {
	return &TestMongo{
		database: database,
		runner: jujutxn.NewRunner(jujutxn.RunnerParams{
			Database: database,
		}),
	}
}

// GetCollection is part of the lease.TestMongo interface.
func (m *TestMongo) GetCollection(name string) (mongo.Collection, func()) {
	return mongo.CollectionFromName(m.database, name)
}

// RunTransaction is part of the lease.TestMongo interface.
func (m *TestMongo) RunTransaction(getTxn jujutxn.TransactionSource) error {
	if m.txnErr != nil {
		return m.txnErr
	}
	return m.runner.Run(getTxn)
}

func (s *mongoLeaseSuite) SetupLeaseStore(_ *gc.C, timeFunc func() int64) {
	tm := NewTestMongo(s.db)
	expiryCollection, close1 := tm.GetCollection(expiriesC)
	s.AddCleanup(func(*gc.C) { close1() })
	holdersCollection, close2 := mongo.CollectionFromName(s.db, leasesC)
	s.AddCleanup(func(*gc.C) { close2() })
	s.leaseStore = &mongoLeaseStore{
		expiryCollection:  expiryCollection,
		holdersCollection: holdersCollection,
		mongo:             tm,
		globalTimeFunc:    timeFunc,
		logger:            loggo.GetLogger("mongo-leases"),
	}
}

func (s *mongoLeaseSuite) getExpiries(c *gc.C) []leaseExpiryDoc {
	c.Assert(s.leaseStore.expiryCollection, gc.NotNil)
	var entries []leaseExpiryDoc
	err := s.leaseStore.expiryCollection.Find(bson.M{}).All(&entries)
	c.Assert(err, jc.ErrorIsNil)
	return entries
}

func (s *mongoLeaseSuite) expectOneExpiry(c *gc.C) leaseExpiryDoc {
	allExpiries := s.getExpiries(c)
	c.Assert(allExpiries, gc.HasLen, 1)
	return allExpiries[0]
}

func (s *mongoLeaseSuite) getHolders(c *gc.C) []leaseHolderDoc {
	c.Assert(s.leaseStore.holdersCollection, gc.NotNil)
	var holders []leaseHolderDoc
	err := s.leaseStore.holdersCollection.Find(bson.M{}).All(&holders)
	c.Assert(err, jc.ErrorIsNil)
	return holders
}

func (s *mongoLeaseSuite) expectOneHolder(c *gc.C) leaseHolderDoc {
	allHolders := s.getHolders(c)
	c.Assert(allHolders, gc.HasLen, 1)
	return allHolders[0]
}

func (s *mongoLeaseSuite) TestClaimLease(c *gc.C) {
	s.SetupLeaseStore(c, func() int64 { return 12345 })
	stopChan := make(chan struct{})
	err := s.leaseStore.ClaimLease(lease.Key{
		Namespace: "application",
		ModelUUID: "deadbeef",
		Lease:     "app",
	}, lease.Request{
		Holder:   "mytest",
		Duration: time.Second,
	}, stopChan,
	)
	c.Assert(err, jc.ErrorIsNil)
	entry := s.expectOneExpiry(c)
	c.Check(entry.ModelUUID, gc.Equals, "deadbeef")
	c.Check(entry.Namespace, gc.Equals, "application")
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	c.Check(entry.Expiry, gc.Equals, int64(12345+time.Second))
	holder := s.expectOneHolder(c)
	c.Check(holder.ModelUUID, gc.Equals, "deadbeef")
	c.Check(holder.Namespace, gc.Equals, "application")
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
}

func (s *mongoLeaseSuite) TestClaimClaimedLease(c *gc.C) {
	s.SetupLeaseStore(c, func() int64 { return 12345 })
	key := lease.Key{
		Namespace: "application",
		ModelUUID: "deadbeef",
		Lease:     "app",
	}
	req := lease.Request{
		Holder:   "mytest",
		Duration: time.Second,
	}
	stopChan := make(chan struct{})
	err := s.leaseStore.ClaimLease(key, req, stopChan)
	c.Assert(err, jc.ErrorIsNil)
	entry := s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	holder := s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
	req.Holder = "othertest"
	err = s.leaseStore.ClaimLease(key, req, stopChan)
	c.Assert(err, gc.NotNil)
	c.Check(errors.Cause(err), gc.Equals, lease.ErrClaimDenied)
	c.Check(err.Error(), gc.Equals,
		`unable to claim: lease [deadbe] application "app" held by "mytest" until 1000012345 (pinned: false): lease claim denied`)
	// The lease holder should not have changed
	entry = s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	holder = s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
}

func (s *mongoLeaseSuite) TestClaimExpiredLease(c *gc.C) {
	s.SetupLeaseStore(c, func() int64 { return 12345 })
	key := lease.Key{
		Namespace: "application",
		ModelUUID: "deadbeef",
		Lease:     "app",
	}
	req := lease.Request{
		Holder:   "mytest",
		Duration: time.Second,
	}
	stopChan := make(chan struct{})
	err := s.leaseStore.ClaimLease(key, req, stopChan)
	c.Assert(err, jc.ErrorIsNil)
	entry := s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	holder := s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
	// To mimic an expired lease, we delete the 'expiries' record (easiest to just remove everything)
	_, err = s.leaseStore.expiryCollection.Writeable().RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	req = lease.Request{
		Holder:   "othertest",
		Duration: time.Minute,
	}
	err = s.leaseStore.ClaimLease(key, req, stopChan)
	c.Assert(err, jc.ErrorIsNil)
	// The lease holder should not have changed
	entry = s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "othertest")
	c.Check(entry.Expiry, gc.Equals, int64(12345+time.Minute))
	holder = s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "othertest")
}

func (s *mongoLeaseSuite) TestClaimContendedLease(c *gc.C) {
	// We create a bunch of claimer objects, and have them all try to claim the same lease with a different holder
	// This should test whether or presumptions around atomicity are handled.
	// start with the normal one
	timeFunc := func() int64 { return 12345 }
	s.SetupLeaseStore(c, timeFunc)
	const concurrent = 10
	key := lease.Key{
		Namespace: "application",
		ModelUUID: "deadbeef",
		Lease:     "app",
	}
	claims := make(chan string)
	done := make(chan bool)
	tryClaim := func(store *mongoLeaseStore, holder string) {
		defer func() {
			select {
			case done <- true:
			case <-time.After(5 * time.Second):
				c.Errorf("timed out trying to report Done")
			}
		}()
		req := lease.Request{
			Holder:   holder,
			Duration: time.Second,
		}
		err := store.ClaimLease(key, req, nil)
		if err == nil {
			select {
			case claims <- holder:
			case <-time.After(5 * time.Second):
				c.Errorf("timed out trying to record a claimed lease for %q", holder)
			}
		} else if errors.Cause(err) == lease.ErrClaimDenied {
			// when having contention, most of them should fail with ErrClaimeDenied
		} else {
			c.Errorf("claiming failed for the wrong reason: %v", err)
		}
	}
	for i := 0; i < concurrent; i++ {
		session := s.db.Session.Copy()
		// These defers are correct, because we want them to exist until all goroutines finish
		defer session.Close()
		db := s.db.With(session)
		tm := NewTestMongo(db)
		holders, close1 := tm.GetCollection(leasesC)
		defer close1()
		expiries, close2 := tm.GetCollection(expiriesC)
		defer close2()
		store := &mongoLeaseStore{
			holdersCollection: holders,
			expiryCollection:  expiries,
			mongo:             tm,
			globalTimeFunc:    timeFunc,
			logger:            loggo.GetLogger("mongo-leases"),
		}
		go tryClaim(store, fmt.Sprintf("holder/%d", i))
	}
	leftToFinish := concurrent
	firstClaim := ""
	timeout := time.After(5 * time.Second)
	for leftToFinish > 0 {
		select {
		case claim := <-claims:
			if firstClaim == "" {
				firstClaim = claim
			} else {
				c.Errorf("claim succeeded for %q while already succeeded for %q")
			}
		case <-done:
			leftToFinish--
		case <-timeout:
			c.Fatalf("timed out waiting for all stores to respond")
		}
	}
	// At least one of them should have succeeded
	c.Check(firstClaim, gc.Not(gc.Equals), "")
}

// TODO (jam 2022-03-05): We should test that ClaimLease exits early if stopChan is closed

func (s *mongoLeaseSuite) TestExtendLease(c *gc.C) {
	currentTime := int64(12345)
	s.SetupLeaseStore(c, func() int64 { return currentTime })
	key := lease.Key{
		Namespace: "application",
		ModelUUID: "deadbeef",
		Lease:     "app",
	}
	req := lease.Request{
		Holder:   "mytest",
		Duration: time.Second,
	}
	err := s.leaseStore.ClaimLease(key, req, nil)
	c.Assert(err, jc.ErrorIsNil)
	entry := s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	c.Check(entry.Expiry, gc.Equals, int64(12345+time.Second))
	holder := s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
	currentTime += int64(500 * time.Millisecond)
	err = s.leaseStore.ExtendLease(key, req, nil)
	c.Assert(err, jc.ErrorIsNil)
	entry = s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	c.Check(entry.Expiry, gc.Equals, currentTime+int64(time.Second))
	holder = s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
}

func (s *mongoLeaseSuite) TestExtendLeaseNotHeld(c *gc.C) {
	currentTime := int64(12345)
	s.SetupLeaseStore(c, func() int64 { return currentTime })
	key := lease.Key{
		Namespace: "application",
		ModelUUID: "deadbeef",
		Lease:     "app",
	}
	req := lease.Request{
		Holder:   "mytest",
		Duration: time.Second,
	}
	err := s.leaseStore.ExtendLease(key, req, nil)
	// Denied because we aren't the current holder
	c.Assert(errors.Cause(err), gc.Equals, lease.ErrNotHeld)
	expiries := s.getExpiries(c)
	c.Check(expiries, gc.HasLen, 0)
	holders := s.getHolders(c)
	c.Check(holders, gc.HasLen, 0)
}

func (s *mongoLeaseSuite) TestExtendLeaseOtherHeld(c *gc.C) {
	currentTime := int64(12345)
	s.SetupLeaseStore(c, func() int64 { return currentTime })
	key := lease.Key{
		Namespace: "application",
		ModelUUID: "deadbeef",
		Lease:     "app",
	}
	req := lease.Request{
		Holder:   "mytest",
		Duration: time.Second,
	}
	err := s.leaseStore.ClaimLease(key, req, nil)
	c.Assert(err, jc.ErrorIsNil)
	entry := s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	c.Check(entry.Expiry, gc.Equals, int64(12345+time.Second))
	holder := s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
	currentTime += int64(500 * time.Millisecond)
	req.Holder = "otherholder"
	err = s.leaseStore.ExtendLease(key, req, nil)
	// Denied because we aren't the current holder
	c.Assert(errors.Cause(err), gc.Equals, lease.ErrNotHeld)
	entry = s.expectOneExpiry(c)
	c.Check(entry.Lease, gc.Equals, "app")
	c.Check(entry.Holder, gc.Equals, "mytest")
	c.Check(entry.Expiry, gc.Equals, int64(12345+time.Second))
	holder = s.expectOneHolder(c)
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
}
