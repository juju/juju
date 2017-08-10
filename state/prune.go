// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// pruneCollection removes collection entries until
// only entries newer than <maxLogTime> remain and also ensures
// that the collection is smaller than <maxLogsMB> after the
// deletion.
func pruneCollection(mb modelBackend, maxHistoryTime time.Duration, maxHistoryMB int, collectionName string, ageField string, timeUnit TimeUnit) error {

	// NOTE(axw) we require a raw collection to obtain the size of the
	// collection. Take care to include model-uuid in queries where
	// appropriate.
	entries, closer := mb.db().GetRawCollection(collectionName)
	defer closer()

	p := collectionPruner{
		st:       mb,
		coll:     entries,
		maxAge:   maxHistoryTime,
		maxSize:  maxHistoryMB,
		ageField: ageField,
		timeUnit: timeUnit,
	}
	if err := p.validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.pruneByAge(); err != nil {
		return errors.Trace(err)
	}
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
	st   modelBackend
	coll *mgo.Collection

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
		return errors.NotValidf("backlog size and age constraints are both 0")
	}
	return nil
}

func (p *collectionPruner) pruneByAge() error {
	if p.maxAge == 0 {
		return nil
	}

	t := p.st.clock().Now().Add(-p.maxAge)
	var age interface{}

	if p.timeUnit == NanoSeconds {
		age = t.UnixNano()
	} else {
		age = t
	}

	iter := p.coll.Find(bson.D{
		{"model-uuid", p.st.modelUUID()},
		{p.ageField, bson.M{"$lt": age}},
	}).Select(bson.M{"_id": 1}).Iter()

	modelName, err := p.st.modelName()
	if err != nil {
		return errors.Trace(err)
	}
	logTemplate := fmt.Sprintf("%s age pruning (%s): %%d rows deleted", p.coll.Name, modelName)
	deleted, err := p.deleteInBatches(iter, logTemplate, noEarlyFinish)
	if err != nil {
		return errors.Trace(err)
	}
	if deleted > 0 {
		logger.Infof("%s age pruning (%s): %d rows deleted", p.coll.Name, modelName, deleted)
	}
	return nil
}

func (p *collectionPruner) pruneBySize() error {
	if !p.st.isController() {
		// Only prune by size in the controller. Otherwise we might
		// find that multiple pruners are trying to delete the latest
		// 1000 rows and end up with more deleted than we expect.
		return nil
	}
	if p.maxSize == 0 {
		return nil
	}
	// Collection Size
	collMB, err := getCollectionMB(p.coll)
	if err != nil {
		return errors.Annotate(err, "retrieving collection size")
	}
	if collMB <= p.maxSize {
		return nil
	}
	// TODO(perrito666) explore if there would be any beneffit from having the
	// size limit be per model
	count, err := p.coll.Count()
	if err == mgo.ErrNotFound || count <= 0 {
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "counting %s records", p.coll.Name)
	}
	// We are making the assumption that status sizes can be averaged for
	// large numbers and we will get a reasonable approach on the size.
	// Note: Capped collections are not used for this because they, currently
	// at least, lack a way to be resized and the size is expected to change
	// as real life data of the history usage is gathered.
	sizePerStatus := float64(collMB) / float64(count)
	if sizePerStatus == 0 {
		return fmt.Errorf("unexpected result calculating %s entry size", p.coll.Name)
	}
	toDelete := int(float64(collMB-p.maxSize) / sizePerStatus)

	iter := p.coll.Find(nil).Sort(p.ageField).Limit(toDelete).Select(bson.M{"_id": 1}).Iter()

	template := fmt.Sprintf("%s size pruning: deleted %%d of %d (estimated)", p.coll.Name, toDelete)
	deleted, err := p.deleteInBatches(iter, template, func() (bool, error) {
		// Check that we still need to delete more
		collMB, err := getCollectionMB(p.coll)
		if err != nil {
			return false, errors.Annotatef(err, "retrieving %s collection size", p.coll.Name)
		}
		if collMB <= p.maxSize {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("%s size pruning finished: %d rows deleted", p.coll.Name, deleted)

	return nil
}

func (p *collectionPruner) deleteInBatches(iter *mgo.Iter, logTemplate string, shouldStop doneCheck) (int, error) {
	var doc bson.M
	chunk := p.coll.Bulk()
	chunkSize := 0

	lastUpdate := time.Now()
	deleted := 0
	for iter.Next(&doc) {
		chunk.Remove(bson.D{{"_id", doc["_id"]}})
		chunkSize++
		if chunkSize == historyPruneBatchSize {
			_, err := chunk.Run()
			// NotFound indicates that records were already deleted.
			if err != nil && err != mgo.ErrNotFound {
				return 0, errors.Annotate(err, "removing status history batch")
			}

			deleted += chunkSize
			chunk = p.coll.Bulk()
			chunkSize = 0

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
				logger.Infof(logTemplate, deleted)
				lastUpdate = now
			}
		}
	}

	if chunkSize > 0 {
		_, err := chunk.Run()
		if err != nil && err != mgo.ErrNotFound {
			return 0, errors.Annotate(err, "removing status history remainder")
		}
	}

	return deleted + chunkSize, nil
}

func noEarlyFinish() (bool, error) {
	return false, nil
}
