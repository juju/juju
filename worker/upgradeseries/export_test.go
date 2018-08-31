// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchHostSeries(patcher patcher, series string) {
	patcher.PatchValue(&hostSeries, func() (string, error) { return series, nil })
}
