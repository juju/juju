// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"math"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/mongo"
)

// pruneCollection removes collection entries until
// only entries newer than <maxLogTime> remain and also ensures
// that the collection is smaller than <maxLogsMB> after the
// deletion.
func pruneCollection(
	stop <-chan struct{},
	mb modelBackend, maxHistoryTime time.Duration, maxHistoryMB int,
	coll *mgo.Collection, ageField string, filter bson.D,
	timeUnit TimeUnit,
) error {
	return pruneCollectionAndChildren(stop, mb, maxHistoryTime, maxHistoryMB, coll, nil, ageField, "", filter, 1, timeUnit)
}

// pruneCollectionAndChildren removes collection entries until
// only entries newer than <maxLogTime> remain and also ensures
// that the collection (or child collection if specified) is smaller
// than <maxLogsMB> after the deletion.
func pruneCollectionAndChildren(stop <-chan struct{}, mb modelBackend, maxHistoryTime time.Duration, maxHistoryMB int,
	coll, childColl *mgo.Collection, ageField, parentRefField string,
	filter bson.D, sizeFactor float64, timeUnit TimeUnit,
) error {
	p := collectionPruner{
		st:              mb,
		coll:            coll,
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
	if err := p.pruneByAge(stop); err != nil {
		return errors.Trace(err)
	}
	// First try pruning, excluding any items that
	// have an age field that is not yet set.
	// ie only prune completed items.
	if err := p.pruneBySize(stop); err != nil {
		return errors.Trace(err)
	}
	if ageField == "" {
		return nil
	}
	// If needed, prune additional incomplete items to
	// get under the size limit.
	p.ageField = ""
	return errors.Trace(p.pruneBySize(stop))
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

func (p *collectionPruner) pruneByAge(stop <-chan struct{}) error {
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
	deleted, err := deleteInBatches(stop, p.coll, p.childColl, p.parentRefField, iter, logTemplate, corelogger.INFO, noEarlyFinish)
	if err != nil {
		return errors.Trace(err)
	}
	if deleted > 0 {
		logger.Debugf("%s age pruning (%s): %d rows deleted", p.coll.Name, modelName, deleted)
	}
	return errors.Trace(iter.Close())
}

func collStats(coll *mgo.Collection) (bson.M, error) {
	var result bson.M
	err := coll.Database.Run(bson.D{
		{"collStats", coll.Name},
		{"scale", humanize.KiByte},
	}, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// For mongo > 4.4, if the collection exists,
	// there will be a "capped" attribute.
	_, ok := result["capped"]
	if !ok {
		return nil, errors.NotFoundf("Collection [%s.%s]", coll.Database.Name, coll.Name)
	}
	return result, nil
}

// dbCollectionSizeToInt processes the result of Database.collStats()
func dbCollectionSizeToInt(result bson.M, collectionName string) (int, error) {
	size, ok := result["size"]
	if !ok {
		logger.Warningf("mongo collStats did not return a size field for %q", collectionName)
		// this wasn't considered an error in the past, just treat it as size 0
		return 0, nil
	}
	if asint, ok := size.(int); ok {
		if asint < 0 {
			return 0, errors.Errorf("mongo collStats for %q returned a negative value: %v", collectionName, size)
		}
		return asint, nil
	}
	if asint64, ok := size.(int64); ok {
		// 2billion megabytes is 2 petabytes, which is outside our range anyway.
		if asint64 > math.MaxInt32 {
			return math.MaxInt32, nil
		}
		if asint64 < 0 {
			return 0, errors.Errorf("mongo collStats for %q returned a negative value: %v", collectionName, size)
		}
		return int(asint64), nil
	}
	return 0, errors.Errorf(
		"mongo collStats for %q did not return an int or int64 for size, returned %T: %v",
		collectionName, size, size)
}

// getCollectionKB returns the size of a MongoDB collection (in
// kilobytes), excluding space used by indexes.
func getCollectionKB(coll *mgo.Collection) (int, error) {
	stats, err := collStats(coll)
	if err != nil {
		return 0, errors.Trace(err)
	}
	return dbCollectionSizeToInt(stats, coll.Name)
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
	sizePerItem := float64(collKB) / float64(count)
	if sizePerItem == 0 {
		return 0, errors.Errorf("unexpected result calculating %s entry size", coll.Name)
	}
	return int(float64(collKB-maxSizeKB) / (sizePerItem * countRatio)), nil
}

func (p *collectionPruner) pruneBySize(stop <-chan struct{}) error {
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
	deleted, err := deleteInBatches(stop, p.coll, p.childColl, p.parentRefField, iter, template, corelogger.INFO, func() (bool, error) {
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
	stop <-chan struct{},
	coll *mgo.Collection,
	childColl *mgo.Collection,
	childField string,
	iter mongo.Iterator,
	logTemplate string,
	logLevel corelogger.Level,
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
		select {
		case <-stop:
			return deleted, nil
		default:
		}
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
