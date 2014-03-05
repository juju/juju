// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"fmt"
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

var formats = make(map[string]formatter)

// The formatter defines the two methods needed by the formatters for
// translating to and from the internal, format agnostic, structure.
type formatter interface {
	version() string
	unmarshal(data []byte) (Config, error)
}

func registerFormat(format formatter) {
	formats["format "+format.version()] = formatter
}

// Once a new format version is introduced:
// - Create a formatter for the new version (including a marshal() method);
// - Call registerFormat in the new format's init() function.
// - Change this to point to the new format;
// - Remove the marshal() method from the old format;

// currentFormat holds the current agent config version's formatter.
var currentFormat = format_1_18

// agentConfFile is the default file name of used for the agent
// config.
const agentConfFile = "agent.conf"

// formatPrefix is prefix of the first line in an agent config file.
const formatPrefix = "# format "

func writeFileCommands(filename string, contents []byte, permission int) []string {
	quotedFilename := utils.ShQuote(filename)
	return []string{
		fmt.Sprintf("install -m %o /dev/null %s", permission, quotedFilename),
		fmt.Sprintf(`printf '%%s\n' %s > %s`, utils.ShQuote(contents), quotedFilename),
	}
}

func getFormatter(version string) (formatter, error) {
	version = strings.TrimSpace(version)
	format, ok := formats[version]
	if !ok {
		return nil, fmt.Errorf("unknown agent config format %q", version)
	}
	return format, nil
}

func parseConfigData(data []byte) (formatter, Config, error) {
	i := bytes.IndexByte(data, '\n')
	if i == -1 {
		return nil, nil, fmt.Errorf("invalid agent config format: %s", string(data))
	}
	version, configData := string(data[0:i]), data[i+1:]
	if !strings.HasPrefix(version, formatPrefix) {
		return nil, nil, fmt.Errorf("malformed agent config format %q", version)
	}
	version = strings.TrimPrefix(version, formatPrefix)
	format, err := getFormatter(version)
	if err != nil {
		return nil, nil, err
	}
	config, err := format.unmarshal(configData)
	if err != nil {
		return nil, nil, err
	}
	return format, config, nil
}
