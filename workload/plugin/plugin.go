// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package plugin contains the code that interfaces with plugins for workload
// technologies such as Docker, Rocket, or systemd.
//
// Workload plugins are either executable files located on $PATH or
// libraries compiled into the jujud binary. Executable plugins
// have precedence. This allows charms to override built-in plugins,
// thus maintaining control of the functionality of the plugin. Doing
// so is desirable in a few situations which correlate closely with the
// overall benefits of plugins.
//
// In general, support for executable plugins here has the same benefits
// as the use of plugins in other parts of Juju; it allows charmers to:
//
// * guarantee that a plugin will not change during a Juju upgrade
// * use an updated version of a plugin before it is updated in Juju
// * use an updated version of a plugin without upgrading Juju
// * use a custom plugin before it is incorporated into Juju
// * use a custom plugin that might not be appropriate for Juju
// * use a proprietary plugin that should not be released publicly
// * create a plugin without needing to modify the Juju code base
//
// The possible down-side to the plugin approach is that a charm could
// use a plugin which performs poorly or causes failures. In turn that
// could be misinterpreted as a problem with Juju, thus inaccurately
// reflecting poorly on Juju itself. This is a concern that cannot be
// dismissed lightly.
//
// This isn't much different from the situation with plugin commands at
// the Juju CLI or even with charms themselves. However, with charms the
// distinction between "official" and other charms is more obvious and
// charms in the charm store are rigorously vetted. Clear rules about
// plugins used by charms in the charm store would help mitigate that
// concern. So would tighter control over how such plugins are
// distributed and updated (e.g. apt, bundled with charm).
//
// There is also a specific concern with charms that override built-in
// plugins, with similar consequences to the perception of Juju: such
// charms will not benefit from fixes and improvements to the plugin
// in newer Juju releases. The solution is much the same as with
// plugins in general.
package plugin

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin/executable"
)

var logger = loggo.GetLogger("juju.workload.plugin")

// We special-case a specific name for the sake of the feature tests.
const testingPluginName = "testing-plugin"

// Find returns the plugin for the given name.
//
// If the plugin is not found then errors.NotFound is returned.
func Find(name, dataDir string) (workload.Plugin, error) {
	// Currently we only support executable plugins for a specific
	// plugin name.
	if name != testingPluginName {
		// Effectively, this is the only thing we do in production.
		plugin, err := findBuiltin(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return plugin, nil
	}

	// For now the rest of this function is used only during testing.
	// Once executable plugins are generally supported, the preceding
	// portion of this function may be removed.
	//
	// That may happen once we resolve concerns about the plugin
	// approach. Until then we default to supporting only built-in
	// plugins.

	findExecutable := func(name string) (workload.Plugin, error) {
		return executable.FindPlugin(name, dataDir)
	}
	plugin, err := find(name, findExecutable, findBuiltin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return plugin, nil
}

type findPluginFunc func(name string) (workload.Plugin, error)

// find returns the plugin for the given name. First it looks for an
// executable plugin and then it falls back to one of the built-in
// plugins. Favoring executable plugins allows charms to maintain
// control over the plugin's behavior.
//
// If the plugin is not found then errors.NotFound is returned.
func find(name string, findExecutable, findBuiltin findPluginFunc) (workload.Plugin, error) {
	findPluginFuncs := []findPluginFunc{
		findExecutable,
		findBuiltin,
	}
	for _, findPlugin := range findPluginFuncs {
		plugin, err := findPlugin(name)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return plugin, nil
	}
	return nil, errors.NotFoundf("plugin %q", name)
}
