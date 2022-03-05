// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongolease

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	jujutxn "github.com/juju/txn/v2"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/mongo"
)

// leaseHolderDoc is used to serialise lease holder info.
// This tracks who the current holder of the lease is. This document is updated with a Transaction,
// while the leaseExpiryDocument is not.
type leaseHolderDoc struct {
	Id        string `bson:"_id"`
	Namespace string `bson:"namespace"`
	ModelUUID string `bson:"model-uuid"`
	Lease     string `bson:"lease"`
	Holder    string `bson:"holder"`
}

func (lhd leaseHolderDoc) String() string {
	return fmt.Sprintf("lease [%s] %s %q held by %q",
		lhd.ModelUUID[:6], lhd.Namespace, lhd.Lease, lhd.Holder)
}

// leaseExpiryDoc tracks when the current holder of a lease would expire
type leaseExpiryDoc struct {
	Id        string `bson:"_id"`
	Namespace string `bson:"namespace"`
	ModelUUID string `bson:"model-uuid"`
	Lease     string `bson:"lease"`
	Holder    string `bson:"holder"`
	Expiry    int    `bson:"expiry_timestamp"`
	Pinned    bool   `bson:"pinned"`
}

func (led leaseExpiryDoc) String() string {
	return fmt.Sprintf("lease [%s] %s %q held by %q until %d (pinned: %t)",
		led.ModelUUID[:6], led.Namespace, led.Lease, led.Holder, led.Expiry, led.Pinned)
}

const (
	fieldNamespace = "namespace"
	fieldHolder    = "holder"
	fieldExpiry    = "expiry_timestamp"
)

type RequestOp string

const (
	Claim  RequestOp = "claim"
	Extend RequestOp = "extend"
	Revoke RequestOp = "revoke"
	Pin    RequestOp = "pin"
)

var knownRequestOps = map[RequestOp]string{
	Claim:  string(Claim),
	Extend: string(Extend),
	Revoke: string(Revoke),
	Pin:    string(Pin),
}

func (rop RequestOp) Validate() error {
	if _, ok := knownRequestOps[rop]; !ok {
		return errors.Errorf("unknown lease request operation: %q", string(rop))
	}
	return nil
}

// Mongo exposes MongoDB operations for use by the lease package.
type Mongo interface {

	// RunTransaction should probably delegate to a jujutxn.Runner's Run method.
	RunTransaction(jujutxn.TransactionSource) error

	// GetCollection should probably call the mongo.CollectionFromName func.
	GetCollection(name string) (collection mongo.Collection, closer func())
}

type logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// DBRequest is a list of requests for claiming/extending a set of leases
// TODO: (jam 2022-05-05) Future use. do we want to have a way to issue a single DB query
//  that can claim and extend a bunch of leases? What about at least supporting a batch of leases to extend?
type DBRequest struct {
	Op       RequestOp
	Key      lease.Key
	Holder   string
	Duration time.Duration
}

// mongoLeaseStore conforms to the Lease.Store interface, providing queries against the database to
// track who currently holds leases, and what leases are ready to expire.
// For now, we don't track anything in memory, we just use the database as the source of truth.
type mongoLeaseStore struct {
	expiryCollection  mongo.Collection
	holdersCollection mongo.Collection
	mongo             Mongo
	globalTimeFunc    func() int // A function to get the 'global' time
	logger            logger
}

var _ (lease.Store) = (*mongoLeaseStore)(nil)

func leaseDBId(key lease.Key) string {
	return key.ModelUUID + "#" + key.Namespace + "#" + key.Namespace
}

func (mls *mongoLeaseStore) ClaimLease(key lease.Key, request lease.Request, stop <-chan struct{}) error {
	// When claiming a lease, we can only claim a lease that is not currently held. It is not currently allowed to
	// *claim* a lease that you currently hold (you must request to Extend the lease)
	// TODO: (jam 2022-03-05) should we support claiming a lease that has 'expired'?
	//  for now, the lease must not exist for you to claim it
	var expiry leaseExpiryDoc
	// TODO: (jam 2022-03-05) should we just look this up by leaseId?,
	//  we should probably also check if it does exist that it has the right fields
	leaseId := leaseDBId(key)
	mls.logger.Tracef("claiming lease [%v] %v %q for %q duration %s",
		key.ModelUUID[6:], key.Namespace, key.Lease, request.Holder, request.Duration)
	err := mls.expiryCollection.FindId(leaseId).One(&expiry)
	if err == nil {
		// Lease already held
		// TODO: (jam 2022-03-05) is it better to explicitly format the error than defer to another string function?
		// Probably we shouldn't trace here if we are returning an error, as the caller should log
		mls.logger.Tracef("claiming lease for %q %s failed: held by %v",
			request.Holder, request.Duration, expiry)
		return errors.Annotatef(lease.ErrClaimDenied, "unable to claim: %v", expiry.String())
	} else if errors.Cause(err) != mgo.ErrNotFound {
		// TODO: (jam 2022-03-05) Are there special errors here that we should treat differently?
		mls.logger.Tracef("claiming lease [%v] %v %q for %q duration %s failed with: %v",
			key.ModelUUID[6:], key.Namespace, key.Lease, request.Holder, request.Duration,
			err)
		return errors.Trace(err)
	}
	// NotFound as expected, so create one
	// TODO: (jam 2022-03-05) Ensure that if we are creating an object here, it cannot race with someone else
	//  creating a similar record without them colliding
	newExpiry := leaseExpiryDoc{
		// We have to use fixed ids so that 2 processes trying to create the same lease will collide
		Id:        leaseId,
		ModelUUID: key.ModelUUID,
		Namespace: key.Namespace,
		Lease:     key.Lease,
		Holder:    request.Holder,
		Expiry:    mls.getExpiryTime(request.Duration),
		Pinned:    false,
	}
	err = mls.expiryCollection.Writeable().Insert(newExpiry)
	if err != nil {
		mls.logger.Tracef("claiming %s failed: %v", newExpiry, err)
		return errors.Trace(err)
	}
	// Now that we have the expiry collection entry, we need to update the holder in a txn
	err = mls.mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		// the default assumption is that the holder doesn't exist (because we are making a claim)
		if attempt > 0 {
			var existing leaseHolderDoc
			err := mls.holdersCollection.FindId(leaseId).One(&existing)
			if err != nil {
				if !errors.IsNotFound(err) {
					// TODO (jam 2022-03-05): are there specific errors we should be trapping?
					mls.logger.Tracef("updating holder of %v failed: %v", newExpiry, err)
					return nil, errors.Trace(err)
				} else {
					// Nothing to do because the object doesn't exist, so fall back to the normal creation steps
				}
			} else {
				// Handle the Holder document already existing
				if existing.Holder == request.Holder {
					// No-op
					mls.logger.Tracef("updating holder of %v no-op (already held by %q)", newExpiry, request.Holder)
					return nil, jujutxn.ErrNoOperations
				}
				mls.logger.Tracef("updating holder of %v, new holder %q", existing, request.Holder)
				return []txn.Op{{
					C:      mls.holdersCollection.Name(),
					Id:     leaseId,
					Assert: txn.DocExists,
					Update: bson.M{"$set": bson.M{"holder": request.Holder}},
				}}, nil
			}
		}
		// Assume the case where the document doesn't exist
		return []txn.Op{{
			C:      mls.holdersCollection.Name(),
			Id:     leaseId,
			Assert: txn.DocMissing,
			Insert: leaseHolderDoc{
				Id:        leaseId,
				ModelUUID: key.ModelUUID,
				Namespace: key.Namespace,
				Lease:     key.Lease,
				Holder:    request.Holder,
			},
		}}, nil
	})
	if err != nil {
		mls.logger.Tracef("failed to update holder of %v: %v",
			newExpiry, err)
		return errors.Trace(err)
	}
	return nil
}

func (mls *mongoLeaseStore) getExpiryTime(t time.Duration) int {
	return mls.globalTimeFunc() + int(t)
}

func (mls *mongoLeaseStore) ExtendLease(key lease.Key, request lease.Request, stop <-chan struct{}) error {
	return nil
}
func (mls *mongoLeaseStore) RevokeLease(key lease.Key, holder string, stop <-chan struct{}) error {
	return nil
}

func (mls *mongoLeaseStore) Leases(keys ...lease.Key) map[lease.Key]lease.Info {
	return nil
}
func (mls *mongoLeaseStore) LeaseGroup(namespace, modelUUID string) map[lease.Key]lease.Info {
	return nil
}
func (mls *mongoLeaseStore) PinLease(key lease.Key, entity string, stop <-chan struct{}) error {
	return nil
}
func (mls *mongoLeaseStore) UnpinLease(key lease.Key, entity string, stop <-chan struct{}) error {
	return nil
}
func (mls *mongoLeaseStore) Pinned() map[lease.Key][]string {
	return nil
}
