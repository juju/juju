// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type bootstrapInternalSuite struct{}

func TestBootstrapInternalSuite(t *testing.T) {
	tc.Run(t, &bootstrapInternalSuite{})
}

func (*bootstrapInternalSuite) TestBootstrapMachineAddressesReachDqlite(c *tc.C) {
	addresses := network.NewMachineAddresses([]string{"10.0.0.1"}).AsProviderAddresses()
	var gotAddresses network.ProviderAddresses
	bootstrap, err := NewAgentBootstrap(AgentBootstrapArgs{
		AdminUser:                 names.NewLocalUserTag("admin"),
		AgentConfig:               stubAgentConfig{dataDir: c.MkDir()},
		BootstrapEnviron:          stubBootstrapEnviron{},
		BootstrapMachineAddresses: addresses,
		BootstrapDqlite: func(
			_ context.Context, manager database.BootstrapNodeManager,
			bootstrapAddresses network.ProviderAddresses, _ model.UUID,
			_ corelogger.Logger,
			_ ...database.BootstrapOpt,
		) error {
			c.Check(manager, tc.NotNil)
			gotAddresses = bootstrapAddresses
			return nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.initializeDqlite(c.Context(), tc.Must0(c, model.NewUUID))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotAddresses, tc.DeepEquals, addresses)
}

type stubAgentConfig struct {
	agent.ConfigSetter
	dataDir string
}

func (s stubAgentConfig) DataDir() string {
	return s.dataDir
}

func (stubAgentConfig) CACert() string {
	return "ca-cert"
}

func (stubAgentConfig) ControllerAgentInfo() (controller.ControllerAgentInfo, bool) {
	return controller.ControllerAgentInfo{
		Cert:       "controller-cert",
		PrivateKey: "controller-key",
	}, true
}

type stubBootstrapEnviron struct {
	environs.BootstrapEnviron
}
