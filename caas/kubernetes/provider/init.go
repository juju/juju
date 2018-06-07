// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import "github.com/juju/juju/caas"

const (
	providerType = "kubernetes"
)

func init() {
	caas.RegisterContainerProvider(providerType, providerInstance)
}
