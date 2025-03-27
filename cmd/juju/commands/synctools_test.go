// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&syncToolSuite{})

func (s *syncToolSuite) SetUpTest(c *gc.C) {
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

func (s *syncToolSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *syncToolSuite) getSyncAgentBinariesCommand(c *gc.C, args ...string) (*gomock.Controller, func() (*cmd.Context, error)) {
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

func (s *syncToolSuite) TestSyncToolsCommand(c *gc.C) {
	runTest := func(idx int, test syncToolCommandTestCase, c *gc.C) {
		c.Logf("test %d: %s", idx, test.description)
		ctrl, run := s.getSyncAgentBinariesCommand(c, test.args...)
		defer ctrl.Finish()

		s.fakeSyncToolAPI.EXPECT().Close()

		called := false
		syncTools = func(_ context.Context, sctx *sync.SyncContext) error {
			c.Assert(sctx.AllVersions, gc.Equals, false)
			c.Assert(sctx.ChosenVersion, gc.Equals, semversion.MustParse("2.9.99"))
			c.Assert(sctx.DryRun, gc.Equals, test.dryRun)
			c.Assert(sctx.Stream, gc.Equals, test.stream)
			c.Assert(sctx.Source, gc.Equals, test.source)

			c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, syncToolAPIAdaptor{})
			uploader := sctx.TargetToolsUploader.(syncToolAPIAdaptor)
			c.Assert(uploader.SyncToolAPI, gc.Equals, s.fakeSyncToolAPI)

			called = true
			return nil
		}
		ctx, err := run()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ctx, gc.NotNil)
		c.Assert(called, jc.IsTrue)
		s.Reset(c)
	}

	for i, test := range syncToolCommandTests {
		runTest(i, test, c)
	}
}

func (s *syncToolSuite) TestSyncToolsCommandTargetDirectory(c *gc.C) {
	dir := c.MkDir()
	ctrl, run := s.getSyncAgentBinariesCommand(
		c, "--agent-version", "2.9.99", "-m", "test-target", "--local-dir", dir, "--stream", "proposed")
	defer ctrl.Finish()

	called := false
	syncTools = func(_ context.Context, sctx *sync.SyncContext) error {
		c.Assert(sctx.AllVersions, jc.IsFalse)
		c.Assert(sctx.DryRun, jc.IsFalse)
		c.Assert(sctx.Stream, gc.Equals, "proposed")
		c.Assert(sctx.Source, gc.Equals, "")
		c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		c.Assert(uploader.WriteMirrors, gc.Equals, envtools.DoNotWriteMirrors)
		url, err := uploader.Storage.URL("")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(url, gc.Equals, utils.MakeFileURL(dir))
		called = true
		return nil
	}
	ctx, err := run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
}

func (s *syncToolSuite) TestSyncToolsCommandTargetDirectoryPublic(c *gc.C) {
	dir := c.MkDir()
	ctrl, run := s.getSyncAgentBinariesCommand(
		c, "--agent-version", "2.9.99", "-m", "test-target", "--local-dir", dir, "--public")
	defer ctrl.Finish()

	called := false
	syncTools = func(_ context.Context, sctx *sync.SyncContext) error {
		c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		c.Assert(uploader.WriteMirrors, gc.Equals, envtools.WriteMirrors)
		called = true
		return nil
	}
	ctx, err := run()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
}

func (s *syncToolSuite) TestAPIAdaptorUploadTools(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	fakeAPI := mocks.NewMockSyncToolAPI(ctrl)

	current := coretesting.CurrentVersion()
	uploadToolsErr := errors.New("uh oh")
	fakeAPI.EXPECT().UploadTools(context.Background(), bytes.NewReader([]byte("abc")), current).Return(nil, uploadToolsErr)

	a := syncToolAPIAdaptor{fakeAPI}
	err := a.UploadTools(context.Background(), "released", "released", &coretools.Tools{Version: current}, []byte("abc"))
	c.Assert(err, gc.Equals, uploadToolsErr)
}

func (s *syncToolSuite) TestAPIAdaptorBlockUploadTools(c *gc.C) {
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
