// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package txn

import (
	"fmt"
	"sort"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// OracleIterator is used to walk over the remaining transactions.
// See the mgo.Iter as a similar iteration mechanism. Standard use is to do:
// iter := oracle.IterTxns()
// return EOF when we get to the end of the iterator, or some other error if
// there is another failure.
// for txnId := iter.Next(); err != nil; txnId := iter.Next()  {
// }
// if err != txn.EOF {
// }
type OracleIterator interface {
	// Grab the next transaction id. Will return nil if there are no
	// more transactions.
	Next() (bson.ObjectId, error)
}

// Oracle is the general interface that is used to track what transactions
// are considered completed, and can be pruned.
type Oracle interface {
	// Count returns the number of transactions that we are working with
	Count() int

	// CompletedTokens is called with a list of tokens to be checked. The
	// returned map will have a 'true' for any token that references a
	// completed transaction.
	CompletedTokens(tokens []string) (map[string]bool, error)

	// RemoveTxns can be used to flag that a given transaction should not
	// be considered part of the valid set.
	RemoveTxns(txnIds []bson.ObjectId) (int, error)

	// IterTxns lets you iterate over all of the transactions that have
	// not been removed.
	IterTxns() (OracleIterator, error)
}

// checkMongoSupportsOut verifies that Mongo supports "$out" in an aggregation
// pipeline. This was introduced in Mongo 2.6
// https://docs.mongodb.com/manual/reference/operator/aggregation/out/
func checkMongoSupportsOut(db *mgo.Database) bool {
	var dbInfo struct {
		VersionArray []int `bson:"versionArray"`
	}
	if err := db.Run(bson.M{"buildInfo": 1}, &dbInfo); err != nil {
		return false
	}
	logger.Debugf("buildInfo reported: %v", dbInfo.VersionArray)
	if len(dbInfo.VersionArray) < 2 {
		return false
	}
	// Check if we are at least 2.6
	v := dbInfo.VersionArray
	return v[0] > 2 || (v[0] == 2 && v[1] >= 6)
}

// completedOldTransactionMatch creates a search parameter for transactions
// that are flagged as completed, and were generated older than the given
// timestamp. If the timestamp is empty,then only the completed status is evaluated.
// The returned object is suitable for being passed to a $match or a Find() operation.
func completedOldTransactionMatch(timestamp time.Time) bson.M {
	match := bson.M{"s": bson.M{"$gte": taborted}}
	if !timestamp.IsZero() {
		match["_id"] = bson.M{"$lt": bson.NewObjectIdWithTime(timestamp)}
	}
	return match
}

// NewDBOracle uses a database collection to manage the queue of remaining
// transactions.
// The caller is responsible to call the returned cleanup() function, to ensure
// that any resources are freed.
// thresholdTime is used to omit transactions that are newer than this time
// (eg, don't consider transactions that are less than 1 hr old to be considered completed yet.)
func NewDBOracle(txns *mgo.Collection, thresholdTime time.Time) (*DBOracle, func(), error) {
	oracle := &DBOracle{
		db:            txns.Database,
		txns:          txns,
		thresholdTime: thresholdTime,
		usingMongoOut: checkMongoSupportsOut(txns.Database),
	}
	cleanup, err := oracle.prepare()
	return oracle, cleanup, err
}

var _ Oracle = (*DBOracle)(nil)

func noopCleanup() {}

// DBOracle uses a temporary table on disk to track what transactions are
// considered completed and purgeable.
type DBOracle struct {
	db              *mgo.Database
	txns            *mgo.Collection
	working         *mgo.Collection
	thresholdTime   time.Time
	usingMongoOut   bool
	checkedTokens   uint64
	completedTokens uint64
	foundTxns       uint64
}

// prepareWorkingDirectly iterates the working set from the pipeline and
// populates the working set by inserting them from the client. This is less
// efficient that a $out in the pipeline, but must be used when Mongo doesn't
// support pipelines.
func (o *DBOracle) prepareWorkingDirectly() error {
	logger.Debugf("iterating the transactions collection to build the working set: %q", o.working.Name)
	// Make sure the working set is clean
	o.working.DropCollection()
	query := o.txns.Find(completedOldTransactionMatch(o.thresholdTime))
	query.Select(bson.M{"_id": 1})
	query.Batch(maxBatchDocs)
	iter := query.Iter()
	var txnDoc struct {
		Id bson.ObjectId `bson:"_id"`
	}
	t := newSimpleTimer(logInterval)
	docCount := 0
	// Batching the insert into 1000 at a time made a dramatic improvement in
	// time. Doing one-by-one insert after 1hr wall-clock time it had only
	// copied 11M transaction ids.
	// With a 1000 item batch, it took 20min to do copy 36M documents (approx
	// 10x speedup)
	// For reference, it is about 9min to use $out, and 13min to read the data
	// into memory.
	docsToInsert := make([]interface{}, 0, maxBulkOps)
	flush := func() error {
		if len(docsToInsert) == 0 {
			return nil
		}
		err := o.working.Insert(docsToInsert...)
		docCount += len(docsToInsert)
		docsToInsert = docsToInsert[:0]
		return err
	}
	for iter.Next(&txnDoc) {
		aCopy := txnDoc
		docsToInsert = append(docsToInsert, aCopy)
		if len(docsToInsert) >= maxBulkOps {
			if err := flush(); err != nil {
				return err
			}
		}
		if t.isAfter() {
			logger.Debugf("copied %d documents", docCount)
		}
	}
	if err := flush(); err != nil {
		return err
	}
	return iter.Close()
}

// prepareWorkingWithPipeline adds a $out stage to the pipeline, and has mongo
// populate the working set. This is the preferred method if Mongo supports $out.
func (o *DBOracle) prepareWorkingWithPipeline() error {
	logger.Debugf("searching for transactions older than %s", o.thresholdTime)
	pipeline := []bson.M{
		// This used to use $in but that's much slower than $gte.
		{"$match": completedOldTransactionMatch(o.thresholdTime)},
		{"$project": bson.M{"_id": 1}},
		{"$out": o.working.Name},
	}
	pipe := o.txns.Pipe(pipeline)
	pipe.Batch(maxBatchDocs)
	pipe.AllowDiskUse()
	return pipe.All(&bson.D{})
}

func (o *DBOracle) prepare() (func(), error) {
	if o.working != nil {
		return noopCleanup, fmt.Errorf("Prepare called twice")
	}
	workingSetName := o.txns.Name + ".prunetemp"
	o.working = o.db.C(workingSetName)

	// Load the ids of all completed and aborted txns into a separate
	// temporary collection.
	logger.Debugf("loading all completed transactions")
	var err error
	if o.usingMongoOut {
		err = o.prepareWorkingWithPipeline()
	} else {
		err = o.prepareWorkingDirectly()
	}
	if err != nil {
		o.cleanup()
		return noopCleanup, fmt.Errorf("reading completed txns: %v", err)
	}
	return o.cleanup, nil
}

func (o *DBOracle) Count() int {
	count, err := o.working.Count()
	if err != nil {
		return -1
	}
	return count
}

func (o *DBOracle) cleanup() {
	if o.working != nil {
		name := o.working.Name
		err := o.working.DropCollection()
		o.working = nil
		if err != nil {
			logger.Warningf("cleanup of %q failed: %v", name, err)
		}
	}
}

// CompletedTokens looks at the list of tokens and finds what referenced txns
// are completed, and then returns the set of tokens that are completed.
func (o *DBOracle) CompletedTokens(tokens []string) (map[string]bool, error) {
	objectIds := make([]bson.ObjectId, 0, len(tokens))

	// The nonce is generated during preparing, and if 2 flushers race,
	// only one nonce makes it into the final transaction. However, other
	// nonces can also be considered 'completed'. (afaict, they are ignored,
	// thus won't be applied and can be considered completed.)
	for _, token := range tokens {
		objId := txnTokenToId(token)
		objectIds = append(objectIds, objId)
	}
	query := o.working.Find(bson.M{"_id": bson.M{"$in": objectIds}})
	query = query.Select(bson.M{"_id": 1})
	iter := query.Iter()
	var txnDoc struct {
		Id bson.ObjectId `bson:"_id"`
	}
	foundIdHex := make(map[string]bool, len(objectIds))
	for iter.Next(&txnDoc) {
		foundIdHex[txnDoc.Id.Hex()] = true
	}
	if err := iter.Close(); err != nil {
		if err != mgo.ErrNotFound {
			// Not found is ok, the transactions may not be complete
			return nil, err
		}
	}
	result := make(map[string]bool, len(foundIdHex))
	// because multiple tokens could map to a single txn, we iterate the
	// passed in tokens instead of caching them in the map.
	for _, token := range tokens {
		objIdHex := txnTokenToId(token).Hex()
		if foundIdHex[objIdHex] {
			result[token] = true
		}
	}
	o.checkedTokens += uint64(len(tokens))
	o.completedTokens += uint64(len(result))
	o.foundTxns += uint64(len(foundIdHex))
	return result, nil
}

// RemoveTxns can be used to flag that a given transaction should not
// be considered part of the valid set.
func (o *DBOracle) RemoveTxns(txnIds []bson.ObjectId) (int, error) {
	info, err := o.working.RemoveAll(bson.M{"_id": bson.M{"$in": txnIds}})
	if err != nil {
		return 0, fmt.Errorf("error removing transaction ids: %v", err)
	}
	if info != nil {
		return info.Removed, nil
	}
	return 0, nil
}

type dbIterWrapper struct {
	iter *mgo.Iter
}

var _ OracleIterator = (*dbIterWrapper)(nil)

var EOF = fmt.Errorf("end of transaction ids")

func (d *dbIterWrapper) Next() (bson.ObjectId, error) {
	var txnId struct {
		Id bson.ObjectId `bson:"_id"`
	}
	if d.iter.Next(&txnId) {
		return txnId.Id, nil
	}
	if err := d.iter.Close(); err != nil {
		return txnId.Id, err
	}
	return txnId.Id, EOF
}
func (d *dbIterWrapper) Close() error {
	return d.iter.Close()
}

// IterTxns lets you iterate over all of the transactions that have
// not been removed.
func (o *DBOracle) IterTxns() (OracleIterator, error) {
	iter := o.working.Find(nil).Select(bson.M{"_id": 1}).Iter()
	return &dbIterWrapper{iter: iter}, nil
}

// MemOracle uses an in-memory cache to track what transactions are considered
// completed and purgeable.
type MemOracle struct {
	txns            *mgo.Collection
	thresholdTime   time.Time
	completed       map[bson.ObjectId]struct{}
	checkedTokens   uint64
	completedTokens uint64
	foundTxns       uint64
}

// NewMemOracle uses an in-memory map to manage the queue of  remaining
// transactions.
func NewMemOracle(txns *mgo.Collection, thresholdTime time.Time) (*MemOracle, func(), error) {
	oracle := &MemOracle{
		txns:          txns,
		thresholdTime: thresholdTime,
	}
	err := oracle.prepare()
	return oracle, noopCleanup, err
}

var _ Oracle = (*MemOracle)(nil)

func (o *MemOracle) prepare() error {
	if o.completed != nil {
		return fmt.Errorf("Prepare called twice")
	}
	// Load the ids of all completed and aborted txns into a separate
	// temporary collection.
	// Max memory consumed when dealing with 36M transactions was around 4GB
	// when testing this.
	logger.Debugf("loading all completed transactions")
	pipe := o.txns.Pipe([]bson.M{
		// This used to use $in but that's much slower than $gte.
		{"$match": completedOldTransactionMatch(o.thresholdTime)},
		{"$project": bson.M{"_id": 1}},
	})
	pipe.Batch(maxBatchDocs)
	pipe.AllowDiskUse()
	var txnId struct {
		Id bson.ObjectId `bson:"_id"`
	}
	completed := make(map[bson.ObjectId]struct{})
	iter := pipe.Iter()
	t := newSimpleTimer(logInterval)
	docCount := 0
	for iter.Next(&txnId) {
		completed[txnId.Id] = struct{}{}
		docCount++
		if t.isAfter() {
			logger.Debugf("loaded %d documents", docCount)
		}
	}
	if err := iter.Close(); err != nil {
		return err
	}
	o.completed = completed
	return nil
}

// CompletedTokens looks at the list of tokens and finds what referenced txns
// are completed, and then returns the set of tokens that are completed.
func (o *MemOracle) CompletedTokens(tokens []string) (map[string]bool, error) {
	result := make(map[string]bool, len(tokens))

	// The nonce is generated during preparing, and if 2 flushers race,
	// only one nonce makes it into the final transaction. However, other
	// nonces can also be considered 'completed'. (afaict, they are ignored,
	// thus won't be applied and can be considered completed.)
	for _, token := range tokens {
		objId := txnTokenToId(token)
		if _, ok := o.completed[objId]; ok {
			result[token] = true
			// this isn't exactly the same metric as the other
			// one, which noticed when the same txn object was
			// referred to by a different token
			o.foundTxns += 1
		}
	}
	o.checkedTokens += uint64(len(tokens))
	o.completedTokens += uint64(len(result))
	return result, nil
}

// RemoveTxns can be used to flag that a given transaction should not
// be considered part of the valid set.
func (o *MemOracle) RemoveTxns(txnIds []bson.ObjectId) (int, error) {
	removedCount := 0
	for _, txnId := range txnIds {
		if _, ok := o.completed[txnId]; ok {
			removedCount++
		}
		delete(o.completed, txnId)
	}
	return removedCount, nil
}

type memIterator struct {
	txnIds []bson.ObjectId
}

var _ OracleIterator = (*memIterator)(nil)

func (m *memIterator) Next() (bson.ObjectId, error) {
	var txnId bson.ObjectId
	if len(m.txnIds) == 0 {
		return txnId, EOF
	}
	txnId = m.txnIds[0]
	m.txnIds = m.txnIds[1:]
	return txnId, nil
}

type sortedTxnIds []bson.ObjectId

func (s sortedTxnIds) Len() int           { return len(s) }
func (s sortedTxnIds) Less(i, j int) bool { return s[i] < s[j] }
func (s sortedTxnIds) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// IterTxns lets you iterate over all of the transactions that have
// not been removed.
func (o *MemOracle) IterTxns() (OracleIterator, error) {
	all := make([]bson.ObjectId, 0, len(o.completed))
	for txnId, _ := range o.completed {
		all = append(all, txnId)
	}
	sort.Sort(sortedTxnIds(all))
	return &memIterator{txnIds: all}, nil
}

func (o *MemOracle) Count() int {
	return len(o.completed)
}
