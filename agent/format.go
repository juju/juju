// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/juju-core/utils"
)

// The format file in the agent config directory was used to identify
// the method of serialization. This was used by individual legacy
// (pre 1.18) format readers and writers to be able to translate from
// the file format to the in-memory structure. From version 1.18, the
// format is part of the agent configuration file, so there is only a
// single source of truth.
//
// Juju only supports upgrading from single steps, so Juju only needs
// to know about the current format and the format of the previous
// stable release. For convenience, the format name includes the
// version number of the stable release that it will be released with.
// Once this release has happened, the format should be considered
// FIXED, and should no longer be modified. If changes are necessary
// to the format, a new format should be created.
//
// We don't need to create new formats for each release, the version
// number is just a convenience for us to know which stable release
// introduced that format.

const (
	legacyFormatFilename = "format"
	currentFormat        = format_1_18
	previousFormat       = format_1_16
)

var (
	currentFormatter  = &formatter_1_18{}
	previousFormatter = &formatter_1_16{}
)

// The formatter defines the two methods needed by the formatters for
// translating to and from the internal, format agnostic, structure.
type formatter interface {
	read(location string) (*configInternal, error)
	write(config *configInternal) error
	writeCommands(config *configInternal) ([]string, error)
	// migrate is called when upgrading from the previous format to
	// the current format.
	migrate(config *configInternal)
}

func formatFile(dirName string) string {
	return filepath.Join(dirName, legacyFormatFilename)
}

func readFormat(dirName string) (string, error) {
	contents, err := ioutil.ReadFile(formatFile(dirName))
	if err != nil {
		// In pre-1.12 agents the format file will be missing,
		// but we no longer support them. It will be missing
		// also with 1.18 or later agents.
		return currentFormat, nil
	}
	return strings.TrimSpace(string(contents)), nil
}

func newFormatter(format string) (formatter, error) {
	switch format {
	case currentFormat:
		return currentFormatter, nil
	case previousFormat:
		return previousFormatter, nil
	}
	return nil, fmt.Errorf("unknown agent config format %q", format)
}

func writeFormatFile(dirName string, format string) error {
	if format == currentFormat {
		// In 1.18 we no longer use a format file.
		return nil
	}
	if err := os.MkdirAll(dirName, 0755); err != nil {
		return err
	}
	newFile := formatFile(dirName) + "-new"
	if err := ioutil.WriteFile(newFile, []byte(format+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(newFile, formatFile(dirName))
}

func writeFileCommands(filename, contents string, permission int) []string {
	quotedFilename := utils.ShQuote(filename)
	return []string{
		fmt.Sprintf("install -m %o /dev/null %s", permission, quotedFilename),
		fmt.Sprintf(`printf '%%s\n' %s > %s`, utils.ShQuote(contents), quotedFilename),
	}
}

func writeCommandsForFormat(dirName, format string) []string {
	commands := []string{"mkdir -p " + utils.ShQuote(dirName)}
	commands = append(commands, writeFileCommands(formatFile(dirName), format, 0644)...)
	return commands
}
