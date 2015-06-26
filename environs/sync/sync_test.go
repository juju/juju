// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type syncSuite struct {
	coretesting.FakeJujuHomeSuite
	envtesting.ToolsFixture
	origVersion  version.Binary
	storage      storage.Storage
	localStorage string
}

var _ = gc.Suite(&syncSuite{})
var _ = gc.Suite(&uploadSuite{})
var _ = gc.Suite(&badBuildSuite{})

func (s *syncSuite) setUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.origVersion = version.Current
	// It's important that this be v1.8.x to match the test data.
	version.Current.Number = version.MustParse("1.8.3")

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
	version.Current = s.origVersion
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
}

var tests = []struct {
	description string
	ctx         *sync.SyncContext
	source      bool
	tools       []version.Binary
	version     version.Number
	major       int
	minor       int
}{
	{
		description: "copy newest from the filesystem",
		ctx:         &sync.SyncContext{},
		source:      true,
		tools:       v180all,
	},
	{
		description: "copy newest from the dummy environment",
		ctx:         &sync.SyncContext{},
		tools:       v180all,
	},
	{
		description: "copy matching dev from the dummy environment",
		ctx:         &sync.SyncContext{},
		version:     version.MustParse("1.9.3"),
		tools:       v190all,
	},
	{
		description: "copy matching major, minor from the dummy environment",
		ctx:         &sync.SyncContext{},
		major:       3,
		minor:       2,
		tools:       []version.Binary{v320p64},
	},
	{
		description: "copy matching major, minor dev from the dummy environment",
		ctx:         &sync.SyncContext{},
		major:       3,
		minor:       1,
		tools:       []version.Binary{v310p64},
	},
	{
		description: "copy all from the dummy environment",
		ctx: &sync.SyncContext{
			AllVersions: true,
		},
		tools: v1all,
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
			if test.version != version.Zero {
				version.Current.Number = test.version
			}
			if test.major > 0 {
				test.ctx.MajorVersion = test.major
				test.ctx.MinorVersion = test.minor
			}
			uploader := fakeToolsUploader{
				uploaded: make(map[version.Binary]bool),
			}
			test.ctx.TargetToolsFinder = mockToolsFinder{}
			test.ctx.TargetToolsUploader = &uploader

			err := sync.SyncTools(test.ctx)
			c.Assert(err, jc.ErrorIsNil)

			var uploaded []version.Binary
			for v := range uploader.uploaded {
				uploaded = append(uploaded, v)
			}
			c.Assert(uploaded, jc.SameContents, test.tools)
		}()
	}
}

type fakeToolsUploader struct {
	uploaded map[version.Binary]bool
}

func (u *fakeToolsUploader) UploadTools(toolsDir, stream string, tools *coretools.Tools, data []byte) error {
	u.uploaded[tools.Version] = true
	return nil
}

var (
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100all = []version.Binary{v100p64, v100q64, v100q32}
	v180q64 = version.MustParseBinary("1.8.0-quantal-amd64")
	v180p32 = version.MustParseBinary("1.8.0-precise-i386")
	v180all = []version.Binary{v180q64, v180p32}
	v190q64 = version.MustParseBinary("1.9.0-quantal-amd64")
	v190p32 = version.MustParseBinary("1.9.0-precise-i386")
	v190all = []version.Binary{v190q64, v190p32}
	v1all   = append(append(v100all, v180all...), v190all...)
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	v310p64 = version.MustParseBinary("3.1.0-precise-amd64")
	v320p64 = version.MustParseBinary("3.2.0-precise-amd64")
	vAll    = append(append(v1all, v200p64), v310p64, v320p64)
)

type uploadSuite struct {
	env environs.Environ
	coretesting.FakeJujuHomeSuite
	envtesting.ToolsFixture
	targetStorage storage.Storage
}

func (s *uploadSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// Create a target storage.
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	s.targetStorage = stor

	// Mock out building of tools. Sync should not care about the contents
	// of tools archives, other than that they hash correctly.
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
}

func (s *uploadSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *uploadSuite) TestUpload(c *gc.C) {
	t, err := sync.Upload(s.targetStorage, "released", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	c.Assert(t.URL, gc.Not(gc.Equals), "")
	s.assertUploadedTools(c, t, []string{version.Current.Series}, "released")
}

func (s *uploadSuite) TestUploadFakeSeries(c *gc.C) {
	seriesToUpload := "precise"
	if seriesToUpload == version.Current.Series {
		seriesToUpload = "raring"
	}
	t, err := sync.Upload(s.targetStorage, "released", nil, "quantal", seriesToUpload)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUploadedTools(c, t, []string{seriesToUpload, "quantal", version.Current.Series}, "released")
}

func (s *uploadSuite) TestUploadAndForceVersion(c *gc.C) {
	// This test actually tests three things:
	//   the writing of the FORCE-VERSION file;
	//   the reading of the FORCE-VERSION file by the version package;
	//   and the reading of the version from jujud.
	vers := version.Current
	vers.Patch++
	t, err := sync.Upload(s.targetStorage, "released", &vers.Number)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t.Version, gc.Equals, vers)
}

func (s *uploadSuite) TestSyncTools(c *gc.C) {
	builtTools, err := sync.BuildToolsTarball(nil, "released")
	c.Assert(err, jc.ErrorIsNil)
	t, err := sync.SyncBuiltTools(s.targetStorage, "released", builtTools)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	c.Assert(t.URL, gc.Not(gc.Equals), "")
}

func (s *uploadSuite) TestSyncToolsFakeSeries(c *gc.C) {
	seriesToUpload := "precise"
	if seriesToUpload == version.Current.Series {
		seriesToUpload = "raring"
	}
	builtTools, err := sync.BuildToolsTarball(nil, "testing")
	c.Assert(err, jc.ErrorIsNil)

	t, err := sync.SyncBuiltTools(s.targetStorage, "testing", builtTools, "quantal", seriesToUpload)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUploadedTools(c, t, []string{seriesToUpload, "quantal", version.Current.Series}, "testing")
}

func (s *uploadSuite) TestSyncAndForceVersion(c *gc.C) {
	// This test actually tests three things:
	//   the writing of the FORCE-VERSION file;
	//   the reading of the FORCE-VERSION file by the version package;
	//   and the reading of the version from jujud.
	vers := version.Current
	vers.Patch++
	builtTools, err := sync.BuildToolsTarball(&vers.Number, "released")
	c.Assert(err, jc.ErrorIsNil)
	t, err := sync.SyncBuiltTools(s.targetStorage, "released", builtTools)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t.Version, gc.Equals, vers)
}

func (s *uploadSuite) assertUploadedTools(c *gc.C, t *coretools.Tools, expectSeries []string, stream string) {
	c.Assert(t.Version, gc.Equals, version.Current)
	expectRaw := downloadToolsRaw(c, t)

	list, err := envtools.ReadList(s.targetStorage, stream, version.Current.Major, version.Current.Minor)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list.AllSeries(), jc.SameContents, expectSeries)
	sort.Strings(expectSeries)
	c.Assert(list.AllSeries(), gc.DeepEquals, expectSeries)
	for _, t := range list {
		c.Logf("checking %s", t.URL)
		c.Assert(t.Version.Number, gc.Equals, version.Current.Number)
		actualRaw := downloadToolsRaw(c, t)
		c.Assert(string(actualRaw), gc.Equals, string(expectRaw))
	}
	metadata, err := envtools.ReadMetadata(s.targetStorage, stream)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 0)
}

// downloadToolsRaw downloads the supplied tools and returns the raw bytes.
func downloadToolsRaw(c *gc.C, t *coretools.Tools) []byte {
	resp, err := utils.GetValidatingHTTPClient().Get(t.URL)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	return buf.Bytes()
}

func bundleTools(c *gc.C) (version.Binary, string, error) {
	f, err := ioutil.TempFile("", "juju-tgz")
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	defer os.Remove(f.Name())

	return envtools.BundleTools(f, &version.Current.Number)
}

type badBuildSuite struct {
	env environs.Environ
	gitjujutesting.LoggingSuite
	gitjujutesting.CleanupSuite
	envtesting.ToolsFixture
}

var badGo = `
#!/bin/bash --norc
exit 1
`[1:]

func (s *badBuildSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}
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
	err := ioutil.WriteFile(path, []byte(badGo), 0755)
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

func (s *badBuildSuite) TestBundleToolsBadBuild(c *gc.C) {
	// Test that original bundleTools Func fails as expected
	vers, sha256Hash, err := bundleTools(c)
	c.Assert(vers, gc.DeepEquals, version.Binary{})
	c.Assert(sha256Hash, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `build command "go" failed: exit status 1; `)

	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))

	// Test that BundleTools func passes after it is
	// mocked out
	vers, sha256Hash, err = bundleTools(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers.Number, gc.Equals, version.Current.Number)
	c.Assert(sha256Hash, gc.Equals, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
}

func (s *badBuildSuite) TestUploadToolsBadBuild(c *gc.C) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	// Test that original Upload Func fails as expected
	t, err := sync.Upload(stor, "released", nil)
	c.Assert(t, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `build command "go" failed: exit status 1; `)

	// Test that Upload func passes after BundleTools func is mocked out
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
	t, err = sync.Upload(stor, "released", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	c.Assert(t.URL, gc.Not(gc.Equals), "")
}

func (s *badBuildSuite) TestBuildToolsBadBuild(c *gc.C) {
	// Test that original BuildToolsTarball fails
	builtTools, err := sync.BuildToolsTarball(nil, "released")
	c.Assert(err, gc.ErrorMatches, `build command "go" failed: exit status 1; `)
	c.Assert(builtTools, gc.IsNil)

	// Test that BuildToolsTarball func passes after BundleTools func is
	// mocked out
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
	builtTools, err = sync.BuildToolsTarball(nil, "released")
	c.Assert(builtTools.Version, gc.Equals, version.Current)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *uploadSuite) TestMockBundleTools(c *gc.C) {
	var (
		writer       io.Writer
		forceVersion *version.Number
		n            int
		p            bytes.Buffer
	)
	p.WriteString("Hello World")

	s.PatchValue(&envtools.BundleTools, func(writerArg io.Writer, forceVersionArg *version.Number) (vers version.Binary, sha256Hash string, err error) {
		writer = writerArg
		n, err = writer.Write(p.Bytes())
		c.Assert(err, jc.ErrorIsNil)
		forceVersion = forceVersionArg
		return
	})

	_, err := sync.BuildToolsTarball(&version.Current.Number, "released")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*forceVersion, gc.Equals, version.Current.Number)
	c.Assert(writer, gc.NotNil)
	c.Assert(n, gc.Equals, len(p.Bytes()))
}

func (s *uploadSuite) TestMockBuildTools(c *gc.C) {
	s.PatchValue(&version.Current, version.MustParseBinary("1.9.1-trusty-amd64"))
	buildToolsFunc := toolstesting.GetMockBuildTools(c)
	builtTools, err := buildToolsFunc(nil, "released")
	c.Assert(err, jc.ErrorIsNil)

	builtTools.Dir = ""

	expectedBuiltTools := &sync.BuiltTools{
		StorageName: "name",
		Version:     version.Current,
		Size:        127,
		Sha256Hash:  "6a19d08ca4913382ca86508aa38eb8ee5b9ae2d74333fe8d862c0f9e29b82c39",
	}
	c.Assert(builtTools, gc.DeepEquals, expectedBuiltTools)

	vers := version.MustParseBinary("1.5.3-trusty-amd64")
	builtTools, err = buildToolsFunc(&vers.Number, "released")
	c.Assert(err, jc.ErrorIsNil)
	builtTools.Dir = ""
	expectedBuiltTools = &sync.BuiltTools{
		StorageName: "name",
		Version:     vers,
		Size:        127,
		Sha256Hash:  "cad8ccedab8f26807ff379ddc2f2f78d9a7cac1276e001154cee5e39b9ddcc38",
	}
	c.Assert(builtTools, gc.DeepEquals, expectedBuiltTools)
}

func (s *uploadSuite) TestStorageToolsUploaderWriteMirrors(c *gc.C) {
	s.testStorageToolsUploaderWriteMirrors(c, envtools.WriteMirrors)
}

func (s *uploadSuite) TestStorageToolsUploaderDontWriteMirrors(c *gc.C) {
	s.testStorageToolsUploaderWriteMirrors(c, envtools.DoNotWriteMirrors)
}

func (s *uploadSuite) testStorageToolsUploaderWriteMirrors(c *gc.C, writeMirrors envtools.ShouldWriteMirrors) {
	storageDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)

	uploader := &sync.StorageToolsUploader{
		Storage:       stor,
		WriteMetadata: true,
		WriteMirrors:  writeMirrors,
	}

	err = uploader.UploadTools(
		"released",
		"released",
		&coretools.Tools{
			Version: version.Current,
			Size:    7,
			SHA256:  "ed7002b439e9ac845f22357d822bac1444730fbdb6016d3ec9432297b9ec9f73",
		}, []byte("content"))
	c.Assert(err, jc.ErrorIsNil)

	mirrorsPath := simplestreams.MirrorsPath(envtools.StreamsVersionV1) + simplestreams.UnsignedSuffix
	r, err := stor.Get(path.Join(storage.BaseToolsPath, mirrorsPath))
	if writeMirrors == envtools.WriteMirrors {
		c.Assert(err, jc.ErrorIsNil)
		data, err := ioutil.ReadAll(r)
		r.Close()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), jc.Contains, `"mirrors":`)
	} else {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

type mockToolsFinder struct{}

func (mockToolsFinder) FindTools(major int, stream string) (coretools.List, error) {
	return nil, coretools.ErrNoMatches
}
