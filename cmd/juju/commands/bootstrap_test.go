// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type BootstrapSuite struct {
	coretesting.FakeJujuHomeSuite
	gitjujutesting.MgoSuite
	envtesting.ToolsFixture
	mockBlockClient *mockBlockClient
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

	// Set version.Current to a known value, for which we
	// will make tools available. Individual tests may
	// override this.
	s.PatchValue(&version.Current, v100p64)

	// Set up a local source with tools.
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))

	s.mockBlockClient = &mockBlockClient{}
	s.PatchValue(&blockAPI, func(c *envcmd.EnvCommandBase) (block.BlockListAPI, error) {
		return s.mockBlockClient, nil
	})
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

type mockBlockClient struct {
	retry_count int
	num_retries int
}

func (c *mockBlockClient) List() ([]params.Block, error) {
	c.retry_count += 1
	if c.retry_count == 5 {
		return nil, fmt.Errorf("upgrade in progress")
	}
	if c.num_retries < 0 {
		return nil, fmt.Errorf("other error")
	}
	if c.retry_count < c.num_retries {
		return nil, fmt.Errorf("upgrade in progress")
	}
	return []params.Block{}, nil
}

func (c *mockBlockClient) Close() error {
	return nil
}

func (s *BootstrapSuite) TestBootstrapAPIReadyRetries(c *gc.C) {
	s.PatchValue(&bootstrapReadyPollDelay, 1*time.Millisecond)
	s.PatchValue(&bootstrapReadyPollCount, 5)
	defaultSeriesVersion := version.Current
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)
	for _, t := range []struct {
		num_retries int
		err         string
	}{
		{0, ""},                    // agent ready immediately
		{2, ""},                    // agent ready after 2 polls
		{6, "upgrade in progress"}, // agent ready after 6 polls but that's too long
		{-1, "other error"},        // another error is returned
	} {
		resetJujuHome(c, "devenv")

		s.mockBlockClient.num_retries = t.num_retries
		s.mockBlockClient.retry_count = 0
		_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "-e", "devenv")
		if t.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, t.err)
		}
		expectedRetries := t.num_retries
		if t.num_retries <= 0 {
			expectedRetries = 1
		}
		// Only retry maximum of bootstrapReadyPollCount times.
		if expectedRetries > 5 {
			expectedRetries = 5
		}
		c.Check(s.mockBlockClient.retry_count, gc.Equals, expectedRetries)
	}
}

func (s *BootstrapSuite) TestRunTests(c *gc.C) {
	for i, test := range bootstrapTests {
		c.Logf("\ntest %d: %s", i, test.info)
		restore := s.run(c, test)
		restore()
	}
}

type bootstrapTest struct {
	info string
	// binary version string used to set version.Current
	version string
	sync    bool
	args    []string
	err     string
	// binary version string for expected tools; if set, no default tools
	// will be uploaded before running the test.
	upload      string
	constraints constraints.Value
	placement   string
	hostArch    string
	keepBroken  bool
}

func (s *BootstrapSuite) run(c *gc.C, test bootstrapTest) (restore gitjujutesting.Restorer) {
	// Create home with dummy provider and remove all
	// of its envtools.
	env := resetJujuHome(c, "peckham")

	// Although we're testing PrepareEndpointsForCaching interactions
	// separately in the juju package, here we just ensure it gets
	// called with the right arguments.
	prepareCalled := false
	addrConnectedTo := "localhost:17070"
	restore = gitjujutesting.PatchValue(
		&prepareEndpointsForCaching,
		func(info configstore.EnvironInfo, hps [][]network.HostPort, addr network.HostPort) (_, _ []string, _ bool) {
			prepareCalled = true
			addrs, hosts, changed := juju.PrepareEndpointsForCaching(info, hps, addr)
			// Because we're bootstrapping the addresses will always
			// change, as there's no .jenv file saved yet.
			c.Assert(changed, jc.IsTrue)
			return addrs, hosts, changed
		},
	)

	if test.version != "" {
		useVersion := strings.Replace(test.version, "%LTS%", config.LatestLtsSeries(), 1)
		origVersion := version.Current
		version.Current = version.MustParseBinary(useVersion)
		restore = restore.Add(func() {
			version.Current = origVersion
		})
	}

	if test.hostArch != "" {
		origArch := arch.HostArch
		arch.HostArch = func() string {
			return test.hostArch
		}
		restore = restore.Add(func() {
			arch.HostArch = origArch
		})
	}

	// Run command and check for uploads.
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), envcmd.Wrap(new(BootstrapCommand)), test.args...)
	// Check for remaining operations/errors.
	if test.err != "" {
		err := <-errc
		c.Assert(err, gc.NotNil)
		stripped := strings.Replace(err.Error(), "\n", "", -1)
		c.Check(stripped, gc.Matches, test.err)
		return restore
	}
	if !c.Check(<-errc, gc.IsNil) {
		return restore
	}

	opBootstrap := (<-opc).(dummy.OpBootstrap)
	c.Check(opBootstrap.Env, gc.Equals, "peckham")
	c.Check(opBootstrap.Args.Constraints, gc.DeepEquals, test.constraints)
	c.Check(opBootstrap.Args.Placement, gc.Equals, test.placement)

	opFinalizeBootstrap := (<-opc).(dummy.OpFinalizeBootstrap)
	c.Check(opFinalizeBootstrap.Env, gc.Equals, "peckham")
	c.Check(opFinalizeBootstrap.InstanceConfig.Tools, gc.NotNil)
	if test.upload != "" {
		c.Check(opFinalizeBootstrap.InstanceConfig.Tools.Version.String(), gc.Equals, test.upload)
	}

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	// Check a CA cert/key was generated by reloading the environment.
	env, err = environs.NewFromName("peckham", store)
	c.Assert(err, jc.ErrorIsNil)
	_, hasCert := env.Config().CACert()
	c.Check(hasCert, jc.IsTrue)
	_, hasKey := env.Config().CAPrivateKey()
	c.Check(hasKey, jc.IsTrue)
	info, err := store.ReadInfo("peckham")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.NotNil)
	c.Assert(prepareCalled, jc.IsTrue)
	c.Assert(info.APIEndpoint().Addresses, gc.DeepEquals, []string{addrConnectedTo})
	return restore
}

var bootstrapTests = []bootstrapTest{{
	info: "no args, no error, no upload, no constraints",
}, {
	info: "bad --constraints",
	args: []string{"--constraints", "bad=wrong"},
	err:  `invalid value "bad=wrong" for flag --constraints: unknown constraint "bad"`,
}, {
	info: "conflicting --constraints",
	args: []string{"--constraints", "instance-type=foo mem=4G"},
	err:  `failed to bootstrap environment: ambiguous constraints: "instance-type" overlaps with "mem"`,
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
	err:     `failed to bootstrap environment: dummy.Bootstrap is broken`,
}, {
	info:        "constraints",
	args:        []string{"--constraints", "mem=4G cpu-cores=4"},
	constraints: constraints.MustParse("mem=4G cpu-cores=4"),
}, {
	info:        "unsupported constraint passed through but no error",
	args:        []string{"--constraints", "mem=4G cpu-cores=4 cpu-power=10"},
	constraints: constraints.MustParse("mem=4G cpu-cores=4 cpu-power=10"),
}, {
	info:        "--upload-tools uses arch from constraint if it matches current version",
	version:     "1.3.3-saucy-ppc64el",
	hostArch:    "ppc64el",
	args:        []string{"--upload-tools", "--constraints", "arch=ppc64el"},
	upload:      "1.3.3.1-raring-ppc64el", // from version.Current
	constraints: constraints.MustParse("arch=ppc64el"),
}, {
	info:     "--upload-tools rejects mismatched arch",
	version:  "1.3.3-saucy-amd64",
	hostArch: "amd64",
	args:     []string{"--upload-tools", "--constraints", "arch=ppc64el"},
	err:      `failed to bootstrap environment: cannot build tools for "ppc64el" using a machine running on "amd64"`,
}, {
	info:     "--upload-tools rejects non-supported arch",
	version:  "1.3.3-saucy-mips64",
	hostArch: "mips64",
	args:     []string{"--upload-tools"},
	err:      `failed to bootstrap environment: environment "peckham" of type dummy does not support instances running on "mips64"`,
}, {
	info:     "--upload-tools always bumps build number",
	version:  "1.2.3.4-raring-amd64",
	hostArch: "amd64",
	args:     []string{"--upload-tools"},
	upload:   "1.2.3.5-raring-amd64",
}, {
	info:      "placement",
	args:      []string{"--to", "something"},
	placement: "something",
}, {
	info:       "keep broken",
	args:       []string{"--keep-broken"},
	keepBroken: true,
}, {
	info: "additional args",
	args: []string{"anything", "else"},
	err:  `unrecognized args: \["anything" "else"\]`,
}, {
	info: "--agent-version with --upload-tools",
	args: []string{"--agent-version", "1.1.0", "--upload-tools"},
	err:  `--agent-version and --upload-tools can't be used together`,
}, {
	info: "--agent-version with --no-auto-upgrade",
	args: []string{"--agent-version", "1.1.0", "--no-auto-upgrade"},
	err:  `--agent-version and --no-auto-upgrade can't be used together`,
}, {
	info: "invalid --agent-version value",
	args: []string{"--agent-version", "foo"},
	err:  `invalid version "foo"`,
}, {
	info:    "agent-version doesn't match client version major",
	version: "1.3.3-saucy-ppc64el",
	args:    []string{"--agent-version", "2.3.0"},
	err:     `requested agent version major.minor mismatch`,
}, {
	info:    "agent-version doesn't match client version minor",
	version: "1.3.3-saucy-ppc64el",
	args:    []string{"--agent-version", "1.4.0"},
	err:     `requested agent version major.minor mismatch`,
}}

func (s *BootstrapSuite) TestRunEnvNameMissing(c *gc.C) {
	s.PatchValue(&getEnvName, func(*BootstrapCommand) string { return "" })

	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}))

	c.Check(err, gc.ErrorMatches, "the name of the environment must be specified")
}

const provisionalEnvs = `
environments:
    devenv:
        type: dummy
    cloudsigma:
        type: cloudsigma
    vsphere:
        type: vsphere
`

func (s *BootstrapSuite) TestCheckProviderProvisional(c *gc.C) {
	coretesting.WriteEnvironments(c, provisionalEnvs)

	err := checkProviderType("devenv")
	c.Assert(err, jc.ErrorIsNil)

	for name, flag := range provisionalProviders {
		// vsphere is disabled for gccgo. See lp:1440940.
		if name == "vsphere" && runtime.Compiler == "gccgo" {
			continue
		}
		c.Logf(" - trying %q -", name)
		err := checkProviderType(name)
		c.Check(err, gc.ErrorMatches, ".* provider is provisional .* set JUJU_DEV_FEATURE_FLAGS=.*")

		err = os.Setenv(osenv.JujuFeatureFlagEnvKey, flag)
		c.Assert(err, jc.ErrorIsNil)
		err = checkProviderType(name)
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *BootstrapSuite) TestBootstrapTwice(c *gc.C) {
	env := resetJujuHome(c, "devenv")
	defaultSeriesVersion := version.Current
	defaultSeriesVersion.Series = config.PreferredSeries(env.Config())
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)

	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "-e", "devenv")
	c.Assert(err, jc.ErrorIsNil)

	_, err = coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "-e", "devenv")
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")
}

type mockBootstrapInstance struct {
	instance.Instance
}

func (*mockBootstrapInstance) Addresses() ([]network.Address, error) {
	return []network.Address{{Value: "localhost"}}, nil
}

func (s *BootstrapSuite) TestSeriesDeprecation(c *gc.C) {
	ctx := s.checkSeriesArg(c, "--series")
	c.Check(coretesting.Stderr(ctx), gc.Equals,
		"Use of --series is obsolete. --upload-tools now expands to all supported series of the same operating system.\nBootstrap complete\n")
}

func (s *BootstrapSuite) TestUploadSeriesDeprecation(c *gc.C) {
	ctx := s.checkSeriesArg(c, "--upload-series")
	c.Check(coretesting.Stderr(ctx), gc.Equals,
		"Use of --upload-series is obsolete. --upload-tools now expands to all supported series of the same operating system.\nBootstrap complete\n")
}

func (s *BootstrapSuite) checkSeriesArg(c *gc.C, argVariant string) *cmd.Context {
	_bootstrap := &fakeBootstrapFuncs{}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return _bootstrap
	})
	resetJujuHome(c, "devenv")
	s.PatchValue(&allInstances, func(environ environs.Environ) ([]instance.Instance, error) {
		return []instance.Instance{&mockBootstrapInstance{}}, nil
	})

	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "--upload-tools", argVariant, "foo,bar")

	c.Assert(err, jc.ErrorIsNil)
	return ctx
}

// In the case where we cannot examine an environment, we want the
// error to propagate back up to the user.
func (s *BootstrapSuite) TestBootstrapPropagatesEnvErrors(c *gc.C) {
	//TODO(bogdanteleaga): fix this for windows once permissions are fixed
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: this is very platform specific. When/if we will support windows state machine, this will probably be rewritten.")
	}

	const envName = "devenv"
	env := resetJujuHome(c, envName)
	defaultSeriesVersion := version.Current
	defaultSeriesVersion.Series = config.PreferredSeries(env.Config())
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)
	s.PatchValue(&environType, func(string) (string, error) { return "", nil })

	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "-e", envName)
	c.Assert(err, jc.ErrorIsNil)

	// Change permissions on the jenv file to simulate some kind of
	// unexpected error when trying to read info from the environment
	jenvFile := gitjujutesting.HomePath(".juju", "environments", envName+".jenv")
	err = os.Chmod(jenvFile, os.FileMode(0200))
	c.Assert(err, jc.ErrorIsNil)

	// The second bootstrap should fail b/c of the propogated error
	_, err = coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "-e", envName)
	c.Assert(err, gc.ErrorMatches, "there was an issue examining the environment: .*")
}

func (s *BootstrapSuite) TestBootstrapCleansUpIfEnvironPrepFails(c *gc.C) {

	cleanupRan := false

	s.PatchValue(&environType, func(string) (string, error) { return "", nil })
	s.PatchValue(
		&environFromName,
		func(
			*cmd.Context,
			string,
			string,
			func(environs.Environ) error,
		) (environs.Environ, func(), error) {
			return nil, func() { cleanupRan = true }, fmt.Errorf("mock")
		},
	)

	ctx := coretesting.Context(c)
	_, errc := cmdtesting.RunCommand(ctx, envcmd.Wrap(new(BootstrapCommand)), "-e", "peckham")
	c.Check(<-errc, gc.Not(gc.IsNil))
	c.Check(cleanupRan, jc.IsTrue)
}

// When attempting to bootstrap, check that when prepare errors out,
// the code cleans up the created jenv file, but *not* any existing
// environment that may have previously been bootstrapped.
func (s *BootstrapSuite) TestBootstrapFailToPrepareDiesGracefully(c *gc.C) {

	destroyedEnvRan := false
	destroyedInfoRan := false

	// Mock functions
	mockDestroyPreparedEnviron := func(
		*cmd.Context,
		environs.Environ,
		configstore.Storage,
		string,
	) {
		destroyedEnvRan = true
	}

	mockDestroyEnvInfo := func(
		ctx *cmd.Context,
		cfgName string,
		store configstore.Storage,
		action string,
	) {
		destroyedInfoRan = true
	}

	mockEnvironFromName := func(
		ctx *cmd.Context,
		envName string,
		action string,
		_ func(environs.Environ) error,
	) (environs.Environ, func(), error) {
		// Always show that the environment is bootstrapped.
		return environFromNameProductionFunc(
			ctx,
			envName,
			action,
			func(env environs.Environ) error {
				return environs.ErrAlreadyBootstrapped
			})
	}

	mockPrepare := func(
		string,
		environs.BootstrapContext,
		configstore.Storage,
	) (environs.Environ, error) {
		return nil, fmt.Errorf("mock-prepare")
	}

	// Simulation: prepare should fail and we should only clean up the
	// jenv file. Any existing environment should not be destroyed.
	s.PatchValue(&destroyPreparedEnviron, mockDestroyPreparedEnviron)
	s.PatchValue(&environType, func(string) (string, error) { return "", nil })
	s.PatchValue(&environFromName, mockEnvironFromName)
	s.PatchValue(&environs.PrepareFromName, mockPrepare)
	s.PatchValue(&destroyEnvInfo, mockDestroyEnvInfo)

	ctx := coretesting.Context(c)
	_, errc := cmdtesting.RunCommand(ctx, envcmd.Wrap(new(BootstrapCommand)), "-e", "peckham")
	c.Check(<-errc, gc.ErrorMatches, ".*mock-prepare$")
	c.Check(destroyedEnvRan, jc.IsFalse)
	c.Check(destroyedInfoRan, jc.IsTrue)
}

func (s *BootstrapSuite) TestBootstrapJenvWarning(c *gc.C) {
	env := resetJujuHome(c, "devenv")
	defaultSeriesVersion := version.Current
	defaultSeriesVersion.Series = config.PreferredSeries(env.Config())
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	ctx := coretesting.Context(c)
	environs.PrepareFromName("devenv", envcmd.BootstrapContext(ctx), store)

	logger := "jenv.warning.test"
	var testWriter loggo.TestWriter
	loggo.RegisterWriter(logger, &testWriter, loggo.WARNING)
	defer loggo.RemoveWriter(logger)

	_, errc := cmdtesting.RunCommand(ctx, envcmd.Wrap(new(BootstrapCommand)), "-e", "devenv")
	c.Assert(<-errc, gc.IsNil)
	c.Assert(testWriter.Log(), jc.LogMatches, []string{"ignoring environments.yaml: using bootstrap config in .*"})
}

func (s *BootstrapSuite) TestInvalidLocalSource(c *gc.C) {
	s.PatchValue(&version.Current.Number, version.MustParse("1.2.0"))
	env := resetJujuHome(c, "devenv")

	// Bootstrap the environment with an invalid source.
	// The command returns with an error.
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "--metadata-source", c.MkDir())
	c.Check(err, gc.ErrorMatches, `failed to bootstrap environment: Juju cannot bootstrap because no tools are available for your environment(.|\n)*`)

	// Now check that there are no tools available.
	_, err = envtools.FindTools(
		env, version.Current.Major, version.Current.Minor, "released", coretools.Filter{})
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
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", im, cloudSpec, sourceStor)
	c.Assert(err, jc.ErrorIsNil)
	return sourceDir, im
}

func (s *BootstrapSuite) TestBootstrapCalledWithMetadataDir(c *gc.C) {
	sourceDir, _ := createImageMetadata(c)
	resetJujuHome(c, "devenv")

	_bootstrap := &fakeBootstrapFuncs{}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return _bootstrap
	})

	coretesting.RunCommand(
		c, envcmd.Wrap(&BootstrapCommand{}),
		"--metadata-source", sourceDir, "--constraints", "mem=4G",
	)
	c.Assert(_bootstrap.args.MetadataDir, gc.Equals, sourceDir)
}

func (s *BootstrapSuite) checkBootstrapWithVersion(c *gc.C, vers, expect string) {
	resetJujuHome(c, "devenv")

	_bootstrap := &fakeBootstrapFuncs{}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return _bootstrap
	})

	currentVersion := version.Current
	currentVersion.Major = 2
	currentVersion.Minor = 3
	s.PatchValue(&version.Current, currentVersion)
	coretesting.RunCommand(
		c, envcmd.Wrap(&BootstrapCommand{}),
		"--agent-version", vers,
	)
	c.Assert(_bootstrap.args.AgentVersion, gc.NotNil)
	c.Assert(*_bootstrap.args.AgentVersion, gc.Equals, version.MustParse(expect))
}

func (s *BootstrapSuite) TestBootstrapWithVersionNumber(c *gc.C) {
	s.checkBootstrapWithVersion(c, "2.3.4", "2.3.4")
}

func (s *BootstrapSuite) TestBootstrapWithBinaryVersionNumber(c *gc.C) {
	s.checkBootstrapWithVersion(c, "2.3.4-trusty-ppc64", "2.3.4")
}

func (s *BootstrapSuite) TestBootstrapWithNoAutoUpgrade(c *gc.C) {
	resetJujuHome(c, "devenv")

	_bootstrap := &fakeBootstrapFuncs{}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return _bootstrap
	})

	currentVersion := version.Current
	currentVersion.Major = 2
	currentVersion.Minor = 22
	currentVersion.Patch = 46
	currentVersion.Series = "trusty"
	currentVersion.Arch = "amd64"
	s.PatchValue(&version.Current, currentVersion)
	coretesting.RunCommand(
		c, envcmd.Wrap(&BootstrapCommand{}),
		"--no-auto-upgrade",
	)
	c.Assert(*_bootstrap.args.AgentVersion, gc.Equals, version.MustParse("2.22.46"))
}

func (s *BootstrapSuite) TestAutoSyncLocalSource(c *gc.C) {
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&version.Current.Number, version.MustParse("1.2.0"))
	env := resetJujuHome(c, "peckham")

	// Bootstrap the environment with the valid source.
	// The bootstrapping has to show no error, because the tools
	// are automatically synchronized.
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "--metadata-source", sourceDir)
	c.Assert(err, jc.ErrorIsNil)

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
	s.PatchValue(&version.Current, version.MustParseBinary(vers+"-"+series+"-"+arch.HostArch()))

	// Create home with dummy provider and remove all
	// of its envtools.
	return resetJujuHome(c, "devenv")
}

func (s *BootstrapSuite) TestAutoUploadAfterFailedSync(c *gc.C) {
	s.PatchValue(&version.Current.Series, config.LatestLtsSeries())
	s.setupAutoUploadTest(c, "1.7.3", "quantal")
	// Run command and check for that upload has been run for tools matching
	// the current juju version.
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), envcmd.Wrap(new(BootstrapCommand)), "-e", "devenv")
	c.Assert(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpBootstrap).Env, gc.Equals, "devenv")
	icfg := (<-opc).(dummy.OpFinalizeBootstrap).InstanceConfig
	c.Assert(icfg, gc.NotNil)
	c.Assert(icfg.Tools.Version.String(), gc.Equals, "1.7.3.1-raring-"+arch.HostArch())
}

func (s *BootstrapSuite) TestAutoUploadOnlyForDev(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")
	_, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), envcmd.Wrap(new(BootstrapCommand)))
	err := <-errc
	c.Assert(err, gc.ErrorMatches,
		"failed to bootstrap environment: Juju cannot bootstrap because no tools are available for your environment(.|\n)*")
}

func (s *BootstrapSuite) TestMissingToolsError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")

	_, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}))
	c.Assert(err, gc.ErrorMatches,
		"failed to bootstrap environment: Juju cannot bootstrap because no tools are available for your environment(.|\n)*")
}

func (s *BootstrapSuite) TestMissingToolsUploadFailedError(c *gc.C) {

	buildToolsTarballAlwaysFails := func(forceVersion *version.Number, stream string) (*sync.BuiltTools, error) {
		return nil, fmt.Errorf("an error")
	}

	s.setupAutoUploadTest(c, "1.7.3", "precise")
	s.PatchValue(&sync.BuildToolsTarball, buildToolsTarballAlwaysFails)

	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&BootstrapCommand{}), "-e", "devenv")

	c.Check(coretesting.Stderr(ctx), gc.Equals, fmt.Sprintf(`
Bootstrapping environment "devenv"
Starting new instance for initial state server
Building tools to upload (1.7.3.1-raring-%s)
`[1:], arch.HostArch()))
	c.Check(err, gc.ErrorMatches, "failed to bootstrap environment: cannot upload bootstrap tools: an error")
}

func (s *BootstrapSuite) TestBootstrapDestroy(c *gc.C) {
	resetJujuHome(c, "devenv")
	devVersion := version.Current
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion.Build = 1234
	s.PatchValue(&version.Current, devVersion)
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), envcmd.Wrap(new(BootstrapCommand)), "-e", "brokenenv")
	err := <-errc
	c.Assert(err, gc.ErrorMatches, "failed to bootstrap environment: dummy.Bootstrap is broken")
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

func (s *BootstrapSuite) TestBootstrapKeepBroken(c *gc.C) {
	resetJujuHome(c, "devenv")
	devVersion := version.Current
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion.Build = 1234
	s.PatchValue(&version.Current, devVersion)
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), envcmd.Wrap(new(BootstrapCommand)), "-e", "brokenenv", "--keep-broken")
	err := <-errc
	c.Assert(err, gc.ErrorMatches, "failed to bootstrap environment: dummy.Bootstrap is broken")
	done := false
	for !done {
		select {
		case op, ok := <-opc:
			if !ok {
				done = true
				break
			}
			switch op.(type) {
			case dummy.OpDestroy:
				c.Error("unexpected call to env.Destroy")
				break
			}
		default:
			break
		}
	}
}

// createToolsSource writes the mock tools and metadata into a temporary
// directory and returns it.
func createToolsSource(c *gc.C, versions []version.Binary) string {
	versionStrings := make([]string, len(versions))
	for i, vers := range versions {
		versionStrings[i] = vers.String()
	}
	source := c.MkDir()
	toolstesting.MakeTools(c, source, "released", versionStrings)
	return source
}

// resetJujuHome restores an new, clean Juju home environment without tools.
func resetJujuHome(c *gc.C, envName string) environs.Environ {
	jenvDir := gitjujutesting.HomePath(".juju", "environments")
	err := os.RemoveAll(jenvDir)
	c.Assert(err, jc.ErrorIsNil)
	coretesting.WriteEnvironments(c, envConfig)
	dummy.Reset()
	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.PrepareFromName(envName, envcmd.BootstrapContext(cmdtesting.NullContext(c)), store)
	c.Assert(err, jc.ErrorIsNil)
	return env
}

// checkTools check if the environment contains the passed envtools.
func checkTools(c *gc.C, env environs.Environ, expected []version.Binary) {
	list, err := envtools.FindTools(
		env, version.Current.Major, version.Current.Minor, "released", coretools.Filter{})
	c.Check(err, jc.ErrorIsNil)
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
	args bootstrap.BootstrapParams
}

func (fake *fakeBootstrapFuncs) EnsureNotBootstrapped(env environs.Environ) error {
	return nil
}

func (fake *fakeBootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args bootstrap.BootstrapParams) error {
	fake.args = args
	return nil
}
