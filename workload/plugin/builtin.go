// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin/docker"
)

var builtinPlugins = map[string]workload.Plugin{
	"docker": docker.NewPlugin(),
}

func findBuiltin(name string) (workload.Plugin, error) {
	plugin, ok := builtinPlugins[name]
	if !ok {
		return nil, errors.NotFoundf("plugin %q", name)
	}
	return plugin, nil
}
