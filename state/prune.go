// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"

	"github.com/juju/juju/mongo"
)

// pruneCollection removes collection entries until
// only entries newer than <maxLogTime> remain and also ensures
// that the collection is smaller than <maxLogsMB> after the
// deletion.
func pruneCollection(
	mb modelBackend, maxHistoryTime time.Duration, maxHistoryMB int,
	collectionName string, ageField string, filter bson.D,
	timeUnit TimeUnit,
) error {
	return pruneCollectionAndChildren(mb, maxHistoryTime, maxHistoryMB, collectionName, ageField, "", "", filter, 1, timeUnit)
}

// pruneCollectionAndChildren removes collection entries until
// only entries newer than <maxLogTime> remain and also ensures
// that the collection (or child collection if specified) is smaller
// than <maxLogsMB> after the deletion.
func pruneCollectionAndChildren(mb modelBackend, maxHistoryTime time.Duration, maxHistoryMB int,
	collectionName, ageField, childCollectionName, parentRefField string,
	filter bson.D, sizeFactor float64, timeUnit TimeUnit,
) error {
	// NOTE(axw) we require a raw collection to obtain the size of the
	// collection. Take care to include model-uuid in queries where
	// appropriate.
	entries, closer := mb.db().GetRawCollection(collectionName)
	defer closer()
	var childColl *mgo.Collection
	if childCollectionName != "" {
		coll, chCloser := mb.db().GetRawCollection(childCollectionName)
		defer chCloser()
		childColl = coll
	}

	p := collectionPruner{
		st:              mb,
		coll:            entries,
		childColl:       childColl,
		parentRefField:  parentRefField,
		childCountRatio: sizeFactor,
		maxAge:          maxHistoryTime,
		maxSize:         maxHistoryMB,
		ageField:        ageField,
		filter:          filter,
		timeUnit:        timeUnit,
	}
	if err := p.validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.pruneByAge(); err != nil {
		return errors.Trace(err)
	}
	// First try pruning, excluding any items that
	// have an age field that is not yet set.
	// ie only prune completed items.
	if err := p.pruneBySize(); err != nil {
		return errors.Trace(err)
	}
	if ageField == "" {
		return nil
	}
	// If needed, prune additional incomplete items to
	// get under the size limit.
	p.ageField = ""
	return errors.Trace(p.pruneBySize())
}

const historyPruneBatchSize = 1000
const historyPruneProgressSeconds = 15

type doneCheck func() (bool, error)

type TimeUnit string

const (
	NanoSeconds TimeUnit = "nanoseconds"
	GoTime      TimeUnit = "goTime"
)

type collectionPruner struct {
	st     modelBackend
	coll   *mgo.Collection
	filter bson.D

	// If specified, these fields define subordinate
	// entries to delete in a related collection.
	// The child records refer to the parents via
	// the value of the parentRefField.
	childColl       *mgo.Collection
	parentRefField  string
	childCountRatio float64 // ratio of child records to parent records.

	maxAge  time.Duration
	maxSize int

	ageField string
	timeUnit TimeUnit
}

func (p *collectionPruner) validate() error {
	if p.maxSize < 0 {
		return errors.NotValidf("non-positive max size")
	}
	if p.maxAge < 0 {
		return errors.NotValidf("non-positive max age")
	}
	if p.maxSize == 0 && p.maxAge == 0 {
		return errors.NewNotValid(nil, "backlog size and age constraints are both 0")
	}
	if p.childColl != nil && p.parentRefField == "" {
		return errors.NewNotValid(nil, "missing parent reference field when a child collection is specified")
	}
	return nil
}

func (p *collectionPruner) pruneByAge() error {
	if p.maxAge == 0 {
		return nil
	}

	t := p.st.clock().Now().Add(-p.maxAge)
	var age interface{}
	var notSet interface{}

	if p.timeUnit == NanoSeconds {
		age = t.UnixNano()
		notSet = 0
	} else {
		age = t
		notSet = time.Time{}
	}

	query := bson.D{
		{"model-uuid", p.st.ModelUUID()},
		{p.ageField, bson.M{"$gt": notSet, "$lt": age}},
	}
	query = append(query, p.filter...)
	iter := p.coll.Find(query).Select(bson.M{"_id": 1}).Iter()
	defer func() { _ = iter.Close() }()

	modelName, err := p.st.modelName()
	if err != nil {
		return errors.Trace(err)
	}
	logTemplate := fmt.Sprintf("%s age pruning (%s): %%d rows deleted", p.coll.Name, modelName)
	deleted, err := deleteInBatches(p.coll, p.childColl, p.parentRefField, iter, logTemplate, loggo.INFO, noEarlyFinish)
	if err != nil {
		return errors.Trace(err)
	}
	if deleted > 0 {
		logger.Debugf("%s age pruning (%s): %d rows deleted", p.coll.Name, modelName, deleted)
	}
	return errors.Trace(iter.Close())
}

func (*collectionPruner) toDeleteCalculator(coll *mgo.Collection, maxSizeMB int, countRatio float64) (int, error) {
	collKB, err := getCollectionKB(coll)
	if err != nil {
		return 0, errors.Annotate(err, "retrieving collection size")
	}
	maxSizeKB := maxSizeMB * humanize.KiByte
	if collKB <= maxSizeKB {
		return 0, nil
	}
	count, err := coll.Count()
	if err == mgo.ErrNotFound || count <= 0 {
		return 0, nil
	}
	if err != nil {
		return 0, errors.Annotatef(err, "counting %s records", coll.Name)
	}
	// For large numbers of items we are making an assumption that the size of
	// items can be averaged to give a reasonable number of items to drop to
	// reach the goal size.
	// Note: Capped collections are not used for this because they, currently
	// at least, lack a way to be resized and the size is expected to change
	// as real life data of the history usage is gathered.
	sizePerItem := float64(collKB) / float64(count)
	if sizePerItem == 0 {
		return 0, errors.Errorf("unexpected result calculating %s entry size", coll.Name)
	}
	return int(float64(collKB-maxSizeKB) / (sizePerItem * countRatio)), nil
}

func (p *collectionPruner) pruneBySize() error {
	if !p.st.IsController() {
		// Only prune by size in the controller. Otherwise we might
		// find that multiple pruners are trying to delete the latest
		// 1000 rows and end up with more deleted than we expect.
		return nil
	}
	if p.maxSize == 0 {
		return nil
	}
	var toDelete int
	var err error
	if p.childColl == nil {
		// We are only operating on a single collection so calculate the number
		// of items to delete based on the size of that collection.
		toDelete, err = p.toDeleteCalculator(p.coll, p.maxSize, 1.0)
	} else {
		// We need to free up space in a child collection so calculate the number
		// of parent items to delete based on the size of the child collection and
		// the ratio of child items per parent item.
		toDelete, err = p.toDeleteCalculator(p.childColl, p.maxSize, p.childCountRatio)
	}
	if err != nil {
		return errors.Annotate(err, "calculating items to delete")
	}
	if toDelete <= 0 {
		return nil
	}

	// If age field is set, add a filter which
	// excludes those items where the age field
	// is not set, ie only prune completed items.
	var filter bson.D
	if p.ageField != "" {
		var notSet interface{}
		if p.timeUnit == NanoSeconds {
			notSet = 0
		} else {
			notSet = time.Time{}
		}
		filter = bson.D{
			{p.ageField, bson.M{"$gt": notSet}},
		}
	}
	filter = append(filter, p.filter...)
	query := p.coll.Find(filter)
	if p.ageField != "" {
		query = query.Sort(p.ageField)
	}
	iter := query.Limit(toDelete).Select(bson.M{"_id": 1}).Iter()
	defer func() { _ = iter.Close() }()

	template := fmt.Sprintf("%s size pruning: deleted %%d of %d (estimated)", p.coll.Name, toDelete)
	deleted, err := deleteInBatches(p.coll, p.childColl, p.parentRefField, iter, template, loggo.INFO, func() (bool, error) {
		// Check that we still need to delete more
		collKB, err := getCollectionKB(p.coll)
		if err != nil {
			return false, errors.Annotatef(err, "retrieving %s collection size", p.coll.Name)
		}
		if collKB <= p.maxSize*humanize.KiByte {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("%s size pruning finished: %d rows deleted", p.coll.Name, deleted)
	return errors.Trace(iter.Close())
}

func deleteInBatches(
	coll *mgo.Collection,
	childColl *mgo.Collection,
	childField string,
	iter mongo.Iterator,
	logTemplate string,
	logLevel loggo.Level,
	shouldStop doneCheck,
) (int, error) {
	var doc bson.M
	chunk := coll.Bulk()
	chunkSize := 0

	var childChunk *mgo.Bulk
	if childColl != nil {
		childChunk = childColl.Bulk()
	}

	lastUpdate := time.Now()
	deleted := 0
	for iter.Next(&doc) {
		parentId := doc["_id"]
		chunk.Remove(bson.D{{"_id", parentId}})
		chunkSize++
		if childChunk != nil {
			if idStr, ok := parentId.(string); ok {
				_, localParentId, ok := splitDocID(idStr)
				if ok {
					childChunk.RemoveAll(bson.D{{childField, localParentId}})
				}
			}
		}
		if chunkSize == historyPruneBatchSize {
			_, err := chunk.Run()
			// NotFound indicates that records were already deleted.
			if err != nil && err != mgo.ErrNotFound {
				return 0, errors.Annotate(err, "removing batch")
			}

			deleted += chunkSize
			chunk = coll.Bulk()
			chunkSize = 0

			if childChunk != nil {
				_, err := childChunk.Run()
				// NotFound indicates that records were already deleted.
				if err != nil && err != mgo.ErrNotFound {
					return 0, errors.Annotate(err, "removing child batch")
				}
				childChunk = childColl.Bulk()
			}

			// Check that we still need to delete more
			done, err := shouldStop()
			if err != nil {
				return 0, errors.Annotate(err, "checking whether to stop")
			}
			if done {
				return deleted, nil
			}

			now := time.Now()
			if now.Sub(lastUpdate) >= historyPruneProgressSeconds*time.Second {
				logger.Logf(logLevel, logTemplate, deleted)
				lastUpdate = now
			}
		}
	}
	if err := iter.Close(); err != nil {
		return 0, errors.Annotate(err, "closing iterator")
	}

	if chunkSize > 0 {
		_, err := chunk.Run()
		if err != nil && err != mgo.ErrNotFound {
			return 0, errors.Annotate(err, "removing remainder")
		}
		if childChunk != nil {
			_, err := childChunk.Run()
			if err != nil && err != mgo.ErrNotFound {
				return 0, errors.Annotate(err, "removing child remainder")
			}
		}
	}

	return deleted + chunkSize, nil
}

func noEarlyFinish() (bool, error) {
	return false, nil
}
