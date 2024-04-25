// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import "sort"

// These methods are used only in various tests.

// SupportedLts are the current supported LTS series in ascending order.
func SupportedLts() []string {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersionsOnce()

	versions := []string{}
	for _, version := range ubuntuSeries {
		if !version.LTS || !version.Supported {
			continue
		}
		versions = append(versions, version.Version)
	}
	sort.Strings(versions)
	sorted := []string{}
	for _, v := range versions {
		sorted = append(sorted, versionSeries[v])
	}
	return sorted
}

// ESMSupportedJujuSeries returns a slice of just juju extended security
// maintenance supported ubuntu series.
func ESMSupportedJujuSeries() []string {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersionsOnce()
	var series []string
	for s, version := range ubuntuSeries {
		if !version.ESMSupported {
			continue
		}
		series = append(series, string(s))
	}
	return series
}

// SupportedJujuWorkloadSeries returns a slice of juju supported series that
// target a workload (deploying a charm).
func SupportedJujuWorkloadSeries() []string {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersionsOnce()
	var series []string
	for s, version := range allSeriesVersions {
		if !version.Supported || version.WorkloadType == UnsupportedWorkloadType {
			continue
		}
		series = append(series, string(s))
	}
	return series
}
