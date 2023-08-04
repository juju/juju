// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiapplication "github.com/juju/juju/api/client/application"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type RemoveUnitSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	mockApi            *mocks.MockRemoveApplicationAPI
	mockModelConfigAPI *mocks.MockModelConfigClient

	facadeVersion int

	store *jujuclient.MemStore
}

var _ = gc.Suite(&RemoveUnitSuite{})

func (s *RemoveUnitSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclienttesting.MinimalStore()
	s.facadeVersion = 16
}

func (s *RemoveUnitSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockApi = mocks.NewMockRemoveApplicationAPI(ctrl)
	s.mockApi.EXPECT().BestAPIVersion().Return(s.facadeVersion).AnyTimes()
	s.mockApi.EXPECT().Close()

	s.mockModelConfigAPI = mocks.NewMockModelConfigClient(ctrl)
	// We don't always instantiate this client
	s.mockModelConfigAPI.EXPECT().Close().MaxTimes(1)

	return ctrl
}

func (s *RemoveUnitSuite) runRemoveUnit(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewRemoveUnitCommandForTest(s.mockApi, s.mockModelConfigAPI, s.store), args...)
}

func (s *RemoveUnitSuite) runWithContext(ctx *cmd.Context, args ...string) chan error {
	remove := application.NewRemoveUnitCommandForTest(s.mockApi, s.mockModelConfigAPI, s.store)
	return cmdtesting.RunCommandWithContext(ctx, remove, args...)
}

func (s *RemoveUnitSuite) TestRemoveUnit(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(apiapplication.DestroyUnitsParams{
		Units: []string{"unit/0", "unit/1", "unit/2"},
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-0"}}},
	}, {
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-1"}}},
	}, {
		Error: apiservererrors.ServerError(errors.New("doink")),
	}}, nil)

	ctx, err := s.runRemoveUnit(c, "--no-prompt", "unit/0", "unit/1", "unit/2")
	c.Assert(err, gc.Equals, cmd.ErrSilent)

	stdout := cmdtesting.Stdout(ctx)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stdout, gc.Equals, `
will remove unit unit/0
- will detach storage data/0
will remove unit unit/1
- will detach storage data/1
`[1:])
	c.Assert(stderr, gc.Equals, `
ERROR removing unit unit/2 failed: doink
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitDestroyStorage(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(apiapplication.DestroyUnitsParams{
		Units:          []string{"unit/0", "unit/1", "unit/2"},
		DestroyStorage: true,
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{DestroyedStorage: []params.Entity{{Tag: "storage-data-0"}}},
	}, {
		Info: &params.DestroyUnitInfo{DestroyedStorage: []params.Entity{{Tag: "storage-data-1"}}},
	}, {
		Error: apiservererrors.ServerError(errors.New("doink")),
	}}, nil)

	ctx, err := s.runRemoveUnit(c, "--no-prompt", "unit/0", "unit/1", "unit/2", "--destroy-storage")
	c.Assert(err, gc.Equals, cmd.ErrSilent)

	stdout := cmdtesting.Stdout(ctx)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stdout, gc.Equals, `
will remove unit unit/0
- will remove storage data/0
will remove unit unit/1
- will remove storage data/1
`[1:])
	c.Assert(stderr, gc.Equals, `
ERROR removing unit unit/2 failed: doink
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitNoWaitWithoutForce(c *gc.C) {
	_, err := s.runRemoveUnit(c, "unit/0", "--no-wait")
	c.Assert(err, gc.ErrorMatches, `--no-wait without --force not valid`)
}

func (s *RemoveUnitSuite) TestBlockRemoveUnit(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(apiapplication.DestroyUnitsParams{
		Units: []string{"some-unit-name/0"},
	}).Return(nil, apiservererrors.OperationBlockedError("TestBlockRemoveUnit"))

	s.runRemoveUnit(c, "--no-prompt", "some-unit-name/0")

	c.Check(c.GetTestLog(), gc.Matches, "(?s).*TestBlockRemoveUnit.*")
}

func (s *RemoveUnitSuite) TestRemoveUnitDryRun(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(apiapplication.DestroyUnitsParams{
		Units:  []string{"unit/0", "unit/1"},
		DryRun: true,
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-0"}}},
	}, {
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-1"}}},
	}}, nil)

	ctx, err := s.runRemoveUnit(c, "--dry-run", "unit/0", "unit/1")
	c.Assert(err, jc.ErrorIsNil)

	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, gc.Equals, `
will remove unit unit/0
- will detach storage data/0
will remove unit unit/1
- will detach storage data/1
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitDryRunOldFacade(c *gc.C) {
	s.facadeVersion = 15
	defer s.setup(c).Finish()

	_, err := s.runRemoveUnit(c, "--dry-run", "unit/0", "unit/1")
	c.Assert(err, gc.ErrorMatches, "Your controller does not support `--dry-run`")
}

func (s *RemoveUnitSuite) TestRemoveUnitWithPrompt(c *gc.C) {
	defer s.setup(c).Finish()

	var stdin bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdin = &stdin

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: config.RequiresPromptsMode})
	s.mockModelConfigAPI.EXPECT().ModelGet().Return(attrs, nil)

	s.mockApi.EXPECT().DestroyUnits(apiapplication.DestroyUnitsParams{
		Units:  []string{"unit/0"},
		DryRun: true,
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{},
	}}, nil)
	s.mockApi.EXPECT().DestroyUnits(apiapplication.DestroyUnitsParams{
		Units: []string{"unit/0"},
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{},
	}}, nil)

	stdin.WriteString("y")
	errc := s.runWithContext(ctx, "unit/0")

	select {
	case err := <-errc:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}

	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, `
(?s)will remove unit unit/0
.*`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitWithPromptOldFacade(c *gc.C) {
	s.facadeVersion = 15
	defer s.setup(c).Finish()

	var stdin bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdin = &stdin

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: config.RequiresPromptsMode})
	s.mockModelConfigAPI.EXPECT().ModelGet().Return(attrs, nil)

	s.mockApi.EXPECT().DestroyUnits(apiapplication.DestroyUnitsParams{
		Units: []string{"unit/0"},
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{},
	}}, nil)

	stdin.WriteString("y")
	errc := s.runWithContext(ctx, "unit/0")

	select {
	case err := <-errc:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}

	c.Assert(c.GetTestLog(), gc.Matches, `(?s).*Your controller does not support dry runs.*`)
}

func (s *RemoveUnitSuite) setCaasModel() {
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m
}

func (s *RemoveUnitSuite) TestCAASRemoveUnit(c *gc.C) {
	defer s.setup(c).Finish()

	s.setCaasModel()
	s.mockApi.EXPECT().ScaleApplication(apiapplication.ScaleApplicationParams{
		ApplicationName: "some-application-name",
		ScaleChange:     -2,
	}).Return(params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 3},
	}, nil)

	ctx, err := s.runRemoveUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, jc.ErrorIsNil)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
scaling down to 3 units
`[1:])
}

func (s *RemoveUnitSuite) TestCAASRemoveUnitNotSupported(c *gc.C) {
	defer s.setup(c).Finish()

	s.setCaasModel()
	s.mockApi.EXPECT().ScaleApplication(apiapplication.ScaleApplicationParams{
		ApplicationName: "some-application-name",
		ScaleChange:     -2,
	}).Return(params.ScaleApplicationResult{}, apiservererrors.ServerError(errors.NotSupportedf(`scale a "daemon" charm`)))

	_, err := s.runRemoveUnit(c, "some-application-name", "--num-units", "2")

	c.Assert(err, gc.ErrorMatches, `can not remove unit: scale a "daemon" charm not supported`)
}

func (s *RemoveUnitSuite) TestCAASAllowsNumUnitsOnly(c *gc.C) {
	s.setCaasModel()

	_, err := s.runRemoveUnit(c, "some-application-name")
	c.Assert(err, gc.ErrorMatches, `specify the number of units \(> 0\) to remove using --num-units`)

	_, err = s.runRemoveUnit(c)
	c.Assert(err, gc.ErrorMatches, `no application specified`)

	_, err = s.runRemoveUnit(c, "some-application-name", "--destroy-storage")
	c.Assert(err, gc.ErrorMatches, "k8s models only support --num-units")

	_, err = s.runRemoveUnit(c, "some-application-name/0")
	c.Assert(err, gc.ErrorMatches, "(?s)k8s models do not support removing named units.*")

	_, err = s.runRemoveUnit(c, "some-application-name-", "--num-units", "2")
	c.Assert(err, gc.ErrorMatches, "application name \"some-application-name-\" not valid")

	_, err = s.runRemoveUnit(c, "some-application-name", "another-application", "--num-units", "2")
	c.Assert(err, gc.ErrorMatches, "only single application supported")

	_, err = s.runRemoveUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, jc.ErrorIsNil)
}
