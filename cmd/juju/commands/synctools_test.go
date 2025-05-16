// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/jujuclient"
)

type syncToolSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	fakeSyncToolAPI *mocks.MockSyncToolAPI
	store           *jujuclient.MemStore
}

func TestSyncToolSuite(t *stdtesting.T) { tc.Run(t, &syncToolSuite{}) }
func (s *syncToolSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "ctrl"
	s.store.Controllers["ctrl"] = jujuclient.ControllerDetails{}
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/test-target": {ModelType: "iaas"}}}
	s.store.Accounts["ctrl"] = jujuclient.AccountDetails{
		User: "admin",
	}
}

func (s *syncToolSuite) Reset(c *tc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *syncToolSuite) getSyncAgentBinariesCommand(c *tc.C, args ...string) (*gomock.Controller, func() (*cmd.Context, error)) {
	ctrl := gomock.NewController(c)
	s.fakeSyncToolAPI = mocks.NewMockSyncToolAPI(ctrl)

	syncToolCMD := &syncAgentBinaryCommand{syncToolAPI: s.fakeSyncToolAPI}
	syncToolCMD.SetClientStore(s.store)
	return ctrl, func() (*cmd.Context, error) {
		return cmdtesting.RunCommand(c, modelcmd.Wrap(syncToolCMD), args...)
	}
}

type syncToolCommandTestCase struct {
	description string
	args        []string
	dryRun      bool
	public      bool
	source      string
	stream      string
}

var syncToolCommandTests = []syncToolCommandTestCase{
	{
		description: "minimum argument",
		args:        []string{"--agent-version", "2.9.99", "-m", "test-target"},
	},
	{
		description: "specifying also the synchronization source",
		args:        []string{"--agent-version", "2.9.99", "-m", "test-target", "--source", "/foo/bar"},
		source:      "/foo/bar",
	},
	{
		description: "just make a dry run",
		args:        []string{"--agent-version", "2.9.99", "-m", "test-target", "--dry-run"},
		dryRun:      true,
	},
	{
		description: "specified public (ignored by API)",
		args:        []string{"--agent-version", "2.9.99", "-m", "test-target", "--public"},
		public:      false,
	},
}

func (s *syncToolSuite) TestSyncToolsCommand(c *tc.C) {
	runTest := func(idx int, test syncToolCommandTestCase, c *tc.C) {
		c.Logf("test %d: %s", idx, test.description)
		ctrl, run := s.getSyncAgentBinariesCommand(c, test.args...)
		defer ctrl.Finish()

		s.fakeSyncToolAPI.EXPECT().Close()

		called := false
		syncTools = func(_ context.Context, sctx *sync.SyncContext) error {
			c.Assert(sctx.AllVersions, tc.Equals, false)
			c.Assert(sctx.ChosenVersion, tc.Equals, semversion.MustParse("2.9.99"))
			c.Assert(sctx.DryRun, tc.Equals, test.dryRun)
			c.Assert(sctx.Stream, tc.Equals, test.stream)
			c.Assert(sctx.Source, tc.Equals, test.source)

			c.Assert(sctx.TargetToolsUploader, tc.FitsTypeOf, syncToolAPIAdaptor{})
			uploader := sctx.TargetToolsUploader.(syncToolAPIAdaptor)
			c.Assert(uploader.SyncToolAPI, tc.Equals, s.fakeSyncToolAPI)

			called = true
			return nil
		}
		ctx, err := run()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ctx, tc.NotNil)
		c.Assert(called, tc.IsTrue)
		s.Reset(c)
	}

	for i, test := range syncToolCommandTests {
		runTest(i, test, c)
	}
}

func (s *syncToolSuite) TestSyncToolsCommandTargetDirectory(c *tc.C) {
	dir := c.MkDir()
	ctrl, run := s.getSyncAgentBinariesCommand(
		c, "--agent-version", "2.9.99", "-m", "test-target", "--local-dir", dir, "--stream", "proposed")
	defer ctrl.Finish()

	called := false
	syncTools = func(_ context.Context, sctx *sync.SyncContext) error {
		c.Assert(sctx.AllVersions, tc.IsFalse)
		c.Assert(sctx.DryRun, tc.IsFalse)
		c.Assert(sctx.Stream, tc.Equals, "proposed")
		c.Assert(sctx.Source, tc.Equals, "")
		c.Assert(sctx.TargetToolsUploader, tc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		c.Assert(uploader.WriteMirrors, tc.Equals, envtools.DoNotWriteMirrors)
		url, err := uploader.Storage.URL("")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(url, tc.Equals, utils.MakeFileURL(dir))
		called = true
		return nil
	}
	ctx, err := run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx, tc.NotNil)
	c.Assert(called, tc.IsTrue)
}

func (s *syncToolSuite) TestSyncToolsCommandTargetDirectoryPublic(c *tc.C) {
	dir := c.MkDir()
	ctrl, run := s.getSyncAgentBinariesCommand(
		c, "--agent-version", "2.9.99", "-m", "test-target", "--local-dir", dir, "--public")
	defer ctrl.Finish()

	called := false
	syncTools = func(_ context.Context, sctx *sync.SyncContext) error {
		c.Assert(sctx.TargetToolsUploader, tc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		c.Assert(uploader.WriteMirrors, tc.Equals, envtools.WriteMirrors)
		called = true
		return nil
	}
	ctx, err := run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx, tc.NotNil)
	c.Assert(called, tc.IsTrue)
}

func (s *syncToolSuite) TestAPIAdaptorUploadTools(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	fakeAPI := mocks.NewMockSyncToolAPI(ctrl)

	current := coretesting.CurrentVersion()
	uploadToolsErr := errors.New("uh oh")
	fakeAPI.EXPECT().UploadTools(gomock.Any(), bytes.NewReader([]byte("abc")), current).Return(nil, uploadToolsErr)

	a := syncToolAPIAdaptor{fakeAPI}
	err := a.UploadTools(c.Context(), "released", "released", &coretools.Tools{Version: current}, []byte("abc"))
	c.Assert(err, tc.Equals, uploadToolsErr)
}

func (s *syncToolSuite) TestAPIAdaptorBlockUploadTools(c *tc.C) {
	ctrl, run := s.getSyncAgentBinariesCommand(
		c, "-m", "test-target", "--agent-version", "2.9.99", "--local-dir", c.MkDir(), "--stream", "released")
	defer ctrl.Finish()

	syncTools = func(_ context.Context, sctx *sync.SyncContext) error {
		// Block operation
		return apiservererrors.OperationBlockedError("TestAPIAdaptorBlockUploadTools")
	}
	_, err := run()
	coretesting.AssertOperationWasBlocked(c, err, ".*TestAPIAdaptorBlockUploadTools.*")
}
