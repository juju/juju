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
	Expiry    int64  `bson:"expiry_timestamp"`
	Pinned    bool   `bson:"pinned"`
}

func (led leaseExpiryDoc) String() string {
	shortUUID := led.ModelUUID
	if len(shortUUID) > 6 {
		shortUUID = shortUUID[:6]
	}
	return fmt.Sprintf("lease [%s] %s %q held by %q until %d (pinned: %t)",
		shortUUID, led.Namespace, led.Lease, led.Holder, led.Expiry, led.Pinned)
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
	globalTimeFunc    func() int64 // A function to get the 'global' time
	logger            logger
}

var _ (lease.Store) = (*mongoLeaseStore)(nil)

func leaseDBId(key lease.Key) string {
	return key.ModelUUID + "#" + key.Namespace + "#" + key.Namespace
}

func (mls *mongoLeaseStore) ClaimLease(key lease.Key, request lease.Request, stop <-chan struct{}) error {
	// TODO: (jam 2022-03-05) We should probably be validating key and request
	// When claiming a lease, we can only claim a lease that is not currently held. It is not currently allowed to
	// *claim* a lease that you currently hold (you must request to Extend the lease)
	// TODO: (jam 2022-03-05) should we support claiming a lease that has 'expired'?
	//  for now, the lease must not exist for you to claim it. However, we could change it to allow claiming
	//  any lease that has already expired and is not pinned
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
		mls.logger.Tracef("claiming lease for %q %s failed: held by %v", request.Holder, request.Duration, expiry)
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
		if mgo.IsDup(err) {
			// ... Error: E11000 duplicate key error collection: .*
			return errors.Annotatef(lease.ErrClaimDenied, "unable to claim (duplicate): %v", expiry.String())
		}
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
	mls.logger.Debugf("claimed: %v", newExpiry)
	return nil
}

func (mls *mongoLeaseStore) getExpiryTime(t time.Duration) int64 {
	return mls.globalTimeFunc() + int64(t)
}

func (mls *mongoLeaseStore) ExtendLease(key lease.Key, request lease.Request, stop <-chan struct{}) error {
	// TODO: (jam 2022-03-05) We should probably be validating key and request
	// We default to assuming the extend is correct
	leaseId := leaseDBId(key)
	newExpiry := mls.getExpiryTime(request.Duration)
	// TODO: (jam 2022-03-05) One check we could do, is to include
	//  current expiry time is < requested expiry time.
	//  It would be an error to 'extend' a lease into the past
	// TODO: (jam 2022-03-05) Retries?
	// TODO: (jam 2022-03-05) We should respect stop channel
	err := mls.expiryCollection.Writeable().Update(
		bson.M{"_id": leaseId, "holder": request.Holder}, // "expiry": bson.M{"$lt": newExpiry}
		bson.M{"$set": bson.M{"expiry_timestamp": newExpiry}},
	)
	if err == nil {
		// We successfully extended the expiration, job done
		doc := leaseExpiryDoc{
			Namespace: key.Namespace,
			ModelUUID: key.ModelUUID,
			Lease:     key.Lease,
			Holder:    request.Holder,
			Expiry:    newExpiry,
			Pinned:    false, // technically unknown, is that a problem?
		}
		// We didn't read back from the database, but this is what we expect to be true after doing the update
		mls.logger.Debugf("extended lease: %v", doc)
		return nil
	}
	if errors.Cause(err) == mgo.ErrNotFound {
		// Check to see if the lease is actually held by someone else
		var existing leaseExpiryDoc
		err2 := mls.expiryCollection.FindId(leaseId).One(&existing)
		if errors.Cause(err2) == mgo.ErrNotFound {
			// The least wasn't held by anyone
			// TODO: (jam 2022-03-05) Validate the content here, we should be giving the short form of model-uuid, etc.
			mls.logger.Tracef("extending lease failed to find lease: %v, not held", key)
			return errors.Annotatef(lease.ErrNotHeld, "lease %v not held by anything", key)
		}
		// TODO: (jam 2022-03-05) Validate the content here, we should be giving the short form of model-uuid, etc.
		// TODO: (jam 2022-03-05) If we do trap based on expiry time, we should check if existing.holder == request.holder
		mls.logger.Tracef("extending lease failed to find lease: %v, held by %v", key, existing.Holder)
		return errors.Annotatef(lease.ErrNotHeld, "lease held by: %v", existing)
	}
	// I don't know what other failures we might hit
	// TODO: (jam 2022-03-05) This should probably not be leaseId, but be 'short lease identifier' with the short model-uuid
	mls.logger.Tracef("db error extending lease %v: %v", key, err)
	return errors.Annotatef(err, "error updating lease expiry document %q", leaseId)
}

// TODO: (jam 2022-03-05) we should probably build an API that would let you extend multiple at once
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
