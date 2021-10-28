// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/params"
)

var (
	ClientGet      = &clientGet
	WebbrowserOpen = &webbrowserOpen

	ClientGUIArchives      = &clientGUIArchives
	ClientSelectGUIVersion = &clientSelectGUIVersion
	ClientUploadGUIArchive = &clientUploadGUIArchive
	GUIFetchMetadata       = &guiFetchMetadata
)

func NewGUICommandForTest(getGUIVersions func(connection api.Connection) ([]params.GUIArchiveVersion, error)) cmd.Command {
	return modelcmd.Wrap(&guiCommand{
		getGUIVersions: getGUIVersions,
	})
}
