// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"io/ioutil"
	"strings"
	)

func readSeries(releaseFile string) string {
	data, err := ioutil.ReadFile(releaseFile)
	if err != nil {
		// Failed to read the LSB Release file, so fall back to OS probing
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		const prefix = "DISTRIB_CODENAME="
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(line[len(prefix):], "\t '\"")
		}
	}
	return "unknown"
}

