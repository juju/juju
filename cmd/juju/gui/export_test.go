// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

var (
	ClientGet      = &clientGet
	WebbrowserOpen = &webbrowserOpen

	ClientDashboardArchives      = &clientDashboardArchives
	ClientSelectDashboardVersion = &clientSelectDashboardVersion
	ClientUploadDashboardArchive = &clientUploadDashboardArchive
	DashboardFetchMetadata       = &dashboardFetchMetadata
)

func NewDashboardCommandForTest(getGUIVersions func(connection api.Connection) ([]params.DashboardArchiveVersion, error)) cmd.Command {
	return modelcmd.Wrap(&dashboardCommand{
		getDashboardVersions: getGUIVersions,
	})
}
