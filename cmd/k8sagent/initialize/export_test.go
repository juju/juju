// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"github.com/juju/cmd"
)

type (
	ConfigFromEnv = configFromEnv
)

var (
	DefaultIdentity = defaultIdentity
)

func NewInitCommandForTest(applicationAPI ApplicationAPI, fileReaderWriter FileReaderWriter) cmd.Command {
	return &initCommand{
		config:           defaultConfig,
		identity:         defaultIdentity,
		applicationAPI:   applicationAPI,
		fileReaderWriter: fileReaderWriter,
	}
}
