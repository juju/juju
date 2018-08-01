// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package txn

import (
	"fmt"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// CollectionConfig is the definition of what we will be cleaning up.
type CollectionConfig struct {
	// Oracle is an Oracle that we can use to determine if a given
	// transaction token should be considered a 'completed' transaction.
	Oracle Oracle

	// Source is the mongo collection holding documents created and managed
	// by transactions.
	Source *mgo.Collection

	// NumBatchTokens is the number of tokens that we will cache before
	// doing a query to find out whether their referenced transactions are
	// completed. It is useful to have a number in the hundreds so that we
	// efficiently query the mongo transaction database.
	NumBatchTokens int

	// MaxRemoveQueue is the maximum number of document ids that we will
	// hold on to in memory before we go back to the database to purge those
	// documents. This only affects StashCollectionCleaner, as the generic
	// cleaner never removes documents.
	MaxRemoveQueue int

	// LogInterval defines how often we will show progress
	LogInterval time.Duration
}

// CollectionStats tracks various counters that signal how the collector operated.
type CollectionStats struct {

	// DocCount is the total number of documents evaluated.
	DocCount int

	// TokenCount is the total number of transaction tokens that were
	// referenced by the documents.
	TokenCount int

	// CompletedTokenCount is the number of unique tokens that referenced
	// completed transactions.
	CompletedTokenCount int

	// CompletedTxnCount is the number of completed transactions that we
	// looked up.
	CompletedTxnCount int

	// UpdatedDocCount is the number of documents we modified without
	// removing them
	UpdatedDocCount int

	// PulledCount is the number of tokens that were removed from documents.
	PulledTokenCount int

	// RemovedCount represents the number of txns.stash documents that we
	// decided to remove entirely.
	RemovedCount int
}

// txnDocument represents the fields we care about for objects that participate
// in transactions. They must all have an _id so we can find them again, and a
// txn-queue so that they can refer to transactions. (we don't need the
// txn-revno and other fields at this point.)
type txnDocument struct {
	// Id is the _id field of the document. We use a bson.Raw so that we
	// don't enforce the structure, we just need to pass it back in.
	// interface{} causes bson to Unmarshall string/int correctly but
	// creates a bson.M for objects which loses ordering. bson.D causes
	// objects to deserialize correctly but fails to deserialize a simple
	// int or string.
	Id    bson.Raw `bson:"_id"`
	Queue []string `bson:"txn-queue"`
}

// collectionCleaner represents the state while we process a collection for
// transaction ids that no longer need to be referenced because they refer to
// transactions that have been completed.
type collectionCleaner struct {
	config         CollectionConfig
	docIdsToRemove []interface{}
	docsToProcess  []txnDocument
	tokensToLookup []string
	stats          CollectionStats
	// removeIfEmpty will remove documents that have all references removed.
	// This should only be set True for txns.stash
	removeIfEmpty bool
}

func (stats CollectionStats) HasChanges() bool {
	if stats.RemovedCount == 0 && stats.UpdatedDocCount == 0 &&
		stats.PulledTokenCount == 0 {
		return false
	}
	return true
}

func (stats CollectionStats) Details() string {
	return fmt.Sprintf("processed %d documents, removed %d, updated %d (%d tokens)\n"+
		"checked %d tokens (%d completed unique) across %d completed transactions",
		stats.DocCount, stats.RemovedCount, stats.UpdatedDocCount, stats.PulledTokenCount,
		stats.TokenCount, stats.CompletedTokenCount, stats.CompletedTxnCount)
}

// NewCollectionCleaner creates an object that can remove transaction tokens
// from document queues when the transactions have been marked as completed.
func NewCollectionCleaner(config CollectionConfig) *collectionCleaner {
	if config.NumBatchTokens == 0 {
		config.NumBatchTokens = queueBatchSize
	}
	if config.MaxRemoveQueue == 0 {
		config.MaxRemoveQueue = maxMemoryTokens
	}
	if config.LogInterval == 0 {
		config.LogInterval = logInterval
	}
	return &collectionCleaner{
		config:         config,
		docIdsToRemove: make([]interface{}, 0),
		docsToProcess:  make([]txnDocument, 0),
		tokensToLookup: make([]string, 0),
		removeIfEmpty:  false,
	}
}

// NewStashCleaner returns an object suitable for cleaning up the txns.stash collection.
// It is different because when we find all references from a document have been
// removed, we can remove the document.
func NewStashCleaner(config CollectionConfig) *collectionCleaner {
	return &collectionCleaner{
		config:         config,
		docIdsToRemove: make([]interface{}, 0),
		docsToProcess:  make([]txnDocument, 0),
		tokensToLookup: make([]string, 0),
		removeIfEmpty:  true,
	}
}

// includeDoc queues this doc to be processed. It returns 'true' if the docs
// should be processed.
func (cleaner *collectionCleaner) includeDoc(doc txnDocument) error {
	cleaner.stats.DocCount++
	cleaner.docsToProcess = append(cleaner.docsToProcess, doc)
	for _, token := range doc.Queue {
		cleaner.tokensToLookup = append(cleaner.tokensToLookup, token)
	}
	return nil
}

// findCompletedTokens looks at the list of tokens and finds what txns are
// referenced as completed, and then returns the set of tokens that are completed.
func (cleaner *collectionCleaner) findCompletedTokens() (map[string]bool, error) {
	result, err := cleaner.config.Oracle.CompletedTokens(cleaner.tokensToLookup)

	cleaner.stats.TokenCount += len(cleaner.tokensToLookup)
	cleaner.stats.CompletedTokenCount += len(result)
	// TODO:
	// cleaner.stats.CompletedTxnCount += len(foundIdHex)
	return result, err
}

// findPullableTokens checks to see what transaction tokens should be removed
// from this document.
func (*collectionCleaner) findPullableTokens(queue []string, completedTokens map[string]bool) []string {
	toRemove := make([]string, 0, len(queue))
	for _, token := range queue {
		if completedTokens[token] {
			// We found the completed token, thus it can be removed
			toRemove = append(toRemove, token)
		}
	}
	return toRemove
}

// processStashDocs operates on the queue of documents that we have pending
// to be processed.
func (cleaner *collectionCleaner) processStashDocs() error {
	if len(cleaner.docsToProcess) == 0 {
		return nil
	}
	completedTokens, err := cleaner.findCompletedTokens()
	if err != nil {
		return fmt.Errorf("error looking up completed transactions: %v", err)
	}
	pullChunk := cleaner.config.Source.Bulk()
	pullChunk.Unordered()
	pullCount := 0
	pullsToApply := 0
	flushPulls := func() error {
		result, err := pullChunk.Run()
		if err != nil {
			if err != mgo.ErrNotFound {
				// not found is odd, but not considered fatal,
				// all others are
				return fmt.Errorf("error while updating documents: %v", err)
			}
		}
		cleaner.stats.UpdatedDocCount += result.Matched
		cleaner.stats.PulledTokenCount += pullsToApply
		pullChunk = cleaner.config.Source.Bulk()
		pullChunk.Unordered()
		pullCount = 0
		pullsToApply = 0
		return nil
	}
	for _, doc := range cleaner.docsToProcess {
		toPull := cleaner.findPullableTokens(doc.Queue, completedTokens)
		if cleaner.removeIfEmpty && len(toPull) == len(doc.Queue) {
			// this document can just be removed from the stash
			cleaner.docIdsToRemove = append(cleaner.docIdsToRemove, doc.Id)
		} else if len(toPull) > 0 {
			// Note: (jam 2017-04-04) An observation, if it is legal
			// to pull a token from one document, it is legal to
			// pull it from all other documents. We could do
			// bulk operations by using the union of all
			// document ids and the union of all tokens to pull.
			pullsToApply += len(toPull)
			pull := bson.M{"$pullAll": bson.M{"txn-queue": toPull}}
			pullChunk.Update(bson.M{"_id": doc.Id}, pull)
			pullCount += 1
			if pullCount >= maxBulkOps {
				if err := flushPulls(); err != nil {
					return err
				}
			}
		}
	}
	if err := flushPulls(); err != nil {
		return err
	}
	// We've handled these tokens and documents
	cleaner.tokensToLookup = cleaner.tokensToLookup[:0]
	cleaner.docsToProcess = cleaner.docsToProcess[:0]
	return nil
}

// checkFlush checks if it is worth processing documents now, and returns
// 'true' if we actually processed them and caused a removal pass to run.
func (cleaner *collectionCleaner) checkFlush() (bool, error) {
	if len(cleaner.tokensToLookup) <= cleaner.config.NumBatchTokens {
		return false, nil
	}
	if err := cleaner.processStashDocs(); err != nil {
		return false, err
	}
	if len(cleaner.docIdsToRemove) < cleaner.config.MaxRemoveQueue {
		return false, nil
	}
	if err := cleaner.flushRemoveQueue(); err != nil {
		return true, err
	}
	return true, nil
}

// flushRemoveQueue ensures that all pending removals are flushed to the database.
func (cleaner *collectionCleaner) flushRemoveQueue() error {
	if len(cleaner.docIdsToRemove) == 0 {
		return nil
	}
	remover := newBatchRemover(cleaner.config.Source)
	for _, docId := range cleaner.docIdsToRemove {
		if err := remover.Remove(docId); err != nil {
			return fmt.Errorf("failed while removing document %v from %q",
				docId, cleaner.config.Source.Name)
		}
	}
	if err := remover.Flush(); err != nil {
		return fmt.Errorf("failed while removing documents from %q",
			cleaner.config.Source.Name)
	}
	cleaner.stats.RemovedCount += remover.Removed()
	logger.Debugf("flushing %d documents removed %d (%d total)",
		len(cleaner.docIdsToRemove), remover.Removed(), cleaner.stats.RemovedCount)
	cleaner.docIdsToRemove = cleaner.docIdsToRemove[:0]
	return nil
}

// Cleanup iterates the collection and ensures that all documents no longer
// reference completed transactions.
func (cleaner *collectionCleaner) Cleanup() error {
	startCount, _ := cleaner.config.Source.Count()
	logger.Debugf("cleaning up completed references from %q with %d docs",
		cleaner.config.Source.Name, startCount)
	t := newSimpleTimer(cleaner.config.LogInterval)
	// If we delete documents while we iterate, it can cause the iterator to
	// miss documents. So we do multiple passes on the database to make sure
	// we catch everything.
	var doc txnDocument
	for iterCount := 0; iterCount < maxIterCount; iterCount++ {
		removedWhileIterating := false
		// We only need to consider documents that have at least 1
		// entry in their txn-queue
		filter := bson.M{"txn-queue.0": bson.M{"$exists": 1}}
		if cleaner.removeIfEmpty {
			// Unless we are going to remove empty documents,
			// then we need to handle ones that have a queue.
			filter = bson.M{"txn-queue": bson.M{"$exists": 1}}
		}
		query := cleaner.config.Source.Find(filter)
		query.Batch(maxBatchDocs)
		iter := query.Iter()
		for iter.Next(&doc) {
			if err := cleaner.includeDoc(doc); err != nil {
				return err
			}
			if t.isAfter() {
				logger.Debugf("processed %d/%d docs from %q (removed %d)",
					cleaner.stats.DocCount, startCount,
					cleaner.config.Source.Name, cleaner.stats.RemovedCount)
			}
			didFlush, err := cleaner.checkFlush()
			if err != nil {
				return err
			}
			if didFlush {
				removedWhileIterating = true
			}
		}
		if err := cleaner.processStashDocs(); err != nil {
			return err
		}
		if err := cleaner.flushRemoveQueue(); err != nil {
			return err
		}
		if err := iter.Close(); err != nil {
			return fmt.Errorf("error while iterating %q: %v",
				cleaner.config.Source.Name, err)
		}
		if !removedWhileIterating {
			break
		}
	}
	if cleaner.stats.HasChanges() {
		logger.Debugf("%q %s",
			cleaner.config.Source.Name, cleaner.stats.Details())
	} else {
		logger.Debugf("%q: nothing to do",
			cleaner.config.Source.Name)
	}
	if cleaner.removeIfEmpty {
		finalCount, _ := cleaner.config.Source.Count()
		logger.Debugf("%s has %d documents left",
			cleaner.config.Source.Name, finalCount)
	}
	return nil
}

// cleanupStash goes through the txns.stash and removes documents that are no longer needed.
func cleanupStash(oracle Oracle, txnsStash *mgo.Collection, stats *CleanupStats) error {
	cleaner := NewStashCleaner(CollectionConfig{
		Oracle:         oracle,
		Source:         txnsStash,
		NumBatchTokens: queueBatchSize,
		MaxRemoveQueue: maxMemoryTokens,
		LogInterval:    logInterval,
	})
	err := cleaner.Cleanup()
	if stats != nil {
		stats.CollectionsInspected += 1
		stats.DocsInspected += cleaner.stats.DocCount
		stats.StashDocumentsRemoved += cleaner.stats.RemovedCount
		stats.DocsCleaned += cleaner.stats.UpdatedDocCount
	}
	return err
}
