// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metadata

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/juju/errors"
)

// BuildMetadata generates the metadata for a file.
func BuildMetadata(arFile *os.File, origin Origin, notes string) (*Metadata, error) {

	// Extract the file size.
	fi, err := arFile.Stat()
	if err != nil {
		return nil, errors.Trace(err)
	}
	size := fi.Size()

	// Extract the timestamp.
	var timestamp *time.Time
	rawstat := fi.Sys()
	if rawstat != nil {
		stat, ok := rawstat.(*syscall.Stat_t)
		if ok {
			ts := time.Unix(int64(stat.Ctim.Sec), 0)
			timestamp = &ts
		}
	}
	if timestamp == nil {
		// Fall back to modification time.
		ts := fi.ModTime()
		timestamp = &ts
	}

	// Get the checksum.
	hasher := sha1.New()
	_, err = io.Copy(hasher, arFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rawsum := hasher.Sum(nil)
	checksum := base64.StdEncoding.EncodeToString(rawsum)

	// Build the metadata.
	meta := NewMetadata(origin, notes, timestamp)
	err = meta.Finish(size, checksum, ChecksumFormat, timestamp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return meta, nil
}
