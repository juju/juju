// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package txn

import (
	"fmt"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// Transaction states copied From mgo/txn.
const (
	taborted = 5 // Pre-conditions failed, nothing done
	tapplied = 6 // All changes applied
)

type pruneStats struct {
	Id         bson.ObjectId `bson:"_id"`
	Started    time.Time     `bson:"started"`
	Completed  time.Time     `bson:"completed"`
	TxnsBefore int           `bson:"txns-before"`
	TxnsAfter  int           `bson:"txns-after"`
}

func maybePrune(db *mgo.Database, txnsName string, pruneFactor float32) error {
	txnsPrune := db.C(txnsPruneC(txnsName))
	txns := db.C(txnsName)

	txnsCount, err := txns.Count()
	if err != nil {
		return fmt.Errorf("failed to retrieve starting txns count: %v", err)
	}
	lastTxnsCount, err := getPruneLastTxnsCount(txnsPrune)
	if err != nil {
		return fmt.Errorf("failed to retrieve pruning stats: %v", err)
	}

	required := lastTxnsCount == 0 || float32(txnsCount) >= float32(lastTxnsCount)*pruneFactor
	logger.Infof("txns after last prune: %d, txns now = %d, pruning required: %v", lastTxnsCount, txnsCount, required)

	if required {
		started := time.Now()
		err := pruneTxns(txnsPrune.Database, txns)
		if err != nil {
			return err
		}
		completed := time.Now()

		txnsCountAfter, err := txns.Count()
		if err != nil {
			return fmt.Errorf("failed to retrieve final txns count: %v", err)
		}
		logger.Infof("txn pruning complete. txns now = %d", txnsCountAfter)
		return writePruneTxnsCount(txnsPrune, started, completed, txnsCount, txnsCountAfter)
	}

	return nil
}

func getPruneLastTxnsCount(txnsPrune *mgo.Collection) (int, error) {
	// Retrieve the doc which points to the latest stats entry.
	var ptrDoc bson.M
	err := txnsPrune.FindId("last").One(&ptrDoc)
	if err == mgo.ErrNotFound {
		return 0, nil
	} else if err != nil {
		return -1, fmt.Errorf("failed to load pruning stats pointer: %v", err)
	}

	// Get the stats.
	var doc pruneStats
	err = txnsPrune.FindId(ptrDoc["id"]).One(&doc)
	if err == mgo.ErrNotFound {
		// Pointer was broken. Recover by returning 0 which will force
		// pruning.
		logger.Warningf("pruning stats pointer was broken - will recover")
		return 0, nil
	} else if err != nil {
		return -1, fmt.Errorf("failed to load pruning stats: %v", err)
	}
	return doc.TxnsAfter, nil
}

func writePruneTxnsCount(
	txnsPrune *mgo.Collection,
	started, completed time.Time,
	txnsBefore, txnsAfter int,
) error {
	id := bson.NewObjectId()
	err := txnsPrune.Insert(pruneStats{
		Id:         id,
		Started:    started,
		Completed:  completed,
		TxnsBefore: txnsBefore,
		TxnsAfter:  txnsAfter,
	})
	if err != nil {
		return fmt.Errorf("failed to write prune stats: %v", err)
	}

	// Set pointer to latest stats document.
	_, err = txnsPrune.UpsertId("last", bson.M{"$set": bson.M{"id": id}})
	if err != nil {
		return fmt.Errorf("failed to write prune stats pointer: %v", err)
	}
	return nil
}

func txnsPruneC(txnsName string) string {
	return txnsName + ".prune"
}

// pruneTxns removes applied and aborted entries from the txns
// collection that are no longer referenced by any document.
//
// Warning: this is a fairly heavyweight activity and therefore should
// be done infrequently.
//
// TODO(mjs) - this knows way too much about mgo/txn's internals and
// with a bit of luck something like this will one day be part of
// mgo/txn.
func pruneTxns(db *mgo.Database, txns *mgo.Collection) error {
	present := struct{}{}

	// Load the ids of all completed txns and all collections
	// referred to by those txns.
	//
	// This set could potentially contain many entries, however even
	// 500,000 entries requires only ~44MB of memory. Given that the
	// memory hit is short-lived this is probably acceptable.
	txnIds := make(map[bson.ObjectId]struct{})
	collNames := make(map[string]struct{})

	var txnDoc struct {
		Id  bson.ObjectId `bson:"_id"`
		Ops []txn.Op      `bson:"o"`
	}

	completed := bson.M{
		"s": bson.M{"$in": []int{taborted, tapplied}},
	}
	iter := txns.Find(completed).Select(bson.M{"_id": 1, "o": 1}).Iter()
	for iter.Next(&txnDoc) {
		txnIds[txnDoc.Id] = present
		for _, op := range txnDoc.Ops {
			collNames[op.C] = present
		}
	}
	if err := iter.Close(); err != nil {
		return fmt.Errorf("failed to read all txns: %v", err)
	}

	// Transactions may also be referenced in the stash.
	collNames["txns.stash"] = present

	// Now remove the txn ids referenced by all documents in all
	// txn using collections from the set of known txn ids.
	//
	// Working the other way - starting with the set of txns
	// referenced by documents and then removing any not in that set
	// from the txns collection - is unsafe as it will result in the
	// removal of transactions run while pruning executes.
	//
	for collName := range collNames {
		coll := db.C(collName)
		var tDoc struct {
			Queue []string `bson:"txn-queue"`
		}
		iter := coll.Find(nil).Select(bson.M{"txn-queue": 1}).Iter()
		for iter.Next(&tDoc) {
			for _, token := range tDoc.Queue {
				delete(txnIds, txnTokenToId(token))
			}
		}
		if err := iter.Close(); err != nil {
			return fmt.Errorf("failed to read docs: %v", err)
		}
	}

	// Remove the unreferenced transactions.
	err := bulkRemoveTxns(txns, txnIds)
	if err != nil {
		return fmt.Errorf("txn removal failed: %v", err)
	}
	return nil
}

func txnTokenToId(token string) bson.ObjectId {
	// mgo/txn transaction tokens are the 24 character txn id
	// followed by "_<nonce>"
	return bson.ObjectIdHex(token[:24])
}

// bulkRemoveTxns removes transaction documents in chunks. It should
// be significantly more efficient than removing one document per
// remove query while also not trigger query document size limits.
func bulkRemoveTxns(txns *mgo.Collection, txnIds map[bson.ObjectId]struct{}) error {
	removeTxns := func(ids []bson.ObjectId) error {
		_, err := txns.RemoveAll(bson.M{"_id": bson.M{"$in": ids}})
		switch err {
		case nil, mgo.ErrNotFound:
			// It's OK for txns to no longer exist. Another process
			// may have concurrently pruned them.
			return nil
		default:
			return err
		}
	}

	const chunkMax = 1024
	chunk := make([]bson.ObjectId, 0, chunkMax)
	for txnId := range txnIds {
		chunk = append(chunk, txnId)
		if len(chunk) == chunkMax {
			if err := removeTxns(chunk); err != nil {
				return err
			}
			chunk = chunk[:0] // Avoid reallocation.
		}
	}
	if len(chunk) > 0 {
		if err := removeTxns(chunk); err != nil {
			return err
		}
	}

	return nil
}
