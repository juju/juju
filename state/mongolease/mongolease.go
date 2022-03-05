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
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
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
	// TODO (jam 2022-03-05): this should be a getExpiryTime(time.Duration) rather than a global time offset.
	globalTimeFunc func() int64 // A function to get the 'global' time
	logger         logger
}

var _ (lease.Store) = (*mongoLeaseStore)(nil)

func leaseDBId(key lease.Key) string {
	return key.ModelUUID + "#" + key.Namespace + "#" + key.Lease
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
	expiryTime := mls.getExpiryTime(request.Duration)
	maybeExpiry := leaseExpiryDoc{
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		Holder:    request.Holder,
		Expiry:    expiryTime,
		Pinned:    false, // unknown
	}
	mls.logger.Debugf("claiming: %q %v", leaseId, maybeExpiry)
	err := mls.expiryCollection.FindId(leaseId).One(&expiry)
	if err == nil {
		// Lease already held
		// TODO: (jam 2022-03-05) is it better to explicitly format the error than defer to another string function?
		// Probably we shouldn't trace here if we are returning an error, as the caller should log
		mls.logger.Debugf("claiming lease for %v %q %s failed: held by %v", key, request.Holder, request.Duration, expiry)
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
		Expiry:    expiryTime,
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

func iterToMap(iter mongo.Iterator) map[lease.Key]lease.Info {
	leases := make(map[lease.Key]lease.Info)
	var expiry leaseExpiryDoc
	for iter.Next(&expiry) {
		key := lease.Key{
			Namespace: expiry.Namespace,
			ModelUUID: expiry.ModelUUID,
			Lease:     expiry.Lease,
		}
		info := lease.Info{
			Holder: expiry.Holder,
			// TODO: (jam 2022-03-05) This is probably an incorrect mapping to time
			Expiry: time.Unix(0, expiry.Expiry),
			// Not sure what to put here
			Trapdoor: nil,
		}
		leases[key] = info
	}
	return leases
}

func (mls *mongoLeaseStore) Leases(keys ...lease.Key) map[lease.Key]lease.Info {
	var iter mongo.Iterator
	if len(keys) == 0 {
		// return all
		query := mls.expiryCollection.Find(nil)
		iter = query.Iter()
	} else {
		// Do we query all and just return some? or do we assume that the list is short and just query those?
		ids := make([]string, len(keys))
		for i, key := range keys {
			ids[i] = leaseDBId(key)
		}
		query := mls.expiryCollection.Find(bson.M{"_id": bson.M{"$in": ids}})
		iter = query.Iter()
	}
	leases := iterToMap(iter)
	err := iter.Close()
	if err != nil {
		// Leases interface doesn't allow returning errors :(
		mls.logger.Errorf("failed while reading leases (error suppressed): %v", err)
	}
	return leases
}

func (mls *mongoLeaseStore) LeaseGroup(namespace, modelUUID string) map[lease.Key]lease.Info {
	query := mls.expiryCollection.Find(
		bson.M{"namespace": namespace, "model-uuid": modelUUID},
	)
	iter := query.Iter()
	leases := iterToMap(iter)
	err := iter.Close()
	if err != nil {
		// TODO: (jam 2022-03-05) LeaseGroup interface doesn't allow returning errors :(
		mls.logger.Errorf("failed while reading leases (error suppressed): %v", err)
	}
	return leases
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

// expireOne removes a single entry from the expiriesCollection
// it will update doc with the actual value of what was removed. It will also return 'bool' false if
// something happened that we did not delete the document. This could happen for a variety
// of reasons. (someone updated the doc while we were thinking about deleting it)
// Generally it is ok to not expire something. If it really does need expiration the next
// run will find it, too.
func (mls *mongoLeaseStore) expireOne(doc *leaseExpiryDoc) (bool, error) {

	// Check one more time that the value really is what we think it is
	qq := mls.expiryCollection.Find(bson.M{
		"_id": doc.Id,
		// The _id handles model-uuid, namespace, lease name.
		// We just need to make sure the holder and Expiry aren't changing while we remove it
		"holder":           doc.Holder,
		"expiry_timestamp": doc.Expiry,
	})
	mls.logger.Debugf("expiring: %v", *doc)
	changeInfo, err := qq.Apply(mgo.Change{Remove: true}, doc)
	if err == nil {
		if changeInfo.Removed != 1 {
			// TODO: (jam 2022-03-05) Confirm if we get NotFound or we get Removed = 0
			mls.logger.Debugf("expired lease not removed: %v", doc)
			return false, nil
		}
		mls.logger.Tracef("removed from expiries: %v", *doc)
		return true, nil
	}
	if errors.Cause(err) == mgo.ErrNotFound {
		// The expectation is either someone else expired this, in which case there is nothing to do, or the
		// least was extended, in which case there is again, nothing to do.
		mls.logger.Debugf("expired lease not found: %v", *doc)
		return false, nil
	}
	// TODO: (jam 2022-03-05) Probably needs some testing to figure out what other
	//  relevant errors we might see.
	mls.logger.Debugf("error while expiring: %v", *doc)
	return false, errors.Annotatef(err, "while removing %v", *doc)
}

func (mls *mongoLeaseStore) removeHolders(removed map[lease.Key]lease.Info) error {
	// Note that we generally will suppress txn errors at this point, because expiries is always
	// the source of truth, we just need txns on holders to make everything else play nice.
	// However, we should at least report if they get out of sync
	failed := false
	err := mls.mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// If we have to retry, we switch back to a one-by-one pattern
			failed = true
			return nil, jujutxn.ErrNoOperations
		}
		ops := make([]txn.Op, 0, len(removed))
		for key, info := range removed {
			ops = append(ops, txn.Op{
				C:      mls.holdersCollection.Name(),
				Id:     leaseDBId(key),
				Remove: true,
				Assert: bson.M{
					"holder": info.Holder,
				},
			})
		}
		return ops, nil
	})
	// TODO (jam 2022-03-05): Does ErrNoOperations bubble up or does it just become err nil?
	if err != nil {
		// it may be that we should just treat this as a logged error and not do anything else
		return errors.Trace(err)
	}
	if !failed {
		return nil
	}
	// Something went wrong with our all-in-one attempt, do them one by one and report any issues
	for key, info := range removed {
		err := mls.mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
			var holder leaseHolderDoc
			err := mls.holdersCollection.FindId(leaseDBId(key)).One(&holder)
			if err != nil {
				if errors.Cause(err) == mgo.ErrNotFound {
					mls.logger.Debugf("trying to remove %v we got 'not found', ignoring", key)
					return nil, jujutxn.ErrNoOperations
				} else {
					// Not sure what to do, this could be db disconnected, EOF, something else
					// For example, we might notice EOF, and trigger a session.Refresh
					mls.logger.Debugf("error while removing lease holder %v: %v", key, err)
					return nil, errors.Trace(err)
				}
			}
			if holder.Holder != info.Holder {
				mls.logger.Warningf("we tried to expire %v with holder %q but holder is now %q, removing anyway",
					key, info.Holder, holder.Holder)
			}

			// TODO(jam 2022-03-05): do we want to report the holder that we removed from Expiry, or the holder
			//   that we removed from holders? They shouldn't be different, but they were...
			// removed[key] = lease.Info{
			// 	Holder: holder.Holder,
			// 	Expiry: info.Expiry,
			// }
			return []txn.Op{{
				C:      mls.holdersCollection.Name(),
				Id:     leaseDBId(key),
				Remove: true,
				Assert: bson.M{
					"holder": holder.Holder,
				},
			}}, nil
		})
		if err != nil {
			mls.logger.Warningf("failed to cleanup lease holders collection for %v: %v",
				key, err)
			// Note, we don't bubble up this error, because we want to make sure to
			// delete all of the other holder records that we can.
		}
	}
	return nil
}

// ExpireLeases removes leases that have expired before the given timestamp
// The records will be removed from the database (along with the holder records),
// and the list of what has been expired will be returned.
// TODO (jam 2022-03-05) What is the right data type to return? For now I'm just returning
//  map[Key]Info because we use that in a bunch of other interactions
func (mls *mongoLeaseStore) ExpireLeases(before int64) (map[lease.Key]lease.Info, error) {
	// New versions of Mongo have 'findOneAndDelete' which might be an interesting way to implement this.
	// https://docs.mongodb.com/realm/mongodb/actions/collection.findOneAndDelete/#collection.findoneanddelete--
	// However, we'd like to do it in larger batches.
	// That said, mgo does support 'findAndModify' which is essentialy the more generic operation.
	// It lets you remove an entry and return the original value, based on a query.
	// We also have to watch out for items getting extended while we are expiring them.
	// First, we start by finding all of the entries that may want to be expired
	query := mls.expiryCollection.Find(bson.M{
		"expiry_timestamp": bson.M{"$lt": before},
		// TODO: (jam 2022-03-05) We shouldn't expire entries that are pinned
		// "pinned":           false,
	})
	iter := query.Iter()
	removed := make(map[lease.Key]lease.Info)
	var expiry leaseExpiryDoc
	for iter.Next(&expiry) {
		// confirm that the expiry time is still old
		if expiry.Expiry > before {
			// This entry was modified since we started the query, ignore it
			mls.logger.Debugf("lease %v moved past threshold %d", expiry, before)
			continue
		}
		isRemoved, err := mls.expireOne(&expiry)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if isRemoved {
			// if we get to this point, then we've just removed this entry from the db, record it
			key := lease.Key{Namespace: expiry.Namespace, ModelUUID: expiry.ModelUUID, Lease: expiry.Lease}
			info := lease.Info{Holder: expiry.Holder, Expiry: time.Unix(0, expiry.Expiry)}
			removed[key] = info
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotatef(err, "while expiring leases")
	}
	// Now we have a bunch of leases that we really did remove, lets create the TXNs to remove
	// it from the other collection
	err := mls.removeHolders(removed)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return removed, nil
}
