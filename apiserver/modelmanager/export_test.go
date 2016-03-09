// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

func RestrictedProviderFields(mm *ModelManagerAPI, providerType string) ([]string, error) {
	return mm.restrictedProviderFields(providerType)
}
