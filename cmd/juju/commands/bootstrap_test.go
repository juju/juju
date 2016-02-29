// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type BootstrapSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testing.MgoSuite
	envtesting.ToolsFixture
	mockBlockClient *mockBlockClient
	store           jujuclient.CredentialStore
	legacyMemStore  configstore.Storage
}

var _ = gc.Suite(&BootstrapSuite{})

func init() {
	dummyProvider, err := environs.Provider("dummy")
	if err != nil {
		panic(err)
	}
	environs.RegisterProvider("no-cloud-region-detection", noCloudRegionDetectionProvider{})
	environs.RegisterProvider("no-cloud-regions", noCloudRegionsProvider{dummyProvider})
	environs.RegisterProvider("no-credentials", noCredentialsProvider{})
	environs.RegisterProvider("many-credentials", manyCredentialsProvider{})
}

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.PatchValue(&simplestreams.SimplestreamsJujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// Set version.Current to a known value, for which we
	// will make tools available. Individual tests may
	// override this.
	s.PatchValue(&version.Current, v100p64.Number)
	s.PatchValue(&arch.HostArch, func() string { return v100p64.Arch })
	s.PatchValue(&series.HostSeries, func() string { return v100p64.Series })
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Ubuntu })

	// Set up a local source with tools.
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))

	s.mockBlockClient = &mockBlockClient{}
	s.PatchValue(&blockAPI, func(c *modelcmd.ModelCommandBase) (block.BlockListAPI, error) {
		if s.mockBlockClient.discoveringSpacesError > 0 {
			s.mockBlockClient.discoveringSpacesError -= 1
			return nil, errors.New("space discovery still in progress")
		}
		return s.mockBlockClient, nil
	})
	s.legacyMemStore = configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return s.legacyMemStore, nil
	})

	// TODO(wallyworld) - add test data when tests are improved
	s.store = jujuclienttesting.NewMemStore()
}

func (s *BootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
	dummy.Reset()
}

func (s *BootstrapSuite) newBootstrapCommand() cmd.Command {
	return modelcmd.Wrap(&bootstrapCommand{
		CredentialStore: s.store,
	})
}

type mockBlockClient struct {
	retryCount             int
	numRetries             int
	discoveringSpacesError int
}

func (c *mockBlockClient) List() ([]params.Block, error) {
	c.retryCount += 1
	if c.retryCount == 5 {
		return nil, fmt.Errorf("upgrade in progress")
	}
	if c.numRetries < 0 {
		return nil, fmt.Errorf("other error")
	}
	if c.retryCount < c.numRetries {
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
		numRetries int
		err        string
	}{
		{0, ""},                    // agent ready immediately
		{2, ""},                    // agent ready after 2 polls
		{6, "upgrade in progress"}, // agent ready after 6 polls but that's too long
		{-1, "other error"},        // another error is returned
	} {
		resetJujuXDGDataHome(c)
		dummy.Reset()
		s.legacyMemStore = configstore.NewMem()

		s.mockBlockClient.numRetries = t.numRetries
		s.mockBlockClient.retryCount = 0
		_, err := coretesting.RunCommand(
			c, s.newBootstrapCommand(),
			"devcontroller", "dummy", "--auto-upgrade",
		)
		if t.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, t.err)
		}
		expectedRetries := t.numRetries
		if t.numRetries <= 0 {
			expectedRetries = 1
		}
		// Only retry maximum of bootstrapReadyPollCount times.
		if expectedRetries > 5 {
			expectedRetries = 5
		}
		c.Check(s.mockBlockClient.retryCount, gc.Equals, expectedRetries)
	}
}

func (s *BootstrapSuite) TestBootstrapAPIReadyWaitsForSpaceDiscovery(c *gc.C) {
	defaultSeriesVersion := version.Current
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	defaultSeriesVersion.Build = 1234
	s.PatchValue(&version.Current, defaultSeriesVersion)
	resetJujuXDGDataHome(c)

	s.mockBlockClient.discoveringSpacesError = 2
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "devcontroller", "dummy", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockBlockClient.discoveringSpacesError, gc.Equals, 0)
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
	upload               string
	constraints          constraints.Value
	bootstrapConstraints constraints.Value
	placement            string
	hostArch             string
	keepBroken           bool
}

func (s *BootstrapSuite) patchVersionAndSeries(c *gc.C, hostSeries string) {
	resetJujuXDGDataHome(c)
	s.PatchValue(&series.HostSeries, func() string { return hostSeries })
	s.patchVersion(c)
}

func (s *BootstrapSuite) patchVersion(c *gc.C) {
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	num := version.Current
	num.Build = 1234
	s.PatchValue(&version.Current, num)
}

func (s *BootstrapSuite) run(c *gc.C, test bootstrapTest) testing.Restorer {
	// Create home with dummy provider and remove all
	// of its envtools.
	resetJujuXDGDataHome(c)
	dummy.Reset()

	addrConnectedTo := "localhost:17070"
	var restore testing.Restorer = func() {
		s.legacyMemStore = configstore.NewMem()
	}
	if test.version != "" {
		useVersion := strings.Replace(test.version, "%LTS%", config.LatestLtsSeries(), 1)
		v := version.MustParseBinary(useVersion)
		restore = restore.Add(testing.PatchValue(&version.Current, v.Number))
		restore = restore.Add(testing.PatchValue(&arch.HostArch, func() string { return v.Arch }))
		restore = restore.Add(testing.PatchValue(&series.HostSeries, func() string { return v.Series }))
	}

	if test.hostArch != "" {
		restore = restore.Add(testing.PatchValue(&arch.HostArch, func() string { return test.hostArch }))
	}

	controllerName := "peckham-controller"
	// Run command and check for uploads.
	args := append([]string{
		controllerName, "dummy",
		"--config", "default-series=raring",
	}, test.args...)
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), s.newBootstrapCommand(), args...)
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
	c.Check(opBootstrap.Env, gc.Equals, "peckham-controller")
	c.Check(opBootstrap.Args.EnvironConstraints, gc.DeepEquals, test.constraints)
	if test.bootstrapConstraints == (constraints.Value{}) {
		test.bootstrapConstraints = test.constraints
	}
	c.Check(opBootstrap.Args.BootstrapConstraints, gc.DeepEquals, test.bootstrapConstraints)
	c.Check(opBootstrap.Args.Placement, gc.Equals, test.placement)

	opFinalizeBootstrap := (<-opc).(dummy.OpFinalizeBootstrap)
	c.Check(opFinalizeBootstrap.Env, gc.Equals, "peckham-controller")
	c.Check(opFinalizeBootstrap.InstanceConfig.Tools, gc.NotNil)
	if test.upload != "" {
		c.Check(opFinalizeBootstrap.InstanceConfig.Tools.Version.String(), gc.Equals, test.upload)
	}

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)

	// The controller should be recorded with the specified
	// controller name, but the model should be called "admin".
	//
	// Check a CA cert/key was generated by reloading the controller.
	expectedBootstrappedControllerName := bootstrappedControllerName(controllerName)
	info, err := store.ReadInfo(expectedBootstrappedControllerName + ":" + controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.NotNil)
	cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Name(), gc.Equals, "peckham-controller")
	_, hasCert := cfg.CACert()
	c.Check(hasCert, jc.IsTrue)
	_, hasKey := cfg.CAPrivateKey()
	c.Check(hasKey, jc.IsTrue)
	c.Assert(info.APIEndpoint().Addresses, gc.DeepEquals, []string{addrConnectedTo})

	// Check controllers.yaml has controller
	endpoint := info.APIEndpoint()
	controllerStore := jujuclient.NewFileClientStore()
	controller, err := controllerStore.ControllerByName(expectedBootstrappedControllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller.CACert, gc.Equals, endpoint.CACert)
	c.Assert(controller.Servers, gc.DeepEquals, endpoint.Hostnames)
	c.Assert(controller.APIEndpoints, gc.DeepEquals, endpoint.Addresses)
	c.Assert(controller.ControllerUUID, gc.Equals, endpoint.ServerUUID)
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
	err:  `ambiguous constraints: "instance-type" overlaps with "mem"`,
}, {
	info:    "bad model",
	version: "1.2.3-%LTS%-amd64",
	args:    []string{"--config", "broken=Bootstrap Destroy", "--auto-upgrade"},
	err:     `failed to bootstrap model: dummy.Bootstrap is broken`,
}, {
	info:        "constraints",
	args:        []string{"--constraints", "mem=4G cpu-cores=4"},
	constraints: constraints.MustParse("mem=4G cpu-cores=4"),
}, {
	info:                 "bootstrap and environ constraints",
	args:                 []string{"--constraints", "mem=4G cpu-cores=4", "--bootstrap-constraints", "mem=8G"},
	constraints:          constraints.MustParse("mem=4G cpu-cores=4"),
	bootstrapConstraints: constraints.MustParse("mem=8G cpu-cores=4"),
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
	err:      `failed to bootstrap model: cannot build tools for "ppc64el" using a machine running on "amd64"`,
}, {
	info:     "--upload-tools rejects non-supported arch",
	version:  "1.3.3-saucy-mips64",
	hostArch: "mips64",
	args:     []string{"--upload-tools"},
	err:      `failed to bootstrap model: model "peckham-controller" of type dummy does not support instances running on "mips64"`,
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

func (s *BootstrapSuite) TestRunControllerNameMissing(c *gc.C) {
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand())
	c.Check(err, gc.ErrorMatches, "controller name and cloud name are required")
}

func (s *BootstrapSuite) TestRunCloudNameMissing(c *gc.C) {
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "my-controller")
	c.Check(err, gc.ErrorMatches, "controller name and cloud name are required")
}

func (s *BootstrapSuite) TestCheckProviderProvisional(c *gc.C) {
	err := checkProviderType("devcontroller")
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
	const controllerName = "dev"
	s.patchVersionAndSeries(c, "raring")

	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), controllerName, "dummy", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)

	_, err = coretesting.RunCommand(c, s.newBootstrapCommand(), controllerName, "dummy", "--auto-upgrade")
	c.Assert(err, gc.ErrorMatches, `controller "local.dev" already exists`)
}

func (s *BootstrapSuite) TestBootstrapSetsCurrentModel(c *gc.C) {
	s.patchVersionAndSeries(c, "raring")

	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "devcontroller", "dummy", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)
	currentController, err := modelcmd.ReadCurrentController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentController, gc.Equals, bootstrappedControllerName("devcontroller"))
}

type mockBootstrapInstance struct {
	instance.Instance
}

func (*mockBootstrapInstance) Addresses() ([]network.Address, error) {
	return []network.Address{{Value: "localhost"}}, nil
}

// In the case where we cannot examine the client store, we want the
// error to propagate back up to the user.
func (s *BootstrapSuite) TestBootstrapPropagatesStoreErrors(c *gc.C) {
	const controllerName = "devcontroller"
	bootstrappedControllerName(controllerName)
	s.patchVersionAndSeries(c, "raring")

	store := jujuclienttesting.NewStubStore()
	store.SetErrors(errors.New("oh noes"))
	cmd := &bootstrapCommand{CredentialStore: store}
	cmd.SetClientStore(store)
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(cmd), controllerName, "dummy", "--auto-upgrade")
	c.Assert(err, gc.ErrorMatches, `loading credentials: oh noes`)
}

// When attempting to bootstrap, check that when prepare errors out,
// bootstrap will stop immediately. Nothing will be destroyed.
func (s *BootstrapSuite) TestBootstrapFailToPrepareDiesGracefully(c *gc.C) {

	destroyed := false
	s.PatchValue(&environsDestroy, func(string, environs.Environ, configstore.Storage, jujuclient.ControllerRemover) error {
		destroyed = true
		return nil
	})

	s.PatchValue(&environsPrepare, func(
		environs.BootstrapContext,
		configstore.Storage,
		jujuclient.ClientStore,
		string,
		environs.PrepareForBootstrapParams,
	) (environs.Environ, error) {
		return nil, fmt.Errorf("mock-prepare")
	})

	ctx := coretesting.Context(c)
	_, errc := cmdtesting.RunCommand(
		ctx, s.newBootstrapCommand(),
		"devcontroller", "dummy",
	)
	c.Check(<-errc, gc.ErrorMatches, ".*mock-prepare$")
	c.Check(destroyed, jc.IsFalse)
}

func (s *BootstrapSuite) TestBootstrapAlreadyExists(c *gc.C) {
	const controllerName = "devcontroller"
	expectedBootstrappedName := bootstrappedControllerName(controllerName)
	s.patchVersionAndSeries(c, "raring")

	store := jujuclient.NewFileClientStore()
	err := store.UpdateController("local.devcontroller", jujuclient.ControllerDetails{
		CACert:         "x",
		ControllerUUID: "y",
	})
	c.Assert(err, jc.ErrorIsNil)

	ctx := coretesting.Context(c)
	_, errc := cmdtesting.RunCommand(ctx, s.newBootstrapCommand(), controllerName, "dummy", "--auto-upgrade")
	err = <-errc
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`controller %q already exists`, expectedBootstrappedName))
}

func (s *BootstrapSuite) TestInvalidLocalSource(c *gc.C) {
	s.PatchValue(&version.Current, version.MustParse("1.2.0"))
	resetJujuXDGDataHome(c)

	// Bootstrap the controller with an invalid source.
	// The command returns with an error.
	_, err := coretesting.RunCommand(
		c, s.newBootstrapCommand(), "--metadata-source", c.MkDir(),
		"devcontroller", "dummy",
	)
	c.Check(err, gc.ErrorMatches, `failed to bootstrap model: Juju cannot bootstrap because no tools are available for your model(.|\n)*`)
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
	resetJujuXDGDataHome(c)

	var bootstrap fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrap
	})

	coretesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--metadata-source", sourceDir, "--constraints", "mem=4G",
		"devcontroller", "dummy-cloud/region-1",
		"--config", "default-series=raring",
	)
	c.Assert(bootstrap.args.MetadataDir, gc.Equals, sourceDir)
}

func (s *BootstrapSuite) checkBootstrapWithVersion(c *gc.C, vers, expect string) {
	resetJujuXDGDataHome(c)

	var bootstrap fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrap
	})

	num := version.Current
	num.Major = 2
	num.Minor = 3
	s.PatchValue(&version.Current, num)
	coretesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--agent-version", vers,
		"devcontroller", "dummy-cloud/region-1",
		"--config", "default-series=raring",
	)
	c.Assert(bootstrap.args.AgentVersion, gc.NotNil)
	c.Assert(*bootstrap.args.AgentVersion, gc.Equals, version.MustParse(expect))
}

func (s *BootstrapSuite) TestBootstrapWithVersionNumber(c *gc.C) {
	s.checkBootstrapWithVersion(c, "2.3.4", "2.3.4")
}

func (s *BootstrapSuite) TestBootstrapWithBinaryVersionNumber(c *gc.C) {
	s.checkBootstrapWithVersion(c, "2.3.4-trusty-ppc64", "2.3.4")
}

func (s *BootstrapSuite) TestBootstrapWithAutoUpgrade(c *gc.C) {
	resetJujuXDGDataHome(c)

	var bootstrap fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrap
	})
	coretesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--auto-upgrade",
		"devcontroller", "dummy-cloud/region-1",
	)
	c.Assert(bootstrap.args.AgentVersion, gc.IsNil)
}

func (s *BootstrapSuite) TestAutoSyncLocalSource(c *gc.C) {
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&version.Current, version.MustParse("1.2.0"))
	resetJujuXDGDataHome(c)

	// Bootstrap the controller with the valid source.
	// The bootstrapping has to show no error, because the tools
	// are automatically synchronized.
	_, err := coretesting.RunCommand(
		c, s.newBootstrapCommand(), "--metadata-source", sourceDir,
		"devcontroller", "dummy-cloud/region-1",
	)
	c.Assert(err, jc.ErrorIsNil)

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)

	info, err := store.ReadInfo(bootstrappedControllerName("devcontroller") + ":devcontroller")
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// Now check the available tools which are the 1.2.0 envtools.
	checkTools(c, env, v120All)
}

func (s *BootstrapSuite) setupAutoUploadTest(c *gc.C, vers, ser string) {
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(c))
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	// Change the tools location to be the test location and also
	// the version and ensure their later restoring.
	// Set the current version to be something for which there are no tools
	// so we can test that an upload is forced.
	s.PatchValue(&version.Current, version.MustParse(vers))
	s.PatchValue(&series.HostSeries, func() string { return ser })

	// Create home with dummy provider and remove all
	// of its envtools.
	resetJujuXDGDataHome(c)
}

func (s *BootstrapSuite) TestAutoUploadAfterFailedSync(c *gc.C) {
	s.PatchValue(&series.HostSeries, func() string { return config.LatestLtsSeries() })
	s.setupAutoUploadTest(c, "1.7.3", "quantal")
	// Run command and check for that upload has been run for tools matching
	// the current juju version.
	opc, errc := cmdtesting.RunCommand(
		cmdtesting.NullContext(c), s.newBootstrapCommand(),
		"devcontroller", "dummy-cloud/region-1",
		"--config", "default-series=raring",
		"--auto-upgrade",
	)
	c.Assert(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpBootstrap).Env, gc.Equals, "devcontroller")
	icfg := (<-opc).(dummy.OpFinalizeBootstrap).InstanceConfig
	c.Assert(icfg, gc.NotNil)
	c.Assert(icfg.Tools.Version.String(), gc.Equals, "1.7.3.1-raring-"+arch.HostArch())
}

func (s *BootstrapSuite) TestAutoUploadOnlyForDev(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")
	_, errc := cmdtesting.RunCommand(
		cmdtesting.NullContext(c), s.newBootstrapCommand(),
		"devcontroller", "dummy-cloud/region-1",
	)
	err := <-errc
	c.Assert(err, gc.ErrorMatches,
		"failed to bootstrap model: Juju cannot bootstrap because no tools are available for your model(.|\n)*")
}

func (s *BootstrapSuite) TestMissingToolsError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")

	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(),
		"devcontroller", "dummy-cloud/region-1",
		"--config", "default-series=raring",
	)
	c.Assert(err, gc.ErrorMatches,
		"failed to bootstrap model: Juju cannot bootstrap because no tools are available for your model(.|\n)*")
}

func (s *BootstrapSuite) TestMissingToolsUploadFailedError(c *gc.C) {

	buildToolsTarballAlwaysFails := func(forceVersion *version.Number, stream string) (*sync.BuiltTools, error) {
		return nil, fmt.Errorf("an error")
	}

	s.setupAutoUploadTest(c, "1.7.3", "precise")
	s.PatchValue(&sync.BuildToolsTarball, buildToolsTarballAlwaysFails)

	ctx, err := coretesting.RunCommand(
		c, s.newBootstrapCommand(),
		"devcontroller", "dummy-cloud/region-1",
		"--config", "default-series=raring",
		"--config", "agent-stream=proposed",
		"--auto-upgrade",
	)

	c.Check(coretesting.Stderr(ctx), gc.Equals, fmt.Sprintf(`
Creating Juju controller "local.devcontroller" on dummy-cloud/region-1
Bootstrapping model "devcontroller"
Starting new instance for initial controller
Building tools to upload (1.7.3.1-raring-%s)
`[1:], arch.HostArch()))
	c.Check(err, gc.ErrorMatches, "failed to bootstrap model: cannot upload bootstrap tools: an error")
}

func (s *BootstrapSuite) TestBootstrapDestroy(c *gc.C) {
	resetJujuXDGDataHome(c)
	s.patchVersion(c)

	opc, errc := cmdtesting.RunCommand(
		cmdtesting.NullContext(c), s.newBootstrapCommand(),
		"devcontroller", "dummy-cloud/region-1",
		"--config", "broken=Bootstrap Destroy",
		"--auto-upgrade",
	)
	err := <-errc
	c.Assert(err, gc.ErrorMatches, "failed to bootstrap model: dummy.Bootstrap is broken")
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
	resetJujuXDGDataHome(c)
	s.patchVersion(c)

	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), s.newBootstrapCommand(),
		"--keep-broken",
		"devcontroller", "dummy-cloud/region-1",
		"--config", "broken=Bootstrap Destroy",
		"--auto-upgrade",
	)
	err := <-errc
	c.Assert(err, gc.ErrorMatches, "failed to bootstrap model: dummy.Bootstrap is broken")
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

func (s *BootstrapSuite) TestBootstrapUnknownCloudOrProvider(c *gc.C) {
	s.patchVersionAndSeries(c, "raring")
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "ctrl", "no-such-provider")
	c.Assert(err, gc.ErrorMatches, `cloud "no-such-provider" not found`)
}

func (s *BootstrapSuite) TestBootstrapProviderNoRegionDetection(c *gc.C) {
	s.patchVersionAndSeries(c, "raring")
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "ctrl", "no-cloud-region-detection")
	c.Assert(err, gc.ErrorMatches, `cloud "no-cloud-region-detection" not found`)
}

func (s *BootstrapSuite) TestBootstrapProviderNoRegions(c *gc.C) {
	ctx, err := coretesting.RunCommand(
		c, s.newBootstrapCommand(), "ctrl", "no-cloud-regions",
		"--config", "default-series=precise",
	)
	c.Check(coretesting.Stderr(ctx), gc.Matches, "Creating Juju controller \"local.ctrl\" on no-cloud-regions(.|\n)*")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestBootstrapCloudNoRegions(c *gc.C) {
	resetJujuXDGDataHome(c)
	ctx, err := coretesting.RunCommand(
		c, s.newBootstrapCommand(), "ctrl", "dummy-cloud-without-regions",
		"--config", "default-series=precise",
	)
	c.Check(coretesting.Stderr(ctx), gc.Matches, "Creating Juju controller \"local.ctrl\" on dummy-cloud-without-regions(.|\n)*")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestBootstrapCloudNoRegionsOneSpecified(c *gc.C) {
	resetJujuXDGDataHome(c)
	ctx, err := coretesting.RunCommand(
		c, s.newBootstrapCommand(), "ctrl", "dummy-cloud-without-regions/my-region",
		"--config", "default-series=precise",
	)
	// If the cloud doesn't have any regions defined, we still allow the
	// user to pass a region through. This enables the manual provider to
	// take the bootstrap-host from the region name, and later, will
	// enable the lxd provider to take the lxd remote from the region
	// name.
	c.Check(coretesting.Stderr(ctx), gc.Matches,
		"Creating Juju controller \"local.ctrl\" on dummy-cloud-without-regions/my-region(.|\n)*")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestBootstrapProviderNoCredentials(c *gc.C) {
	s.patchVersionAndSeries(c, "raring")
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "ctrl", "no-credentials")
	c.Assert(err, gc.ErrorMatches, `detecting credentials for "no-credentials" cloud provider: credentials not found`)
}

func (s *BootstrapSuite) TestBootstrapProviderManyCredentials(c *gc.C) {
	s.patchVersionAndSeries(c, "raring")
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "ctrl", "many-credentials")
	c.Assert(err, gc.ErrorMatches, ambiguousCredentialError.Error())
}

func (s *BootstrapSuite) TestBootstrapProviderDetectRegions(c *gc.C) {
	s.patchVersionAndSeries(c, "raring")
	_, err := coretesting.RunCommand(c, s.newBootstrapCommand(), "ctrl", "dummy/not-dummy")
	c.Assert(err, gc.ErrorMatches, `region "not-dummy" in cloud "dummy" not found \(expected one of \["dummy"\]\)`)
}

func (s *BootstrapSuite) TestBootstrapConfigFile(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("controller: not-a-bool\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.patchVersionAndSeries(c, "raring")
	_, err = coretesting.RunCommand(
		c, s.newBootstrapCommand(), "ctrl", "dummy",
		"--config", configFile,
	)
	c.Assert(err, gc.ErrorMatches, `controller: expected bool, got string.*`)
}

func (s *BootstrapSuite) TestBootstrapMultipleConfigFiles(c *gc.C) {
	tmpdir := c.MkDir()
	configFile1 := filepath.Join(tmpdir, "config-1.yaml")
	err := ioutil.WriteFile(configFile1, []byte(
		"controller: not-a-bool\nbroken: Bootstrap\n",
	), 0644)
	c.Assert(err, jc.ErrorIsNil)
	configFile2 := filepath.Join(tmpdir, "config-2.yaml")
	err = ioutil.WriteFile(configFile2, []byte(
		"controller: false\n",
	), 0644)

	s.patchVersionAndSeries(c, "raring")
	_, err = coretesting.RunCommand(
		c, s.newBootstrapCommand(), "ctrl", "dummy",
		"--auto-upgrade",
		// the second config file should replace attributes
		// with the same name from the first, but leave the
		// others alone.
		"--config", configFile1,
		"--config", configFile2,
	)
	c.Assert(err, gc.ErrorMatches, "failed to bootstrap model: dummy.Bootstrap is broken")
}

func (s *BootstrapSuite) TestBootstrapConfigFileAndAdHoc(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("controller: not-a-bool\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.patchVersionAndSeries(c, "raring")
	_, err = coretesting.RunCommand(
		c, s.newBootstrapCommand(), "ctrl", "dummy",
		"--auto-upgrade",
		// Configuration specified on the command line overrides
		// anything specified in files, no matter what the order.
		"--config", "controller=false",
		"--config", configFile,
	)
	c.Assert(err, jc.ErrorIsNil)
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

// resetJujuXDGDataHome restores an new, clean Juju home environment without tools.
func resetJujuXDGDataHome(c *gc.C) {
	jenvDir := testing.JujuXDGDataHomePath("models")
	err := os.RemoveAll(jenvDir)
	c.Assert(err, jc.ErrorIsNil)

	for _, path := range []string{
		jujuclient.JujuControllersPath(),
		jujuclient.JujuModelsPath(),
		jujuclient.JujuAccountsPath(),
	} {
		os.Remove(path)
	}

	cloudsPath := cloud.JujuPersonalCloudsPath()
	err = ioutil.WriteFile(cloudsPath, []byte(`
clouds:
    dummy-cloud:
        type: dummy
        regions:
            region-1:
            region-2:
    dummy-cloud-without-regions:
        type: dummy
`[1:]), 0644)
	c.Assert(err, jc.ErrorIsNil)
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

func (fake *fakeBootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args bootstrap.BootstrapParams) error {
	fake.args = args
	return nil
}

type noCloudRegionDetectionProvider struct {
	environs.EnvironProvider
}

type noCloudRegionsProvider struct {
	environs.EnvironProvider
}

func (noCloudRegionsProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

type noCredentialsProvider struct {
	environs.EnvironProvider
}

func (noCredentialsProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "region"}}, nil
}

func (noCredentialsProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

type manyCredentialsProvider struct {
	environs.EnvironProvider
}

func (manyCredentialsProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "region"}}, nil
}

func (manyCredentialsProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"one": {}, "two": {},
		},
	}, nil
}
