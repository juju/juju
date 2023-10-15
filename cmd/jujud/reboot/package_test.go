// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

// NewRebootForTest returns a Reboot object to be used for testing.
func NewRebootForTest(acfg AgentConfig, reboot RebootWaiter) *Reboot {
	return &Reboot{acfg: acfg, reboot: reboot}
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/service_mock.go github.com/juju/juju/cmd/jujud/reboot AgentConfig,Manager,Model,RebootWaiter,Service
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/instance_mock.go github.com/juju/juju/environs/instances Instance
