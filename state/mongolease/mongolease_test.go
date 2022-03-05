// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongolease

import (
	"github.com/juju/loggo"
	"time"

	"github.com/juju/errors"
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

func (s *mongoLeaseSuite) SetupLeaseStore(c *gc.C, timeFunc func() int) {
	expiryCollection, close1 := mongo.CollectionFromName(s.db, expiriesC)
	s.AddCleanup(func(*gc.C) { close1() })
	holdersCollection, close2 := mongo.CollectionFromName(s.db, leasesC)
	s.AddCleanup(func(*gc.C) { close2() })
	s.leaseStore = &mongoLeaseStore{
		expiryCollection:  expiryCollection,
		holdersCollection: holdersCollection,
		mongo:             NewTestMongo(s.db),
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
	s.SetupLeaseStore(c, func() int { return 12345 })
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
	c.Check(entry.Expiry, gc.Equals, 12345+int(time.Second))
	holder := s.expectOneHolder(c)
	c.Check(holder.ModelUUID, gc.Equals, "deadbeef")
	c.Check(holder.Namespace, gc.Equals, "application")
	c.Check(holder.Lease, gc.Equals, "app")
	c.Check(holder.Holder, gc.Equals, "mytest")
}

func (s *mongoLeaseSuite) TestClaimClaimedLease(c *gc.C) {
	s.SetupLeaseStore(c, func() int { return 12345 })
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
