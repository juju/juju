// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	TimestampFormat  = "%04d%02d%02d-%02d%02d%02d" // YYMMDD-hhmmss
	FilenameTemplate = "jujubackup-%s.tar.gz"      // takes a timestamp
)

func defaultFilename(now *time.Time) string {
	if now == nil {
		_now := time.Now().UTC()
		now = &_now
	}
	Y, M, D := now.Date()
	h, m, s := now.Clock()
	formattedDate := fmt.Sprintf(TimestampFormat, Y, M, D, h, m, s)
	return fmt.Sprintf(FilenameTemplate, formattedDate)
}

// CreateEmptyFile returns a new file (and its filename).  The file is
// created fresh and is intended as the target for writing a new backup
// archive.  If excl is true, a file cannot exist at the filename already.
//
// If the provided filename is an empty string, a default filename is
// generated using the current UTC timestamp.  Likewise if the filename
// ends with the path separator (e.g. "/"), the default filename is
// generated and appended to the provided one.
func CreateEmptyFile(filename string, excl bool) (*os.File, string, error) {
	if filename == "" {
		filename = defaultFilename(nil)
	} else if strings.HasSuffix(filename, string(os.PathSeparator)) {
		filename += defaultFilename(nil)
	}

	var file *os.File
	var err error
	if excl {
		flags := os.O_RDWR | os.O_CREATE | os.O_EXCL
		file, err = os.OpenFile(filename, flags, 0666)
	} else {
		file, err = os.Create(filename)
	}
	if err != nil {
		return nil, "", fmt.Errorf("could not create backup file: %v", err)
	}
	return file, filename, nil
}

// WriteBackup writes an input stream into an archive file.  It returns
// the SHA-1 hash of the data written to the archive.
//
// Note that the hash is of the compressed file rather than uncompressed
// data since it is simpler.  Ultimately it doesn't matter as long as
// the API server does the same thing (which it will if the juju version
// is the same).
func WriteBackup(archive io.Writer, infile io.Reader) (string, error) {
	// Set up hashing the archive.
	hasher := sha1.New()
	target := io.MultiWriter(archive, hasher)

	// Copy into the archive.
	_, err := io.Copy(target, infile)
	if err != nil {
		return "", fmt.Errorf("could not write to the backup file: %v", err)
	}

	// Compute the hash.
	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	return hash, nil
}
