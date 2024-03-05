// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"github.com/juju/clock"
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/cmd/containeragent/utils"
)

type (
	ConfigFromEnv = configFromEnv
)

var (
	Identity = identityFromK8sMetadata
)

func NewInitCommandForTest(applicationAPI ApplicationAPI,
	fileReaderWriter utils.FileReaderWriter,
	environment utils.Environment,
	clock clock.Clock) cmd.Command {
	return &initCommand{
		config:           defaultConfig,
		identity:         identityFromK8sMetadata,
		applicationAPI:   applicationAPI,
		fileReaderWriter: fileReaderWriter,
		environment:      environment,
		clock:            clock,
	}
}
