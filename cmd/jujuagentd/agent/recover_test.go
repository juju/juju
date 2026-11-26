// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type recoverControllerSuite struct {
	testhelpers.IsolationSuite
}

func TestRecoverControllerSuite(t *testing.T) {
	tc.Run(t, &recoverControllerSuite{})
}

func (s *recoverControllerSuite) TestRecoverySkipsDqliteInitialisation(c *tc.C) {
	s.assertRecoveryController(c, agent.BootstrapControllerId)
}

func (s *recoverControllerSuite) TestRecoveryUsesArchivedMachineId(c *tc.C) {
	s.assertRecoveryController(c, "7")
}

func (s *recoverControllerSuite) TestRecoveryK8sControllerNotSupported(c *tc.C) {
	dataDir := c.MkDir()
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	modelConfig := jujutesting.CustomModelConfig(c, jujutesting.Attrs{
		"name": "controller", "uuid": modelUUID.String(), "type": cloud.CloudTypeKubernetes,
		"agent-version": jujuversion.Current.String(),
	})
	params := instancecfg.StateInitializationParams{
		ControllerConfig:      jujutesting.FakeControllerConfig(),
		ControllerModelConfig: modelConfig,
		ControllerCloud: cloud.Cloud{
			Name: "test-cloud", Type: cloud.CloudTypeKubernetes, AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		},
		RecoveryMode: true,
	}
	paramsData, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams), paramsData, 0600), tc.ErrorIsNil)
	writeRecoveryMarker(c, dataDir)

	command := NewRecoverCommand()
	command.AgentConf = agentconf.NewAgentConf(dataDir)
	err = command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *recoverControllerSuite) TestRecoverRejectsFreshBootstrapParams(c *tc.C) {
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

func (s *recoverControllerSuite) TestRecoverRequiresRecoveryMarker(c *tc.C) {
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

// assertRecoveryController runs recover-state for the given archived machine
// ID against a pre-written agent config and checks that the replacement
// controller persists a fresh system identity without touching the bootstrap
// password.
func (s *recoverControllerSuite) assertRecoveryController(c *tc.C, machineId string) {
	dataDir := c.MkDir()
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	tag := names.Tag(names.NewMachineTag(machineId))
	modelConfig := jujutesting.CustomModelConfig(c, jujutesting.Attrs{
		"name": "controller", "uuid": modelUUID.String(), "type": "dummy",
		"agent-version": jujuversion.Current.String(),
	})
	params := instancecfg.StateInitializationParams{
		ControllerConfig:      jujutesting.FakeControllerConfig(),
		ControllerModelConfig: modelConfig,
		ControllerCloud: cloud.Cloud{
			Name: "test-cloud", Type: "dummy", AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		},
		RecoveryMode: true,
	}
	paramsData, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams), paramsData, 0600)
	c.Assert(err, tc.ErrorIsNil)
	// cloud-init writes the recovery marker before invoking recover-state.
	writeRecoveryMarker(c, dataDir)

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
	c.Assert(cfg.Write(), tc.ErrorIsNil)

	s.PatchValue(&sshGenerateKey, func(string) (string, string, error) {
		return "fresh-system-identity", "fresh-public-key", nil
	})

	command := NewRecoverCommand()
	command.AgentConf = agentconf.NewAgentConf(dataDir)
	command.MachineId = machineId
	err = command.Run(cmdtesting.Context(c))
	c.Assert(err, tc.ErrorIsNil)

	stored, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, tc.ErrorIsNil)
	info, ok := stored.ControllerAgentInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(info.SystemIdentity, tc.Equals, "fresh-system-identity")
	identity, err := os.ReadFile(stored.SystemIdentityPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(identity), tc.Equals, "fresh-system-identity")
	apiInfo, ok := stored.APIInfo()
	c.Assert(ok, tc.IsTrue)
	// Recovery persists the fresh identity but never touches the
	// bootstrap password.
	c.Check(apiInfo.Password, tc.Equals, "agent-before")
}
