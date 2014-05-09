// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ubuntu

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.ubuntu")

// seriesVersions provides a mapping between Ubuntu series names and version numbers.
// The values here are current as of the time of writing. On Ubuntu systems, we update
// these values from /usr/share/distro-info/ubuntu.csv to ensure we have the latest values.
// On non-Ubuntu systems, these values provide a nice fallback option.
// Exported so tests can change the values to ensure the distro-info lookup works.
var seriesVersions = map[string]string{
	"precise": "12.04",
	"quantal": "12.10",
	"raring":  "13.04",
	"saucy":   "13.10",
	"trusty":  "14.04",
	"utopic":  "14.10",
}

var (
	seriesVersionsMutex   sync.Mutex
	updatedseriesVersions bool
)

// SeriesVersion returns the version number for the specified Ubuntu series.
func SeriesVersion(series string) (string, error) {
	if series == "" {
		panic("cannot pass empty series to SeriesVersion()")
	}
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	if vers, ok := seriesVersions[series]; ok {
		return vers, nil
	}
	updateSeriesVersions()
	if vers, ok := seriesVersions[series]; ok {
		return vers, nil
	}
	return "", fmt.Errorf("invalid series %q", series)
}

// SupportedSeries returns the Ubuntu series on which we can run Juju workloads.
func SupportedSeries() []string {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersions()
	var series []string
	for s := range seriesVersions {
		series = append(series, s)
	}
	return series
}

func updateSeriesVersions() {
	if !updatedseriesVersions {
		err := updateDistroInfo()
		if err != nil {
			logger.Warningf("failed to update distro info: %v", err)
		}
		updatedseriesVersions = true
	}
}

// updateDistroInfo updates seriesVersions from /usr/share/distro-info/ubuntu.csv if possible..
func updateDistroInfo() error {
	// We need to find the series version eg 12.04 from the series eg precise. Use the information found in
	// /usr/share/distro-info/ubuntu.csv provided by distro-info-data package.
	f, err := os.Open("/usr/share/distro-info/ubuntu.csv")
	if err != nil {
		// On non-Ubuntu systems this file won't exist but that's expected.
		return nil
	}
	defer f.Close()
	bufRdr := bufio.NewReader(f)
	// Only find info for precise or later.
	preciseOrLaterFound := false
	for {
		line, err := bufRdr.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading distro info file file: %v", err)
		}
		// lines are of the form: "12.04 LTS,Precise Pangolin,precise,2011-10-13,2012-04-26,2017-04-26"
		parts := strings.Split(line, ",")
		// Ignore any malformed lines.
		if len(parts) < 3 {
			continue
		}
		series := parts[2]
		if series == "precise" {
			preciseOrLaterFound = true
		}
		if series != "precise" && !preciseOrLaterFound {
			continue
		}
		// the numeric version may contain a LTS moniker so strip that out.
		seriesInfo := strings.Split(parts[0], " ")
		seriesVersions[series] = seriesInfo[0]
	}
	return nil
}
