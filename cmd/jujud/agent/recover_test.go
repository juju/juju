// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type recoverySuite struct {
	testhelpers.IsolationSuite
}

func TestRecoverySuite(t *stdtesting.T) {
	tc.Run(t, &recoverySuite{})
}

var (
	testControllerUUID      = "5798ee67-e167-4b99-9c92-9f2e4bacc9e6"
	testControllerModelUUID = "2dce0e9a-6bcb-4b0d-a15a-e6d2e8072b69"
)

// writeRuntimeConf writes a valid runtime.conf into a fresh data directory
// and returns the directory.
func (s *recoverySuite) writeRuntimeConf(c *tc.C) string {
	dataDir := c.MkDir()
	runtimeCfg := controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       testControllerUUID,
		ControllerModelUUID:  testControllerModelUUID,
		DataDir:              dataDir,
		SocketDir:            filepath.Join(dataDir, "sockets"),
		SharedAgentDir:       filepath.Join(dataDir, "agents"),
		LogDir:               filepath.Join(dataDir, "logs"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert",
		CAPrivateKey:         "ca-private-key",
		ControllerCert:       "controller-cert",
		ControllerPrivateKey: "controller-private-key",
		APIAddresses:         []string{"127.0.0.1:17070"},
	}
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(
		filepath.Join(dataDir, controllerruntimeconfig.Filename), runtimeCfg)
	c.Assert(err, tc.ErrorIsNil)
	return dataDir
}

// writeRecoveryMarker stages the empty recovery params marker into a fresh
// $SNAP_COMMON directory, as jujud init would deliver it.
func (s *recoverySuite) writeRecoveryMarker(c *tc.C) {
	commonDir := c.MkDir()
	marker := filepath.Join(commonDir, agent.RecoveryParamsFile)
	c.Assert(os.MkdirAll(filepath.Dir(marker), 0700), tc.ErrorIsNil)
	c.Assert(os.WriteFile(marker, nil, 0600), tc.ErrorIsNil)
	s.PatchEnvironment("SNAP_COMMON", commonDir)
}

// recoveryParams returns marshalling-valid params for exercising the recovery
// guards without a full bootstrap setup.
func (s *recoverySuite) recoveryParams(
	c *tc.C, recovery bool, cloudType string,
) instancecfg.StateInitializationParams {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	return instancecfg.StateInitializationParams{
		ControllerConfig: jujutesting.FakeControllerConfig(),
		ControllerModelConfig: jujutesting.CustomModelConfig(c, jujutesting.Attrs{
			"name": "controller", "uuid": modelUUID.String(), "type": "lxd",
			"agent-version": jujuversion.Current.String(),
		}),
		ControllerCloud: cloud.Cloud{
			Name: "test-cloud", Type: cloudType, AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		},
		RecoveryMode: recovery,
	}
}

func (s *recoverySuite) writeBootstrapParams(
	c *tc.C, dataDir string, params instancecfg.StateInitializationParams,
) {
	paramsData, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.WriteFile(bootstrap.BootstrapParamsPath(dataDir), paramsData, 0600), tc.ErrorIsNil)
}

func (s *recoverySuite) newRecoverCommand(dataDir string) *RecoverCommand {
	command := NewRecoverCommand()
	command.AgentConf = agentconf.NewAgentConf(dataDir)
	return command
}

func (s *recoverySuite) TestRecoverStateRejectsFreshBootstrapParams(c *tc.C) {
	dataDir := s.writeRuntimeConf(c)
	s.writeBootstrapParams(c, dataDir, s.recoveryParams(c, false, "lxd"))

	err := s.newRecoverCommand(dataDir).Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches, "cannot initialise recovery controller: bootstrap params are not for a recovery allocation")
}

func (s *recoverySuite) TestRecoverStateRequiresRecoveryMarker(c *tc.C) {
	dataDir := s.writeRuntimeConf(c)
	// A markerless $SNAP_COMMON: no recovery allocation happened here.
	s.PatchEnvironment("SNAP_COMMON", c.MkDir())
	s.writeBootstrapParams(c, dataDir, s.recoveryParams(c, true, "lxd"))

	err := s.newRecoverCommand(dataDir).Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches, "cannot initialise recovery controller: controller data directories hold no recovery marker")
}

func (s *recoverySuite) TestRecoverStateRejectsK8sController(c *tc.C) {
	dataDir := s.writeRuntimeConf(c)
	s.writeRecoveryMarker(c)
	s.writeBootstrapParams(c, dataDir, s.recoveryParams(c, true, cloud.CloudTypeKubernetes))

	err := s.newRecoverCommand(dataDir).Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches, "cannot initialise recovery controller: Kubernetes controller recovery not supported")
}

func (s *recoverySuite) TestRecoverStatePersistsIdentityWithoutProvider(c *tc.C) {
	dataDir := s.writeRuntimeConf(c)
	s.writeRecoveryMarker(c)
	s.writeBootstrapParams(c, dataDir, s.recoveryParams(c, true, "lxd"))

	// Recovery never opens the provider: state import is a later stage.
	s.PatchValue(&environsNewIAAS, func(_ context.Context, _ environs.OpenParams, _ environs.CredentialInvalidator) (environs.Environ, error) {
		c.Fatalf("recovery must not open the provider")
		return nil, nil
	})
	s.PatchValue(&environsNewCAAS, func(_ context.Context, _ environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
		c.Fatalf("recovery must not open the provider")
		return nil, nil
	})
	// The replacement gets its own generated system identity.
	s.PatchValue(&sshGenerateKey, func(string) (string, string, error) {
		return "fake-system-identity", "fake-public-key", nil
	})

	err := s.newRecoverCommand(dataDir).Run(cmdtesting.Context(c))
	c.Assert(err, tc.ErrorIsNil)

	// runtime.conf carries the generated identity; the archived material is
	// untouched.
	runtimeCfg, err := controllerruntimeconfig.ReadControllerRuntimeConfig(
		filepath.Join(dataDir, controllerruntimeconfig.Filename))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runtimeCfg.SystemIdentity, tc.Equals, "fake-system-identity")
	c.Check(runtimeCfg.ControllerCert, tc.Equals, "controller-cert")
	c.Check(runtimeCfg.CAPrivateKey, tc.Equals, "ca-private-key")
	c.Check(runtimeCfg.ControllerUUID, tc.Equals, testControllerUUID)

	// The system identity file is written to the data directory.
	identity, err := os.ReadFile(filepath.Join(dataDir, "system-identity"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(identity), tc.Equals, "fake-system-identity")

	// No agent.conf is created for the controller.
	_, err = os.Stat(filepath.Join(dataDir, "agents", "controller-0", "agent.conf"))
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *recoverySuite) TestEngineCreatorWaitsForRecoveryImport(c *tc.C) {
	dataDir := s.writeRuntimeConf(c)
	s.writeRecoveryMarker(c)

	app := &ControllerApplication{controllerRuntimePath: filepath.Join(dataDir, controllerruntimeconfig.Filename)}
	createEngine := app.makeEngineCreator("jujud", semversion.Number{}, nil)
	w, err := createEngine(c.Context())
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "recovery import has not completed")
}

func (s *recoverySuite) TestEngineCreatorRejectsInvalidMarker(c *tc.C) {
	dataDir := s.writeRuntimeConf(c)
	commonDir := c.MkDir()
	c.Assert(os.MkdirAll(filepath.Join(commonDir, "recovery"), 0700), tc.ErrorIsNil)
	// A directory marker is invalid rather than read as absent.
	c.Assert(os.Mkdir(filepath.Join(commonDir, "recovery", "params"), 0700), tc.ErrorIsNil)
	s.PatchEnvironment("SNAP_COMMON", commonDir)

	app := &ControllerApplication{controllerRuntimePath: filepath.Join(dataDir, controllerruntimeconfig.Filename)}
	createEngine := app.makeEngineCreator("jujud", semversion.Number{}, nil)
	w, err := createEngine(c.Context())
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "checking recovery marker: invalid recovery params path .*")
}
