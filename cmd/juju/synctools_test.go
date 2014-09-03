// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io"
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type syncToolsSuite struct {
	coretesting.BaseSuite
	fakeSyncToolsAPI *fakeSyncToolsAPI
}

var _ = gc.Suite(&syncToolsSuite{})

func (s *syncToolsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.fakeSyncToolsAPI = &fakeSyncToolsAPI{}
	s.PatchValue(&getSyncToolsAPI, func(c *SyncToolsCommand) (syncToolsAPI, error) {
		return s.fakeSyncToolsAPI, nil
	})
}

func (s *syncToolsSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func runSyncToolsCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return coretesting.RunCommand(c, envcmd.Wrap(&SyncToolsCommand{}), args...)
}

var syncToolsCommandTests = []struct {
	description string
	args        []string
	sctx        *sync.SyncContext
	public      bool
}{
	{
		description: "environment as only argument",
		args:        []string{"-e", "test-target"},
		sctx:        &sync.SyncContext{},
	},
	{
		description: "specifying also the synchronization source",
		args:        []string{"-e", "test-target", "--source", "/foo/bar"},
		sctx: &sync.SyncContext{
			Source: "/foo/bar",
		},
	},
	{
		description: "synchronize all version including development",
		args:        []string{"-e", "test-target", "--all", "--dev"},
		sctx: &sync.SyncContext{
			AllVersions: true,
			Dev:         true,
		},
	},
	{
		description: "just make a dry run",
		args:        []string{"-e", "test-target", "--dry-run"},
		sctx: &sync.SyncContext{
			DryRun: true,
		},
	},
	{
		description: "specified public (ignored by API)",
		args:        []string{"-e", "test-target", "--public"},
		sctx:        &sync.SyncContext{},
	},
	{
		description: "specify version",
		args:        []string{"-e", "test-target", "--version", "1.2"},
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
			c.Assert(sctx.Dev, gc.Equals, test.sctx.Dev)
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
		ctx, err := runSyncToolsCommand(c, test.args...)
		c.Assert(err, gc.IsNil)
		c.Assert(ctx, gc.NotNil)
		c.Assert(called, jc.IsTrue)
		s.Reset(c)
	}
}

func (s *syncToolsSuite) TestSyncToolsCommandTargetDirectory(c *gc.C) {
	called := false
	dir := c.MkDir()
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.AllVersions, gc.Equals, false)
		c.Assert(sctx.DryRun, gc.Equals, false)
		c.Assert(sctx.Dev, gc.Equals, false)
		c.Assert(sctx.Source, gc.Equals, "")
		c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		c.Assert(uploader.WriteMirrors, gc.Equals, envtools.DoNotWriteMirrors)
		url, err := uploader.Storage.URL("")
		c.Assert(err, gc.IsNil)
		c.Assert(url, gc.Equals, "file://"+dir)
		called = true
		return nil
	}
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--local-dir", dir)
	c.Assert(err, gc.IsNil)
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
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--local-dir", dir, "--public")
	c.Assert(err, gc.IsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
}

func (s *syncToolsSuite) TestSyncToolsCommandDeprecatedDestination(c *gc.C) {
	called := false
	dir := c.MkDir()
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.AllVersions, gc.Equals, false)
		c.Assert(sctx.DryRun, gc.Equals, false)
		c.Assert(sctx.Dev, gc.Equals, false)
		c.Assert(sctx.Source, gc.Equals, "")
		c.Assert(sctx.TargetToolsUploader, gc.FitsTypeOf, sync.StorageToolsUploader{})
		uploader := sctx.TargetToolsUploader.(sync.StorageToolsUploader)
		url, err := uploader.Storage.URL("")
		c.Assert(err, gc.IsNil)
		c.Assert(url, gc.Equals, "file://"+dir)
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
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--destination", dir)
	c.Assert(err, gc.IsNil)
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
	list, err := a.FindTools(2)
	c.Assert(err, gc.IsNil)
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
	list, err := a.FindTools(1)
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
	list, err := a.FindTools(1)
	c.Assert(err, gc.Equals, findToolsErr) // error comes through untranslated
	c.Assert(list, gc.HasLen, 0)
}

func (s *syncToolsSuite) TestAPIAdapterUploadTools(c *gc.C) {
	uploadToolsErr := errors.New("uh oh")
	fake := fakeSyncToolsAPI{
		uploadTools: func(r io.Reader, v version.Binary) (*coretools.Tools, error) {
			data, err := ioutil.ReadAll(r)
			c.Assert(err, gc.IsNil)
			c.Assert(string(data), gc.Equals, "abc")
			c.Assert(v, gc.Equals, version.Current)
			return nil, uploadToolsErr
		},
	}
	a := syncToolsAPIAdapter{&fake}
	err := a.UploadTools(&coretools.Tools{Version: version.Current}, []byte("abc"))
	c.Assert(err, gc.Equals, uploadToolsErr)
}

type fakeSyncToolsAPI struct {
	findTools   func(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error)
	uploadTools func(r io.Reader, v version.Binary) (*coretools.Tools, error)
}

func (f *fakeSyncToolsAPI) FindTools(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error) {
	return f.findTools(majorVersion, minorVersion, series, arch)
}

func (f *fakeSyncToolsAPI) UploadTools(r io.Reader, v version.Binary) (*coretools.Tools, error) {
	return f.uploadTools(r, v)
}

func (f *fakeSyncToolsAPI) Close() error {
	return nil
}
