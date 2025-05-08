// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"bytes"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiapplication "github.com/juju/juju/api/client/application"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type RemoveUnitSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	mockApi            *mocks.MockRemoveApplicationAPI
	mockModelConfigAPI *mocks.MockModelConfigClient

	facadeVersion int

	store *jujuclient.MemStore
}

var _ = tc.Suite(&RemoveUnitSuite{})

func (s *RemoveUnitSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclienttesting.MinimalStore()
	s.facadeVersion = 16
}

func (s *RemoveUnitSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockApi = mocks.NewMockRemoveApplicationAPI(ctrl)
	s.mockApi.EXPECT().BestAPIVersion().Return(s.facadeVersion).AnyTimes()
	s.mockApi.EXPECT().Close()

	s.mockModelConfigAPI = mocks.NewMockModelConfigClient(ctrl)
	// We don't always instantiate this client
	s.mockModelConfigAPI.EXPECT().Close().MaxTimes(1)

	return ctrl
}

func (s *RemoveUnitSuite) runRemoveUnit(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewRemoveUnitCommandForTest(s.mockApi, s.mockModelConfigAPI, s.store), args...)
}

func (s *RemoveUnitSuite) runWithContext(ctx *cmd.Context, args ...string) chan error {
	remove := application.NewRemoveUnitCommandForTest(s.mockApi, s.mockModelConfigAPI, s.store)
	return cmdtesting.RunCommandWithContext(ctx, remove, args...)
}

func (s *RemoveUnitSuite) TestRemoveUnit(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(gomock.Any(), apiapplication.DestroyUnitsParams{
		Units: []string{"unit/0", "unit/1", "unit/2"},
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-0"}}},
	}, {
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-1"}}},
	}, {
		Error: apiservererrors.ServerError(errors.New("doink")),
	}}, nil)

	ctx, err := s.runRemoveUnit(c, "--no-prompt", "unit/0", "unit/1", "unit/2")
	c.Assert(err, tc.Equals, cmd.ErrSilent)

	stdout := cmdtesting.Stdout(ctx)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stdout, tc.Equals, `
will remove unit unit/0
- will detach storage data/0
will remove unit unit/1
- will detach storage data/1
`[1:])
	c.Assert(stderr, tc.Equals, `
ERROR removing unit unit/2 failed: doink
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitDestroyStorage(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(gomock.Any(), apiapplication.DestroyUnitsParams{
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
	c.Assert(err, tc.Equals, cmd.ErrSilent)

	stdout := cmdtesting.Stdout(ctx)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stdout, tc.Equals, `
will remove unit unit/0
- will remove storage data/0
will remove unit unit/1
- will remove storage data/1
`[1:])
	c.Assert(stderr, tc.Equals, `
ERROR removing unit unit/2 failed: doink
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitNoWaitWithoutForce(c *tc.C) {
	_, err := s.runRemoveUnit(c, "unit/0", "--no-wait")
	c.Assert(err, tc.ErrorMatches, `--no-wait without --force not valid`)
}

func (s *RemoveUnitSuite) TestBlockRemoveUnit(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(gomock.Any(), apiapplication.DestroyUnitsParams{
		Units: []string{"some-unit-name/0"},
	}).Return(nil, apiservererrors.OperationBlockedError("TestBlockRemoveUnit"))

	s.runRemoveUnit(c, "--no-prompt", "some-unit-name/0")
}

func (s *RemoveUnitSuite) TestRemoveUnitDryRun(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyUnits(gomock.Any(), apiapplication.DestroyUnitsParams{
		Units:  []string{"unit/0", "unit/1"},
		DryRun: true,
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-0"}}},
	}, {
		Info: &params.DestroyUnitInfo{DetachedStorage: []params.Entity{{Tag: "storage-data-1"}}},
	}}, nil)

	ctx, err := s.runRemoveUnit(c, "--dry-run", "unit/0", "unit/1")
	c.Assert(err, tc.ErrorIsNil)

	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, tc.Equals, `
will remove unit unit/0
- will detach storage data/0
will remove unit unit/1
- will detach storage data/1
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitDryRunOldFacade(c *tc.C) {
	s.facadeVersion = 15
	defer s.setup(c).Finish()

	_, err := s.runRemoveUnit(c, "--dry-run", "unit/0", "unit/1")
	c.Assert(err, tc.ErrorMatches, "Your controller does not support `--dry-run`")
}

func (s *RemoveUnitSuite) TestRemoveUnitWithPrompt(c *tc.C) {
	defer s.setup(c).Finish()

	var stdin bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdin = &stdin

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: config.RequiresPromptsMode})
	s.mockModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(attrs, nil)

	s.mockApi.EXPECT().DestroyUnits(gomock.Any(), apiapplication.DestroyUnitsParams{
		Units:  []string{"unit/0"},
		DryRun: true,
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{},
	}}, nil)
	s.mockApi.EXPECT().DestroyUnits(gomock.Any(), apiapplication.DestroyUnitsParams{
		Units: []string{"unit/0"},
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{},
	}}, nil)

	stdin.WriteString("y")
	errc := s.runWithContext(ctx, "unit/0")

	select {
	case err := <-errc:
		c.Check(err, tc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}

	c.Assert(cmdtesting.Stdout(ctx), tc.Matches, `
(?s)will remove unit unit/0
.*`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitWithPromptOldFacade(c *tc.C) {
	s.facadeVersion = 15
	defer s.setup(c).Finish()

	var stdin bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdin = &stdin

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: config.RequiresPromptsMode})
	s.mockModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(attrs, nil)

	s.mockApi.EXPECT().DestroyUnits(gomock.Any(), apiapplication.DestroyUnitsParams{
		Units: []string{"unit/0"},
	}).Return([]params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{},
	}}, nil)

	stdin.WriteString("y")
	errc := s.runWithContext(ctx, "unit/0")

	select {
	case err := <-errc:
		c.Check(err, tc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}
}

func (s *RemoveUnitSuite) setCaasModel() {
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m
}

func (s *RemoveUnitSuite) TestCAASRemoveUnit(c *tc.C) {
	defer s.setup(c).Finish()

	s.setCaasModel()
	s.mockApi.EXPECT().ScaleApplication(gomock.Any(), apiapplication.ScaleApplicationParams{
		ApplicationName: "some-application-name",
		ScaleChange:     -2,
	}).Return(params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 3},
	}, nil)

	ctx, err := s.runRemoveUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, tc.ErrorIsNil)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, tc.Equals, `
scaling down to 3 units
`[1:])
}

func (s *RemoveUnitSuite) TestCAASRemoveUnitNotSupported(c *tc.C) {
	defer s.setup(c).Finish()

	s.setCaasModel()
	s.mockApi.EXPECT().ScaleApplication(gomock.Any(), apiapplication.ScaleApplicationParams{
		ApplicationName: "some-application-name",
		ScaleChange:     -2,
	}).Return(params.ScaleApplicationResult{}, apiservererrors.ServerError(errors.NotSupportedf(`scale a "daemon" charm`)))

	_, err := s.runRemoveUnit(c, "some-application-name", "--num-units", "2")

	c.Assert(err, tc.ErrorMatches, `can not remove unit: scale a "daemon" charm not supported`)
}

func (s *RemoveUnitSuite) TestCAASAllowsNumUnitsOnly(c *tc.C) {
	s.setCaasModel()

	_, err := s.runRemoveUnit(c, "some-application-name")
	c.Assert(err, tc.ErrorMatches, `specify the number of units \(> 0\) to remove using --num-units`)

	_, err = s.runRemoveUnit(c)
	c.Assert(err, tc.ErrorMatches, `no application specified`)

	_, err = s.runRemoveUnit(c, "some-application-name", "--destroy-storage")
	c.Assert(err, tc.ErrorMatches, "k8s models only support --num-units")

	_, err = s.runRemoveUnit(c, "some-application-name/0")
	c.Assert(err, tc.ErrorMatches, "(?s)k8s models do not support removing named units.*")

	_, err = s.runRemoveUnit(c, "some-application-name-", "--num-units", "2")
	c.Assert(err, tc.ErrorMatches, "application name \"some-application-name-\" not valid")

	_, err = s.runRemoveUnit(c, "some-application-name", "another-application", "--num-units", "2")
	c.Assert(err, tc.ErrorMatches, "only single application supported")

	_, err = s.runRemoveUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, tc.ErrorIsNil)
}
