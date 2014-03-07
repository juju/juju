// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	imtesting "launchpad.net/juju-core/environs/imagemetadata/testing"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	ttesting "launchpad.net/juju-core/environs/tools/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type BootstrapSuite struct {
	testbase.LoggingSuite
	coretesting.MgoSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&BootstrapSuite{})

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// Set up a local source with tools.
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)
}

func (s *BootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
	dummy.Reset()
}

type bootstrapRetryTest struct {
	info               string
	args               []string
	expectedAllowRetry []bool
	err                string
	// If version != "", version.Current will be
	// set to it for the duration of the test.
	version string
	// If addVersionToSource is true, then "version"
	// above will be populated in the tools source.
	addVersionToSource bool
}

var bootstrapRetryTests = []bootstrapRetryTest{{
	info:               "no tools uploaded, first check has no retries; no matching binary in source; sync fails with no second attempt",
	expectedAllowRetry: []bool{false},
	err:                "cannot find bootstrap tools: no matching tools available",
	version:            "1.16.0-precise-amd64",
}, {
	info:               "no tools uploaded, first check has no retries; matching binary in source; check after sync has retries",
	expectedAllowRetry: []bool{false, true},
	err:                "cannot find bootstrap tools: tools not found",
	version:            "1.16.0-precise-amd64",
	addVersionToSource: true,
}, {
	info:               "no tools uploaded, first check has no retries; no matching binary in source; check after upload has retries",
	expectedAllowRetry: []bool{false, true},
	err:                "cannot find bootstrap tools: tools not found",
	version:            "1.15.1-precise-amd64", // dev version to force upload
}, {
	info:               "new tools uploaded, so we want to allow retries to give them a chance at showing up",
	args:               []string{"--upload-tools"},
	expectedAllowRetry: []bool{true},
	err:                "cannot find bootstrap tools: no matching tools available",
}}

// Test test checks that bootstrap calls FindTools with the expected allowRetry flag.
func (s *BootstrapSuite) TestAllowRetries(c *gc.C) {
	for i, test := range bootstrapRetryTests {
		c.Logf("test %d: %s\n", i, test.info)
		s.runAllowRetriesTest(c, test)
	}
}

func (s *BootstrapSuite) runAllowRetriesTest(c *gc.C, test bootstrapRetryTest) {
	toolsVersions := envtesting.VAll
	if test.version != "" {
		testVersion := version.MustParseBinary(test.version)
		restore := testbase.PatchValue(&version.Current, testVersion)
		defer restore()
		if test.addVersionToSource {
			toolsVersions = append([]version.Binary{}, toolsVersions...)
			toolsVersions = append(toolsVersions, testVersion)
		}
	}
	_, fake := makeEmptyFakeHome(c)
	defer fake.Restore()
	sourceDir := createToolsSource(c, toolsVersions)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	var findToolsRetryValues []bool
	mockFindTools := func(cloudInst environs.ConfigGetter, majorVersion, minorVersion int,
		filter coretools.Filter, allowRetry bool) (list coretools.List, err error) {
		findToolsRetryValues = append(findToolsRetryValues, allowRetry)
		return nil, errors.NotFoundf("tools")
	}

	restore := envtools.TestingPatchBootstrapFindTools(mockFindTools)
	defer restore()

	_, errc := runCommand(nullContext(), new(BootstrapCommand), test.args...)
	err := <-errc
	c.Check(findToolsRetryValues, gc.DeepEquals, test.expectedAllowRetry)
	c.Check(err, gc.ErrorMatches, test.err)
}

func (s *BootstrapSuite) TestTest(c *gc.C) {
	s.PatchValue(&sync.Upload, mockUploadTools)
	for i, test := range bootstrapTests {
		c.Logf("\ntest %d: %s", i, test.info)
		test.run(c)
	}
}

type bootstrapTest struct {
	info string
	// binary version string used to set version.Current
	version string
	sync    bool
	args    []string
	err     string
	// binary version strings for expected tools; if set, no default tools
	// will be uploaded before running the test.
	uploads     []string
	constraints constraints.Value
}

func (test bootstrapTest) run(c *gc.C) {
	// Create home with dummy provider and remove all
	// of its envtools.
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	if test.version != "" {
		origVersion := version.Current
		version.Current = version.MustParseBinary(test.version)
		defer func() { version.Current = origVersion }()
	}

	uploadCount := len(test.uploads)
	if uploadCount == 0 {
		usefulVersion := version.Current
		usefulVersion.Series = env.Config().DefaultSeries()
		envtesting.AssertUploadFakeToolsVersions(c, env.Storage(), usefulVersion)
	}

	// Run command and check for uploads.
	opc, errc := runCommand(nullContext(), new(BootstrapCommand), test.args...)
	if uploadCount > 0 {
		for i := 0; i < uploadCount; i++ {
			c.Check((<-opc).(dummy.OpPutFile).Env, gc.Equals, "peckham")
		}
		list, err := envtools.FindTools(
			env, version.Current.Major, version.Current.Minor, coretools.Filter{}, envtools.DoNotAllowRetry)
		c.Check(err, gc.IsNil)
		c.Logf("found: " + list.String())
		urls := list.URLs()
		c.Check(urls, gc.HasLen, len(test.uploads))
		for _, v := range test.uploads {
			c.Logf("seeking: " + v)
			vers := version.MustParseBinary(v)
			_, found := urls[vers]
			c.Check(found, gc.Equals, true)
		}
	}

	// Check for remaining operations/errors.
	if test.err != "" {
		c.Check(<-errc, gc.ErrorMatches, test.err)
		return
	}
	if !c.Check(<-errc, gc.IsNil) {
		return
	}
	if len(test.uploads) > 0 {
		indexFile := (<-opc).(dummy.OpPutFile)
		c.Check(indexFile.FileName, gc.Equals, "tools/streams/v1/index.json")
		productFile := (<-opc).(dummy.OpPutFile)
		c.Check(productFile.FileName, gc.Equals, "tools/streams/v1/com.ubuntu.juju:released:tools.json")
	}
	opPutBootstrapVerifyFile := (<-opc).(dummy.OpPutFile)
	c.Check(opPutBootstrapVerifyFile.Env, gc.Equals, "peckham")
	c.Check(opPutBootstrapVerifyFile.FileName, gc.Equals, environs.VerificationFilename)

	opPutBootstrapInitFile := (<-opc).(dummy.OpPutFile)
	c.Check(opPutBootstrapInitFile.Env, gc.Equals, "peckham")
	c.Check(opPutBootstrapInitFile.FileName, gc.Equals, "provider-state")

	opBootstrap := (<-opc).(dummy.OpBootstrap)
	c.Check(opBootstrap.Env, gc.Equals, "peckham")
	c.Check(opBootstrap.Constraints, gc.DeepEquals, test.constraints)

	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	// Check a CA cert/key was generated by reloading the environment.
	env, err = environs.NewFromName("peckham", store)
	c.Assert(err, gc.IsNil)
	_, hasCert := env.Config().CACert()
	c.Check(hasCert, gc.Equals, true)
	_, hasKey := env.Config().CAPrivateKey()
	c.Check(hasKey, gc.Equals, true)
}

var bootstrapTests = []bootstrapTest{{
	info: "no args, no error, no uploads, no constraints",
}, {
	info: "bad arg",
	args: []string{"twiddle"},
	err:  `unrecognized args: \["twiddle"\]`,
}, {
	info: "bad --constraints",
	args: []string{"--constraints", "bad=wrong"},
	err:  `invalid value "bad=wrong" for flag --constraints: unknown constraint "bad"`,
}, {
	info: "bad --series",
	args: []string{"--series", "bad1"},
	err:  `invalid value "bad1" for flag --series: invalid series name "bad1"`,
}, {
	info: "lonely --series",
	args: []string{"--series", "fine"},
	err:  `--series requires --upload-tools`,
}, {
	info:    "bad environment",
	version: "1.2.3-precise-amd64",
	args:    []string{"-e", "brokenenv"},
	err:     `dummy.Bootstrap is broken`,
}, {
	info:        "constraints",
	args:        []string{"--constraints", "mem=4G cpu-cores=4"},
	constraints: constraints.MustParse("mem=4G cpu-cores=4"),
}, {
	info:    "--upload-tools picks all reasonable series",
	version: "1.2.3-saucy-amd64",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.1-saucy-amd64",   // from version.Current
		"1.2.3.1-raring-amd64",  // from env.Config().DefaultSeries()
		"1.2.3.1-precise-amd64", // from environs/config.DefaultSeries
	},
}, {
	info:    "--upload-tools only uploads each file once",
	version: "1.2.3-precise-amd64",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.1-raring-amd64",
		"1.2.3.1-precise-amd64",
	},
}, {
	info:    "--upload-tools rejects invalid series",
	version: "1.2.3-saucy-amd64",
	args:    []string{"--upload-tools", "--series", "ping,ping,pong"},
	err:     `invalid series "ping"`,
}, {
	info:    "--upload-tools always bumps build number",
	version: "1.2.3.4-raring-amd64",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.5-raring-amd64",
		"1.2.3.5-precise-amd64",
	},
}}

func (s *BootstrapSuite) TestBootstrapTwice(c *gc.C) {
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()
	defaultSeriesVersion := version.Current
	defaultSeriesVersion.Series = env.Config().DefaultSeries()

	ctx := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, ctx, nil)
	c.Check(code, gc.Equals, 0)

	ctx2 := coretesting.Context(c)
	code2 := cmd.Main(&BootstrapCommand{}, ctx2, nil)
	c.Check(code2, gc.Equals, 1)
	c.Check(coretesting.Stderr(ctx2), gc.Equals, "error: environment is already bootstrapped\n")
	c.Check(coretesting.Stdout(ctx2), gc.Equals, "")
}

func (s *BootstrapSuite) TestBootstrapJenvWarning(c *gc.C) {
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()
	defaultSeriesVersion := version.Current
	defaultSeriesVersion.Series = env.Config().DefaultSeries()

	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	ctx := coretesting.Context(c)
	environs.PrepareFromName("peckham", ctx, store)

	logger := "jenv.warning.test"
	testWriter := &loggo.TestWriter{}
	loggo.RegisterWriter(logger, testWriter, loggo.WARNING)
	defer loggo.RemoveWriter(logger)

	_, errc := runCommand(ctx, new(BootstrapCommand), "-e", "peckham")
	c.Assert(<-errc, gc.IsNil)
	c.Assert(testWriter.Log, jc.LogMatches, []string{"ignoring environments.yaml: using bootstrap config in .*"})
}

func (s *BootstrapSuite) TestInvalidLocalSource(c *gc.C) {
	s.PatchValue(&version.Current.Number, version.MustParse("1.2.0"))
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	// Bootstrap the environment with an invalid source.
	// The command returns with an error.
	ctx := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, ctx, []string{"--metadata-source", c.MkDir()})
	c.Check(code, gc.Equals, 1)

	// Now check that there are no tools available.
	_, err := envtools.FindTools(
		env, version.Current.Major, version.Current.Minor, coretools.Filter{}, envtools.DoNotAllowRetry)
	c.Assert(err, gc.FitsTypeOf, errors.NotFoundf(""))
}

// createImageMetadata creates some image metadata in a local directory.
func createImageMetadata(c *gc.C) (string, []*imagemetadata.ImageMetadata) {
	// Generate some image metadata.
	im := []*imagemetadata.ImageMetadata{
		{
			Id:         "1234",
			Arch:       "amd64",
			Version:    "13.04",
			RegionName: "region",
			Endpoint:   "endpoint",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	sourceDir := c.MkDir()
	sourceStor, err := filestorage.NewFileStorageWriter(sourceDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", im, cloudSpec, sourceStor)
	c.Assert(err, gc.IsNil)
	return sourceDir, im
}

// checkImageMetadata checks that the environment contains the expected image metadata.
func checkImageMetadata(c *gc.C, stor storage.StorageReader, expected []*imagemetadata.ImageMetadata) {
	metadata := imtesting.ParseMetadataFromStorage(c, stor)
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(expected[0], gc.DeepEquals, metadata[0])
}

func (s *BootstrapSuite) TestUploadLocalImageMetadata(c *gc.C) {
	sourceDir, expected := createImageMetadata(c)
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	// Bootstrap the environment with the valid source.
	ctx := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, ctx, []string{"--metadata-source", sourceDir})
	c.Check(code, gc.Equals, 0)
	c.Assert(imagemetadata.DefaultBaseURL, gc.Equals, imagemetadata.UbuntuCloudImagesURL)

	// Now check the image metadata has been uploaded.
	checkImageMetadata(c, env.Storage(), expected)
}

func (s *BootstrapSuite) TestAutoSyncLocalSource(c *gc.C) {
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&version.Current.Number, version.MustParse("1.2.0"))
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	// Bootstrap the environment with the valid source.
	// The bootstrapping has to show no error, because the tools
	// are automatically synchronized.
	ctx := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, ctx, []string{"--metadata-source", sourceDir})
	c.Check(code, gc.Equals, 0)

	// Now check the available tools which are the 1.2.0 envtools.
	checkTools(c, env, v120All)
}

func (s *BootstrapSuite) setupAutoUploadTest(c *gc.C, vers, series string) environs.Environ {
	s.PatchValue(&sync.Upload, mockUploadTools)
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	// Change the tools location to be the test location and also
	// the version and ensure their later restoring.
	// Set the current version to be something for which there are no tools
	// so we can test that an upload is forced.
	origVersion := version.Current
	version.Current.Number = version.MustParse(vers)
	version.Current.Series = series
	s.AddCleanup(func(*gc.C) { version.Current = origVersion })

	// Create home with dummy provider and remove all
	// of its envtools.
	env, fake := makeEmptyFakeHome(c)
	s.AddCleanup(func(*gc.C) { fake.Restore() })
	return env
}

func (s *BootstrapSuite) TestAutoUploadAfterFailedSync(c *gc.C) {
	otherSeries := "precise"
	if otherSeries == version.Current.Series {
		otherSeries = "raring"
	}
	env := s.setupAutoUploadTest(c, "1.7.3", otherSeries)
	// Run command and check for that upload has been run for tools matching the current juju version.
	opc, errc := runCommand(nullContext(), new(BootstrapCommand))
	c.Assert(<-errc, gc.IsNil)
	c.Assert((<-opc).(dummy.OpPutFile).Env, gc.Equals, "peckham")
	list, err := envtools.FindTools(env, version.Current.Major, version.Current.Minor, coretools.Filter{}, false)
	c.Assert(err, gc.IsNil)
	c.Logf("found: " + list.String())
	urls := list.URLs()
	c.Assert(urls, gc.HasLen, 2)
	expectedVers := []version.Binary{
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", otherSeries, version.Current.Arch)),
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", version.Current.Series, version.Current.Arch)),
	}
	for _, vers := range expectedVers {
		c.Logf("seeking: " + vers.String())
		_, found := urls[vers]
		c.Check(found, gc.Equals, true)
	}
}

func (s *BootstrapSuite) TestAutoUploadOnlyForDev(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")
	_, errc := runCommand(nullContext(), new(BootstrapCommand))
	err := <-errc
	c.Assert(err, gc.ErrorMatches, "cannot find bootstrap tools: no matching tools available")
}

func (s *BootstrapSuite) TestMissingToolsError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")
	context := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, context, nil)
	c.Assert(code, gc.Equals, 1)
	errText := context.Stderr.(*bytes.Buffer).String()
	errText = strings.Replace(errText, "\n", "", -1)
	expectedErrText := "error: cannot find bootstrap tools: no matching tools available"
	c.Assert(errText, gc.Matches, expectedErrText)
}

func uploadToolsAlwaysFails(stor storage.Storage, forceVersion *version.Number, series ...string) (*coretools.Tools, error) {
	return nil, fmt.Errorf("an error")
}

func (s *BootstrapSuite) TestMissingToolsUploadFailedError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.7.3", "precise")
	s.PatchValue(&sync.Upload, uploadToolsAlwaysFails)
	context := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, context, nil)
	c.Assert(code, gc.Equals, 1)
	errText := context.Stderr.(*bytes.Buffer).String()
	errText = strings.Replace(errText, "\n", "", -1)
	expectedErrText := "error: cannot find bootstrap tools: an error"
	c.Assert(errText, gc.Matches, expectedErrText)
}

func (s *BootstrapSuite) TestBootstrapDestroy(c *gc.C) {
	_, fake := makeEmptyFakeHome(c)
	defer fake.Restore()
	opc, errc := runCommand(nullContext(), new(BootstrapCommand), "-e", "brokenenv")
	err := <-errc
	c.Assert(err, gc.ErrorMatches, "dummy.Bootstrap is broken")
	var opDestroy *dummy.OpDestroy
	for opDestroy == nil {
		select {
		case op := <-opc:
			switch op := op.(type) {
			case dummy.OpDestroy:
				opDestroy = &op
			}
		default:
			c.Error("expected call to env.Destroy")
			return
		}
	}
	c.Assert(opDestroy.Error, gc.ErrorMatches, "dummy.Destroy is broken")
}

// createToolsSource writes the mock tools and metadata into a temporary
// directory and returns it.
func createToolsSource(c *gc.C, versions []version.Binary) string {
	versionStrings := make([]string, len(versions))
	for i, vers := range versions {
		versionStrings[i] = vers.String()
	}
	source := c.MkDir()
	ttesting.MakeTools(c, source, "releases", versionStrings)
	return source
}

// makeEmptyFakeHome creates a faked home without envtools.
func makeEmptyFakeHome(c *gc.C) (environs.Environ, *coretesting.FakeHome) {
	fake := coretesting.MakeFakeHome(c, envConfig)
	dummy.Reset()
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	env, err := environs.PrepareFromName("peckham", nullContext(), store)
	c.Assert(err, gc.IsNil)
	envtesting.RemoveAllTools(c, env)
	return env, fake
}

// checkTools check if the environment contains the passed envtools.
func checkTools(c *gc.C, env environs.Environ, expected []version.Binary) {
	list, err := envtools.FindTools(
		env, version.Current.Major, version.Current.Minor, coretools.Filter{}, envtools.DoNotAllowRetry)
	c.Check(err, gc.IsNil)
	c.Logf("found: " + list.String())
	urls := list.URLs()
	c.Check(urls, gc.HasLen, len(expected))
}

var (
	v100d64 = version.MustParseBinary("1.0.0-raring-amd64")
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v120d64 = version.MustParseBinary("1.2.0-raring-amd64")
	v120p64 = version.MustParseBinary("1.2.0-precise-amd64")
	v120q32 = version.MustParseBinary("1.2.0-quantal-i386")
	v120q64 = version.MustParseBinary("1.2.0-quantal-amd64")
	v190p32 = version.MustParseBinary("1.9.0-precise-i386")
	v190q64 = version.MustParseBinary("1.9.0-quantal-amd64")
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	v100All = []version.Binary{
		v100d64, v100p64, v100q64, v100q32,
	}
	v120All = []version.Binary{
		v120d64, v120p64, v120q64, v120q32,
	}
	vAll = []version.Binary{
		v100d64, v100p64, v100q32, v100q64,
		v120d64, v120p64, v120q32, v120q64,
		v190p32, v190q64,
		v200p64,
	}
)
