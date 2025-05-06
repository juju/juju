// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/tar"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/names"
)

type syncSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	storage      storage.Storage
	localStorage string
}

var _ = gc.Suite(&syncSuite{})

var _ = gc.Suite(&uploadSuite{})

var _ = gc.Suite(&badBuildSuite{})

func (s *syncSuite) setUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// It's important that this be v1.8.x to match the test data.
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.8.3"))

	// Create a source storage.
	baseDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(baseDir)
	c.Assert(err, jc.ErrorIsNil)
	s.storage = stor

	// Create a local tools directory.
	s.localStorage = c.MkDir()

	// Populate both local and default tools locations with the public tools.
	versionStrings := make([]string, len(vAll))
	for i, vers := range vAll {
		versionStrings[i] = vers.String()
	}
	toolstesting.MakeTools(c, baseDir, "released", versionStrings)
	toolstesting.MakeTools(c, s.localStorage, "released", versionStrings)

	// Switch the default tools location.
	baseURL, err := s.storage.URL(storage.BaseToolsPath)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&envtools.DefaultBaseURL, baseURL)
}

func (s *syncSuite) tearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

var tests = []struct {
	description string
	ctx         *sync.SyncContext
	source      bool
	tools       []semversion.Binary
	major       int
	minor       int
}{
	{
		description: "copy newest from the filesystem",
		ctx: &sync.SyncContext{
			ChosenVersion: semversion.MustParse("1.8.0"),
		},
		source: true,
		tools:  v180all,
	},
	{
		description: "copy newest from the dummy model",
		ctx: &sync.SyncContext{
			ChosenVersion: semversion.MustParse("1.8.0"),
		},
		tools: v180all,
	},
	{
		description: "copy matching dev from the dummy model",
		ctx: &sync.SyncContext{
			ChosenVersion: semversion.MustParse("1.9.0"),
		},
		tools: v190all,
	},
	{
		description: "copy matching version from the dummy model",
		ctx: &sync.SyncContext{
			ChosenVersion: semversion.MustParse("3.2.0"),
		},
		tools: []semversion.Binary{v320u64},
	},
	{
		description: "copy matching major, minor dev from the dummy model",
		ctx: &sync.SyncContext{
			ChosenVersion: semversion.MustParse("3.1.0"),
		},
		tools: []semversion.Binary{v310u64},
	},
}

func (s *syncSuite) TestSyncing(c *gc.C) {
	for i, test := range tests {
		// Perform all tests in a "clean" environment.
		func() {
			s.setUpTest(c)
			defer s.tearDownTest(c)

			c.Logf("test %d: %s", i, test.description)

			if test.source {
				test.ctx.Source = s.localStorage
			}
			if test.ctx.ChosenVersion != semversion.Zero {
				jujuversion.Current = test.ctx.ChosenVersion
			}

			uploader := fakeToolsUploader{
				uploaded: make(map[semversion.Binary]bool),
			}
			test.ctx.TargetToolsFinder = mockToolsFinder{}
			test.ctx.TargetToolsUploader = &uploader

			err := sync.SyncTools(context.Background(), test.ctx)
			c.Assert(err, jc.ErrorIsNil)

			ds, err := sync.SelectSourceDatasource(context.Background(), test.ctx)
			c.Assert(err, jc.ErrorIsNil)

			// This data source does not require to contain signed data.
			// However, it may still contain it.
			// Since we will always try to read signed data first,
			// we want to be able to try to read this signed data
			// with public key with Juju-known public key for tools.
			// Bugs #1542127, #1542131
			c.Assert(ds.PublicSigningKey(), gc.Not(gc.Equals), "")

			var uploaded []semversion.Binary
			for v := range uploader.uploaded {
				uploaded = append(uploaded, v)
			}
			c.Assert(uploaded, jc.SameContents, test.tools)
		}()
	}
}

// regression test for https://pad.lv/2029881
func (s *syncSuite) TestSyncToolsOldPatchVersion(c *gc.C) {
	s.setUpTest(c)
	defer s.tearDownTest(c)

	// Add some extra tools for the newer patch versions
	toolstesting.MakeTools(c, s.localStorage, "released", []string{"1.8.3-ubuntu-amd64"})

	err := sync.SyncTools(context.Background(), &sync.SyncContext{
		Source: s.localStorage,
		// Request an older patch version of the current series (1.8.x)
		ChosenVersion: semversion.MustParse("1.8.0"),
		TargetToolsUploader: &fakeToolsUploader{
			uploaded: make(map[semversion.Binary]bool),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

type fakeToolsUploader struct {
	uploaded map[semversion.Binary]bool
}

func (u *fakeToolsUploader) UploadTools(_ context.Context, _, _ string, tools *coretools.Tools, _ []byte) error {
	u.uploaded[tools.Version] = true
	return nil
}

var (
	v100u64 = semversion.MustParseBinary("1.0.0-ubuntu-amd64")
	v100u32 = semversion.MustParseBinary("1.0.0-ubuntu-arm64")
	v100all = []semversion.Binary{v100u64, v100u32}
	v180u64 = semversion.MustParseBinary("1.8.0-ubuntu-amd64")
	v180u32 = semversion.MustParseBinary("1.8.0-ubuntu-arm64")
	v180all = []semversion.Binary{v180u64, v180u32}
	v190u64 = semversion.MustParseBinary("1.9.0-ubuntu-amd64")
	v190u32 = semversion.MustParseBinary("1.9.0-ubuntu-arm64")
	v190all = []semversion.Binary{v190u64, v190u32}
	v1all   = append(append(v100all, v180all...), v190all...)
	v200u64 = semversion.MustParseBinary("2.0.0-ubuntu-amd64")
	v310u64 = semversion.MustParseBinary("3.1.0-ubuntu-amd64")
	v320u64 = semversion.MustParseBinary("3.2.0-ubuntu-amd64")
	vAll    = append(append(v1all, v200u64), v310u64, v320u64)
)

type uploadSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	targetStorage storage.Storage
}

func (s *uploadSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// Create a target storage.
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	s.targetStorage = stor
}

func (s *uploadSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func bundleTools(c *gc.C) (semversion.Binary, bool, string, error) {
	f, err := os.CreateTemp("", "juju-tgz")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = f.Close() }()
	defer func() { _ = os.Remove(f.Name()) }()

	tvers, _, official, sha256hash, err := envtools.BundleTools(true, f,
		func(semversion.Number) semversion.Number { return jujuversion.Current },
	)
	return tvers, official, sha256hash, err
}

type badBuildSuite struct {
	jujutesting.LoggingSuite
	jujutesting.CleanupSuite
	envtesting.ToolsFixture
	jujutesting.PatchExecHelper
}

var badGo = `
#!/bin/bash --norc
exit 1
`[1:]

func (s *badBuildSuite) SetUpSuite(c *gc.C) {
	s.CleanupSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *badBuildSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.CleanupSuite.TearDownSuite(c)
}

func (s *badBuildSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// Mock go cmd
	testPath := c.MkDir()
	s.PatchEnvPathPrepend(testPath)
	path := filepath.Join(testPath, "go")
	err := os.WriteFile(path, []byte(badGo), 0755)
	c.Assert(err, jc.ErrorIsNil)

	// Check mocked go cmd errors
	out, err := exec.Command("go").CombinedOutput()
	c.Assert(err, gc.ErrorMatches, "exit status 1")
	c.Assert(string(out), gc.Equals, "")
}

func (s *badBuildSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
	s.CleanupSuite.TearDownTest(c)
}

func (s *badBuildSuite) assertEqualsCurrentVersion(c *gc.C, v semversion.Binary) {
	current := coretesting.CurrentVersion()
	c.Assert(v, gc.Equals, current)
}

func (s *badBuildSuite) TestBundleToolsBadBuild(c *gc.C) {
	s.patchExecCommand(c)

	// Test that original bundleTools Func fails as expected
	vers, official, sha256Hash, err := bundleTools(c)
	c.Assert(vers, gc.DeepEquals, semversion.Binary{})
	c.Assert(official, jc.IsFalse)
	c.Assert(sha256Hash, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `(?m)cannot build jujud agent binary from source: .*`)

	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(jujuversion.Current))

	// Test that BundleTools func passes after it is
	// mocked out
	vers, official, sha256Hash, err = bundleTools(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers.Number, gc.Equals, jujuversion.Current)
	c.Assert(official, jc.IsFalse)
	c.Assert(sha256Hash, gc.Equals, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
}

func (s *badBuildSuite) patchExecCommand(c *gc.C) {
	execCommand := s.GetExecCommand(jujutesting.PatchExecConfig{
		Stdout: coretesting.CurrentVersion().String(),
		Args:   make(chan []string, 2),
	})
	s.PatchValue(&envtools.ExecCommand, execCommand)
}

func (s *badBuildSuite) TestBuildToolsBadBuild(c *gc.C) {
	s.patchExecCommand(c)

	// Test that original BuildAgentTarball fails
	builtTools, err := sync.BuildAgentTarball(true, "released",
		func(semversion.Number) semversion.Number { return semversion.Zero },
	)
	c.Assert(err, gc.ErrorMatches, `(?m)cannot build jujud agent binary from source: .*`)
	c.Assert(builtTools, gc.IsNil)

	// Test that BuildAgentTarball func passes after BundleTools func is
	// mocked out
	forceVersion := coretesting.CurrentVersion().Number
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(forceVersion))
	builtTools, err = sync.BuildAgentTarball(true, "released",
		func(semversion.Number) semversion.Number { return forceVersion },
	)
	s.assertEqualsCurrentVersion(c, builtTools.Version)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *badBuildSuite) TestBuildToolsNoBinaryAvailable(c *gc.C) {
	s.patchExecCommand(c)

	builtTools, err := sync.BuildAgentTarball(false, "released",
		func(semversion.Number) semversion.Number { return semversion.Zero },
	)
	c.Assert(err, gc.ErrorMatches, `no prepackaged agent available and no jujud binary can be found`)
	c.Assert(builtTools, gc.IsNil)
}

func (s *uploadSuite) TestMockBundleTools(c *gc.C) {
	var (
		writer       io.Writer
		forceVersion semversion.Number
		n            int
		p            bytes.Buffer
	)
	p.WriteString("Hello World")

	s.PatchValue(&envtools.BundleTools,
		func(
			build bool, writerArg io.Writer,
			getForceVersion func(semversion.Number) semversion.Number,
		) (vers semversion.Binary, fVersion semversion.Number, official bool, sha256Hash string, err error) {
			c.Assert(build, jc.IsTrue)
			writer = writerArg
			n, err = writer.Write(p.Bytes())
			c.Assert(err, jc.ErrorIsNil)
			forceVersion = getForceVersion(semversion.Zero)
			fVersion = forceVersion
			vers.Number = jujuversion.Current
			return
		},
	)

	_, err := sync.BuildAgentTarball(true, "released",
		func(semversion.Number) semversion.Number { return jujuversion.Current },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(forceVersion, gc.Equals, jujuversion.Current)
	c.Assert(writer, gc.NotNil)
	c.Assert(n, gc.Equals, len(p.Bytes()))
}

func (s *uploadSuite) TestMockBuildTools(c *gc.C) {
	checkTools := func(tools *sync.BuiltAgent, vers semversion.Binary) {
		c.Check(tools.StorageName, gc.Equals, "name")
		c.Check(tools.Version, jc.DeepEquals, vers)

		f, err := os.Open(filepath.Join(tools.Dir, "name"))
		c.Assert(err, jc.ErrorIsNil)
		defer f.Close()

		gzr, err := gzip.NewReader(f)
		c.Assert(err, jc.ErrorIsNil)

		_, tr, err := tar.FindFile(gzr, names.Jujud)
		c.Assert(err, jc.ErrorIsNil)

		content, err := io.ReadAll(tr)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content), gc.Equals, fmt.Sprintf("jujud contents %s", vers))
	}

	current := semversion.MustParseBinary("1.9.1-ubuntu-amd64")
	s.PatchValue(&jujuversion.Current, current.Number)
	s.PatchValue(&arch.HostArch, func() string { return current.Arch })
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })
	buildToolsFunc := toolstesting.GetMockBuildTools(c)
	builtTools, err := buildToolsFunc(true, "released",
		func(semversion.Number) semversion.Number { return jujuversion.Current },
	)
	c.Assert(err, jc.ErrorIsNil)
	checkTools(builtTools, current)

	vers := semversion.MustParseBinary("1.5.3-ubuntu-amd64")
	builtTools, err = buildToolsFunc(true, "released",
		func(semversion.Number) semversion.Number { return vers.Number },
	)
	c.Assert(err, jc.ErrorIsNil)
	checkTools(builtTools, vers)
}

func (s *uploadSuite) TestStorageToolsUploaderWriteMirrors(c *gc.C) {
	s.testStorageToolsUploaderWriteMirrors(c, envtools.WriteMirrors)
}

func (s *uploadSuite) TestStorageToolsUploaderDontWriteMirrors(c *gc.C) {
	s.testStorageToolsUploaderWriteMirrors(c, envtools.DoNotWriteMirrors)
}

func (s *uploadSuite) testStorageToolsUploaderWriteMirrors(c *gc.C, writeMirrors envtools.ShouldWriteMirrors) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ss := NewMockSimplestreamsFetcher(ctrl)
	ss.EXPECT().GetMetadata(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	storageDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)

	uploader := &sync.StorageToolsUploader{
		Fetcher:       ss,
		Storage:       stor,
		WriteMetadata: true,
		WriteMirrors:  writeMirrors,
	}

	err = uploader.UploadTools(
		context.Background(),
		"released",
		"released",
		&coretools.Tools{
			Version: coretesting.CurrentVersion(),
			Size:    7,
			SHA256:  "ed7002b439e9ac845f22357d822bac1444730fbdb6016d3ec9432297b9ec9f73",
		}, []byte("content"))
	c.Assert(err, jc.ErrorIsNil)

	mirrorsPath := simplestreams.MirrorsPath(envtools.StreamsVersionV1) + simplestreams.UnsignedSuffix
	r, err := stor.Get(path.Join(storage.BaseToolsPath, mirrorsPath))
	if writeMirrors == envtools.WriteMirrors {
		c.Assert(err, jc.ErrorIsNil)
		data, err := io.ReadAll(r)
		r.Close()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), jc.Contains, `"mirrors":`)
	} else {
		c.Assert(err, jc.ErrorIs, errors.NotFound)
	}
}

type mockToolsFinder struct{}

func (mockToolsFinder) FindTools(major int, stream string) (coretools.List, error) {
	return nil, coretools.ErrNoMatches
}
