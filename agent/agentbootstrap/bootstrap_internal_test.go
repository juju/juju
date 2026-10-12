// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
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

	err = bootstrap.initialiseDqlite(c.Context(), tc.Must0(c, model.NewUUID))
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

// recordingAgentConfig records password mutations so tests can prove
// Initialize never produces or rotates a credential.
type recordingAgentConfig struct {
	agent.ConfigSetter

	tag             names.Tag
	oldPassword     string
	apiPasswords    []string
	controllerInfos []controller.ControllerAgentInfo
}

func (cfg *recordingAgentConfig) Tag() names.Tag {
	return cfg.tag
}

func (cfg *recordingAgentConfig) DataDir() string {
	return "/tmp/agent-data-dir"
}

func (cfg *recordingAgentConfig) CACert() string {
	return "ca-cert"
}

func (cfg *recordingAgentConfig) OldPassword() string {
	return cfg.oldPassword
}

func (cfg *recordingAgentConfig) ControllerAgentInfo() (controller.ControllerAgentInfo, bool) {
	return controller.ControllerAgentInfo{
		APIPort:      17070,
		Cert:         "controller-cert",
		PrivateKey:   "controller-key",
		CAPrivateKey: "controller-ca-key",
	}, true
}

func (cfg *recordingAgentConfig) SetPassword(password string) {
	cfg.apiPasswords = append(cfg.apiPasswords, password)
}

func (cfg *recordingAgentConfig) SetControllerAgentInfo(info controller.ControllerAgentInfo) {
	cfg.controllerInfos = append(cfg.controllerInfos, info)
}

// TestInitializeDoesNotRotatePassword proves AgentBootstrap.Initialize
// performs no password rotation on either topology. The K8s pod machine
// agent's credential is the pre-rendered controller-node password, and
// the IAAS snap path keeps runtime.conf one-shot and seed-only, so on
// both topologies Initialize must leave the agent config password
// untouched.
func (*bootstrapInternalSuite) TestInitializeDoesNotRotatePassword(c *tc.C) {
	for _, test := range []struct {
		about     string
		cloudType string
	}{
		{about: "IAAS", cloudType: "lxd"},
		{about: "K8s", cloudType: cloud.CloudTypeKubernetes},
	} {
		c.Logf("topology: %s", test.about)
		modelConfig := coretesting.CustomModelConfig(c, coretesting.Attrs{})
		agentCfg := &recordingAgentConfig{
			tag:         names.NewControllerAgentTag(agent.BootstrapControllerId),
			oldPassword: "seed-password-0123456789",
		}
		bootstrap, err := NewAgentBootstrap(AgentBootstrapArgs{
			AdminUser:        names.NewLocalUserTag(coreuser.AdminUserName.Name()),
			AgentConfig:      agentCfg,
			BootstrapEnviron: stubBootstrapEnviron{},
			StateInitialisationParams: instancecfg.StateInitializationParams{
				ControllerConfig: controller.Config{
					controller.ControllerUUIDKey: coretesting.ControllerTag.Id(),
				},
				ControllerModelConfig: modelConfig,
				ControllerCloud: cloud.Cloud{
					Name:            test.cloudType,
					Type:            test.cloudType,
					HostCloudRegion: "cloud/region",
				},
				SSHServerHostKey: "ssh-host-key",
				AgentVersion:     jujuversion.Current,
			},
			BootstrapDqlite: func(
				_ context.Context, _ database.BootstrapNodeManager,
				_ network.ProviderAddresses, _ model.UUID,
				_ corelogger.Logger,
				_ ...database.BootstrapOpt,
			) error {
				return nil
			},
			Logger: loggertesting.WrapCheckLog(c),
		})
		c.Assert(err, tc.ErrorIsNil)

		err = bootstrap.Initialise(c.Context())
		c.Assert(err, tc.ErrorIsNil)

		// The admin user is seeded from the agent config's old password,
		// but no rotation ever happens: SetPassword is never called and
		// the seed password survives Initialize.
		c.Check(agentCfg.apiPasswords, tc.HasLen, 0)
		c.Check(agentCfg.OldPassword(), tc.Equals, "seed-password-0123456789")
		c.Assert(agentCfg.controllerInfos, tc.HasLen, 1)
	}
}
