// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/tomb"
)

// GetOplog returns the the oplog collection in the local database.
func GetOplog(session *mgo.Session) *mgo.Collection {
	return session.DB("local").C("oplog.rs")
}

// NewOplogTailer returns a new OplogTailer.
//
// Arguments:
// - "oplog" is the collection to use for the oplog. Typically this
//   would be the result of GetOpLog.
// - "query" can be used to limit the returned oplog entries. A
//    typical filter would limit based on ns ("<database>.<collection>")
//    and o (object).
//
// Remember to call Stop on the returned OplogTailer when it is no
// longer needed.
func NewOplogTailer(oplog *mgo.Collection, query bson.D) *OplogTailer {
	// Use a fresh session for the tailer.
	session := oplog.Database.Session.Copy()
	t := &OplogTailer{
		oplog: oplog.With(session),
		query: query,
		outCh: make(chan *OplogDoc, 1),
	}
	go func() {
		defer func() {
			t.tomb.Done()
			session.Close()
		}()
		t.tomb.Kill(t.loop())
	}()
	return t
}

// OplogTailer tails MongoDB's replication oplog.
type OplogTailer struct {
	tomb  tomb.Tomb
	oplog *mgo.Collection
	query bson.D
	outCh chan *OplogDoc
}

// OplogDoc represents a document in the oplog.rs collection.
// See: http://www.kchodorow.com/blog/2010/10/12/replication-internals/
type OplogDoc struct {
	Timestamp    bson.MongoTimestamp `bson:"ts"`
	OperationId  int64               `bson:"h"`
	MongoVersion int                 `bson:"v"`
	Operation    string              `bson:"op"` // "i" - insert, "u" - update, "d" - delete
	Namespace    string              `bson:"ns"`
	Object       bson.D              `bson:"o"`
	UpdateObject bson.D              `bson:"o2"`
}

// Out returns a channel that reports the oplog entries matching the
// query passed to NewOplogTailer as they appear.
func (t *OplogTailer) Out() <-chan *OplogDoc {
	return t.outCh
}

// Dying returns a channel that will be closed with the OplogTailer is
// shutting down.
func (t *OplogTailer) Dying() <-chan struct{} {
	return t.tomb.Dying()
}

// Stop shuts down the OplogTailer. It will block until shutdown is
// complete.
func (t *OplogTailer) Stop() error {
	t.tomb.Kill(nil)
	return t.tomb.Wait()
}

const oplogTailTimeout = time.Second

func (t *OplogTailer) loop() error {
	var iter *mgo.Iter

	// lastTimestamp tracks the most recent oplog timestamp reported.
	var lastTimestamp bson.MongoTimestamp

	// idsForLastTimestamp records the unique operation ids that have
	// been reported for the most recently reported oplog
	// timestamp. This is used to avoid re-reporting oplog entries
	// when the iterator is restarted. It's possible for there to be
	// many oplog entries for a given timestamp.
	var idsForLastTimestamp []int64

	for {
		if t.dying() {
			return tomb.ErrDying
		}

		if iter == nil {
			// When recreating the iterator (required when the cursor
			// is invalidated) avoid reporting oplog entries that have
			// already been reported.
			query := append(t.query,
				bson.DocElem{"ts", bson.D{{"$gte", lastTimestamp}}},
				bson.DocElem{"h", bson.D{{"$nin", idsForLastTimestamp}}},
			)
			// Time the tail call out every second so that requests to
			// stop can be honoured.
			//
			// TODO(mjs): Ideally -1 (no timeout) could be used here,
			// with session.Close() being used to unblock Next() if
			// the tailer should stop (these semantics are hinted at
			// by the mgo docs). Unfortunately this can trigger
			// panics. See: https://github.com/go-mgo/mgo/issues/121
			iter = t.oplog.Find(query).LogReplay().Tail(oplogTailTimeout)
		}

		var doc OplogDoc
		if iter.Next(&doc) {
			t.outCh <- &doc

			if doc.Timestamp > lastTimestamp {
				lastTimestamp = doc.Timestamp
				idsForLastTimestamp = nil
			}
			idsForLastTimestamp = append(idsForLastTimestamp, doc.OperationId)
		} else {
			if err := iter.Err(); err != nil {
				return err
			}
			if iter.Timeout() {
				continue
			}
			// No timeout and no error so cursor must have
			// expired. Force it to be recreated next loop by marking
			// it as nil.
			iter = nil
		}
	}
}

func (t *OplogTailer) dying() bool {
	select {
	case <-t.tomb.Dying():
		return true
	default:
		return false
	}
}
