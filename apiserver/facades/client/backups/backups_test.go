// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	backupsAPI "github.com/juju/juju/apiserver/facades/client/backups"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type backupsSuite struct {
	testing.ApiServerSuite
	authorizer *apiservertesting.FakeAuthorizer
	api        *backupsAPI.API
	meta       *backups.Metadata
	machineTag names.MachineTag

	dataDir string
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) agentConfigForTag(c *gc.C, tag names.Tag) agent.ConfigSetterWriter {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	defaultPaths := agent.DefaultPaths
	defaultPaths.DataDir = s.dataDir
	agentConfig, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             defaultPaths,
			Tag:               tag,
			UpgradedToVersion: jujuversion.Current,
			Password:          password,
			CACert:            coretesting.ServerCert,
			Nonce:             "nonce",
			APIAddresses:      s.ControllerModelApiInfo().Addrs,
			Controller:        s.ControllerModel(c).ControllerTag(),
			Model:             names.NewModelTag(s.ControllerModelUUID()),
		})
	c.Assert(err, jc.ErrorIsNil)
	return agentConfig
}

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.dataDir = c.MkDir()
	s.machineTag = names.NewMachineTag("0")

	st := s.ControllerModel(c).State()
	agentConfig := s.agentConfigForTag(c, s.machineTag)
	cfg, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentConfig.SetStateServingInfo(controller.StateServingInfo{
		PrivateKey:   coretesting.ServerKey,
		Cert:         coretesting.ServerCert,
		CAPrivateKey: coretesting.CAKey,
		SharedSecret: "a secret",
		APIPort:      cfg.APIPort(),
		StatePort:    cfg.StatePort(),
	})
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)

	tag := names.NewLocalUserTag("admin")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	shim := &stateShim{
		State:            st,
		Model:            s.ControllerModel(c),
		controllerNodesF: func() ([]state.ControllerNode, error) { return nil, nil },
		machineF:         func(id string) (backupsAPI.Machine, error) { return &testMachine{}, nil },
	}
	s.api, err = backupsAPI.NewAPI(shim, s.authorizer, s.machineTag, s.dataDir, "")
	c.Assert(err, jc.ErrorIsNil)
	s.meta = backupstesting.NewMetadataStarted()
}

func (s *backupsSuite) setBackups(meta *backups.Metadata, err string) *backupstesting.FakeBackups {
	fake := backupstesting.FakeBackups{
		Meta:     meta,
		Filename: "test-filename",
	}
	if meta != nil {
		fake.MetaList = append(fake.MetaList, meta)
	}
	if err != "" {
		fake.Error = errors.Errorf(err)
	}
	s.PatchValue(backupsAPI.NewBackups,
		func(paths *backups.Paths) backups.Backups {
			return &fake
		},
	)
	return &fake
}

func (s *backupsSuite) TestNewAPIOkay(c *gc.C) {
	_, err := backupsAPI.NewAPI(
		&stateShim{State: s.ControllerModel(c).State(), Model: s.ControllerModel(c)},
		s.authorizer, s.machineTag, s.dataDir, "")
	c.Check(err, jc.ErrorIsNil)
}

func (s *backupsSuite) TestNewAPINotAuthorized(c *gc.C) {
	s.authorizer.Tag = names.NewApplicationTag("eggs")
	_, err := backupsAPI.NewAPI(
		&stateShim{State: s.ControllerModel(c).State(), Model: s.ControllerModel(c)},
		s.authorizer, s.machineTag, s.dataDir, "")
	c.Check(errors.Cause(err), gc.Equals, apiservererrors.ErrPerm)
}

func (s *backupsSuite) TestNewAPIHostedEnvironmentFails(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	otherState := f.MakeModel(c, nil)
	defer otherState.Close()
	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = backupsAPI.NewAPI(&stateShim{State: otherState, Model: otherModel}, s.authorizer, s.machineTag, s.dataDir, "")
	c.Check(err, gc.ErrorMatches, "backups are only supported from the controller model\nUse juju switch to select the controller model")
}

func (s *backupsSuite) TestBackupsCAASFails(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	otherState := f.MakeCAASModel(c, nil)
	defer otherState.Close()
	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)

	isController := true
	_, err = backupsAPI.NewAPI(&stateShim{State: otherState, Model: otherModel, isController: &isController}, s.authorizer, s.machineTag, s.dataDir, "")
	c.Assert(err, gc.ErrorMatches, "backups on kubernetes controllers not supported")
}
