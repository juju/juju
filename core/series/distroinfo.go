// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"encoding/csv"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
)

// UbuntuDistroInfo references a csv that contains all the distro information
// about info. This includes what the names and versions of a distro and if the
// distro is supported or not.
const UbuntuDistroInfo = "/usr/share/distro-info/ubuntu.csv"

const dateFormat = "2006-01-02"

// FileSystem defines a interface for interacting with the host os.
type FileSystem interface {
	Open(string) (*os.File, error)
}

// DistroInfoSerie holds the information about each distro.
type DistroInfoSerie struct {
	Version  string
	CodeName string
	Series   string
	Created  time.Time
	Released time.Time
	EOL      time.Time
}

// Supported returns true if the underlying series is supported or not.
// It expects the time to be in UTC.
func (d *DistroInfoSerie) Supported(now time.Time) bool {
	return now.After(d.Released.UTC()) && now.Before(d.EOL.UTC())
}

// LTS returns true if the series is an LTS or not.
func (d *DistroInfoSerie) LTS() bool {
	return strings.HasSuffix(d.Version, "LTS")
}

// DistroInfo holds records of which distro is supported or not.
// Refreshing will cause the distro to go out and fetch new information from
// the local file system to update itself.
type DistroInfo struct {
	mutex      sync.RWMutex
	path       string
	info       map[string]DistroInfoSerie
	fileSystem FileSystem
}

// NewDistroInfo creates a new DistroInfo for querying the distro.
func NewDistroInfo(path string) *DistroInfo {
	return &DistroInfo{
		path:       path,
		info:       make(map[string]DistroInfoSerie),
		fileSystem: defaultFileSystem{},
	}
}

// Refresh will attempt to update the information it has about each distro and
// if the distro is supported or not.
func (d *DistroInfo) Refresh() error {
	f, err := d.fileSystem.Open(d.path)
	if err != nil {
		// On non-Ubuntu systems this file won't exist but that's expected.
		if errors.Cause(err) == os.ErrNotExist {
			return nil
		}
		return errors.Trace(err)
	}
	defer func() {
		_ = f.Close()
	}()

	csvReader := csv.NewReader(f)
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil {
		return errors.Annotatef(err, "reading %s", d.path)
	}

	fieldNames := records[0]
	records = records[1:]

	result := make(map[string]DistroInfoSerie)

	// We ignore all series prior to precise.
	var foundPrecise bool
	for _, fields := range records {
		record, ok := consumeRecord(fieldNames, fields)
		if !ok {
			continue
		}

		createdDate, err := time.Parse(dateFormat, record.Created)
		if err != nil {
			continue
		}
		releasedDate, err := time.Parse(dateFormat, record.Released)
		if err != nil {
			continue
		}
		eolDate, err := time.Parse(dateFormat, record.EOL)
		if err != nil {
			continue
		}

		if !foundPrecise {
			if record.Series != "precise" {
				continue
			}
			foundPrecise = true
		}

		result[record.Series] = DistroInfoSerie{
			Version:  record.Version,
			CodeName: record.CodeName,
			Series:   record.Series,
			Created:  createdDate,
			Released: releasedDate,
			EOL:      eolDate,
		}
	}

	// Lock the distro info, as we're going to be updating it.
	d.mutex.Lock()
	d.info = result
	d.mutex.Unlock()

	return nil
}

// SeriesInfo returns the DistroInfoSerie for the series name.
func (d *DistroInfo) SeriesInfo(seriesName string) (DistroInfoSerie, bool) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	info, ok := d.info[seriesName]
	return info, ok
}

// record defines a raw distro line that hasn't been parsed.
type record struct {
	Version  string
	CodeName string
	Series   string
	Created  string
	Released string
	EOL      string
}

func consumeRecord(headers []string, fields []string) (record, bool) {
	var result record
	var malformed bool
	for i, field := range fields {
		if i >= len(headers) {
			break
		}

		if field == "" {
			malformed = true
		}

		switch headers[i] {
		case "version":
			result.Version = field
		case "codename":
			result.CodeName = field
		case "series":
			result.Series = field
		case "created":
			result.Created = field
		case "release":
			result.Released = field
		case "eol":
			result.EOL = field
		}
	}

	// If the record is malformed then the validity of the record is not ok.
	return result, !malformed
}
