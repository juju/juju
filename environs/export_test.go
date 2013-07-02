// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

type BootstrapState bootstrapState

func Providers() map[string]EnvironProvider {
	return providers
}
