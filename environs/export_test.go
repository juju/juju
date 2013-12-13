// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

func Providers() map[string]EnvironProvider {
	return providers
}

func UpdateEnvironAttrs(envs *Environs, name string, newAttrs map[string]interface{}) {
	for k, v := range newAttrs {
		envs.rawEnvirons[name][k] = v
	}
}
