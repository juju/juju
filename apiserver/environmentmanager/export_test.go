// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentmanager

var ConfigValuesFromStateServer = configValuesFromStateServer

func RestrictedProviderFields(em *EnvironmentManagerAPI, providerType string) ([]string, error) {
	return em.restrictedProviderFields(providerType)
}
