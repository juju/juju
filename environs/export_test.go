// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

var (
	Providers       = &globalProviders.providers
	ProviderAliases = &globalProviders.aliases
)

func UpdateEnvironAttrs(envs *Environs, name string, newAttrs map[string]interface{}) {
	for k, v := range newAttrs {
		envs.rawEnvirons[name][k] = v
	}
}
