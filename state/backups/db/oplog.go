// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/mongo"
)

// For more info on the oplog format, see:
//  http://www.kchodorow.com/blog/2010/10/12/replication-internals/
//  http://docs.mongodb.org/manual/tutorial/troubleshoot-replica-sets/
//  http://www.tokutek.com/2014/06/why-tokumx-changed-mongodbs-oplog-format-for-operations/

func entryDB(doc *bson.M) string {
	ns := (*doc)["ns"].(string)
	return strings.Split(ns, ".")[0]
}

// stripOplog removes entries for the specified DBs from the dumped
// oplog.bson file.
func stripOplog(dbnames *set.Strings, dumpDir string) error {
	oplogFilename := filepath.Join(dumpDir, "oplog.bson")
	tempFilename := oplogFilename + ".new"

	info, err := os.Stat(oplogFilename)
	if os.IsNotExist(err) {
		// Do not worry about missing files.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	} else if info.Size() == 0 {
		// Do not worry about empty files.
		return nil
	}

	err = stripOplogPath(dbnames, oplogFilename, tempFilename)
	if err != nil {
		return errors.Trace(err)
	}

	err = os.Rename(tempFilename, oplogFilename)
	return errors.Trace(err)
}

func stripOplogPath(dbnames *set.Strings, oldPath, newPath string) error {
	oplogFile, err := os.Open(oldPath)
	if err != nil {
		return errors.Trace(err)
	}
	defer oplogFile.Close()

	strippedFile, err := os.Create(newPath)
	if err != nil {
		return errors.Trace(err)
	}
	defer strippedFile.Close()

	// Allocate the BSON size header.
	if _, err := strippedFile.Write([]byte{0, 0, 0, 0}); err != nil {
		return errors.Trace(err)
	}

	// Strip the ignored entries.
	size, err := stripOplogFile(dbnames, oplogFile, strippedFile)
	if err != nil {
		return errors.Trace(err)
	}

	// Set the BSON size header.
	if size > 4 {
		_, err = strippedFile.Seek(0, os.SEEK_SET)
		if err != nil {
			return errors.Trace(err)
		}
		_, err = strippedFile.Write([]byte{
			byte(size >> 0 & 255),
			byte(size >> 8 & 255),
			byte(size >> 16 & 255),
			byte(size >> 24 & 255),
		})
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		err := strippedFile.Truncate(0)
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func stripOplogFile(dbnames *set.Strings, oldFile, newFile *os.File) (int32, error) {
	bsonIter, _, err := mongo.IterBSON(oldFile)
	if err != nil {
		return -1, errors.Trace(err)
	}

	size, totalSize := 0, 4
	var doc bson.M
	data := make([]byte, mongo.MaxBSONSize)

	for bsonIter.NextRaw(data, &size) {
		if bsonIter.Err() != nil {
			return -1, errors.Trace(bsonIter.Err())
		}

		if err := bson.Unmarshal(data[0:size], &doc); err != nil {
			return -1, errors.Trace(err)
		}

		dbName := entryDB(&doc)
		if !dbnames.Contains(dbName) {
			_, err := newFile.Write(data[0:size])
			if err != nil {
				return -1, errors.Trace(err)
			}
			totalSize += size
		}
	}

	return int32(totalSize), nil
}
