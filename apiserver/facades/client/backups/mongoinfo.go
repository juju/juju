// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
)

// TODO(axw) find a better way to pass the mongo info to the facade,
// without necessarily making it available to all facades. This was
// moved here so that we could remove the MongoConnectionInfo method
// from State.
func mongoInfo(dataDir, machineId string) (*mongo.MongoInfo, error) {
	path := agent.ConfigPath(dataDir, names.NewMachineTag(machineId))
	config, err := agent.ReadConfig(path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	info, ok := config.MongoInfo()
	if !ok {
		return nil, errors.Errorf("no mongo info found in %q", path)
	}
	return info, nil
}
