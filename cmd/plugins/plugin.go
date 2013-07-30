// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugins

var builtInPlugins []string

// Register records the fact that the command called pluginName is a built-in plugin.
func Register(pluginName string) {
	builtInPlugins = append(builtInPlugins, pluginName)
}

// IsBuiltIn returns true if name is a built in plugin.
func IsBuiltIn(name string) bool {
	for _, n := range builtInPlugins {
		if n == name {
			return true
		}
	}
	return false
}
