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
