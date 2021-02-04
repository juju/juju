// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package initialize

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/containeragent/utils"
)

type (
	ConfigFromEnv = configFromEnv
)

var (
	DefaultIdentity = defaultIdentity
)

func NewInitCommandForTest(applicationAPI ApplicationAPI, fileReaderWriter utils.FileReaderWriter, environment utils.Environment) cmd.Command {
	return &initCommand{
		config:           defaultConfig,
		identity:         defaultIdentity,
		applicationAPI:   applicationAPI,
		fileReaderWriter: fileReaderWriter,
		environment:      environment,
	}
}
