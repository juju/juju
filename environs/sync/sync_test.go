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
	"path/filepath"
	"sort"
	"testing"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	toolstesting "launchpad.net/juju-core/environs/tools/testing"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type syncSuite struct {
	coretesting.FakeJujuHomeSuite
	envtesting.ToolsFixture
	targetEnv    environs.Environ
	origVersion  version.Binary
	storage      storage.Storage
	localStorage string
}

var _ = gc.Suite(&syncSuite{})
var _ = gc.Suite(&uploadSuite{})
var _ = gc.Suite(&badBuildSuite{})

func (s *syncSuite) setUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.origVersion = version.Current
	// It's important that this be v1.8.x to match the test data.
	version.Current.Number = version.MustParse("1.8.3")

	// Create a target environments.yaml.
	envConfig := `
environments:
    test-target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`
	coretesting.WriteEnvironments(c, envConfig)
	var err error
	s.targetEnv, err = environs.PrepareFromName("test-target", coretesting.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	envtesting.RemoveAllTools(c, s.targetEnv)

	// Create a source storage.
	baseDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(baseDir)
	c.Assert(err, gc.IsNil)
	s.storage = stor

	// Create a local tools directory.
	s.localStorage = c.MkDir()

	// Populate both local and default tools locations with the public tools.
	versionStrings := make([]string, len(vAll))
	for i, vers := range vAll {
		versionStrings[i] = vers.String()
	}
	toolstesting.MakeTools(c, baseDir, "releases", versionStrings)
	toolstesting.MakeTools(c, s.localStorage, "releases", versionStrings)

	// Switch the default tools location.
	baseURL, err := s.storage.URL(storage.BaseToolsPath)
	c.Assert(err, gc.IsNil)
	s.PatchValue(&envtools.DefaultBaseURL, baseURL)
}

func (s *syncSuite) tearDownTest(c *gc.C) {
	dummy.Reset()
	version.Current = s.origVersion
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
}

var tests = []struct {
	description   string
	ctx           *sync.SyncContext
	source        bool
	tools         []version.Binary
	version       version.Number
	major         int
	minor         int
	expectMirrors bool
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
		tools: v1noDev,
	},
	{
		description: "copy all and dev from the dummy environment",
		ctx: &sync.SyncContext{
			AllVersions: true,
			Dev:         true,
		},
		tools: v1all,
	},
	{
		description: "write the mirrors files",
		ctx: &sync.SyncContext{
			Public: true,
		},
		tools:         v180all,
		expectMirrors: true,
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
			test.ctx.Target = s.targetEnv.Storage()

			err := sync.SyncTools(test.ctx)
			c.Assert(err, gc.IsNil)

			targetTools, err := envtools.FindTools(
				s.targetEnv, test.ctx.MajorVersion, test.ctx.MinorVersion, coretools.Filter{}, envtools.DoNotAllowRetry)
			c.Assert(err, gc.IsNil)
			assertToolsList(c, targetTools, test.tools)
			assertNoUnexpectedTools(c, s.targetEnv.Storage())
			assertMirrors(c, s.targetEnv.Storage(), test.expectMirrors)
		}()
	}
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
	v1noDev = append(v100all, v180all...)
	v1all   = append(v1noDev, v190all...)
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	v310p64 = version.MustParseBinary("3.1.0-precise-amd64")
	v320p64 = version.MustParseBinary("3.2.0-precise-amd64")
	vAll    = append(append(v1all, v200p64), v310p64, v320p64)
)

func assertNoUnexpectedTools(c *gc.C, stor storage.StorageReader) {
	// We only expect v1.x tools, no v2.x tools.
	list, err := envtools.ReadList(stor, 2, 0)
	if len(list) > 0 {
		c.Logf("got unexpected tools: %s", list)
	}
	c.Assert(err, gc.Equals, coretools.ErrNoMatches)
}

func assertToolsList(c *gc.C, list coretools.List, expected []version.Binary) {
	urls := list.URLs()
	c.Check(urls, gc.HasLen, len(expected))
	for _, vers := range expected {
		c.Assert(urls[vers], gc.Not(gc.Equals), "")
	}
}

func assertMirrors(c *gc.C, stor storage.StorageReader, expectMirrors bool) {
	r, err := storage.Get(stor, "tools/"+simplestreams.UnsignedMirror)
	if err == nil {
		defer r.Close()
	}
	if expectMirrors {
		data, err := ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		c.Assert(string(data), jc.Contains, `"mirrors":`)
	} else {
		c.Assert(err, gc.NotNil)
	}
}

type uploadSuite struct {
	env environs.Environ
	coretesting.FakeJujuHomeSuite
	envtesting.ToolsFixture
}

func (s *uploadSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	// We only want to use simplestreams to find any synced tools.
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	s.env, err = environs.Prepare(cfg, coretesting.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
}

func (s *uploadSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *uploadSuite) TestUpload(c *gc.C) {
	t, err := sync.Upload(s.env.Storage(), nil)
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	c.Assert(t.URL, gc.Not(gc.Equals), "")
	// TODO(waigani) Does this test need to download tools? If not,
	// sync.bundleTools can be mocked to improve test speed.
	dir := downloadTools(c, t)
	out, err := exec.Command(filepath.Join(dir, "jujud"), "version").CombinedOutput()
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Equals, version.Current.String()+"\n")
}

func (s *uploadSuite) TestUploadFakeSeries(c *gc.C) {
	seriesToUpload := "precise"
	if seriesToUpload == version.Current.Series {
		seriesToUpload = "raring"
	}
	t, err := sync.Upload(s.env.Storage(), nil, "quantal", seriesToUpload)
	c.Assert(err, gc.IsNil)
	s.assertUploadedTools(c, t, seriesToUpload)
}

func (s *uploadSuite) TestUploadAndForceVersion(c *gc.C) {
	// This test actually tests three things:
	//   the writing of the FORCE-VERSION file;
	//   the reading of the FORCE-VERSION file by the version package;
	//   and the reading of the version from jujud.
	vers := version.Current
	vers.Patch++
	t, err := sync.Upload(s.env.Storage(), &vers.Number)
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, vers)
}

func (s *uploadSuite) TestSyncTools(c *gc.C) {
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
	builtTools, err := sync.BuildToolsTarball(nil)
	c.Assert(err, gc.IsNil)
	t, err := sync.SyncBuiltTools(s.env.Storage(), builtTools)
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	c.Assert(t.URL, gc.Not(gc.Equals), "")
}

func (s *uploadSuite) TestSyncToolsFakeSeries(c *gc.C) {
	seriesToUpload := "precise"
	if seriesToUpload == version.Current.Series {
		seriesToUpload = "raring"
	}
	builtTools, err := sync.BuildToolsTarball(nil)
	c.Assert(err, gc.IsNil)

	t, err := sync.SyncBuiltTools(s.env.Storage(), builtTools, "quantal", seriesToUpload)
	c.Assert(err, gc.IsNil)
	s.assertUploadedTools(c, t, seriesToUpload)
}

func (s *uploadSuite) TestSyncAndForceVersion(c *gc.C) {
	// This test actually tests three things:
	//   the writing of the FORCE-VERSION file;
	//   the reading of the FORCE-VERSION file by the version package;
	//   and the reading of the version from jujud.
	vers := version.Current
	vers.Patch++
	builtTools, err := sync.BuildToolsTarball(&vers.Number)
	c.Assert(err, gc.IsNil)
	t, err := sync.SyncBuiltTools(s.env.Storage(), builtTools)
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, vers)
}

func (s *uploadSuite) assertUploadedTools(c *gc.C, t *coretools.Tools, uploadedSeries string) {
	c.Assert(t.Version, gc.Equals, version.Current)
	expectRaw := downloadToolsRaw(c, t)

	list, err := envtools.ReadList(s.env.Storage(), version.Current.Major, version.Current.Minor)
	c.Assert(err, gc.IsNil)
	c.Assert(list, gc.HasLen, 3)
	expectSeries := []string{"quantal", uploadedSeries, version.Current.Series}
	sort.Strings(expectSeries)
	c.Assert(list.AllSeries(), gc.DeepEquals, expectSeries)
	for _, t := range list {
		c.Logf("checking %s", t.URL)
		c.Assert(t.Version.Number, gc.Equals, version.Current.Number)
		actualRaw := downloadToolsRaw(c, t)
		c.Assert(string(actualRaw), gc.Equals, string(expectRaw))
	}
	metadata := toolstesting.ParseMetadataFromStorage(c, s.env.Storage(), false)
	c.Assert(metadata, gc.HasLen, 3)
	for i, tm := range metadata {
		c.Assert(tm.Release, gc.Equals, expectSeries[i])
		c.Assert(tm.Version, gc.Equals, version.Current.Number.String())
	}
}

// downloadTools downloads the supplied tools and extracts them into a
// new directory.
func downloadTools(c *gc.C, t *coretools.Tools) string {
	resp, err := utils.GetValidatingHTTPClient().Get(t.URL)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	cmd := exec.Command("tar", "xz")
	cmd.Dir = c.MkDir()
	cmd.Stdin = resp.Body
	out, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil, gc.Commentf(string(out)))
	return cmd.Dir
}

// downloadToolsRaw downloads the supplied tools and returns the raw bytes.
func downloadToolsRaw(c *gc.C, t *coretools.Tools) []byte {
	resp, err := utils.GetValidatingHTTPClient().Get(t.URL)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	c.Assert(err, gc.IsNil)
	return buf.Bytes()
}

func bundleTools(c *gc.C) (version.Binary, string, error) {
	f, err := ioutil.TempFile("", "juju-tgz")
	c.Assert(err, gc.IsNil)
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
	// We only want to use simplestreams to find any synced tools.
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	s.env, err = environs.Prepare(cfg, coretesting.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)

	// Mock go cmd
	testPath := c.MkDir()
	s.PatchEnvPathPrepend(testPath)
	path := filepath.Join(testPath, "go")
	err = ioutil.WriteFile(path, []byte(badGo), 0755)
	c.Assert(err, gc.IsNil)

	// Check mocked go cmd errors
	out, err := exec.Command("go").CombinedOutput()
	c.Assert(err, gc.ErrorMatches, "exit status 1")
	c.Assert(string(out), gc.Equals, "")
}

func (s *badBuildSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
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
	c.Assert(err, gc.IsNil)
	c.Assert(vers.Number, gc.Equals, version.Current.Number)
	c.Assert(sha256Hash, gc.Equals, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
}

func (s *badBuildSuite) TestUploadToolsBadBuild(c *gc.C) {
	// Test that original Upload Func fails as expected
	t, err := sync.Upload(s.env.Storage(), nil)
	c.Assert(t, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `build command "go" failed: exit status 1; `)

	// Test that Upload func passes after BundleTools func is mocked out
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
	t, err = sync.Upload(s.env.Storage(), nil)
	c.Assert(err, gc.IsNil)
	c.Assert(t.Version, gc.Equals, version.Current)
	c.Assert(t.URL, gc.Not(gc.Equals), "")
}

func (s *badBuildSuite) TestBuildToolsBadBuild(c *gc.C) {
	// Test that original BuildToolsTarball fails
	builtTools, err := sync.BuildToolsTarball(nil)
	c.Assert(err, gc.ErrorMatches, `build command "go" failed: exit status 1; `)
	c.Assert(builtTools, gc.IsNil)

	// Test that BuildToolsTarball func passes after BundleTools func is
	// mocked out
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
	builtTools, err = sync.BuildToolsTarball(nil)
	c.Assert(builtTools.Version, gc.Equals, version.Current)
	c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
		forceVersion = forceVersionArg
		return
	})

	_, err := sync.BuildToolsTarball(&version.Current.Number)
	c.Assert(err, gc.IsNil)
	c.Assert(*forceVersion, gc.Equals, version.Current.Number)
	c.Assert(writer, gc.NotNil)
	c.Assert(n, gc.Equals, len(p.Bytes()))
}

func (s *uploadSuite) TestMockBuildTools(c *gc.C) {
	s.PatchValue(&version.Current, version.MustParseBinary("1.9.1-trusty-amd64"))
	buildToolsFunc := toolstesting.GetMockBuildTools(c)
	builtTools, err := buildToolsFunc(nil)
	c.Assert(err, gc.IsNil)

	builtTools.Dir = ""

	expectedBuiltTools := &sync.BuiltTools{
		StorageName: "name",
		Version:     version.Current,
		Size:        127,
		Sha256Hash:  "6a19d08ca4913382ca86508aa38eb8ee5b9ae2d74333fe8d862c0f9e29b82c39",
	}
	c.Assert(builtTools, gc.DeepEquals, expectedBuiltTools)

	vers := version.MustParseBinary("1.5.3-trusty-amd64")
	builtTools, err = buildToolsFunc(&vers.Number)
	c.Assert(err, gc.IsNil)
	builtTools.Dir = ""
	expectedBuiltTools = &sync.BuiltTools{
		StorageName: "name",
		Version:     vers,
		Size:        127,
		Sha256Hash:  "cad8ccedab8f26807ff379ddc2f2f78d9a7cac1276e001154cee5e39b9ddcc38",
	}
	c.Assert(builtTools, gc.DeepEquals, expectedBuiltTools)
}
