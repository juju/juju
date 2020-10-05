// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	apiclient "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/api"
)

type waitForCommandBase struct {
	modelcmd.ModelCommandBase

	newWatchAllAPIFunc func() (api.WatchAllAPI, error)
}

type watchAllAPIShim struct {
	*apiclient.Client
}

func (s watchAllAPIShim) WatchAll() (api.AllWatcher, error) {
	return s.Client.WatchAll()
}
