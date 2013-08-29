// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
)

// The format file in the agent config directory is used to identify the
// method of serialization.  This is used by individual format readers and
// writers to be able to translate from the file format to the in-memory
// structure.
//
// Juju only supports upgrading from single steps, so Juju only needs to know
// about the current format and the format of the previous stable release.
// For convenience, the format name includes the version number of the stable
// release that it will be released with.  Once this release has happened, the
// format should be considered FIXED, and should no longer be modified.  If
// changes are necessary to the format, a new format should be created.
//
// We don't need to create new formats for each release, the version number is
// just a convenience for us to know which stable release introduced that
// format.

const (
	formatFilename = "format"
	currentFormat  = "format 1.12"
)

var currentFormatter = &formatter112{}

// The formatter defines the two methods needed by the formatters for
// translating to and from the internal, format agnostic, structure.
type formatter interface {
	read(dirName string) (*configInternal, error)
	write(config *configInternal) error
	writeCommands(config *configInternal) ([]string, error)
}

func readFormat(dirName string) (string, error) {
	return currentFormat, nil
}

func newFormatter(format string) (formatter, error) {
	switch format {
	case currentFormat:
		return currentFormatter, nil
	}
	return nil, fmt.Errorf("unknown agent config format")
}
