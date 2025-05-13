// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiapplication "github.com/juju/juju/api/client/application"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type removeApplicationSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	mockApi            *mocks.MockRemoveApplicationAPI
	mockModelConfigAPI *mocks.MockModelConfigClient

	facadeVersion int

	store *jujuclient.MemStore
}

var _ = tc.Suite(&removeApplicationSuite{})

func (s *removeApplicationSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclienttesting.MinimalStore()
	s.facadeVersion = 16
}

func (s *removeApplicationSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockApi = mocks.NewMockRemoveApplicationAPI(ctrl)
	s.mockApi.EXPECT().BestAPIVersion().Return(s.facadeVersion).AnyTimes()
	s.mockApi.EXPECT().Close()

	s.mockModelConfigAPI = mocks.NewMockModelConfigClient(ctrl)
	s.mockModelConfigAPI.EXPECT().Close()

	return ctrl
}

func (s *removeApplicationSuite) runRemoveApplication(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, NewRemoveApplicationCommandForTest(s.mockApi, s.mockModelConfigAPI, s.store), args...)
}

func (s *removeApplicationSuite) runWithContext(ctx *cmd.Context, args ...string) chan error {
	remove := NewRemoveApplicationCommandForTest(s.mockApi, s.mockModelConfigAPI, s.store)
	return cmdtesting.RunCommandWithContext(ctx, remove, args...)
}

func (s *removeApplicationSuite) TestRemoveApplication(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app"},
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{},
	}}, nil)

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "real-app")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "will remove application real-app\n")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *removeApplicationSuite) TestRemoveApplicationWithRequiresPromptModeAbsent(c *tc.C) {
	defer s.setup(c).Finish()

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: ""})
	s.mockModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(attrs, nil)

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app"},
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{},
	}}, nil)

	ctx, err := s.runRemoveApplication(c, "real-app")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "will remove application real-app\n")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *removeApplicationSuite) TestRemoveApplicationForce(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app"},
		Force:        true,
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{},
	}}, nil)

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "real-app", "--force")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "will remove application real-app\n")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *removeApplicationSuite) TestRemoveApplicationDryRun(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app"},
		DryRun:       true,
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{},
	}}, nil)

	ctx, err := s.runRemoveApplication(c, "real-app", "--dry-run")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
will remove application real-app
`[1:])
}

func (s *removeApplicationSuite) TestRemoveApplicationDryRunOldFacade(c *tc.C) {
	s.facadeVersion = 15
	defer s.setup(c).Finish()

	_, err := s.runRemoveApplication(c, "real-app", "--dry-run")

	c.Assert(err, tc.ErrorMatches, "Your controller does not support `--dry-run`")
}

func (s *removeApplicationSuite) TestRemoveApplicationPrompt(c *tc.C) {
	defer s.setup(c).Finish()

	var stdin bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdin = &stdin

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: config.RequiresPromptsMode})
	s.mockModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(attrs, nil)

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app"},
		DryRun:       true,
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{},
	}}, nil)
	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app"},
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{},
	}}, nil)

	stdin.WriteString("y")
	errc := s.runWithContext(ctx, "real-app")

	select {
	case err := <-errc:
		c.Check(err, tc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}

	c.Assert(cmdtesting.Stderr(ctx), tc.Matches, `(?s).*Continue [y/N]?.*`)
	c.Assert(cmdtesting.Stdout(ctx), tc.Matches, `(?s)will remove application real-app.*`)
}

func setupRace(raceyApplications []string) func(ctx context.Context, args apiapplication.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error) {
	return func(ctx context.Context, args apiapplication.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error) {
		results := make([]params.DestroyApplicationResult, len(args.Applications))
		for i, app := range args.Applications {
			results[i].Info = &params.DestroyApplicationInfo{}
			for _, poison := range raceyApplications {
				if app == poison {
					err := errors.NewNotSupported(nil, "change detected")
					results[i].Error = apiservererrors.ServerError(err)
				}
			}
		}
		return results, nil
	}
}

func (s *removeApplicationSuite) TestHandlingNotSupportedDoesNotAffectBaseCase(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app"},
	}).DoAndReturn(setupRace([]string{"do-not-remove"}))

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "real-app")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "will remove application real-app\n")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *removeApplicationSuite) TestHandlingNotSupported(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"do-not-remove"},
	}).DoAndReturn(setupRace([]string{"do-not-remove"}))

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "do-not-remove")

	c.Assert(err, tc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
ERROR removing application do-not-remove failed: another user was updating application; please try again
`[1:])
}

func (s *removeApplicationSuite) TestHandlingNotSupportedMultipleApps(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"real-app", "do-not-remove", "another"},
	}).DoAndReturn(setupRace([]string{"do-not-remove"}))

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "real-app", "do-not-remove", "another")

	c.Assert(err, tc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
will remove application real-app
will remove application another
`[1:])
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
ERROR removing application do-not-remove failed: another user was updating application; please try again
`[1:])
}

func (s *removeApplicationSuite) TestDetachStorage(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"storage-app"},
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{
			DetachedStorage: []params.Entity{{Tag: "storage-data-0"}, {Tag: "storage-data-1"}, {Tag: "storage-data-2"}, {Tag: "storage-data-3"}},
		},
	}}, nil)

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "storage-app")

	c.Assert(err, tc.ErrorIsNil)
	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, tc.Equals, `
will remove application storage-app
- will detach storage data/0
- will detach storage data/1
- will detach storage data/2
- will detach storage data/3
`[1:])
}

func (s *removeApplicationSuite) TestDestroyStorage(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications:   []string{"storage-app"},
		DestroyStorage: true,
	}).Return([]params.DestroyApplicationResult{{
		Info: &params.DestroyApplicationInfo{
			DestroyedStorage: []params.Entity{{Tag: "storage-data-0"}, {Tag: "storage-data-1"}, {Tag: "storage-data-2"}, {Tag: "storage-data-3"}},
		},
	}}, nil)

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "storage-app", "--destroy-storage")

	c.Assert(err, tc.ErrorIsNil)
	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, tc.Equals, `
will remove application storage-app
- will remove storage data/0
- will remove storage data/1
- will remove storage data/2
- will remove storage data/3
`[1:])
}

func (s *removeApplicationSuite) TestFailure(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyApplications(gomock.Any(), apiapplication.DestroyApplicationsParams{
		Applications: []string{"gargleblaster"},
	}).Return([]params.DestroyApplicationResult{{
		Error: &params.Error{
			Message: "doink",
		},
	}}, nil)

	ctx, err := s.runRemoveApplication(c, "--no-prompt", "gargleblaster")

	c.Assert(err, tc.Equals, cmd.ErrSilent)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, tc.Equals, `
ERROR removing application gargleblaster failed: doink
`[1:])
}

func (s *removeApplicationSuite) TestInvalidArgs(c *tc.C) {
	_, err := s.runRemoveApplication(c)
	c.Assert(err, tc.ErrorMatches, `no application specified`)

	_, err = s.runRemoveApplication(c, "invalid:name")
	c.Assert(err, tc.ErrorMatches, `invalid application name "invalid:name"`)
}

func (s *removeApplicationSuite) TestNoWaitWithoutForce(c *tc.C) {
	_, err := s.runRemoveApplication(c, "gargleblaster", "--no-wait")
	c.Assert(err, tc.ErrorMatches, `--no-wait without --force not valid`)
}
