// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"gopkg.in/tomb.v2"
)

// OplogDoc represents a document in the oplog.rs collection.
// See: http://www.kchodorow.com/blog/2010/10/12/replication-internals/
//
// The Object and UpdateObject fields are returned raw to allow
// unmarshalling into arbitrary types. Use the UnmarshalObject and
// UnmarshalUpdate methods to unmarshall these fields.
type OplogDoc struct {
	Timestamp    bson.MongoTimestamp `bson:"ts"`
	OperationId  int64               `bson:"h"`
	MongoVersion int                 `bson:"v"`
	Operation    string              `bson:"op"` // "i" - insert, "u" - update, "d" - delete
	Namespace    string              `bson:"ns"`
	Object       *bson.Raw           `bson:"o"`
	UpdateObject *bson.Raw           `bson:"o2"`
}

// UnmarshalObject unmarshals the Object field into out. The out
// argument should be a pointer or a suitable map.
func (d *OplogDoc) UnmarshalObject(out interface{}) error {
	return d.unmarshal(d.Object, out)
}

// UnmarshalUpdate unmarshals the UpdateObject field into out. The out
// argument should be a pointer or a suitable map.
func (d *OplogDoc) UnmarshalUpdate(out interface{}) error {
	return d.unmarshal(d.UpdateObject, out)
}

func (d *OplogDoc) unmarshal(raw *bson.Raw, out interface{}) error {
	if raw == nil {
		// If the field is not set, set out to the zero value for its type.
		v := reflect.ValueOf(out)
		switch v.Kind() {
		case reflect.Ptr:
			v = v.Elem()
			v.Set(reflect.Zero(v.Type()))
		case reflect.Map:
			// Empty the map.
			for _, k := range v.MapKeys() {
				v.SetMapIndex(k, reflect.Value{})
			}
		default:
			return errors.New("output must be a pointer or map")
		}
		return nil
	}
	return raw.Unmarshal(out)
}

// NewMongoTimestamp returns a bson.MongoTimestamp repesentation for
// the time.Time given. Note that these timestamps are not the same
// the usual MongoDB time fields. These are an internal format used
// only in a few places such as the replication oplog.
//
// See: http://docs.mongodb.org/manual/reference/bson-types/#timestamps
func NewMongoTimestamp(t time.Time) bson.MongoTimestamp {
	unixTime := t.Unix()
	if unixTime < 0 {
		unixTime = 0
	}
	return bson.MongoTimestamp(unixTime << 32)
}

// GetOplog returns the the oplog collection in the local database.
func GetOplog(session *mgo.Session) *mgo.Collection {
	return session.DB("local").C("oplog.rs")
}

func isRealOplog(c *mgo.Collection) bool {
	return c.Database.Name == "local" && c.Name == "oplog.rs"
}

// OplogSession represents a connection to the oplog store, used
// to create an iterator to get oplog documents (and recreate it if it
// gets killed or times out).
type OplogSession interface {
	NewIter(bson.MongoTimestamp, []int64) Iterator
	Close()
}

type oplogSession struct {
	session    *mgo.Session
	collection *mgo.Collection
	query      bson.D
}

// NewOplogSession defines a new OplogSession.
//
// Arguments:
//   - "collection" is the collection to use for the oplog. Typically this
//     would be the result of GetOpLog.
//   - "query" can be used to limit the returned oplog entries. A
//     typical filter would limit based on ns ("<database>.<collection>")
//     and o (object).
//
// The returned session should be `Close`d when it's no longer needed.
func NewOplogSession(collection *mgo.Collection, query bson.D) *oplogSession {
	// Use a fresh session for the tailer.
	session := collection.Database.Session.Copy()
	return &oplogSession{
		session:    session,
		collection: collection.With(session),
		query:      query,
	}
}

const oplogTailTimeout = time.Second

func (s *oplogSession) NewIter(fromTimestamp bson.MongoTimestamp, excludeIds []int64) Iterator {
	// When recreating the iterator (required when the cursor
	// is invalidated) avoid reporting oplog entries that have
	// already been reported.
	sel := append(s.query,
		bson.DocElem{"ts", bson.D{{"$gte", fromTimestamp}}},
		bson.DocElem{"h", bson.D{{"$nin", excludeIds}}},
	)

	query := s.collection.Find(sel)
	if isRealOplog(s.collection) {
		// Apply an optimisation that is only supported with
		// the real oplog.
		query = query.LogReplay()
	}

	// Time the tail call out every second so that requests to
	// stop can be honoured.
	return query.Tail(oplogTailTimeout)
}

func (s *oplogSession) Close() {
	s.session.Close()
}

// NewOplogTailer returns a new OplogTailer.
//
// Arguments:
//   - "session" determines the collection and filtering on records that
//     should be returned.
//   - "initialTs" sets the operation timestamp to start returning
//     results from. This can be used to avoid an expensive initial search
//     through the oplog when the tailer first starts.
//
// Remember to call Stop on the returned OplogTailer when it is no
// longer needed.
func NewOplogTailer(
	session OplogSession,
	initialTs time.Time,
) *OplogTailer {
	t := &OplogTailer{
		session:   session,
		initialTs: NewMongoTimestamp(initialTs),
		outCh:     make(chan *OplogDoc),
	}
	t.tomb.Go(func() error {
		defer func() {
			close(t.outCh)
			session.Close()
		}()
		return t.loop()
	})
	return t
}

// OplogTailer tails MongoDB's replication oplog.
type OplogTailer struct {
	tomb      tomb.Tomb
	session   OplogSession
	initialTs bson.MongoTimestamp
	outCh     chan *OplogDoc
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

// Err returns the error that caused the OplogTailer to stop. If it
// finished normally or hasn't stopped then nil will be returned.
func (t *OplogTailer) Err() error {
	return t.tomb.Err()
}

func (t *OplogTailer) loop() error {
	// lastTimestamp tracks the most recent oplog timestamp reported.
	lastTimestamp := t.initialTs

	// idsForLastTimestamp records the unique operation ids that have
	// been reported for the most recently reported oplog
	// timestamp. This is used to avoid re-reporting oplog entries
	// when the iterator is restarted. These timestamps are unique for
	// a given mongod but when there's multiple replicaset members
	// it's possible for there to be multiple oplog entries for a
	// given timestamp.
	//
	// See: http://docs.mongodb.org/v2.4/reference/bson-types/#timestamps
	var idsForLastTimestamp []int64

	newIter := func() Iterator {
		return t.session.NewIter(lastTimestamp, idsForLastTimestamp)
	}
	iter := newIter()
	defer func() { iter.Close() }() // iter may be replaced, hence closure

	for {
		if t.dying() {
			return tomb.ErrDying
		}
		var doc OplogDoc
		if iter.Next(&doc) {
			select {
			case <-t.tomb.Dying():
				return tomb.ErrDying
			case t.outCh <- &doc:
			}
			if doc.Timestamp > lastTimestamp {
				lastTimestamp = doc.Timestamp
				idsForLastTimestamp = nil
			}
			idsForLastTimestamp = append(idsForLastTimestamp, doc.OperationId)
		} else {
			if iter.Timeout() {
				continue
			}
			if err := iter.Close(); err != nil && err != mgo.ErrCursor {
				return err
			}
			// Either there's no error or the error is an expired
			// cursor; Recreate the iterator.
			iter = newIter()
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
