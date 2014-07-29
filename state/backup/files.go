// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/juju/utils/hash"
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

// ExtractTimestamp returns the timestamp embedded in the name.
func ExtractTimestamp(name string) (time.Time, error) {
	var timestamp time.Time
	var Y, M, D, h, m, s int
	template := fmt.Sprintf(FilenameTemplate, TimestampFormat)
	_, err := fmt.Sscanf(name, template, &Y, &M, &D, &h, &m, &s)
	if err != nil {
		return timestamp, fmt.Errorf("error extracting timestamp: %v", err)
	}
	timestamp = time.Date(Y, time.Month(M), D, h, m, s, 0, time.UTC)
	return timestamp, nil
}

// CreateEmptyFile returns a new file (and its filename).  The file is
// created fresh and is intended as the target for writing a new backup
// archive.  If excl is true, a file cannot exist at the filename already.
//
// If the provided filename is an empty string, a default filename is
// generated using the current UTC timestamp.  Likewise if the filename
// ends with the path separator (e.g. "/"), the default filename is
// generated and appended to the provided one.
func CreateEmptyFile(filename string, mode os.FileMode, excl bool) (*os.File, string, error) {
	if filename == "" {
		filename = defaultFilename(nil)
	} else if strings.HasSuffix(filename, string(os.PathSeparator)) {
		filename += defaultFilename(nil)
	}

	var file *os.File
	var err error
	if excl {
		flags := os.O_RDWR | os.O_CREATE | os.O_EXCL
		file, err = os.OpenFile(filename, flags, mode)
	} else {
		file, err = os.Create(filename)
	}
	if err != nil {
		return nil, "", fmt.Errorf("could not create backup file: %v", err)
	}
	logger.Infof("created: %s", filename)
	return file, filename, nil
}
