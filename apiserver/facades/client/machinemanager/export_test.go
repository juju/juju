// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

var InstanceTypes = instanceTypes
var IsBaseLessThan = isBaseLessThan

func NewTestUpgradeSeriesValidator(localValidator, remoteValidator ApplicationValidator) upgradeSeriesValidator {
	return upgradeSeriesValidator{
		localValidator:  localValidator,
		remoteValidator: remoteValidator,
	}
}

func NewTestStateSeriesValidator() stateSeriesValidator {
	return stateSeriesValidator{}
}

func NewTestCharmhubSeriesValidator(client CharmhubClient) charmhubSeriesValidator {
	return charmhubSeriesValidator{
		client: client,
	}
}
