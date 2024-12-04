// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/mongo"
)

const historyPruneBatchSize = 1000
const historyPruneProgressSeconds = 15

type doneCheck func() (bool, error)

type TimeUnit string

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
