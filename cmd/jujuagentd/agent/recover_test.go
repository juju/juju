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
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type recoverSuite struct {
	testhelpers.IsolationSuite
}

func TestRecoverSuite(t *testing.T) {
	tc.Run(t, &recoverSuite{})
}

func (s *recoverSuite) TestK8sTemplateCopyCanBeRetried(c *tc.C) {
	dataDir := c.MkDir()
	tag := names.NewControllerAgentTag(agent.BootstrapControllerId)
	dir := agent.Dir(dataDir, tag)
	c.Assert(os.MkdirAll(dir, 0700), tc.ErrorIsNil)
	configPath := agent.ConfigPath(dataDir, tag)
	c.Assert(copyK8sControllerConfigTemplate(configPath), tc.ErrorIs, os.ErrNotExist)
	_, err := os.Stat(configPath)
	c.Check(err, tc.ErrorIs, os.ErrNotExist)

	template := filepath.Join(dir, k8sconstants.TemplateFileNameAgentConf)
	c.Assert(os.WriteFile(template, []byte("controller config"), 0600), tc.ErrorIsNil)
	c.Assert(copyK8sControllerConfigTemplate(configPath), tc.ErrorIsNil)
	data, err := os.ReadFile(configPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "controller config")
}

func (s *recoverSuite) TestRecoverySkipsDqliteInitialisation(c *tc.C) {
	s.assertRecoveryController(c, false, agent.BootstrapControllerId)
}

func (s *recoverSuite) TestRecoveryUsesArchivedMachineId(c *tc.C) {
	s.assertRecoveryController(c, false, "7")
}

func (s *recoverSuite) TestRecoveryK8sController(c *tc.C) {
	s.assertRecoveryController(c, true, agent.BootstrapControllerId)
}

func (s *recoverSuite) TestRecoverRejectsFreshBootstrapParams(c *tc.C) {
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

func (s *recoverSuite) TestRecoverRequiresRecoveryMarker(c *tc.C) {
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

func (s *recoverSuite) assertRecoveryController(c *tc.C, isK8s bool, machineId string) {
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
	if isK8s {
		// Recovery must create agent.conf from its Kubernetes template.
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

	s.PatchValue(&sshGenerateKey, func(string) (string, string, error) {
		c.Check(isK8s, tc.IsFalse)
		return "fresh-system-identity", "fresh-public-key", nil
	})
	expectedIdentity := "fresh-system-identity"
	if isK8s {
		expectedIdentity = "provided-system-identity"
	}

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
	// Recovery persists the archived identity but never touches the
	// bootstrap password.
	c.Check(apiInfo.Password, tc.Equals, "agent-before")
}
