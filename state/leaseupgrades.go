// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	jujutxn "github.com/juju/txn/v2"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/globalclock"
)

// MigrateLeasesToGlobalTime removes old (<2.3-beta2) lease/clock-skew
// documents, replacing the lease documents with new ones for the
// existing lease holders.
func MigrateLeasesToGlobalTime(pool *StatePool) error {
	return runForAllModelStates(pool, migrateModelLeasesToGlobalTime)
}

const InitialLeaderClaimTime = time.Minute

func migrateModelLeasesToGlobalTime(st *State) error {
	coll, closer := st.db().GetCollection("leases")
	defer closer()

	// Find all old lease/clock-skew documents, remove them
	// and create replacement lease docs in the new format.
	//
	// Replacement leases are created with a duration of a
	// minute, relative to the global time epoch.

	// We can't use st.db().Run here since leases isn't in
	// allCollections anymore. Instead just get a jujutxn.Runner
	// directly.
	db := st.db().(*database)
	runner := db.runner
	if runner == nil {
		runner = jujutxn.NewRunner(jujutxn.RunnerParams{
			Database:               db.raw,
			Clock:                  db.clock,
			ServerSideTransactions: db.serverSideTransactions,
		})
	}
	err := runner.Run(func(int) ([]txn.Op, error) {
		var doc struct {
			DocID     string `bson:"_id"`
			Type      string `bson:"type"`
			Namespace string `bson:"namespace"`
			Name      string `bson:"name"`
			Holder    string `bson:"holder"`
			Expiry    int64  `bson:"expiry"`
			Writer    string `bson:"writer"`
		}

		var ops []txn.Op
		iter := coll.Find(bson.D{{"type", bson.D{{"$exists", true}}}}).Iter()
		defer iter.Close()
		for iter.Next(&doc) {
			ops = append(ops, txn.Op{
				C:      coll.Name(),
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Remove: true,
			})
			if doc.Type != "lease" {
				upgradesLogger.Tracef("deleting old lease doc %q", doc.DocID)
				continue
			}
			// Check if the target exists
			if _, err := lookupLease(coll, doc.Namespace, doc.Name); err == nil {
				// target already exists, it takes precedence over an old doc, which we still want to delete
				upgradesLogger.Infof("new lease %q %q already exists, simply deleting old lease %q",
					doc.Namespace, doc.Name, doc.DocID)
				continue
			} else if err != mgo.ErrNotFound {
				// We got an unknown error looking up this doc, don't suppress it
				return nil, err
			}
			upgradesLogger.Tracef("migrating lease %q to new lease structure", doc.DocID)
			claimOps, err := claimLeaseOps(
				doc.Namespace,
				doc.Name,
				doc.Holder,
				doc.Writer,
				coll.Name(),
				globalclock.GlobalEpoch(),
				InitialLeaderClaimTime,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			// Add model UUID into the ops that came back...
			for i, op := range claimOps {
				op.Id = st.docID(op.Id.(string))
				munged, err := mungeDocForMultiModel(op.Insert, st.ModelUUID(), modelUUIDRequired)
				if err != nil {
					return nil, errors.Annotate(err, "adding modelUUID")
				}
				op.Insert = munged
				claimOps[i] = op
			}

			ops = append(ops, claimOps...)
		}
		if err := iter.Close(); err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	})
	return errors.Annotate(err, "upgrading legacy lease documents")
}

// LegacyLeases returns information about all of the leases in the
// state-based lease store.
func LegacyLeases(pool *StatePool, localTime time.Time) (map[corelease.Key]corelease.Info, error) {
	st := pool.SystemState()
	reader, err := globalclock.NewReader(globalclock.ReaderConfig{
		Config: globalclock.Config{
			Collection: globalClockC,
			Mongo:      &environMongo{state: st},
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	globalTime, err := reader.Now()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// This needs to be the raw collection so we see all leases across
	// models.
	leaseCollection, closer := st.db().GetRawCollection("leases")
	defer closer()
	iter := leaseCollection.Find(nil).Iter()
	results := make(map[corelease.Key]corelease.Info)

	var doc struct {
		Namespace string        `bson:"namespace"`
		ModelUUID string        `bson:"model-uuid"`
		Name      string        `bson:"name"`
		Holder    string        `bson:"holder"`
		Start     int64         `bson:"start"`
		Duration  time.Duration `bson:"duration"`
	}

	for iter.Next(&doc) {
		startTime := time.Unix(0, doc.Start)
		globalExpiry := startTime.Add(doc.Duration)
		remaining := globalExpiry.Sub(globalTime)
		localExpiry := localTime.Add(remaining)
		key := corelease.Key{
			Namespace: doc.Namespace,
			ModelUUID: doc.ModelUUID,
			Lease:     doc.Name,
		}
		results[key] = corelease.Info{
			Holder:   doc.Holder,
			Expiry:   localExpiry,
			Trapdoor: nil,
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// DropLeasesCollection removes the leases collection. Tolerates the
// collection already not existing.
func DropLeasesCollection(pool *StatePool) error {
	st := pool.SystemState()
	names, err := st.MongoSession().DB("juju").CollectionNames()
	if err != nil {
		return errors.Trace(err)
	}
	if !set.NewStrings(names...).Contains("leases") {
		return nil
	}
	coll, closer := st.db().GetRawCollection("leases")
	defer closer()
	return errors.Trace(coll.DropCollection())
}

func claimLeaseOps(
	namespace, name, holder, writer, collection string,
	globalTime time.Time, duration time.Duration,
) ([]txn.Op, error) {
	leaseDoc := &leaseDoc{
		Id:        leaseDocId(namespace, name),
		Namespace: namespace,
		Name:      name,
		Holder:    holder,
		Start:     globalTime.UnixNano(),
		Duration:  duration,
		Writer:    writer,
	}
	claimLeaseOp := txn.Op{
		C:      collection,
		Id:     leaseDoc.Id,
		Assert: txn.DocMissing,
		Insert: leaseDoc,
	}
	return []txn.Op{claimLeaseOp}, nil
}

func lookupLease(coll mongo.Collection, namespace, name string) (leaseDoc, error) {
	var doc leaseDoc
	err := coll.FindId(leaseDocId(namespace, name)).One(&doc)
	if err != nil {
		return leaseDoc{}, err
	}
	return doc, nil
}

func leaseDocId(namespace, lease string) string {
	return fmt.Sprintf("%s#%s#", namespace, lease)
}

type leaseDoc struct {
	Id        string        `bson:"_id"`
	Namespace string        `bson:"namespace"`
	Name      string        `bson:"name"`
	Holder    string        `bson:"holder"`
	Start     int64         `bson:"start"`
	Duration  time.Duration `bson:"duration"`
	Writer    string        `bson:"writer"`
}
