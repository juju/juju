// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	backupsAPI "github.com/juju/juju/apiserver/facades/client/backups"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
	"github.com/juju/juju/testing/factory"
)

type backupsSuite struct {
	testing.JujuConnSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *backupsAPI.APIv2
	meta       *backups.Metadata
	machineTag names.MachineTag
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.machineTag = names.NewMachineTag("0")
	s.resources = common.NewResources()
	s.resources.RegisterNamed("dataDir", common.StringResource(s.DataDir()))
	s.resources.RegisterNamed("machineID", common.StringResource(s.machineTag.Id()))

	ssInfo, err := s.State.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	agentConfig := s.AgentConfigForTag(c, s.machineTag)
	agentConfig.SetStateServingInfo(params.StateServingInfo{
		PrivateKey:   ssInfo.PrivateKey,
		Cert:         ssInfo.Cert,
		CAPrivateKey: ssInfo.CAPrivateKey,
		SharedSecret: ssInfo.SharedSecret,
		APIPort:      ssInfo.APIPort,
		StatePort:    ssInfo.StatePort,
	})
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)

	tag := names.NewLocalUserTag("admin")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	s.api, err = backupsAPI.NewAPIv2(&stateShim{s.State, s.IAASModel.Model}, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.meta = backupstesting.NewMetadataStarted()
}

func (s *backupsSuite) setBackups(c *gc.C, meta *backups.Metadata, err string) *backupstesting.FakeBackups {
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
		func(backupsAPI.Backend) (backups.Backups, io.Closer) {
			return &fake, ioutil.NopCloser(nil)
		},
	)
	return &fake
}

func (s *backupsSuite) TestNewAPIOkay(c *gc.C) {
	_, err := backupsAPI.NewAPIv2(&stateShim{s.State, s.IAASModel.Model}, s.resources, s.authorizer)
	c.Check(err, jc.ErrorIsNil)
}

func (s *backupsSuite) TestNewAPINotAuthorized(c *gc.C) {
	s.authorizer.Tag = names.NewApplicationTag("eggs")
	_, err := backupsAPI.NewAPIv2(&stateShim{s.State, s.IAASModel.Model}, s.resources, s.authorizer)
	c.Check(errors.Cause(err), gc.Equals, common.ErrPerm)
}

func (s *backupsSuite) TestNewAPIHostedEnvironmentFails(c *gc.C) {
	otherState := factory.NewFactory(s.State).MakeModel(c, nil)
	defer otherState.Close()
	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = backupsAPI.NewAPIv2(&stateShim{otherState, otherModel}, s.resources, s.authorizer)
	c.Check(err, gc.ErrorMatches, "backups are only supported from the controller model\nUse juju switch to select the controller model")
}
