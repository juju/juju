// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type freshControllerSuite struct {
	testhelpers.IsolationSuite
}

func TestFreshControllerSuite(t *testing.T) {
	tc.Run(t, &freshControllerSuite{})
}

func (s *freshControllerSuite) TestFreshIAASController(c *tc.C) {
	s.assertFreshController(c, false, nil, false, agent.BootstrapControllerId)
}

func (s *freshControllerSuite) TestFreshK8sController(c *tc.C) {
	s.assertFreshController(c, true, nil, false, agent.BootstrapControllerId)
}

func (s *freshControllerSuite) TestDatabaseFailureDoesNotPersistNewPassword(c *tc.C) {
	s.assertFreshController(c, false, errors.New("database failed"), false, agent.BootstrapControllerId)
}

func (s *freshControllerSuite) TestK8sTemplateCopyCanBeRetried(c *tc.C) {
	dataDir := c.MkDir()
	tag := names.NewControllerAgentTag(agent.BootstrapControllerId)
	dir := agent.Dir(dataDir, tag)
	c.Assert(os.MkdirAll(dir, 0700), tc.ErrorIsNil)
	c.Assert(ensureConfigFilesForK8s(dataDir), tc.ErrorIs, os.ErrNotExist)
	configPath := agent.ConfigPath(dataDir, tag)
	_, err := os.Stat(configPath)
	c.Check(err, tc.ErrorIs, os.ErrNotExist)

	template := filepath.Join(dir, k8sconstants.TemplateFileNameAgentConf)
	c.Assert(os.WriteFile(template, []byte("controller config"), 0600), tc.ErrorIsNil)
	c.Assert(ensureConfigFilesForK8s(dataDir), tc.ErrorIsNil)
	data, err := os.ReadFile(configPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "controller config")
}

func (s *freshControllerSuite) TestRecoverySkipsDqliteInitialisation(c *tc.C) {
	s.assertFreshController(c, false, nil, true, agent.BootstrapControllerId)
}

func (s *freshControllerSuite) TestRecoveryUsesArchivedMachineId(c *tc.C) {
	s.assertFreshController(c, false, nil, true, "7")
}

func (s *freshControllerSuite) TestRecoveryK8sController(c *tc.C) {
	s.assertFreshController(c, true, nil, true, agent.BootstrapControllerId)
}

func (s *freshControllerSuite) TestRecoverRejectsFreshBootstrapParams(c *tc.C) {
	dataDir := c.MkDir()
	writeRecoveryMarker(c, dataDir)
	params := recoveryGuardTestParams(c, false)
	paramsData, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams), paramsData, 0600), tc.ErrorIsNil)

	command := NewRecoverCommand()
	command.AgentConf = agentconf.NewAgentConf(dataDir)
	err = command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches, "bootstrap params are not for a recovery allocation")
}

func (s *freshControllerSuite) TestRecoverRequiresRecoveryMarker(c *tc.C) {
	dataDir := c.MkDir()
	params := recoveryGuardTestParams(c, true)
	paramsData, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams), paramsData, 0600), tc.ErrorIsNil)

	command := NewRecoverCommand()
	command.AgentConf = agentconf.NewAgentConf(dataDir)
	err = command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches, "agent data directory holds no recovery marker")
}

// writeRecoveryMarker mirrors the cloud-init command that creates the
// recovery marker immediately before recover-state is invoked.
func writeRecoveryMarker(c *tc.C, dataDir string) {
	marker := filepath.Join(dataDir, agent.RecoveryParamsFile)
	c.Assert(os.MkdirAll(filepath.Dir(marker), 0700), tc.ErrorIsNil)
	c.Assert(os.WriteFile(marker, nil, 0600), tc.ErrorIsNil)
}

// recoveryGuardTestParams returns marshalling-valid params for exercising
// the recovery guards without a full bootstrap setup.
func recoveryGuardTestParams(c *tc.C, recovery bool) instancecfg.StateInitializationParams {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	return instancecfg.StateInitializationParams{
		ControllerConfig: jujutesting.FakeControllerConfig(),
		ControllerModelConfig: jujutesting.CustomModelConfig(c, jujutesting.Attrs{
			"name": "controller", "uuid": modelUUID.String(), "type": "dummy",
			"agent-version": jujuversion.Current.String(),
		}),
		ControllerCloud: cloud.Cloud{
			Name: "test-cloud", Type: "dummy", AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		},
		RecoveryMode: recovery,
	}
}

func (s *freshControllerSuite) assertFreshController(
	c *tc.C, isK8s bool, databaseErr error, skipDqlite bool, machineId string,
) {
	dataDir := c.MkDir()
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	cloudType := "dummy"
	tag := names.Tag(names.NewMachineTag(machineId))
	if isK8s {
		cloudType = cloud.CloudTypeKubernetes
		tag = names.NewControllerAgentTag(machineId)
	}
	modelConfig := jujutesting.CustomModelConfig(c, jujutesting.Attrs{
		"name": "controller", "uuid": modelUUID.String(), "type": cloudType,
		"agent-version": jujuversion.Current.String(),
	})
	params := instancecfg.StateInitializationParams{
		ControllerConfig:      jujutesting.FakeControllerConfig(),
		ControllerModelConfig: modelConfig,
		ControllerCloud: cloud.Cloud{
			Name: "test-cloud", Type: cloudType, AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		},
		RecoveryMode: skipDqlite,
	}
	paramsData, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams), paramsData, 0600)
	c.Assert(err, tc.ErrorIsNil)
	if skipDqlite {
		// cloud-init writes the recovery marker before invoking recover-state.
		writeRecoveryMarker(c, dataDir)
	}

	cfg, err := agent.NewStateMachineConfig(agent.AgentConfigParams{
		Paths: agent.Paths{DataDir: dataDir}, Tag: tag,
		UpgradedToVersion: jujuversion.Current, Password: "bootstrap-secret",
		Controller: jujutesting.ControllerTag, Model: names.NewModelTag(modelUUID.String()),
		APIAddresses: []string{"127.0.0.1:17070"}, CACert: jujutesting.CACert,
	}, controller.ControllerAgentInfo{
		Cert: jujutesting.ServerCert, PrivateKey: jujutesting.ServerKey,
		CAPrivateKey: jujutesting.CAKey, APIPort: 17070, SystemIdentity: "provided-system-identity",
	})
	c.Assert(err, tc.ErrorIsNil)
	cfg.SetPassword("agent-before")
	if isK8s {
		// Both fresh bootstrap and recovery must create agent.conf from its
		// Kubernetes template.
		template := filepath.Join(
			agent.Dir(dataDir, names.NewControllerAgentTag(agent.BootstrapControllerId)),
			k8sconstants.TemplateFileNameAgentConf,
		)
		err = os.MkdirAll(filepath.Dir(template), 0700)
		c.Assert(err, tc.ErrorIsNil)
		data, err := cfg.Render()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(os.WriteFile(template, data, 0600), tc.ErrorIsNil)
	} else {
		c.Assert(cfg.Write(), tc.ErrorIsNil)
	}

	addresses := network.NewMachineAddresses([]string{"10.0.0.1"}).AsProviderAddresses()
	checkOpen := func(args environs.OpenParams) {
		c.Check(args.Cloud.IsControllerCloud, tc.IsTrue)
		c.Check(args.ControllerUUID, tc.Equals, jujutesting.ControllerTag.Id())
	}
	if skipDqlite {
		// Recovery never opens the provider: state import is a later stage.
		s.PatchValue(&environsNewIAAS, func(_ context.Context, _ environs.OpenParams, _ environs.CredentialInvalidator) (environs.Environ, error) {
			c.Fatalf("recovery must not open the provider")
			return nil, nil
		})
		s.PatchValue(&environsNewK8s, func(_ context.Context, _ environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
			c.Fatalf("recovery must not open the provider")
			return nil, nil
		})
	} else {
		s.PatchValue(&environsNewIAAS, func(_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator) (environs.Environ, error) {
			c.Check(isK8s, tc.IsFalse)
			checkOpen(args)
			return &freshBootstrapEnviron{cfg: modelConfig, addresses: addresses}, nil
		})
		s.PatchValue(&environsNewK8s, func(_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
			c.Check(isK8s, tc.IsTrue)
			checkOpen(args)
			return &freshBootstrapBroker{cfg: modelConfig, addresses: addresses}, nil
		})
	}
	s.PatchValue(&sshGenerateKey, func(string) (string, string, error) {
		c.Check(isK8s, tc.IsFalse)
		return "fresh-system-identity", "fresh-public-key", nil
	})
	expectedIdentity := "fresh-system-identity"
	if isK8s {
		expectedIdentity = "provided-system-identity"
	}

	databaseCalled := false
	if skipDqlite {
		command := NewRecoverCommand()
		command.AgentConf = agentconf.NewAgentConf(dataDir)
		command.MachineId = machineId
		err = command.Run(cmdtesting.Context(c))
		c.Assert(err, tc.ErrorIsNil)
		if isK8s {
			// A pod retry must also repair a truncated legacy template copy.
			c.Assert(os.WriteFile(agent.ConfigPath(dataDir, tag), nil, 0600), tc.ErrorIsNil)
			c.Assert(command.Run(cmdtesting.Context(c)), tc.ErrorIsNil)
		}
	} else {
		command := NewBootstrapCommand()
		command.AgentConf = agentconf.NewAgentConf(dataDir)
		const bindAddress = "192.168.1.10"
		command.DqliteBindAddressResolver = func(
			_ context.Context, gotAddresses network.ProviderAddresses,
		) (string, error) {
			c.Check(gotAddresses, tc.DeepEquals, addresses)
			return bindAddress, nil
		}
		command.BootstrapAgent = func(args agentbootstrap.AgentBootstrapArgs) (*agentbootstrap.AgentBootstrap, error) {
			c.Check(args.StateInitialisationParams.SSHServerHostKey, tc.Not(tc.Equals), "")
			c.Check(args.BootstrapMachineAddresses, tc.DeepEquals, addresses)
			return agentbootstrap.NewAgentBootstrap(args)
		}
		command.DqliteInitialiser = func(_ context.Context, _ database.BootstrapNodeManager, gotBindAddress string, gotModelUUID coremodel.UUID, _ corelogger.Logger, _ ...database.BootstrapOpt) error {
			databaseCalled = true
			c.Check(gotBindAddress, tc.Equals, bindAddress)
			c.Check(gotModelUUID, tc.Equals, modelUUID)
			// Identity is persisted before database work; the new password is not.
			identity, err := os.ReadFile(cfg.SystemIdentityPath())
			c.Assert(err, tc.ErrorIsNil)
			c.Check(string(identity), tc.Equals, expectedIdentity)
			stored, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
			c.Assert(err, tc.ErrorIsNil)
			info, ok := stored.ControllerAgentInfo()
			c.Assert(ok, tc.IsTrue)
			c.Check(info.SystemIdentity, tc.Equals, expectedIdentity)
			apiInfo, ok := stored.APIInfo()
			c.Assert(ok, tc.IsTrue)
			c.Check(apiInfo.Password, tc.Equals, "agent-before")
			return databaseErr
		}

		err = command.Run(cmdtesting.Context(c))
		if databaseErr == nil {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorIs, databaseErr)
		}
		c.Check(databaseCalled, tc.IsTrue)
	}

	stored, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, tc.ErrorIsNil)
	info, ok := stored.ControllerAgentInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(info.SystemIdentity, tc.Equals, expectedIdentity)
	identity, err := os.ReadFile(stored.SystemIdentityPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(identity), tc.Equals, expectedIdentity)
	apiInfo, ok := stored.APIInfo()
	c.Assert(ok, tc.IsTrue)
	switch {
	case skipDqlite:
		// Recovery persists the archived identity but never touches the
		// bootstrap password.
		c.Check(apiInfo.Password, tc.Equals, "agent-before")
		c.Check(databaseCalled, tc.IsFalse)
	case databaseErr == nil:
		c.Check(apiInfo.Password, tc.Not(tc.Equals), "agent-before")
		c.Check(apiInfo.Password, tc.Not(tc.Equals), "")
	default:
		c.Check(apiInfo.Password, tc.Equals, "agent-before")
	}
}

type freshBootstrapEnviron struct {
	environs.Environ
	cfg       *config.Config
	addresses network.ProviderAddresses
}

func (e *freshBootstrapEnviron) Config() *config.Config { return e.cfg }
func (e *freshBootstrapEnviron) BootstrapControllerAddresses(context.Context) (network.ProviderAddresses, error) {
	return e.addresses, nil
}

type freshBootstrapBroker struct {
	caas.Broker
	cfg       *config.Config
	addresses network.ProviderAddresses
}

func (e *freshBootstrapBroker) Config() *config.Config { return e.cfg }
func (e *freshBootstrapBroker) BootstrapControllerAddresses(context.Context) (network.ProviderAddresses, error) {
	return e.addresses, nil
}
