// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows
// +build !windows

package initialize

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/cmd/containeragent/utils"
)

type (
	ConfigFromEnv = configFromEnv
)

var (
	Identity = identityFromK8sMetadata
)

func NewInitCommandForTest(applicationAPI ApplicationAPI, fileReaderWriter utils.FileReaderWriter, environment utils.Environment) cmd.Command {
	return &initCommand{
		config:           defaultConfig,
		identity:         identityFromK8sMetadata,
		applicationAPI:   applicationAPI,
		fileReaderWriter: fileReaderWriter,
		environment:      environment,
	}
}
