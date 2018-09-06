// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import "gopkg.in/juju/names.v2"

var InstanceTypes = instanceTypes
var IsSeriesLessThan = isSeriesLessThan

func (mm *MachineManagerAPI) ValidateSeries(argumentSeries, currentSeries string, machineTag names.MachineTag) error {
	return mm.validateSeries(argumentSeries, currentSeries, machineTag)
}
