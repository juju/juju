// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package config -destination agent_config_mock_test.go github.com/juju/juju/agent/config AgentConfigReader

func Test(t *testing.T) {
	gc.TestingT(t)
}
