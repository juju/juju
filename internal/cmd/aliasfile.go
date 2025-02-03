// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"context"
	"io/ioutil"
	"strings"
)

// ParseAliasFile will read the specified file and convert
// the content to a map of names to the command line arguments
// they relate to.  The function will always return a valid map, even
// if it is empty.
func ParseAliasFile(aliasFilename string) map[string][]string {
	result := map[string][]string{}
	if aliasFilename == "" {
		return result
	}

	content, err := ioutil.ReadFile(aliasFilename)
	if err != nil {
		logger.Tracef(context.TODO(), "unable to read alias file %q: %s", aliasFilename, err)
		return result
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			// skip blank lines and comments
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			logger.Warningf(context.TODO(), "line %d bad in alias file: %s", i+1, line)
			continue
		}
		name, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if name == "" {
			logger.Warningf(context.TODO(), "line %d missing alias name in alias file: %s", i+1, line)
			continue
		}
		if value == "" {
			logger.Warningf(context.TODO(), "line %d missing alias value in alias file: %s", i+1, line)
			continue
		}

		logger.Tracef(context.TODO(), "setting alias %q=%q", name, value)
		result[name] = strings.Fields(value)
	}
	return result
}
