// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type syncToolsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	fakeSyncToolsAPI *fakeSyncToolsAPI
	store            *jujuclienttesting.MemStore
}

var _ = gc.Suite(&syncToolsSuite{})

func (s *syncToolsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fakeSyncToolsAPI = &fakeSyncToolsAPI{}
	s.PatchValue(&getSyncToolsAPI, func(c *syncToolsCommand) (syncToolsAPI, error) {
		return s.fakeSyncToolsAPI, nil
	})
	err := modelcmd.WriteCurrentController("ctrl")
	c.Assert(err, jc.ErrorIsNil)
	s.store = jujuclienttesting.NewMemStore()
	s.store.Accounts["ctrl"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "admin@local",
	}
}

func (s *syncToolsSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *syncToolsSuite) runSyncToolsCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := &syncToolsCommand{}
	cmd.SetClientStore(s.store)
	return coretesting.RunCommand(c, modelcmd.Wrap(cmd), args...)
}

var syncToolsCommandTests = []struct {
	description string
	args        []string
	sctx        *sync.SyncContext
	public      bool
}{
	{
		description: "model as only argument",
		args:        []string{"-m", "test-target"},
		sctx:        &sync.SyncContext{},
	},
	{
		description: "specifying also the synchronization source",
		args:        []string{"-m", "test-target", "--source", "/foo/bar"},
		sctx: &sync.SyncContext{
			Source: "/foo/bar",
		},
	},
	{
		description: "synchronize all version including development",
		args:        []string{"-m", "test-target", "--all", "--dev"},
		sctx: &sync.SyncContext{
			AllVersions: true,
			Stream:      "testing",
		},
	},
	{
		description: "just make a dry run",
		args:        []string{"-m", "test-target", "--dry-run"},
		sctx: &sync.SyncContext{
			DryRun: true,
		},
	},
	{
		description: "specified public (ignored by API)",
		args:        []string{"-m", "test-target", "--public"},
		sctx:        &sync.SyncContext{},
	},
	{
		description: "specify version",
		args:        []string{"-m", "test-target", "--version", "1.2"},
		sctx: &sync.SyncContext{
			MajorVersion: 1,
			MinorVersion: 2,
		},
	},
}

func (s *syncToolsSuite) TestSyncToolsCommand(c *gc.C) {
	for i, test := range syncToolsCommandTests {
		c.Logf("test %d: %s", i, test.description)
		called := false
		syncTools = func(sctx *sync.SyncContext) error {
			c.Assert(sctx.AllVersions, gc.Equals, test.sctx.AllVersions)
			c.Assert(sctx.MajorVersion, gc.Equals, test.sctx.MajorVersion)
			c.Assert(sctx.MinorVersion, gc.Equals, test.sctx.MinorVersion)
			c.Assert(sctx.DryRun, gc.Equals, test.sctx.DryRun)
			c.Assert(sctx.Stream, gc.Equals, test.sctx.Stream)
			c.Assert(sctx.Source, gc.Equals, test.sctx.Source)

			c.Assert(sctx.TargetToolsFinder, gc.FitsTypeOf, syncToolsAPIAdapter{})
			finder := sctx.TargetToolsFinder.(syncToolsAPIAdapter)
			c.Assert(finder.syncToolsAPI, gc.Equals, s.fakeSyncToolsAPI)

			c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, syncToolsAPIAdapter{})
			uploader := sctx.TargetToolsUploader.(syncToolsAPIAdapter)
			c.Assert(uploader.syncToolsAPI, gc.Equals, s.fakeSyncToolsAPI)

			called = true
			return nil
		}
		ctx, err := s.runSyncToolsCommand(c, test.args...)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ctx, gc.NotNil)
		c.Assert(called, jc.IsTrue)
		s.Reset(c)
	}
}

func (s *syncToolsSuite) TestSyncToolsCommandTargetDirectory(c *gc.C) {
	called := false
	dir := c.MkDir()
	syncTools = func(sctx *sync.SyncContext) error {
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
	ctx, err := s.runSyncToolsCommand(c, "-m", "test-target", "--local-dir", dir, "--stream", "proposed")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
}

func (s *syncToolsSuite) TestSyncToolsCommandTargetDirectoryPublic(c *gc.C) {
	called := false
	dir := c.MkDir()
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		c.Assert(uploader.WriteMirrors, gc.Equals, envtools.WriteMirrors)
		called = true
		return nil
	}
	ctx, err := s.runSyncToolsCommand(c, "-m", "test-target", "--local-dir", dir, "--public")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
}

func (s *syncToolsSuite) TestSyncToolsCommandDeprecatedDestination(c *gc.C) {
	called := false
	dir := c.MkDir()
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.AllVersions, jc.IsFalse)
		c.Assert(sctx.DryRun, jc.IsFalse)
		c.Assert(sctx.Stream, gc.Equals, "released")
		c.Assert(sctx.Source, gc.Equals, "")
		c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		url, err := uploader.Storage.URL("")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(url, gc.Equals, utils.MakeFileURL(dir))
		called = true
		return nil
	}
	// Register writer.
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("deprecated-tester", &tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("deprecated-tester")
	// Add deprecated message to be checked.
	messages := []jc.SimpleMessage{
		{loggo.WARNING, "Use of the --destination flag is deprecated in 1.18. Please use --local-dir instead."},
	}
	// Run sync-tools command with --destination flag.
	ctx, err := s.runSyncToolsCommand(c, "-m", "test-target", "--destination", dir, "--stream", "released")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
	// Check deprecated message was logged.
	c.Check(tw.Log(), jc.LogMatches, messages)
}

func (s *syncToolsSuite) TestAPIAdapterFindTools(c *gc.C) {
	var called bool
	result := coretools.List{&coretools.Tools{}}
	fake := fakeSyncToolsAPI{
		findTools: func(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error) {
			called = true
			c.Assert(majorVersion, gc.Equals, 2)
			c.Assert(minorVersion, gc.Equals, -1)
			c.Assert(series, gc.Equals, "")
			c.Assert(arch, gc.Equals, "")
			return params.FindToolsResult{List: result}, nil
		},
	}
	a := syncToolsAPIAdapter{&fake}
	list, err := a.FindTools(2, "released")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list, jc.SameContents, result)
	c.Assert(called, jc.IsTrue)
}

func (s *syncToolsSuite) TestAPIAdapterFindToolsNotFound(c *gc.C) {
	fake := fakeSyncToolsAPI{
		findTools: func(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error) {
			err := common.ServerError(errors.NotFoundf("tools"))
			return params.FindToolsResult{Error: err}, nil
		},
	}
	a := syncToolsAPIAdapter{&fake}
	list, err := a.FindTools(1, "released")
	c.Assert(err, gc.Equals, coretools.ErrNoMatches)
	c.Assert(list, gc.HasLen, 0)
}

func (s *syncToolsSuite) TestAPIAdapterFindToolsAPIError(c *gc.C) {
	findToolsErr := common.ServerError(errors.NotFoundf("tools"))
	fake := fakeSyncToolsAPI{
		findTools: func(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error) {
			return params.FindToolsResult{Error: findToolsErr}, findToolsErr
		},
	}
	a := syncToolsAPIAdapter{&fake}
	list, err := a.FindTools(1, "released")
	c.Assert(err, gc.Equals, findToolsErr) // error comes through untranslated
	c.Assert(list, gc.HasLen, 0)
}

func (s *syncToolsSuite) TestAPIAdapterUploadTools(c *gc.C) {
	uploadToolsErr := errors.New("uh oh")
	current := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	fake := fakeSyncToolsAPI{
		uploadTools: func(r io.Reader, v version.Binary, additionalSeries ...string) (*coretools.Tools, error) {
			data, err := ioutil.ReadAll(r)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(string(data), gc.Equals, "abc")
			c.Assert(v, gc.Equals, current)
			return nil, uploadToolsErr
		},
	}
	a := syncToolsAPIAdapter{&fake}
	err := a.UploadTools("released", "released", &coretools.Tools{Version: current}, []byte("abc"))
	c.Assert(err, gc.Equals, uploadToolsErr)
}

func (s *syncToolsSuite) TestAPIAdapterBlockUploadTools(c *gc.C) {
	syncTools = func(sctx *sync.SyncContext) error {
		// Block operation
		return common.OperationBlockedError("TestAPIAdapterBlockUploadTools")
	}
	_, err := s.runSyncToolsCommand(c, "-m", "test-target", "--destination", c.MkDir(), "--stream", "released")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestAPIAdapterBlockUploadTools.*")
}

type fakeSyncToolsAPI struct {
	findTools   func(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error)
	uploadTools func(r io.Reader, v version.Binary, additionalSeries ...string) (*coretools.Tools, error)
}

func (f *fakeSyncToolsAPI) FindTools(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error) {
	return f.findTools(majorVersion, minorVersion, series, arch)
}

func (f *fakeSyncToolsAPI) UploadTools(r io.ReadSeeker, v version.Binary, additionalSeries ...string) (*coretools.Tools, error) {
	return f.uploadTools(r, v, additionalSeries...)
}

func (f *fakeSyncToolsAPI) Close() error {
	return nil
}
