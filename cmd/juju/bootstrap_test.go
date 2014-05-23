// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	imtesting "launchpad.net/juju-core/environs/imagemetadata/testing"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	toolstesting "launchpad.net/juju-core/environs/tools/testing"
	"launchpad.net/juju-core/juju/arch"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type BootstrapSuite struct {
	coretesting.FakeJujuHomeSuite
	coretesting.MgoSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&BootstrapSuite{})

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// Set up a local source with tools.
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
}

func (s *BootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.FakeJujuHomeSuite.TearDownSuite(c)
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
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

var noToolsAvailableMessage = "cannot upload bootstrap tools: Juju cannot bootstrap because no tools are available for your environment.*"
var toolsNotFoundMessage = "cannot find bootstrap tools: tools not found"

var bootstrapRetryTests = []bootstrapRetryTest{{
	info:               "no tools uploaded, first check has no retries; no matching binary in source; no second attempt",
	expectedAllowRetry: []bool{false},
	err:                noToolsAvailableMessage,
	version:            "1.16.0-precise-amd64",
}, {
	info:               "no tools uploaded, first check has no retries; matching binary in source; check after upload has retries",
	expectedAllowRetry: []bool{false, true},
	err:                toolsNotFoundMessage,
	version:            "1.17.0-precise-amd64", // dev version to force upload
	addVersionToSource: true,
}, {
	info:               "no tools uploaded, first check has no retries; no matching binary in source; check after upload has retries",
	expectedAllowRetry: []bool{false, true},
	err:                toolsNotFoundMessage,
	version:            "1.15.1-precise-amd64", // dev version to force upload
}, {
	info:               "new tools uploaded, so we want to allow retries to give them a chance at showing up",
	args:               []string{"--upload-tools"},
	expectedAllowRetry: []bool{true},
	err:                noToolsAvailableMessage,
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
		useVersion := strings.Replace(test.version, "%LTS%", config.LatestLtsSeries(), 1)
		testVersion := version.MustParseBinary(useVersion)
		s.PatchValue(&version.Current, testVersion)
		if test.addVersionToSource {
			toolsVersions = append([]version.Binary{}, toolsVersions...)
			toolsVersions = append(toolsVersions, testVersion)
		}
	}
	resetJujuHome(c)
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

	_, errc := runCommand(nullContext(c), envcmd.Wrap(new(BootstrapCommand)), test.args...)
	err := <-errc
	c.Check(findToolsRetryValues, gc.DeepEquals, test.expectedAllowRetry)
	stripped := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(stripped, gc.Matches, test.err)
}

func (s *BootstrapSuite) TestTest(c *gc.C) {
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
	placement   string
	hostArch    string
}

func (test bootstrapTest) run(c *gc.C) {
	// Create home with dummy provider and remove all
	// of its envtools.
	env := resetJujuHome(c)

	if test.version != "" {
		useVersion := strings.Replace(test.version, "%LTS%", config.LatestLtsSeries(), 1)
		origVersion := version.Current
		version.Current = version.MustParseBinary(useVersion)
		defer func() { version.Current = origVersion }()
	}

	if test.hostArch != "" {
		origVersion := arch.HostArch
		arch.HostArch = func() string {
			return test.hostArch
		}
		defer func() { arch.HostArch = origVersion }()
	}

	uploadCount := len(test.uploads)
	if uploadCount == 0 {
		usefulVersion := version.Current
		usefulVersion.Series = config.PreferredSeries(env.Config())
		envtesting.AssertUploadFakeToolsVersions(c, env.Storage(), usefulVersion)
	}

	// Run command and check for uploads.
	opc, errc := runCommand(nullContext(c), envcmd.Wrap(new(BootstrapCommand)), test.args...)
	// Check for remaining operations/errors.
	if test.err != "" {
		err := <-errc
		stripped := strings.Replace(err.Error(), "\n", "", -1)
		c.Check(stripped, gc.Matches, test.err)
		return
	}
	if !c.Check(<-errc, gc.IsNil) {
		return
	}

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
			v := strings.Replace(v, "%LTS%", config.LatestLtsSeries(), 1)
			c.Logf("seeking: " + v)
			vers := version.MustParseBinary(v)
			_, found := urls[vers]
			c.Check(found, gc.Equals, true)
		}
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
	c.Check(opBootstrap.Args.Constraints, gc.DeepEquals, test.constraints)
	c.Check(opBootstrap.Args.Placement, gc.Equals, test.placement)

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
	info: "bad --constraints",
	args: []string{"--constraints", "bad=wrong"},
	err:  `invalid value "bad=wrong" for flag --constraints: unknown constraint "bad"`,
}, {
	info: "conflicting --constraints",
	args: []string{"--constraints", "instance-type=foo mem=4G"},
	err:  `ambiguous constraints: "instance-type" overlaps with "mem"`,
}, {
	info: "bad --series",
	args: []string{"--series", "1bad1"},
	err:  `invalid value "1bad1" for flag --series: invalid series name "1bad1"`,
}, {
	info: "lonely --series",
	args: []string{"--series", "fine"},
	err:  `--series requires --upload-tools`,
}, {
	info: "lonely --upload-series",
	args: []string{"--upload-series", "fine"},
	err:  `--upload-series requires --upload-tools`,
}, {
	info: "--upload-series with --series",
	args: []string{"--upload-tools", "--upload-series", "foo", "--series", "bar"},
	err:  `--upload-series and --series can't be used together`,
}, {
	info:    "bad environment",
	version: "1.2.3-%LTS%-amd64",
	args:    []string{"-e", "brokenenv"},
	err:     `dummy.Bootstrap is broken`,
}, {
	info:        "constraints",
	args:        []string{"--constraints", "mem=4G cpu-cores=4"},
	constraints: constraints.MustParse("mem=4G cpu-cores=4"),
}, {
	info:        "unsupported constraint passed through but no error",
	args:        []string{"--constraints", "mem=4G cpu-cores=4 cpu-power=10"},
	constraints: constraints.MustParse("mem=4G cpu-cores=4 cpu-power=10"),
}, {
	info:    "--upload-tools picks all reasonable series",
	version: "1.2.3-saucy-amd64",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.1-saucy-amd64",  // from version.Current
		"1.2.3.1-raring-amd64", // from env.Config().DefaultSeries()
		"1.2.3.1-precise-amd64",
		"1.2.3.1-trusty-amd64",
	},
}, {
	info:     "--upload-tools uses arch from constraint if it matches current version",
	version:  "1.3.3-saucy-ppc64",
	hostArch: "ppc64",
	args:     []string{"--upload-tools", "--constraints", "arch=ppc64"},
	uploads: []string{
		"1.3.3.1-saucy-ppc64",  // from version.Current
		"1.3.3.1-raring-ppc64", // from env.Config().DefaultSeries()
		"1.3.3.1-precise-ppc64",
		"1.3.3.1-trusty-ppc64",
	},
	constraints: constraints.MustParse("arch=ppc64"),
}, {
	info:    "--upload-tools only uploads each file once",
	version: "1.2.3-%LTS%-amd64",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.1-raring-amd64",
		"1.2.3.1-precise-amd64",
		"1.2.3.1-trusty-amd64",
	},
}, {
	info:    "--upload-tools rejects invalid series",
	version: "1.2.3-saucy-amd64",
	args:    []string{"--upload-tools", "--upload-series", "ping,ping,pong"},
	err:     `invalid series "ping"`,
}, {
	info:     "--upload-tools rejects mismatched arch",
	version:  "1.3.3-saucy-amd64",
	hostArch: "amd64",
	args:     []string{"--upload-tools", "--constraints", "arch=ppc64"},
	err:      `cannot build tools for "ppc64" using a machine running on "amd64"`,
}, {
	info:     "--upload-tools rejects non-supported arch",
	version:  "1.3.3-saucy-arm64",
	hostArch: "arm64",
	args:     []string{"--upload-tools"},
	err:      `environment "peckham" of type dummy does not support instances running on "arm64"`,
}, {
	info:    "--upload-tools always bumps build number",
	version: "1.2.3.4-raring-amd64",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.5-raring-amd64",
		"1.2.3.5-precise-amd64",
		"1.2.3.5-trusty-amd64",
	},
}, {
	info:      "placement",
	args:      []string{"--to", "something"},
	placement: "something",
}, {
	info: "additional args",
	args: []string{"anything", "else"},
	err:  `unrecognized args: \["anything" "else"\]`,
}}

func (s *BootstrapSuite) TestBootstrapTwice(c *gc.C) {
	env := resetJujuHome(c)
	defaultSeriesVersion := version.Current
	defaultSeriesVersion.Series = config.PreferredSeries(env.Config())
	// Force a dev version by having an odd minor version number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Minor = 11
	s.PatchValue(&version.Current, defaultSeriesVersion)

	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}))
	c.Assert(err, gc.IsNil)

	_, err = coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}))
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")
}

func (s *BootstrapSuite) TestSeriesDeprecation(c *gc.C) {
	ctx := s.checkSeriesArg(c, "--series")
	c.Check(coretesting.Stderr(ctx), gc.Equals,
		"Use of --series is deprecated. Please use --upload-series instead.\n")
}

func (s *BootstrapSuite) TestNoDeprecationWithUploadSeries(c *gc.C) {
	ctx := s.checkSeriesArg(c, "--upload-series")
	c.Check(coretesting.Stderr(ctx), gc.Equals, "")
}

func (s *BootstrapSuite) checkSeriesArg(c *gc.C, argVariant string) *cmd.Context {
	_bootstrap := &fakeBootstrapFuncs{}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return _bootstrap
	})
	resetJujuHome(c)

	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "--upload-tools", argVariant, "foo,bar")

	c.Assert(err, gc.IsNil)
	c.Check(_bootstrap.uploadToolsSeries, gc.DeepEquals, []string{"foo", "bar"})
	return ctx
}

func (s *BootstrapSuite) TestBootstrapJenvWarning(c *gc.C) {
	env := resetJujuHome(c)
	defaultSeriesVersion := version.Current
	defaultSeriesVersion.Series = config.PreferredSeries(env.Config())
	// Force a dev version by having an odd minor version number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Minor = 11
	s.PatchValue(&version.Current, defaultSeriesVersion)

	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	ctx := coretesting.Context(c)
	environs.PrepareFromName("peckham", ctx, store)

	logger := "jenv.warning.test"
	testWriter := &loggo.TestWriter{}
	loggo.RegisterWriter(logger, testWriter, loggo.WARNING)
	defer loggo.RemoveWriter(logger)

	_, errc := runCommand(ctx, envcmd.Wrap(new(BootstrapCommand)), "-e", "peckham")
	c.Assert(<-errc, gc.IsNil)
	c.Assert(testWriter.Log, jc.LogMatches, []string{"ignoring environments.yaml: using bootstrap config in .*"})
}

func (s *BootstrapSuite) TestInvalidLocalSource(c *gc.C) {
	s.PatchValue(&version.Current.Number, version.MustParse("1.2.0"))
	env := resetJujuHome(c)

	// Bootstrap the environment with an invalid source.
	// The command returns with an error.
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "--metadata-source", c.MkDir())
	c.Check(err, gc.ErrorMatches, "cannot upload bootstrap tools: Juju "+
		"cannot bootstrap because no tools are available for your "+
		"environment(.|\n)*")

	// Now check that there are no tools available.
	_, err = envtools.FindTools(
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
	env := resetJujuHome(c)

	// Bootstrap the environment with the valid source.
	// Force a dev version by having an odd minor version number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion := version.Current
	devVersion.Minor = 11
	s.PatchValue(&version.Current, devVersion)

	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "--metadata-source", sourceDir)
	c.Assert(err, gc.IsNil)
	c.Assert(imagemetadata.DefaultBaseURL, gc.Equals, imagemetadata.UbuntuCloudImagesURL)

	// Now check the image metadata has been uploaded.
	checkImageMetadata(c, env.Storage(), expected)
}

func (s *BootstrapSuite) TestValidateConstraintsCalledWithMetadatasource(c *gc.C) {
	sourceDir, _ := createImageMetadata(c)
	resetJujuHome(c)
	var calledFuncs []string
	s.PatchValue(&uploadCustomMetadata, func(metadataDir string, env environs.Environ) error {
		c.Assert(metadataDir, gc.DeepEquals, sourceDir)
		calledFuncs = append(calledFuncs, "uploadCustomMetadata")
		return nil
	})
	s.PatchValue(&validateConstraints, func(cons constraints.Value, env environs.Environ) error {
		c.Assert(cons, gc.DeepEquals, constraints.MustParse("mem=4G"))
		calledFuncs = append(calledFuncs, "validateConstraints")
		return nil
	})
	_, err := coretesting.RunCommand(
		c, envcmd.Wrap(&BootstrapCommand{}), "--metadata-source", sourceDir, "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)
	c.Assert(calledFuncs, gc.DeepEquals, []string{"uploadCustomMetadata", "validateConstraints"})
}

func (s *BootstrapSuite) TestValidateConstraintsCalledWithoutMetadatasource(c *gc.C) {
	validateCalled := 0
	s.PatchValue(&validateConstraints, func(cons constraints.Value, env environs.Environ) error {
		c.Assert(cons, gc.DeepEquals, constraints.MustParse("mem=4G"))
		validateCalled++
		return nil
	})
	resetJujuHome(c)
	_, err := coretesting.RunCommand(
		c, envcmd.Wrap(&BootstrapCommand{}), "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)
	c.Assert(validateCalled, gc.Equals, 1)
}

func (s *BootstrapSuite) TestAutoSyncLocalSource(c *gc.C) {
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&version.Current.Number, version.MustParse("1.2.0"))
	env := resetJujuHome(c)

	// Bootstrap the environment with the valid source.
	// The bootstrapping has to show no error, because the tools
	// are automatically synchronized.
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "--metadata-source", sourceDir)
	c.Assert(err, gc.IsNil)

	// Now check the available tools which are the 1.2.0 envtools.
	checkTools(c, env, v120All)
}

func (s *BootstrapSuite) setupAutoUploadTest(c *gc.C, vers, series string) environs.Environ {
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
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
	return resetJujuHome(c)
}

func (s *BootstrapSuite) TestAutoUploadAfterFailedSync(c *gc.C) {
	s.PatchValue(&version.Current.Series, config.LatestLtsSeries())
	otherSeries := "quantal"

	env := s.setupAutoUploadTest(c, "1.7.3", otherSeries)
	// Run command and check for that upload has been run for tools matching the current juju version.
	opc, errc := runCommand(nullContext(c), envcmd.Wrap(new(BootstrapCommand)))
	c.Assert(<-errc, gc.IsNil)
	c.Assert((<-opc).(dummy.OpPutFile).Env, gc.Equals, "peckham")
	list, err := envtools.FindTools(env, version.Current.Major, version.Current.Minor, coretools.Filter{}, false)
	c.Assert(err, gc.IsNil)
	c.Logf("found: " + list.String())
	urls := list.URLs()

	// We expect:
	//     supported LTS series precise, trusty,
	//     the specified series (quantal),
	//     and the environment's default series (raring).
	expectedVers := []version.Binary{
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", "quantal", version.Current.Arch)),
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", "raring", version.Current.Arch)),
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", "precise", version.Current.Arch)),
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", "trusty", version.Current.Arch)),
	}
	c.Assert(urls, gc.HasLen, len(expectedVers))
	for _, vers := range expectedVers {
		c.Logf("seeking: " + vers.String())
		_, found := urls[vers]
		c.Check(found, gc.Equals, true)
	}
}

func (s *BootstrapSuite) TestAutoUploadOnlyForDev(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")
	_, errc := runCommand(nullContext(c), envcmd.Wrap(new(BootstrapCommand)))
	err := <-errc
	stripped := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(stripped, gc.Matches, noToolsAvailableMessage)
}

func (s *BootstrapSuite) TestMissingToolsError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")

	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}))
	c.Assert(err, gc.ErrorMatches, "cannot upload bootstrap tools: Juju "+
		"cannot bootstrap because no tools are available for your "+
		"environment(.|\n)*")
}

func uploadToolsAlwaysFails(stor storage.Storage, forceVersion *version.Number, series ...string) (*coretools.Tools, error) {
	return nil, fmt.Errorf("an error")
}

func (s *BootstrapSuite) TestMissingToolsUploadFailedError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.7.3", "precise")
	s.PatchValue(&sync.Upload, uploadToolsAlwaysFails)

	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}))

	c.Check(coretesting.Stderr(ctx), gc.Matches,
		"uploading tools for series \\[precise .* raring\\]\n")
	c.Check(err, gc.ErrorMatches, "cannot upload bootstrap tools: an error")
}

func (s *BootstrapSuite) TestBootstrapDestroy(c *gc.C) {
	resetJujuHome(c)
	devVersion := version.Current
	// Force a dev version by having an odd minor version number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion.Minor = 11
	s.PatchValue(&version.Current, devVersion)
	opc, errc := runCommand(nullContext(c), envcmd.Wrap(new(BootstrapCommand)), "-e", "brokenenv")
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
	toolstesting.MakeTools(c, source, "releases", versionStrings)
	return source
}

// resetJujuHome restores an new, clean Juju home environment without tools.
func resetJujuHome(c *gc.C) environs.Environ {
	jenvDir := coretesting.HomePath(".juju", "environments")
	err := os.RemoveAll(jenvDir)
	c.Assert(err, gc.IsNil)
	coretesting.WriteEnvironments(c, envConfig)
	dummy.Reset()
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	env, err := environs.PrepareFromName("peckham", nullContext(c), store)
	c.Assert(err, gc.IsNil)
	envtesting.RemoveAllTools(c, env)
	return env
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
	v120t32 = version.MustParseBinary("1.2.0-trusty-i386")
	v120t64 = version.MustParseBinary("1.2.0-trusty-amd64")
	v190p32 = version.MustParseBinary("1.9.0-precise-i386")
	v190q64 = version.MustParseBinary("1.9.0-quantal-amd64")
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	v100All = []version.Binary{
		v100d64, v100p64, v100q64, v100q32,
	}
	v120All = []version.Binary{
		v120d64, v120p64, v120q64, v120q32, v120t32, v120t64,
	}
	v190All = []version.Binary{
		v190p32, v190q64,
	}
	v200All = []version.Binary{
		v200p64,
	}
	vAll = joinBinaryVersions(v100All, v120All, v190All, v200All)
)

func joinBinaryVersions(versions ...[]version.Binary) []version.Binary {
	var all []version.Binary
	for _, versions := range versions {
		all = append(all, versions...)
	}
	return all
}

// TODO(menn0): This fake BootstrapInterface implementation is
// currently quite minimal but could be easily extended to cover more
// test scenarios. This could help improve some of the tests in this
// file which execute large amounts of external functionality.
type fakeBootstrapFuncs struct {
	uploadToolsSeries []string
}

func (fake *fakeBootstrapFuncs) EnsureNotBootstrapped(env environs.Environ) error {
	return nil
}

func (fake *fakeBootstrapFuncs) UploadTools(ctx environs.BootstrapContext, env environs.Environ, toolsArch *string, forceVersion bool, bootstrapSeries ...string) error {
	fake.uploadToolsSeries = bootstrapSeries
	return nil
}

func (fake fakeBootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams) error {
	return nil
}
