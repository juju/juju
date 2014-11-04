// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func parseOplogEntryStr(entry string) (db, coll, op, value string) {
	dot := strings.Index(entry, ".")
	if dot >= 0 {
		db = entry[:dot]
		entry = entry[dot+1:]
	}

	parts := strings.Split(entry, ":")
	coll = parts[0]

	op = "u"
	if len(parts) > 1 {
		if parts[1] != "" {
			op = parts[1]
		}
	}
	if len(parts) > 2 {
		value = parts[2]
	}

	return db, coll, op, value
}

func buildOplog(c *gc.C, out io.Writer, entries ...string) int32 {
	var size int

	for _, entry := range entries {
		db, coll, op, value := parseOplogEntryStr(entry)

		entry := bson.M{
			"ts": time.Now().UTC(),
			"h":  rand.Int63(),
			"op": op,
			"ns": db + "." + coll,
			"o":  bson.M{"value": value},
		}
		data, err := bson.Marshal(&entry)
		c.Assert(err, gc.IsNil)
		size += len(data) + 4

		writeOplogSize(c, out, int32(len(data)+4))
		_, err = out.Write(data)
		c.Assert(err, gc.IsNil)
	}

	return int32(size)
}

func writeOplogSize(c *gc.C, out io.Writer, size int32) {
	_, err := out.Write([]byte{
		byte(size >> 0 & 255),
		byte(size >> 8 & 255),
		byte(size >> 16 & 255),
		byte(size >> 24 & 255),
	})
	c.Assert(err, gc.IsNil)
}

func createOplog(c *gc.C, dumpDir string, entries ...string) {
	filename := filepath.Join(dumpDir, "oplog.bson")

	oplogFile, err := os.Create(filename)
	c.Assert(err, gc.IsNil)
	defer oplogFile.Close()

	_, err = oplogFile.Write([]byte{0, 0, 0, 0})
	c.Assert(err, gc.IsNil)
	size := buildOplog(c, oplogFile, entries...) + 4

	// Update doc size (first 4 bytes).
	_, err = oplogFile.Seek(0, os.SEEK_SET)
	c.Assert(err, gc.IsNil)
	writeOplogSize(c, oplogFile, size)

	checkOplogSize(c, dumpDir)
}

func checkOplog(c *gc.C, dumpDir string, entries ...string) {
	filename := filepath.Join(dumpDir, "oplog.bson")

	if len(entries) == 0 {
		// Verify that the file isn't there.
		_, err := os.Stat(filename)
		c.Check(err, jc.Satisfies, os.IsNotExist)
		return
	}

	var buf bytes.Buffer
	size := buildOplog(c, &buf, entries...)
	expected := buf.Bytes()
	buf.Reset()
	writeOplogSize(c, &buf, size)
	expected = append(buf.Bytes(), expected...)

	data, err := ioutil.ReadFile(filename)
	c.Assert(err, gc.IsNil)

	c.Check(data, jc.DeepEquals, expected)
	checkOplogSize(c, dumpDir)
}

func checkOplogSize(c *gc.C, dumpDir string) {
	oplogFile, err := os.Open(filepath.Join(dumpDir, "oplog.bson"))
	if err != nil {
		if !os.IsNotExist(err) {
			c.Assert(err, gc.IsNil)
		}
		return
	}
	defer oplogFile.Close()

	info, err := oplogFile.Stat()
	c.Assert(err, gc.IsNil)
	expected := info.Size()
	c.Assert(int(expected), jc.GreaterThan, 4)

	buf := make([]byte, 4)
	_, err = oplogFile.Read(buf)
	c.Assert(err, gc.IsNil)
	size := int64(
		(uint32(buf[0]) << 0) |
			(uint32(buf[1]) << 8) |
			(uint32(buf[2]) << 16) |
			(uint32(buf[3]) << 24),
	)

	c.Check(size, gc.Equals, expected)
}
