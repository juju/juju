// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"
	k8scmd "k8s.io/client-go/tools/clientcmd"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/provider/openstack"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testcharms"
)

func runCommandWithDummyProvider(ctx *cmd.Context, com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
	if ctx == nil {
		panic("ctx == nil")
	}
	errc = make(chan error, 1)
	opc = make(chan dummy.Operation, 200)
	dummy.Listen(opc)
	go func() {
		defer func() {
			// signal that we're done with this ops channel.
			dummy.Listen(nil)
			// now that dummy is no longer going to send ops on
			// this channel, close it to signal to test cases
			// that we are done.
			close(opc)
		}()

		if err := cmdtesting.InitCommand(com, args); err != nil {
			errc <- err
			return
		}

		errc <- com.Run(ctx)
	}()
	return
}

type BootstrapSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	store *jujuclient.MemStore
	tw    loggo.TestWriter

	bootstrapCmd bootstrapCommand
	clock        *testclock.Clock
}

var _ = gc.Suite(&BootstrapSuite{})

func init() {
	dummyProvider, err := environs.Provider("dummy")
	if err != nil {
		panic(err)
	}
	environs.RegisterProvider("no-cloud-region-detection", noCloudRegionDetectionProvider{})
	environs.RegisterProvider("no-cloud-regions", noCloudRegionsProvider{
		dummyProvider.(environs.CloudEnvironProvider)})
	environs.RegisterProvider("no-credentials", noCredentialsProvider{})
	environs.RegisterProvider("many-credentials", manyCredentialsProvider{
		dummyProvider.(environs.CloudEnvironProvider)})
}

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	// Set jujuversion.Current to a known value, for which we
	// will make tools available. Individual tests may
	// override this.
	s.PatchValue(&jujuversion.Current, v100u64.Number)
	s.PatchValue(&arch.HostArch, func() string { return v100u64.Arch })
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	// Ensure KUBECONFIG doesn't interfere with tests.
	s.PatchEnvironment(k8scmd.RecommendedConfigPathEnvVar, filepath.Join(c.MkDir(), "config"))

	s.PatchEnvironment("JUJU_BOOTSTRAP_MODEL", "")

	// Set up a local source with tools.
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	// NOTE(axw) we cannot patch BundleTools here, as the "gc.C" argument
	// is invalidated once this method returns.
	s.PatchValue(&envtools.BundleTools, func(bool, io.Writer, func(semversion.Number) semversion.Number) (semversion.Binary, semversion.Number, bool, string, error) {
		panic("tests must call setupAutoUploadTest or otherwise patch envtools.BundleTools")
	})

	s.PatchValue(&waitForAgentInitialisation, func(environs.BootstrapContext, *modelcmd.ModelCommandBase, bool, string) error {
		return nil
	})

	// TODO(wallyworld) - add test data when tests are improved
	s.store = jujuclienttesting.MinimalStore()

	// Write bootstrap command logs to an in-memory buffer,
	// so we can inspect the output in tests.
	s.tw.Clear()
	c.Assert(loggo.RegisterWriter("bootstrap-test", &s.tw), jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		_, err := loggo.RemoveWriter("bootstrap-test")
		c.Assert(err, jc.ErrorIsNil)
	})

	s.clock = testclock.NewClock(time.Now())
	s.bootstrapCmd = bootstrapCommand{clock: s.clock}
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *BootstrapSuite) newBootstrapCommand() cmd.Command {
	c := s.bootstrapCmd
	c.SetClientStore(s.store)
	return modelcmd.Wrap(&c,
		modelcmd.WrapSkipModelFlags,
		modelcmd.WrapSkipDefaultModel,
	)
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
	// binary version string used to set jujuversion.Current
	version   string
	args      []string
	err       string
	silentErr bool
	logs      jc.SimpleMessages
	// binary version string for expected tools; if set, no default tools
	// will be uploaded before running the test.
	upload               string
	constraints          constraints.Value
	bootstrapConstraints constraints.Value
	placement            string
	hostArch             string
	keepBroken           bool
}

func (s *BootstrapSuite) patchVersion(c *gc.C) {
	resetJujuXDGDataHome(c)

	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	num := jujuversion.Current
	num.Build = 1234
	s.PatchValue(&jujuversion.Current, num)
}

func (s *BootstrapSuite) run(c *gc.C, test bootstrapTest) testing.Restorer {
	// Create home with dummy provider and remove all
	// of its envtools.
	s.setupAutoUploadTest(c, "1.0.0", "jammy")
	s.tw.Clear()

	var restore testing.Restorer = func() {
		s.store = jujuclienttesting.MinimalStore()
	}
	bootstrapVersion := v100u64
	if test.version != "" {
		bootstrapVersion = semversion.MustParseBinary(test.version)
		restore = restore.Add(testing.PatchValue(&jujuversion.Current, bootstrapVersion.Number))
		restore = restore.Add(testing.PatchValue(&arch.HostArch, func() string { return bootstrapVersion.Arch }))
		bootstrapVersion.Build = 1
		if test.upload != "" {
			uploadVers := semversion.MustParseBinary(test.upload)
			bootstrapVersion.Number = uploadVers.Number
		}
		restore = restore.Add(testing.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(bootstrapVersion.Number)))
	}

	if test.hostArch != "" {
		restore = restore.Add(testing.PatchEnvironment("GOARCH", test.hostArch))
	}

	controllerName := "peckham-controller"
	cloudName := "dummy"

	// Run command and check for uploads.
	args := append([]string{
		cloudName, controllerName,
		"--config", "default-base=ubuntu@22.04",
	}, test.args...)
	opc, errc := runCommandWithDummyProvider(cmdtesting.Context(c), s.newBootstrapCommand(), args...)
	var err error
	select {
	case err = <-errc:
	case <-time.After(3 * testing.LongWait):
		c.Fatal("timed out")
	}
	c.Check(s.tw.Log(), jc.LogMatches, test.logs)
	// Check for remaining operations/errors.
	if test.silentErr {
		c.Assert(err, gc.Equals, cmd.ErrSilent)
		return restore
	} else if test.err != "" {
		c.Assert(err, gc.NotNil)
		stripped := strings.Replace(err.Error(), "\n", "", -1)
		c.Check(stripped, gc.Matches, test.err)
		return restore
	}
	if !c.Check(err, gc.IsNil) {
		return restore
	}

	op, ok := <-opc
	c.Assert(ok, gc.Equals, true)
	opBootstrap := op.(dummy.OpBootstrap)
	c.Check(opBootstrap.Env, gc.Equals, bootstrap.ControllerModelName)
	c.Check(opBootstrap.Args.ModelConstraints, gc.DeepEquals, test.constraints)
	if test.bootstrapConstraints == (constraints.Value{}) {
		test.bootstrapConstraints = test.constraints
	}
	if test.bootstrapConstraints.Mem == nil {
		mem := uint64(3584)
		test.bootstrapConstraints.Mem = &mem
	}
	c.Check(opBootstrap.Args.BootstrapConstraints, gc.DeepEquals, test.bootstrapConstraints)
	c.Check(opBootstrap.Args.Placement, gc.Equals, test.placement)

	opFinalizeBootstrap := (<-opc).(dummy.OpFinalizeBootstrap)
	c.Check(opFinalizeBootstrap.Env, gc.Equals, bootstrap.ControllerModelName)
	c.Check(opFinalizeBootstrap.InstanceConfig.ToolsList(), gc.Not(gc.HasLen), 0)
	if test.upload != "" {
		c.Check(opFinalizeBootstrap.InstanceConfig.AgentVersion().String(), gc.Equals, test.upload)
	}

	// Check controllers.yaml controller details.
	addrConnectedTo := []string{"localhost:17070"}

	controller, err := s.store.ControllerByName(controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller.CACert, gc.Not(gc.Equals), "")
	c.Assert(controller.APIEndpoints, gc.DeepEquals, addrConnectedTo)
	c.Assert(utils.IsValidUUIDString(controller.ControllerUUID), jc.IsTrue)
	// We don't care about build numbers here.
	bootstrapVers := bootstrapVersion.Number.ToPatch()
	controllerVers := semversion.MustParse(controller.AgentVersion).ToPatch()
	c.Assert(controllerVers.String(), gc.Equals, bootstrapVers.String())

	controllerModel, err := s.store.ModelByName(controllerName, "admin/controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(utils.IsValidUUIDString(controllerModel.ModelUUID), jc.IsTrue)

	// Bootstrap config should have been saved, and should only contain
	// the type, name, and any user-supplied configuration.
	bootstrapConfig, err := s.store.BootstrapConfigForController(controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapConfig.Cloud, gc.Equals, "dummy")
	c.Assert(bootstrapConfig.Credential, gc.Equals, "")
	expected := map[string]interface{}{
		"name":         bootstrap.ControllerModelName,
		"type":         "dummy",
		"default-base": "ubuntu@22.04",
		// Dummy provider defaults
		"broken":   "",
		"secret":   "pork",
		"somebool": false,
	}
	for k, v := range config.ConfigDefaults() {
		if _, ok := expected[k]; !ok {
			expected[k] = v
		}
	}
	c.Assert(bootstrapConfig.Config, jc.DeepEquals, expected)

	return restore
}

var bootstrapTests = []bootstrapTest{{
	info: "no args, no error, no upload, no constraints",
}, {
	info: "bad --constraints",
	args: []string{"--constraints", "bad=wrong"},
	err:  `unknown constraint "bad"`,
}, {
	info:      "conflicting --constraints",
	args:      []string{"--constraints", "instance-type=foo mem=4G"},
	silentErr: true,
	logs:      []jc.SimpleMessage{{loggo.ERROR, `ambiguous constraints: "instance-type" overlaps with "mem"`}},
}, {
	info:      "bad model",
	version:   "1.2.3-ubuntu-amd64",
	args:      []string{"--config", "broken=Bootstrap Destroy", "--auto-upgrade"},
	silentErr: true,
	logs:      []jc.SimpleMessage{{loggo.ERROR, `failed to bootstrap model: dummy.Bootstrap is broken`}},
}, {
	info:        "constraints",
	args:        []string{"--constraints", "mem=4G cores=4"},
	constraints: constraints.MustParse("mem=4G cores=4"),
}, {
	info:        "multiple constraints",
	args:        []string{"--constraints", "mem=4G", "--constraints", "cores=4"},
	constraints: constraints.MustParse("mem=4G cores=4"),
}, {
	info:                 "multiple bootstrap constraints",
	args:                 []string{"--bootstrap-constraints", "mem=4G", "--bootstrap-constraints", "cores=4"},
	bootstrapConstraints: constraints.MustParse("mem=4G cores=4"),
}, {
	info:                 "bootstrap and environ constraints",
	args:                 []string{"--constraints", "mem=4G cores=4", "--bootstrap-constraints", "mem=8G"},
	constraints:          constraints.MustParse("mem=4G cores=4"),
	bootstrapConstraints: constraints.MustParse("mem=8G cores=4"),
}, {
	info:        "unsupported constraint passed through but no error",
	args:        []string{"--constraints", "mem=4G cores=4 cpu-power=10"},
	constraints: constraints.MustParse("mem=4G cores=4 cpu-power=10"),
}, {
	info:        "--build-agent uses arch from constraint if it matches current version",
	version:     "1.3.3-ubuntu-ppc64el",
	hostArch:    "ppc64el",
	args:        []string{"--build-agent", "--constraints", "arch=ppc64el"},
	upload:      "1.3.3.1-ubuntu-ppc64el", // from jujuversion.Current
	constraints: constraints.MustParse("arch=ppc64el"),
}, {
	info:      "--build-agent rejects mismatched arch",
	version:   "1.3.3-ubuntu-amd64",
	hostArch:  "amd64",
	args:      []string{"--build-agent", "--constraints", "arch=ppc64el"},
	silentErr: true,
	logs: []jc.SimpleMessage{{
		loggo.ERROR, `failed to bootstrap model: cannot use agent built for "ppc64el" using a machine running on "amd64"`,
	}},
}, {
	info:      "--build-agent rejects non-supported arch",
	version:   "1.3.3-ubuntu-mips64",
	hostArch:  "mips64",
	args:      []string{"--build-agent"},
	silentErr: true,
	logs: []jc.SimpleMessage{{
		loggo.ERROR, fmt.Sprintf(`failed to bootstrap model: model %q of type dummy does not support instances running on "mips64"`, bootstrap.ControllerModelName),
	}},
}, {
	info:     "--build-agent always bumps build number",
	version:  "1.2.3.4-ubuntu-amd64",
	hostArch: "amd64",
	args:     []string{"--build-agent"},
	upload:   "1.2.3.5-ubuntu-amd64",
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
	info: "--agent-version with --build-agent",
	args: []string{"--agent-version", "1.1.0", "--build-agent"},
	err:  `--agent-version and --build-agent can't be used together`,
}, {
	info: "invalid --agent-version value",
	args: []string{"--agent-version", "foo"},
	err:  `invalid version "foo"`,
}, {
	info:    "agent-version doesn't match client version major",
	version: "1.3.3-ubuntu-ppc64el",
	args:    []string{"--agent-version", "2.3.0"},
	err:     regexp.QuoteMeta(`this client can only bootstrap 1.3 agents`),
}, {
	info:    "agent-version doesn't match client version minor",
	version: "1.3.3-ubuntu-ppc64el",
	args:    []string{"--agent-version", "1.4.0"},
	err:     regexp.QuoteMeta(`this client can only bootstrap 1.3 agents`),
}, {
	info: "--clouds with --regions",
	args: []string{"--clouds", "--regions", "aws"},
	err:  `--clouds and --regions can't be used together`,
}, {
	info: "specifying bootstrap attribute as model-default",
	args: []string{"--model-default", "bootstrap-timeout=10"},
	err:  `"bootstrap-timeout" is a bootstrap only attribute, and cannot be set as a model-default`,
}, {
	info: "specifying controller attribute as model-default",
	args: []string{"--model-default", "api-port=12345"},
	err:  `"api-port" is a controller attribute, and cannot be set as a model-default`,
}, {
	info: "k8s config on iaas controller",
	args: []string{"--config", "controller-service-type=loadbalancer"},
	err:  `"controller-service-type", "controller-external-name" and "controller-external-ips"are only allowed for kubernetes controllers`,
}, {
	info: "controller name cannot be set via config",
	args: []string{"--config", "controller-name=test"},
	err:  `controller name cannot be set via config, please use cmd args`,
}, {
	info: "missing storage pool name",
	args: []string{"--storage-pool", "type=ebs"},
	err:  `storage pool requires a name`,
}, {
	info: "missing storage pool type",
	args: []string{"--storage-pool", "name=test"},
	err:  `storage pool requires a type`,
}}

func (s *BootstrapSuite) TestRunCloudNameUnknown(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "unknown", "my-controller")
	c.Check(err, gc.ErrorMatches, `unknown cloud "unknown", please try "juju update-public-clouds"`)
}

func (s *BootstrapSuite) TestRunBadCloudName(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "bad^cloud", "my-controller")
	c.Check(err, gc.ErrorMatches, `cloud name "bad\^cloud" not valid`)
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
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", controllerName, "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", controllerName, "--auto-upgrade")
	c.Assert(err, gc.ErrorMatches, `controller "dev" already exists`)
}

func (s *BootstrapSuite) TestBootstrapDefaultControllerName(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy-cloud/region-1", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)
	currentController := s.store.CurrentControllerName
	c.Assert(currentController, gc.Equals, "dummy-cloud-region-1")
	details, err := s.store.ControllerByName(currentController)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*details.MachineCount, gc.Equals, 1)
	c.Assert(details.AgentVersion, gc.Equals, jujuversion.Current.String())
}

func (s *BootstrapSuite) TestBootstrapDefaultControllerNameWithCaps(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy-cloud/Region-1", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)
	currentController := s.store.CurrentControllerName
	c.Assert(currentController, gc.Equals, "dummy-cloud-region-1")
	details, err := s.store.ControllerByName(currentController)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*details.MachineCount, gc.Equals, 1)
	c.Assert(details.AgentVersion, gc.Equals, jujuversion.Current.String())
}

func (s *BootstrapSuite) TestBootstrapDefaultControllerNameNoRegions(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "no-cloud-regions", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)
	currentController := s.store.CurrentControllerName
	c.Assert(currentController, gc.Equals, "no-cloud-regions")
}

func (s *BootstrapSuite) TestBootstrapNoCurrentModel(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	// If no workload model specified, current model is not set.
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "devcontroller", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)
	currentController := s.store.CurrentControllerName
	c.Assert(currentController, gc.Equals, "devcontroller")
	_, err = s.store.CurrentModel(currentController)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *BootstrapSuite) TestNoSwitch(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "devcontroller", "--no-switch")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.store.CurrentControllerName, gc.Equals, "arthur")
}

func (s *BootstrapSuite) TestBootstrapSetsControllerDetails(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "devcontroller", "--auto-upgrade")
	c.Assert(err, jc.ErrorIsNil)
	currentController := s.store.CurrentControllerName
	c.Assert(currentController, gc.Equals, "devcontroller")
	details, err := s.store.ControllerByName(currentController)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*details.MachineCount, gc.Equals, 1)
	c.Assert(details.AgentVersion, gc.Equals, jujuversion.Current.String())
}

func (s *BootstrapSuite) TestBootstrapNoWorkloadModel(c *gc.C) {
	s.patchVersion(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"dummy", "devcontroller",
		"--auto-upgrade",
		"--config", "foo=bar",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(utils.IsValidUUIDString(bootstrapFuncs.args.ControllerConfig.ControllerUUID()), jc.IsTrue)
}

func (s *BootstrapSuite) TestBootstrapTimeout(c *gc.C) {
	s.patchVersion(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "devcontroller", "--auto-upgrade",
		"--config", "bootstrap-timeout=99",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.DialOpts.Timeout, gc.Equals, 99*time.Second)
}

func (s *BootstrapSuite) TestBootstrapAllSpacesAsConstraintsMerged(c *gc.C) {
	s.patchVersion(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "devcontroller", "--auto-upgrade",
		"--config", "juju-ha-space=ha-space", "--config", "juju-mgmt-space=management-space",
		"--constraints", "spaces=ha-space,random-space",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)

	c.Log(bootstrapFuncs.args.BootstrapConstraints.String())
	got := *(bootstrapFuncs.args.BootstrapConstraints.Spaces)
	c.Check(got, gc.DeepEquals, []string{"ha-space", "management-space", "random-space"})
}

func (s *BootstrapSuite) TestBootstrapAllConstraintsMerged(c *gc.C) {
	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "devcontroller", "--auto-upgrade",
		"--config", "juju-ha-space=ha-space", "--config", "juju-mgmt-space=management-space",
		"--constraints", "spaces=ha-space,random-space", "--constraints", "mem=4G",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)

	bootstrapCons := constraints.MustParse("mem=4G spaces=ha-space,management-space,random-space")
	c.Assert(bootstrapFuncs.args.BootstrapConstraints, gc.DeepEquals, bootstrapCons)
}

// checkConfigs runs bootstrapCmd.getBootstrapConfigs and checks the returned configs match
// the expected values passed in the expect parameter.
func checkConfigs(
	c *gc.C,
	bootstrapCmd bootstrapCommand,
	key string,
	ctx *cmd.Context, cloud *cloud.Cloud, provider environs.EnvironProvider,
	expect map[string]map[string]interface{}) {

	configs, err := bootstrapCmd.bootstrapConfigs(ctx, *cloud, provider)

	c.Assert(err, jc.ErrorIsNil)

	checkConfigEntryMatches(c, configs.bootstrapModel, key, "bootstrapModelConfig", expect)
	checkConfigEntryMatches(c, configs.inheritedControllerAttrs, key, "inheritedControllerAttrs", expect)
	checkConfigEntryMatches(c, configs.userConfigAttrs, key, "userConfigAttrs", expect)

	_, ok := configs.controller[key]
	c.Check(ok, jc.IsFalse)
}

// checkConfigEntryMatches tests that a keys existence and indexed value in configMap
// matches those in expect[name].
func checkConfigEntryMatches(c *gc.C, configMap map[string]interface{}, key, name string, expect map[string]map[string]interface{}) {
	v, ok := configMap[key]
	expectedConfig, expectedConfigOk := expect[name]
	c.Assert(expectedConfigOk, jc.IsTrue)
	vExpect, okExpect := expectedConfig[key]

	c.Logf("checkConfigEntryMatches %v %v", name, key)
	c.Check(ok, gc.Equals, okExpect)
	c.Check(v, gc.Equals, vExpect)
}

func (s *BootstrapSuite) TestBootstrapAttributesInheritedOverDefaults(c *gc.C) {
	/* Test that defaults are overwritten by inherited attributes by setting
	   the inherited attribute enable-os-upgrade to true in the cloud
	   config and ensure that it ends up as true in the model config. */
	s.patchVersion(c)

	bootstrapCmd := bootstrapCommand{}
	ctx := cmdtesting.Context(c)

	// The OpenStack provider has a default of "use-default-secgroup": false, so we
	// use that to test against.
	env := &openstack.Environ{}
	provider := env.Provider()

	// First test that use-default-secgroup defaults to false
	testCloud, err := cloud.CloudByName("dummy-cloud")
	c.Assert(err, jc.ErrorIsNil)

	key := "use-default-secgroup"
	checkConfigs(c, bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: false},
		"inheritedControllerAttrs": {},
		"userConfigAttrs":          {},
	})

	// Second test that use-default-secgroup in the cloud config overwrites the
	// provider default of false with true
	testCloud, err = cloud.CloudByName("dummy-cloud-with-config")
	c.Assert(err, jc.ErrorIsNil)

	checkConfigs(c, bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: true},
		"inheritedControllerAttrs": {key: true},
		"userConfigAttrs":          {},
	})
}

func (s *BootstrapSuite) TestBootstrapRegionConfigNoRegionSpecified(c *gc.C) {
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy-cloud-dummy-region-config")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.ControllerInheritedConfig["secret"], gc.Equals, "region-test")
}

func (s *BootstrapSuite) TestBootstrapRegionConfigAttributesOverCloudConfig(c *gc.C) {
	/* Test that cloud config attributes are overwritten by region config
	   attributes by setting both to something different in the config setup.
	   Only the region config values should be found */
	s.patchVersion(c)

	s.bootstrapCmd.Region = "region-2"
	ctx := cmdtesting.Context(c)

	// The OpenStack provider has a config attribute of network we can use.
	env := &openstack.Environ{}
	provider := env.Provider()

	// First test that the network is set to the cloud config value
	key := "network"
	testCloud, err := cloud.CloudByName("dummy-cloud-with-region-config")
	c.Assert(err, jc.ErrorIsNil)

	checkConfigs(c, s.bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: "cloud-network"},
		"inheritedControllerAttrs": {key: "cloud-network"},
		"userConfigAttrs":          {},
	})

	// Second test that network in the region config overwrites the cloud config network value.
	s.bootstrapCmd.Region = "region-1"
	testCloud, err = cloud.CloudByName("dummy-cloud-with-region-config")
	c.Assert(err, jc.ErrorIsNil)

	checkConfigs(c, s.bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: "region-network"},
		"inheritedControllerAttrs": {key: "region-network"},
		"userConfigAttrs":          {},
	})
}

func (s *BootstrapSuite) TestBootstrapAttributesCLIOverDefaults(c *gc.C) {
	/* Test that defaults are overwritten by CLI passed attributes by setting
	   the inherited attribute enable-os-upgrade to true in the cloud
	   config and ensure that it ends up as true in the model config. */
	s.patchVersion(c)

	ctx := cmdtesting.Context(c)

	// The OpenStack provider has a default of "use-default-secgroup": false, so we
	// use that to test against.
	env := &openstack.Environ{}
	provider := env.Provider()

	// First test that use-default-secgroup defaults to false
	testCloud, err := cloud.CloudByName("dummy-cloud")
	c.Assert(err, jc.ErrorIsNil)

	key := "use-default-secgroup"
	checkConfigs(c, s.bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: false},
		"inheritedControllerAttrs": {},
		"userConfigAttrs":          {},
	})

	// Second test that use-default-secgroup passed on the command line overwrites the
	// provider default of false with true
	err = s.bootstrapCmd.config.Set("use-default-secgroup=true")
	c.Assert(err, jc.ErrorIsNil)
	checkConfigs(c, s.bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: "true"},
		"inheritedControllerAttrs": {},
		"userConfigAttrs":          {key: "true"},
	})
}

func (s *BootstrapSuite) TestBootstrapAttributesCLIOverInherited(c *gc.C) {
	/* Test that defaults are overwritten by CLI passed attributes by setting
	   the inherited attribute enable-os-upgrade to true in the cloud
	   config and ensure that it ends up as true in the model config. */
	s.patchVersion(c)

	ctx := cmdtesting.Context(c)

	// The OpenStack provider has a default of "use-default-secgroup": false, so we
	// use that to test against.
	env := &openstack.Environ{}
	provider := env.Provider()

	// First test that use-default-secgroup defaults to false
	testCloud, err := cloud.CloudByName("dummy-cloud")
	c.Assert(err, jc.ErrorIsNil)

	key := "use-default-secgroup"
	checkConfigs(c, s.bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: false},
		"inheritedControllerAttrs": {},
		"userConfigAttrs":          {},
	})

	// Second test that use-default-secgroup passed on the command line overwrites the
	// inherited attribute
	testCloud, err = cloud.CloudByName("dummy-cloud-with-config")
	c.Assert(err, jc.ErrorIsNil)
	err = s.bootstrapCmd.config.Set("use-default-secgroup=false")
	c.Assert(err, jc.ErrorIsNil)
	checkConfigs(c, s.bootstrapCmd, key, ctx, testCloud, provider, map[string]map[string]interface{}{
		"bootstrapModelConfig":     {key: "false"},
		"inheritedControllerAttrs": {key: true},
		"userConfigAttrs":          {key: "false"},
	})
}

func (s *BootstrapSuite) TestBootstrapWithStoragePool(c *gc.C) {
	s.patchVersion(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"dummy", "devcontroller",
		"--storage-pool", "name=test",
		"--storage-pool", "type=loop",
		"--storage-pool", "foo=bar",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)

	c.Assert(bootstrapFuncs.args.StoragePools, jc.DeepEquals, map[string]storage.Attrs{
		"test": {
			"name": "test",
			"type": "loop",
			"foo":  "bar",
		},
	})
}

func (s *BootstrapSuite) TestBootstrapWithInvalidStoragePool(c *gc.C) {
	s.patchVersion(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"dummy", "devcontroller",
		"--storage-pool", "name=test",
		"--storage-pool", "type=invalid",
		"--storage-pool", "foo=bar",
	)
	c.Assert(err, gc.ErrorMatches, `invalid storage provider config: storage provider "invalid" not found`)
}

// In the case where we cannot examine the client store, we want the
// error to propagate back up to the user.
func (s *BootstrapSuite) TestBootstrapPropagatesStoreErrors(c *gc.C) {
	const controllerName = "devcontroller"
	s.patchVersion(c)

	store := jujuclienttesting.NewStubStore()
	store.CurrentControllerFunc = func() (string, error) {
		return "arthur", nil
	}
	store.PreviousControllerFunc = func() (string, bool, error) {
		return "", false, errors.NotFound
	}
	store.CurrentModelFunc = func(controller string) (string, error) {
		c.Assert(controller, gc.Equals, "arthur")
		return "sword", nil
	}
	store.PreviousModelFunc = func(controller string) (string, error) {
		return "", errors.NotFound
	}
	store.ModelByNameFunc = func(controller, model string) (*jujuclient.ModelDetails, error) {
		c.Assert(controller, gc.Equals, "arthur")
		c.Assert(model, gc.Equals, "sword")
		return &jujuclient.ModelDetails{}, nil
	}
	store.SetErrors(errors.New("oh noes"))
	command := &bootstrapCommand{}
	command.SetClientStore(store)
	wrapped := modelcmd.Wrap(command,
		modelcmd.WrapSkipModelFlags,
		modelcmd.WrapSkipDefaultModel,
	)
	_, err := cmdtesting.RunCommand(c, wrapped, "dummy", controllerName, "--auto-upgrade")
	store.CheckCallNames(c, "CredentialForCloud")
	c.Assert(err, gc.ErrorMatches, `loading credentials: oh noes`)
}

// When attempting to bootstrap, check that when prepare errors out,
// bootstrap will stop immediately. Nothing will be destroyed.
func (s *BootstrapSuite) TestBootstrapFailToPrepareDiesGracefully(c *gc.C) {
	destroyed := false
	s.PatchValue(&environsDestroy, func(name string, _ environs.ControllerDestroyer, _ envcontext.ProviderCallContext, _ jujuclient.ControllerStore) error {
		c.Assert(name, gc.Equals, "decontroller")
		destroyed = true
		return nil
	})

	s.PatchValue(&bootstrapPrepareController, func(
		bool,
		environs.BootstrapContext,
		jujuclient.ClientStore,
		bootstrap.PrepareParams,
	) (environs.BootstrapEnviron, error) {
		return nil, errors.New("mock-prepare")
	})

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "devcontroller")
	c.Check(err, gc.ErrorMatches, ".*mock-prepare$")
	c.Check(destroyed, jc.IsFalse)
}

type controllerModelAccountParams struct {
	controller     string
	controllerUUID string
	model          string
	user           string
}

func (s *BootstrapSuite) writeControllerModelAccountInfo(c *gc.C, context *controllerModelAccountParams) {
	controller := context.controller
	bootstrapModel := context.model
	user := context.user
	controllerUUID := "a-uuid"
	if context.controllerUUID != "" {
		controllerUUID = context.controllerUUID
	}
	err := s.store.AddController(controller, jujuclient.ControllerDetails{
		CACert:         "a-cert",
		ControllerUUID: controllerUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentController(controller)
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.UpdateAccount(controller, jujuclient.AccountDetails{
		User:     user,
		Password: "secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.UpdateModel(controller, bootstrapModel, jujuclient.ModelDetails{
		ModelUUID: "model-uuid",
		ModelType: model.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel(controller, bootstrapModel)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestBootstrapErrorRestoresOldMetadata(c *gc.C) {
	s.patchVersion(c)
	s.PatchValue(&bootstrapPrepareController, func(
		bool,
		environs.BootstrapContext,
		jujuclient.ClientStore,
		bootstrap.PrepareParams,
	) (environs.BootstrapEnviron, error) {
		ctx := controllerModelAccountParams{
			controller: "foo",
			model:      "foobar/bar",
			user:       "foobar",
		}
		s.writeControllerModelAccountInfo(c, &ctx)
		return nil, errors.New("mock-prepare")
	})

	ctx := controllerModelAccountParams{
		controller:     "olddevcontroller",
		controllerUUID: "another-uuid",
		model:          "fred/fredmodel",
		user:           "fred",
	}
	s.writeControllerModelAccountInfo(c, &ctx)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "devcontroller", "--auto-upgrade")
	c.Assert(err, gc.ErrorMatches, "mock-prepare")

	currentController := s.store.CurrentControllerName
	c.Assert(currentController, gc.Equals, "olddevcontroller")
	accountDetails, err := s.store.AccountDetails(currentController)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accountDetails.User, gc.Equals, "fred")
	currentModel, err := s.store.CurrentModel(currentController)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentModel, gc.Equals, "fred/fredmodel")
}

func (s *BootstrapSuite) TestBootstrapAlreadyExists(c *gc.C) {
	const controllerName = "devcontroller"
	s.patchVersion(c)

	cmaCtx := controllerModelAccountParams{
		controller: "devcontroller",
		model:      "fred/fredmodel",
		user:       "fred",
	}
	s.writeControllerModelAccountInfo(c, &cmaCtx)

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", controllerName, "--auto-upgrade")
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`controller %q already exists`, controllerName))
	currentController := s.store.CurrentControllerName
	c.Assert(currentController, gc.Equals, "devcontroller")
	accountDetails, err := s.store.AccountDetails(currentController)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accountDetails.User, gc.Equals, "fred")
	currentModel, err := s.store.CurrentModel(currentController)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentModel, gc.Equals, "fred/fredmodel")
}

func (s *BootstrapSuite) TestInvalidLocalSource(c *gc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.2.0"))
	s.PatchValue(&envtools.BundleTools,
		func(bool, io.Writer, func(localBinaryVersion semversion.Number) semversion.Number) (semversion.Binary, semversion.Number, bool, string, error) {
			return semversion.Binary{}, semversion.Number{}, false, "", errors.New("no agent binaries for you")
		},
	)
	s.PatchValue(&envtools.DefaultBaseURL, c.MkDir())
	resetJujuXDGDataHome(c)

	// Bootstrap the controller with an invalid source.
	// The command will look for prepackaged agent binaries
	// in the source, and then fall back to building.
	ctx, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "--metadata-source", c.MkDir(),
		"dummy", "devcontroller",
	)
	c.Check(err, gc.Equals, cmd.ErrSilent)

	stderr := cmdtesting.Stderr(ctx)
	c.Check(stderr, gc.Matches,
		"Creating Juju controller \"devcontroller\" on dummy/dummy\n"+
			"Looking for packaged Juju agent version 1.2.0 for amd64\n"+
			"No packaged binary found, preparing local Juju agent binary\n",
	)
	c.Check(s.tw.Log(), jc.LogMatches, []jc.SimpleMessage{{
		Level:   loggo.ERROR,
		Message: "failed to bootstrap model: cannot package bootstrap agent binary: no agent binaries for you",
	}})
}

// createImageMetadata creates some image metadata in a local directory.
func createImageMetadata(c *gc.C) (string, []*imagemetadata.ImageMetadata) {
	// Generate some image metadata.
	im := []*imagemetadata.ImageMetadata{
		{
			Id:         "1234",
			Arch:       "amd64",
			Version:    "16.04",
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
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	base := corebase.MustParseBaseFromString("ubuntu@22.04")
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, base, im, cloudSpec, sourceStor)
	c.Assert(err, jc.ErrorIsNil)
	return sourceDir, im
}

func (s *BootstrapSuite) TestBootstrapCalledWithMetadataDir(c *gc.C) {
	sourceDir, _ := createImageMetadata(c)
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--metadata-source", sourceDir, "--constraints", "mem=4G",
		"dummy-cloud/region-1", "devcontroller",
		"--config", "default-base=ubuntu@22.04",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.MetadataDir, gc.Equals, sourceDir)
}

func (s *BootstrapSuite) TestBootstrapCalledWitBase(c *gc.C) {
	sourceDir, _ := createImageMetadata(c)
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--metadata-source", sourceDir, "--constraints", "mem=4G",
		"dummy-cloud/region-1", "devcontroller",
		"--config", "default-base=ubuntu@22.04",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.MetadataDir, gc.Equals, sourceDir)
}

func (s *BootstrapSuite) checkBootstrapWithVersion(c *gc.C, vers, expect string) {
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	num := jujuversion.Current
	num.Major = 2
	num.Minor = 3
	s.PatchValue(&jujuversion.Current, num)
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--agent-version", vers,
		"dummy-cloud/region-1", "devcontroller",
		"--config", "default-base=ubuntu@22.04",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.AgentVersion, gc.NotNil)
	c.Assert(*bootstrapFuncs.args.AgentVersion, gc.Equals, semversion.MustParse(expect))
}

func (s *BootstrapSuite) TestBootstrapWithVersionNumber(c *gc.C) {
	s.checkBootstrapWithVersion(c, "2.3.4", "2.3.4")
}

func (s *BootstrapSuite) TestBootstrapWithBinaryVersionNumber(c *gc.C) {
	s.checkBootstrapWithVersion(c, "2.3.4-jammy-ppc64", "2.3.4")
}

func (s *BootstrapSuite) checkBootstrapBaseWithVersion(c *gc.C, vers, expect string) {
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	num := jujuversion.Current
	num.Major = 2
	num.Minor = 3
	s.PatchValue(&jujuversion.Current, num)
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--agent-version", vers,
		"dummy-cloud/region-1", "devcontroller",
		"--config", "default-base=ubuntu@22.04",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.AgentVersion, gc.NotNil)
	c.Assert(*bootstrapFuncs.args.AgentVersion, gc.Equals, semversion.MustParse(expect))
}

func (s *BootstrapSuite) TestBootstrapBaseWithVersionNumber(c *gc.C) {
	s.checkBootstrapBaseWithVersion(c, "2.3.4", "2.3.4")
}

func (s *BootstrapSuite) TestBootstrapBaseWithBinaryVersionNumber(c *gc.C) {
	s.checkBootstrapBaseWithVersion(c, "2.3.4-jammy-ppc64", "2.3.4")
}

func (s *BootstrapSuite) TestBootstrapWithAutoUpgrade(c *gc.C) {
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"--auto-upgrade",
		"dummy-cloud/region-1", "devcontroller",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.AgentVersion, gc.IsNil)
}

func (s *BootstrapSuite) TestAutoSyncLocalSource(c *gc.C) {
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.2.0"))
	resetJujuXDGDataHome(c)

	// Bootstrap the controller with the valid source.
	// The bootstrapping has to show no error, because the tools
	// are automatically synchronized.
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "--metadata-source", sourceDir,
		"dummy-cloud/region-1", "devcontroller", "--config", "default-base=ubuntu@20.04",
	)
	c.Assert(err, jc.ErrorIsNil)

	bootstrapConfig, spec, cfg, err := modelcmd.NewGetBootstrapConfigParamsFunc(
		cmdtesting.Context(c), s.store, environs.GlobalProviderRegistry(),
	)("devcontroller")
	c.Assert(err, jc.ErrorIsNil)
	provider, err := environs.Provider(bootstrapConfig.CloudType)
	c.Assert(err, jc.ErrorIsNil)
	err = provider.ValidateCloud(context.Background(), *spec)
	c.Assert(err, jc.ErrorIsNil)

	env, err := environs.New(context.Background(), environs.OpenParams{
		Cloud:  *spec,
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, jc.ErrorIsNil)
	err = env.PrepareForBootstrap(envtesting.BootstrapContext(context.Background(), c), "controller-1")
	c.Assert(err, jc.ErrorIsNil)

	// Now check the available tools which are the 1.2.0 envtools.
	checkTools(c, env, v120All)
}

func (s *BootstrapSuite) TestInteractiveBootstrap(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	command := s.newBootstrapCommand()
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	out := bytes.Buffer{}
	ctx.Stdin = strings.NewReader(`
dummy-cloud
region-1
my-dummy-cloud
`[1:])
	ctx.Stdout = &out
	err = command.Run(ctx)
	if err != nil {
		c.Logf(out.String())
	}
	c.Assert(err, jc.ErrorIsNil)

	name := s.store.CurrentControllerName
	c.Assert(name, gc.Equals, "my-dummy-cloud")
	controller := s.store.Controllers[name]
	c.Assert(controller.Cloud, gc.Equals, "dummy-cloud")
	c.Assert(controller.CloudRegion, gc.Equals, "region-1")
}

func (s *BootstrapSuite) setupAutoUploadTest(c *gc.C, vers, ser string) {
	patchedVersion := semversion.MustParse(vers)
	patchedVersion.Build = 1
	s.PatchValue(&envtools.BundleTools, toolstesting.GetMockBundleTools(patchedVersion))
	sourceDir := createToolsSource(c, vAll)
	s.PatchValue(&envtools.DefaultBaseURL, sourceDir)

	// Change the tools location to be the test location and also
	// the version and ensure their later restoring.
	// Set the current version to be something for which there are no tools
	// so we can test that an upload is forced.
	s.PatchValue(&jujuversion.Current, semversion.MustParse(vers))

	// Create home with dummy provider and remove all
	// of its envtools.
	resetJujuXDGDataHome(c)
}

func (s *BootstrapSuite) TestAutoUploadAfterFailedSync(c *gc.C) {
	s.setupAutoUploadTest(c, "1.7.3", "focal")
	// Run command and check for that upload has been run for tools matching
	// the current juju version.
	opc, errc := runCommandWithDummyProvider(
		cmdtesting.Context(c), s.newBootstrapCommand(),
		"dummy-cloud/region-1", "devcontroller",
		"--config", "default-base=ubuntu@20.04",
		"--auto-upgrade",
	)
	select {
	case err := <-errc:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
	c.Check((<-opc).(dummy.OpBootstrap).Env, gc.Equals, bootstrap.ControllerModelName)
	icfg := (<-opc).(dummy.OpFinalizeBootstrap).InstanceConfig
	c.Assert(icfg, gc.NotNil)
	c.Assert(icfg.AgentVersion().String(), gc.Equals, "1.7.3.1-ubuntu-"+arch.HostArch())
}

func (s *BootstrapSuite) TestMissingToolsError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"dummy-cloud/region-1", "devcontroller",
		"--config", "default-base=ubuntu@22.04", "--agent-version=1.8.4",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Check(s.tw.Log(), jc.LogMatches, []jc.SimpleMessage{{
		Level:   loggo.ERROR,
		Message: "failed to bootstrap model: Juju cannot bootstrap because no agent binaries are available for your model",
	}})
}

func (s *BootstrapSuite) TestMissingToolsUploadFailedError(c *gc.C) {
	buildAgentTarballAlwaysFails := func(
		bool, string, func(semversion.Number) semversion.Number,
	) (*sync.BuiltAgent, error) {
		return nil, errors.New("an error")
	}

	s.setupAutoUploadTest(c, "1.7.3", "jammy")
	s.PatchValue(&sync.BuildAgentTarball, buildAgentTarballAlwaysFails)

	ctx, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(),
		"dummy-cloud/region-1", "devcontroller",
		"--config", "default-base=ubuntu@22.04",
		"--config", "agent-stream=proposed",
		"--auto-upgrade", "--agent-version=1.7.3",
	)

	c.Check(cmdtesting.Stderr(ctx), gc.Equals, `
Creating Juju controller "devcontroller" on dummy-cloud/region-1
Looking for packaged Juju agent version 1.7.3 for amd64
No packaged binary found, preparing local Juju agent binary
`[1:])
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Check(s.tw.Log(), jc.LogMatches, []jc.SimpleMessage{{
		Level:   loggo.ERROR,
		Message: "failed to bootstrap model: cannot package bootstrap agent binary: an error",
	}})
}

func (s *BootstrapSuite) TestBootstrapDestroy(c *gc.C) {
	s.setupAutoUploadTest(c, "1.7.3", "jammy")

	opc, errc := runCommandWithDummyProvider(
		cmdtesting.Context(c), s.newBootstrapCommand(),
		"dummy-cloud/region-1", "devcontroller",
		"--config", "broken=Bootstrap Destroy",
		"--auto-upgrade",
	)
	select {
	case err := <-errc:
		c.Assert(err, gc.Equals, cmd.ErrSilent)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

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

	c.Check(s.tw.Log(), jc.LogMatches, []jc.SimpleMessage{
		{Level: loggo.ERROR, Message: "failed to bootstrap model: dummy.Bootstrap is broken"},
		{Level: loggo.DEBUG, Message: "(error details.*)"},
		{Level: loggo.DEBUG, Message: "cleaning up after failed bootstrap"},
		{Level: loggo.ERROR, Message: "error cleaning up: dummy.Destroy is broken"},
	})
}

func (s *BootstrapSuite) TestBootstrapKeepBroken(c *gc.C) {
	s.setupAutoUploadTest(c, "1.7.3", "jammy")

	ctx := cmdtesting.Context(c)
	opc, errc := runCommandWithDummyProvider(ctx, s.newBootstrapCommand(),
		"--keep-broken",
		"dummy-cloud/region-1", "devcontroller",
		"--config", "broken=Bootstrap Destroy",
		"--auto-upgrade",
	)
	select {
	case err := <-errc:
		c.Assert(err, gc.ErrorMatches, "failed to bootstrap model: dummy.Bootstrap is broken")
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
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
	stderr := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(stderr, gc.Matches, `.*See .*juju kill\-controller.*`)
}

func (s *BootstrapSuite) TestBootstrapUnknownCloudOrProvider(c *gc.C) {
	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "no-such-provider", "ctrl")
	c.Assert(err, gc.ErrorMatches, `unknown cloud "no-such-provider", please try "juju update-public-clouds"`)
}

func (s *BootstrapSuite) TestBootstrapProviderNoRegionDetection(c *gc.C) {
	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "no-cloud-region-detection", "ctrl")
	c.Assert(err, gc.ErrorMatches, `unknown cloud "no-cloud-region-detection", please try "juju update-public-clouds"`)
}

func (s *BootstrapSuite) TestBootstrapProviderNoRegions(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "focal")
	ctx, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "no-cloud-regions", "ctrl",
		"--config", "default-base=ubuntu@20.04",
	)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Creating Juju controller \"ctrl\" on no-cloud-regions(.|\n)*")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestBootstrapCloudNoRegions(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")
	ctx, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy-cloud-without-regions", "ctrl",
		"--config", "default-base=ubuntu@20.04",
	)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Creating Juju controller \"ctrl\" on dummy-cloud-without-regions(.|\n)*")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestBootstrapCloudNoRegionsOneSpecified(c *gc.C) {
	resetJujuXDGDataHome(c)
	ctx, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy-cloud-without-regions/my-region", "ctrl",
		"--config", "default-base=ubuntu@22.04",
	)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `region "my-region" for cloud "dummy-cloud-without-regions" not valid`)
}

func (s *BootstrapSuite) TestBootstrapProviderNoCredentials(c *gc.C) {
	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "no-credentials", "ctrl")
	c.Assert(err, gc.ErrorMatches, "detecting credentials for \"no-credentials\" cloud provider: credentials not found\nSee `juju add-credential no-credentials --help` for instructions")
}

func (s *BootstrapSuite) TestBootstrapProviderManyDetectedCredentials(c *gc.C) {
	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "many-credentials", "ctrl")
	c.Assert(err, gc.ErrorMatches, ambiguousDetectedCredentialError.Error())
}

func (s *BootstrapSuite) TestBootstrapWithDeprecatedBase(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "jammy")
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy-cloud-without-regions", "ctrl",
		"--config", "default-base=ubuntu@18.04",
	)
	c.Assert(err, gc.ErrorMatches, `base "ubuntu@18.04" not supported`)
}

func (s *BootstrapSuite) TestBootstrapBaseWithNoBootstrapSeriesUsesFallbackButStillFails(c *gc.C) {
	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "no-cloud-regions", "ctrl", "--config", "default-base=spock",
	)
	c.Assert(err, gc.ErrorMatches, `invalid default base "spock": expected base string to contain os and channel separated by '@'`)
}

func (s *BootstrapSuite) TestBootstrapBaseWithBootstrapSeriesDoesNotUseFallbackButStillFails(c *gc.C) {
	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "no-cloud-regions", "ctrl",
		"--bootstrap-base", "spock",
		"--config", "default-base=kirk",
	)
	c.Assert(err, gc.ErrorMatches, `base "spock" not valid`)
}

func (s *BootstrapSuite) TestBootstrapProviderFileCredential(c *gc.C) {
	dummyProvider, err := environs.Provider("dummy")
	c.Assert(err, jc.ErrorIsNil)

	tmpFile, err := os.CreateTemp("", "juju-bootstrap-test")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		_ = tmpFile.Close()
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

	contents := []byte("{something: special}\n")
	err = os.WriteFile(tmpFile.Name(), contents, 0644)
	c.Assert(err, jc.ErrorIsNil)

	unfinalizedCredential := cloud.NewEmptyCredential()
	finalizedCredential := cloud.NewEmptyCredential()
	fp := fileCredentialProvider{
		dummyProvider.(environs.CloudEnvironProvider),
		tmpFile.Name(),
		&unfinalizedCredential,
		&finalizedCredential}
	environs.RegisterProvider("file-credentials", fp)

	s.setupAutoUploadTest(c, "1.8.3", "focal")
	_, err = cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "file-credentials", "ctrl",
		"--config", "default-base=ubuntu@20.04",
	)
	c.Assert(err, jc.ErrorIsNil)

	// When credentials are "finalized" any credential attribute indicated
	// to be a file path is replaced by that file's contents. Here we check to see
	// that the state of the credential under test before finalization is
	// indeed the file path itself and that the state of the credential
	// after finalization is the contents of that file.
	c.Assert(unfinalizedCredential.Attributes()["file"], gc.Equals, tmpFile.Name())
	c.Assert(finalizedCredential.Attributes()["file"], gc.Equals, string(contents))
}

func (s *BootstrapSuite) TestBootstrapProviderDetectRegionsInvalid(c *gc.C) {
	s.patchVersion(c)
	ctx, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy/not-dummy", "ctrl")
	c.Assert(err, gc.ErrorMatches, `region "not-dummy" for cloud "dummy" not valid`)
	stderr := strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1)
	c.Assert(stderr, gc.Matches, `Available cloud region is dummy`)
}

func (s *BootstrapSuite) TestBootstrapProviderManyCredentialsCloudNoAuthTypes(c *gc.C) {
	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	s.patchVersion(c)
	s.store.Credentials = map[string]cloud.CloudCredential{
		"many-credentials-no-auth-types": {
			AuthCredentials: map[string]cloud.Credential{"one": cloud.NewCredential("one", nil)},
		},
	}
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"many-credentials-no-auth-types", "ctrl",
		"--credential", "one",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.Cloud.AuthTypes, jc.SameContents, cloud.AuthTypes{"one", "two"})
}

func (s *BootstrapSuite) TestManyAvailableCredentialsNoneSpecified(c *gc.C) {
	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	s.patchVersion(c)
	s.store.Credentials = map[string]cloud.CloudCredential{
		"dummy": {
			AuthCredentials: map[string]cloud.Credential{
				"one": cloud.NewCredential("one", nil),
				"two": cloud.NewCredential("two", nil),
			},
		},
	}
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "ctrl")
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "more than one credential is available.*")
}

func (s *BootstrapSuite) TestBootstrapProviderDetectCloud(c *gc.C) {
	resetJujuXDGDataHome(c)

	dummyProvider, err := environs.Provider("dummy")
	c.Assert(err, jc.ErrorIsNil)

	var bootstrapFuncs fakeBootstrapFuncs
	bootstrapFuncs.newCloudDetector = func(p environs.EnvironProvider) (environs.CloudDetector, bool) {
		if p != dummyProvider {
			return nil, false
		}
		return cloudDetectorFunc(func() ([]cloud.Cloud, error) {
			return []cloud.Cloud{{
				Name:      "bruce",
				Type:      "dummy",
				AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
				Regions:   []cloud.Region{{Name: "gazza", Endpoint: "endpoint"}},
			}}, nil
		}), true
	}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	s.patchVersion(c)
	_, err = cmdtesting.RunCommand(c, s.newBootstrapCommand(), "bruce", "ctrl")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.CloudRegion, gc.Equals, "gazza")
	c.Assert(bootstrapFuncs.args.CloudCredentialName, gc.Equals, "default")
	c.Assert(bootstrapFuncs.args.Cloud, jc.DeepEquals, cloud.Cloud{
		Name:      "bruce",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		Regions:   []cloud.Region{{Name: "gazza", Endpoint: "endpoint"}},
	})
}

func (s *BootstrapSuite) TestBootstrapProviderDetectRegions(c *gc.C) {
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	bootstrapFuncs.cloudRegionDetector = cloudRegionDetectorFunc(func() ([]cloud.Region, error) {
		return []cloud.Region{{Name: "bruce", Endpoint: "endpoint"}}, nil
	})
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "ctrl")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.CloudRegion, gc.Equals, "bruce")
	c.Assert(bootstrapFuncs.args.CloudCredentialName, gc.Equals, "default")
	sort.Sort(bootstrapFuncs.args.Cloud.AuthTypes)
	c.Assert(bootstrapFuncs.args.Cloud, jc.DeepEquals, cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "bruce", Endpoint: "endpoint"}},
	})
}

func (s *BootstrapSuite) TestBootstrapProviderDetectNoRegions(c *gc.C) {
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	bootstrapFuncs.cloudRegionDetector = cloudRegionDetectorFunc(func() ([]cloud.Region, error) {
		return nil, errors.NotFoundf("regions")
	})
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "ctrl")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.CloudRegion, gc.Equals, "")
	sort.Sort(bootstrapFuncs.args.Cloud.AuthTypes)
	c.Assert(bootstrapFuncs.args.Cloud, jc.DeepEquals, cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
	})
}

func (s *BootstrapSuite) TestBootstrapProviderFinalizeCloud(c *gc.C) {
	resetJujuXDGDataHome(c)

	var bootstrapFuncs fakeBootstrapFuncs
	bootstrapFuncs.cloudFinalizer = cloudFinalizerFunc(func(ctx environs.FinalizeCloudContext, in cloud.Cloud) (cloud.Cloud, error) {
		c.Assert(in, jc.DeepEquals, cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{"empty", "userpass"},
		})
		in.Name = "override"
		return in, nil
	})
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})

	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy", "ctrl")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.Cloud, jc.DeepEquals, cloud.Cloud{
		Name:      "override",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{"empty", "userpass"},
	})
}

func (s *BootstrapSuite) TestBootstrapProviderCaseInsensitiveRegionCheck(c *gc.C) {
	s.patchVersion(c)

	var prepareParams bootstrap.PrepareParams
	s.PatchValue(&bootstrapPrepareController, func(
		_ bool,
		ctx environs.BootstrapContext,
		stor jujuclient.ClientStore,
		params bootstrap.PrepareParams,
	) (environs.BootstrapEnviron, error) {
		prepareParams = params
		return nil, errors.New("mock-prepare")
	})

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "dummy/DUMMY", "ctrl")
	c.Assert(err, gc.ErrorMatches, "mock-prepare")
	c.Assert(prepareParams.Cloud.Region, gc.Equals, "dummy")
}

func (s *BootstrapSuite) TestBootstrapConfigFile(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("secret: 666\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.patchVersion(c)
	_, err = cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "ctrl",
		"--config", configFile,
	)
	c.Assert(err, gc.ErrorMatches, `invalid attribute value\(s\) for dummy cloud: secret: expected string, got int.*`)
}

func (s *BootstrapSuite) TestBootstrapMultipleConfigFiles(c *gc.C) {
	tmpdir := c.MkDir()
	configFile1 := filepath.Join(tmpdir, "config-1.yaml")
	err := os.WriteFile(configFile1, []byte(
		"controller: not-a-bool\nbroken: Bootstrap\n",
	), 0644)
	c.Assert(err, jc.ErrorIsNil)
	configFile2 := filepath.Join(tmpdir, "config-2.yaml")
	err = os.WriteFile(configFile2, []byte(
		"controller: false\n",
	), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.setupAutoUploadTest(c, "1.8.3", "jammy")
	_, err = cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "ctrl",
		"--auto-upgrade",
		// the second config file should replace attributes
		// with the same name from the first, but leave the
		// others alone.
		"--config", configFile1,
		"--config", configFile2,
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Check(s.tw.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.ERROR, "failed to bootstrap model: dummy.Bootstrap is broken"},
	})
}

func (s *BootstrapSuite) TestBootstrapConfigFileAndAdHoc(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("controller: not-a-bool\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.setupAutoUploadTest(c, "1.8.3", "jammy")
	_, err = cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "ctrl",
		"--auto-upgrade",
		// Configuration specified on the command line overrides
		// anything specified in files, no matter what the order.
		"--config", "controller=false",
		"--config", configFile,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestBootstrapAutocertDNSNameDefaultPort(c *gc.C) {
	s.patchVersion(c)
	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "ctrl",
		"--config", "autocert-dns-name=foo.example",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.ControllerConfig.APIPort(), gc.Equals, 443)
}

func (s *BootstrapSuite) TestBootstrapAutocertDNSNameExplicitAPIPort(c *gc.C) {
	s.patchVersion(c)
	var bootstrapFuncs fakeBootstrapFuncs
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return &bootstrapFuncs
	})
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy", "ctrl",
		"--config", "autocert-dns-name=foo.example",
		"--config", "api-port=12345",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(bootstrapFuncs.args.ControllerConfig.APIPort(), gc.Equals, 12345)
}

func (s *BootstrapSuite) TestBootstrapCloudConfigAndAdHoc(c *gc.C) {
	s.patchVersion(c)
	_, err := cmdtesting.RunCommand(
		c, s.newBootstrapCommand(), "dummy-cloud-with-config", "ctrl",
		"--auto-upgrade",
		// Configuration specified on the command line overrides
		// anything specified in files, no matter what the order.
		"--config", "somebool=not-a-bool",
	)
	c.Assert(err, gc.ErrorMatches, `invalid attribute value\(s\) for dummy cloud: somebool: expected bool, got .*`)
}

func (s *BootstrapSuite) TestBootstrapPrintClouds(c *gc.C) {
	resetJujuXDGDataHome(c)
	s.store.Credentials = map[string]cloud.CloudCredential{
		"aws": {
			DefaultRegion: "us-west-1",
			AuthCredentials: map[string]cloud.Credential{
				"fred": {},
				"mary": {},
			},
		},
		"dummy-cloud": {
			DefaultRegion: "home",
			AuthCredentials: map[string]cloud.Credential{
				"joe": {},
			},
		},
	}
	defer func() {
		s.store = jujuclient.NewMemStore()
	}()

	ctx, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "--clouds")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, `
You can bootstrap on these clouds. See '--regions <cloud>' for all regions.
Cloud                            Credentials  Default Region
aws                              fred         us-west-1
                                 mary         
aws-china                                     
aws-gov                                       
azure                                         
azure-china                                   
google                                        
oracle                                        
?(localhost\s+)?(microk8s\s+)?
dummy-cloud                      joe          home
dummy-cloud-dummy-region-config               
dummy-cloud-with-config                       
dummy-cloud-with-region-config                
dummy-cloud-without-regions                   
many-credentials-no-auth-types                

You will need to have a credential if you want to bootstrap on a cloud, see
'juju autoload-credentials' and 'juju add-credential'. The first credential
listed is the default. Add more clouds with 'juju add-cloud'.
`[1:])
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *BootstrapSuite) TestBootstrapPrintCloudsInvalidCredential(c *gc.C) {
	resetJujuXDGDataHome(c)
	store := jujuclienttesting.NewStubStore()
	store.CredentialForCloudFunc = func(cloudName string) (*cloud.CloudCredential, error) {
		if cloudName == "dummy-cloud" {
			return nil, errors.Errorf("expected error")
		}
		if cloudName == "aws" {
			return &cloud.CloudCredential{
				DefaultRegion: "us-west-1",
				AuthCredentials: map[string]cloud.Credential{
					"fred": {},
					"mary": {},
				},
			}, nil
		}
		return nil, errors.NotFoundf("credentials for cloud %s", cloudName)
	}

	command := s.bootstrapCmd
	command.SetClientStore(store)

	var logWriter loggo.TestWriter
	writerName := "TestBootstrapPrintCloudsInvalidCredential"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), jc.ErrorIsNil)
	defer func() {
		_, _ = loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(&command), "--clouds")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, `
You can bootstrap on these clouds. See '--regions <cloud>' for all regions.
Cloud                            Credentials  Default Region
aws                              fred         us-west-1
                                 mary         
aws-china                                     
aws-gov                                       
azure                                         
azure-china                                   
google                                        
oracle                                        
?(localhost\s+)?(microk8s\s+)?
dummy-cloud-dummy-region-config               
dummy-cloud-with-config                       
dummy-cloud-with-region-config                
dummy-cloud-without-regions                   
many-credentials-no-auth-types                

You will need to have a credential if you want to bootstrap on a cloud, see
'juju autoload-credentials' and 'juju add-credential'. The first credential
listed is the default. Add more clouds with 'juju add-cloud'.
`[1:])

	c.Check(logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{
			Level:   loggo.WARNING,
			Message: `error loading credential for cloud dummy-cloud: expected error`,
		},
	})
}

func (s *BootstrapSuite) TestBootstrapPrintCloudRegions(c *gc.C) {
	resetJujuXDGDataHome(c)
	ctx, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "--regions", "aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, `
Showing regions for aws:
us-east-1
us-east-2
us-west-1
us-west-2
ca-central-1
mx-central-1
eu-west-1
eu-west-2
eu-west-3
eu-central-1
eu-central-2
eu-north-1
eu-south-1
eu-south-2
af-south-1
ap-east-1
ap-south-1
ap-south-2
ap-southeast-1
ap-southeast-2
ap-southeast-3
ap-southeast-4
ap-southeast-5
ap-southeast-7
ap-northeast-1
ap-northeast-2
ap-northeast-3
me-south-1
me-central-1
sa-east-1
`[1:])
}

func (s *BootstrapSuite) TestBootstrapInvalidRegion(c *gc.C) {
	resetJujuXDGDataHome(c)
	ctx, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "aws/eu-west")
	c.Assert(err, gc.ErrorMatches, `region "eu-west" for cloud "aws" not valid`)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Available cloud regions are af-south-1, ap-east-1, ap-northeast-1, ap-northeast-2, ap-northeast-3, ap-south-1, ap-south-2, ap-southeast-1, ap-southeast-2, ap-southeast-3, ap-southeast-4, ap-southeast-5, ap-southeast-7, ca-central-1, eu-central-1, eu-central-2, eu-north-1, eu-south-1, eu-south-2, eu-west-1, eu-west-2, eu-west-3, me-central-1, me-south-1, mx-central-1, sa-east-1, us-east-1, us-east-2, us-west-1, us-west-2\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ``)
}

func (s *BootstrapSuite) TestBootstrapPrintCloudRegionsNoSuchCloud(c *gc.C) {
	resetJujuXDGDataHome(c)
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "--regions", "foo")
	c.Assert(err, gc.ErrorMatches, "cloud foo not found")
}

func (s *BootstrapSuite) TestBootstrapTestingOptions(c *gc.C) {
	s.PatchEnvironment("JUJU_AGENT_TESTING_OPTIONS", "foo=bar, hello = world")
	var gotArgs bootstrap.BootstrapParams
	bootstrapFuncs := &fakeBootstrapFuncs{
		bootstrapF: func(_ environs.BootstrapContext, _ environs.BootstrapEnviron, callCtx envcontext.ProviderCallContext, args bootstrap.BootstrapParams) error {
			gotArgs = args
			return errors.New("test error")
		},
	}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return bootstrapFuncs
	})
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"dummy", "devcontroller",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(gotArgs.ExtraAgentValuesForTesting, jc.DeepEquals, map[string]string{"foo": "bar", "hello": "world"})
}

func (s *BootstrapSuite) TestBootstrapWithLocalControllerCharm(c *gc.C) {
	tmpDir := c.MkDir()
	controllerCharmDir := testcharms.Repo.CharmDir("juju-controller")
	controllerCharmPath := filepath.Join(tmpDir, "juju-controller.charm")
	err := controllerCharmDir.ArchiveToPath(controllerCharmPath)
	c.Assert(err, jc.ErrorIsNil)

	var gotArgs bootstrap.BootstrapParams
	bootstrapFuncs := &fakeBootstrapFuncs{
		bootstrapF: func(_ environs.BootstrapContext, _ environs.BootstrapEnviron, callCtx envcontext.ProviderCallContext, args bootstrap.BootstrapParams) error {
			gotArgs = args
			return errors.New("test error")
		},
	}
	s.PatchValue(&getBootstrapFuncs, func() BootstrapInterface {
		return bootstrapFuncs
	})

	_, err = cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"dummy", "devcontroller", "--controller-charm-path", controllerCharmPath,
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(gotArgs.ControllerCharmPath, gc.DeepEquals, controllerCharmPath)
}

func (s *BootstrapSuite) TestBootstrapWithLocalControllerCharmFailsWithWrongCharm(c *gc.C) {
	tmpDir := c.MkDir()
	controllerCharmDir := testcharms.Repo.CharmDir("mysql")
	controllerCharmPath := filepath.Join(tmpDir, "juju-controller.charm")
	err := controllerCharmDir.ArchiveToPath(controllerCharmPath)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"dummy", "devcontroller", "--controller-charm-path", controllerCharmPath,
	)
	c.Assert(err, gc.ErrorMatches, `--controller-charm-path ".*" is not a "juju-controller" charm`)
}

func (s *BootstrapSuite) TestBootstrapWithLocalControllerCharmFailsWithInvalidCharm(c *gc.C) {
	tmpDir := c.MkDir()
	controllerCharmPath := filepath.Join(tmpDir, "juju-controller.charm")
	err := os.WriteFile(controllerCharmPath, []byte("invalid"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"dummy", "devcontroller", "--controller-charm-path", controllerCharmPath,
	)
	c.Assert(err, gc.ErrorMatches, `--controller-charm-path .* is not a valid charm: .*`)
}

func (s *BootstrapSuite) TestBootstrapWithLocalControllerCharmFailsWithDir(c *gc.C) {
	controllerDirPath := testcharms.Repo.CharmDir("juju-controller").Path

	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"dummy", "devcontroller", "--controller-charm-path", controllerDirPath,
	)
	c.Assert(err, gc.ErrorMatches, `.* is a directory`)
}

func (s *BootstrapSuite) TestBootstrapWithLocalControllerCharmFailsWithNonexistant(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(),
		"dummy", "devcontroller", "--controller-charm-path", "/invalid/path.charm",
	)
	c.Assert(err, gc.ErrorMatches, `problem with --controller-charm-path: .* /invalid/path.charm: .*`)
}

func (s *BootstrapSuite) TestBootstrapInvalidControllerCharmChannel(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newBootstrapCommand(), "--controller-charm-channel", "3.0/foo")
	c.Assert(err, gc.ErrorMatches, `controller charm channel "3.0/foo" not valid`)
}

func (s *BootstrapSuite) TestBootstrapSetsControllerOnBase(c *gc.C) {
	// This test ensures that the controller name is correctly set on
	// on the bootstrap commands embedded ModelCommandBase. Without
	// this, the concurrent bootstraps fail.
	// See https://pad.lv/1604223

	s.setupAutoUploadTest(c, "1.8.3", "jammy")

	const controllerName = "dev"

	// Record the controller name seen by ModelCommandBase at the end of bootstrap.
	var seenControllerName string
	s.PatchValue(&waitForAgentInitialisation, func(_ environs.BootstrapContext, base *modelcmd.ModelCommandBase, _ bool, controllerName string) error {
		seenControllerName = controllerName
		return nil
	})

	// Run the bootstrap command in another goroutine, sending the
	// dummy provider ops to opc.
	errc := make(chan error, 1)
	opc := make(chan dummy.Operation)
	dummy.Listen(opc)
	go func() {
		defer func() {
			dummy.Listen(nil)
			close(opc)
		}()
		com := s.newBootstrapCommand()
		args := []string{"dummy", controllerName, "--auto-upgrade"}
		if err := cmdtesting.InitCommand(com, args); err != nil {
			errc <- err
			return
		}
		errc <- com.Run(cmdtesting.Context(c))
	}()

	// Wait for bootstrap to start.
	select {
	case op := <-opc:
		_, ok := op.(dummy.OpBootstrap)
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

	// Simulate another controller being bootstrapped during the
	// bootstrap. Changing the current controller shouldn't affect the
	// bootstrap process.
	c.Assert(s.store.AddController("another", jujuclient.ControllerDetails{
		ControllerUUID: "uuid",
		CACert:         "cert",
	}), jc.ErrorIsNil)
	c.Assert(s.store.SetCurrentController("another"), jc.ErrorIsNil)

	// Let bootstrap finish.
	select {
	case op := <-opc:
		_, ok := op.(dummy.OpFinalizeBootstrap)
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

	// Ensure there were no errors reported.
	select {
	case err := <-errc:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

	// Wait for the ops channel to close.
	select {
	case _, ok := <-opc:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

	// Expect to see that the correct controller was in use at the end
	// of bootstrap.
	c.Assert(seenControllerName, gc.Equals, controllerName)
}

// createToolsSource writes the mock tools and metadata into a temporary
// directory and returns it.
func createToolsSource(c *gc.C, versions []semversion.Binary) string {
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
	cloudsPath := cloud.JujuPersonalCloudsPath()
	err := os.WriteFile(cloudsPath, []byte(`
clouds:
    dummy-cloud:
        type: dummy
        regions:
            region-1:
            region-2:
    dummy-cloud-without-regions:
        type: dummy
    dummy-cloud-dummy-region-config:
        type: dummy
        regions:
            region-1:
            region-2:
        region-config:
            region-1:
                secret: region-test
    dummy-cloud-with-region-config:
        type: dummy
        regions:
            region-1:
            region-2:
        config:
            network: cloud-network
        region-config:
            region-1:
                network: region-network
    dummy-cloud-with-config:
        type: dummy
        config:
            broken: Bootstrap
            somebool: not-a-bool
            use-default-secgroup: true
    many-credentials-no-auth-types:
        type: many-credentials
`[1:]), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

// checkTools check if the environment contains the passed envtools.
func checkTools(c *gc.C, env environs.Environ, expected []semversion.Binary) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	list, err := envtools.FindTools(context.Background(), ss,
		env, jujuversion.Current.Major, jujuversion.Current.Minor, []string{"released"}, coretools.Filter{})
	c.Check(err, jc.ErrorIsNil)
	c.Logf("found: " + list.String())
	urls := list.URLs()
	c.Check(urls, gc.HasLen, len(expected))
}

var (
	v100u64 = semversion.MustParseBinary("1.0.0-ubuntu-amd64")
	v120u64 = semversion.MustParseBinary("1.2.0-ubuntu-amd64")
	v200u64 = semversion.MustParseBinary("2.0.0-ubuntu-amd64")
	v100All = []semversion.Binary{
		v100u64,
	}
	v120All = []semversion.Binary{
		v120u64,
	}
	v200All = []semversion.Binary{
		v200u64,
	}
	vAll = joinBinaryVersions(v100All, v120All, v200All)
)

func joinBinaryVersions(versions ...[]semversion.Binary) []semversion.Binary {
	var all []semversion.Binary
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
	args                bootstrap.BootstrapParams
	newCloudDetector    func(environs.EnvironProvider) (environs.CloudDetector, bool)
	cloudRegionDetector environs.CloudRegionDetector
	cloudFinalizer      environs.CloudFinalizer
	bootstrapF          func(environs.BootstrapContext, environs.BootstrapEnviron, envcontext.ProviderCallContext, bootstrap.BootstrapParams) error
}

func (fake *fakeBootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.BootstrapEnviron, callCtx envcontext.ProviderCallContext, args bootstrap.BootstrapParams) error {
	if fake.bootstrapF != nil {
		return fake.bootstrapF(ctx, env, callCtx, args)
	}
	fake.args = args
	return nil
}

func (fake *fakeBootstrapFuncs) CloudDetector(p environs.EnvironProvider) (environs.CloudDetector, bool) {
	if fake.newCloudDetector != nil {
		return fake.newCloudDetector(p)
	}
	return nil, false
}

func (fake *fakeBootstrapFuncs) CloudRegionDetector(environs.EnvironProvider) (environs.CloudRegionDetector, bool) {
	detector := fake.cloudRegionDetector
	if detector == nil {
		detector = cloudRegionDetectorFunc(func() ([]cloud.Region, error) {
			return nil, errors.NotFoundf("regions")
		})
	}
	return detector, true
}

func (fake *fakeBootstrapFuncs) CloudFinalizer(environs.EnvironProvider) (environs.CloudFinalizer, bool) {
	finalizer := fake.cloudFinalizer
	return finalizer, finalizer != nil
}

type noCloudRegionDetectionProvider struct {
	environs.CloudEnvironProvider
}

type noCloudRegionsProvider struct {
	environs.CloudEnvironProvider
}

func (noCloudRegionsProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

func (noCloudRegionsProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{cloud.EmptyAuthType: {}}
}

type noCredentialsProvider struct {
	environs.CloudEnvironProvider
}

func (noCredentialsProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "region"}}, nil
}

func (noCredentialsProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

func (noCredentialsProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return nil
}

type manyCredentialsProvider struct {
	environs.CloudEnvironProvider
}

func (manyCredentialsProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "region"}}, nil
}

func (manyCredentialsProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	return &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"one": cloud.NewCredential("one", nil),
			"two": {},
		},
	}, nil
}

func (manyCredentialsProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{"one": {}, "two": {}}
}

type cloudDetectorFunc func() ([]cloud.Cloud, error)

type fileCredentialProvider struct {
	environs.CloudEnvironProvider
	testFileName          string
	unFinalizedCredential *cloud.Credential
	finalizedCredential   *cloud.Credential
}

func (f fileCredentialProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "region"}}, nil
}

func (f fileCredentialProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	credential := cloud.NewCredential(cloud.JSONFileAuthType,
		map[string]string{"file": f.testFileName})
	cc := &cloud.CloudCredential{AuthCredentials: map[string]cloud.Credential{
		"cred": credential,
	}}
	*f.unFinalizedCredential = credential
	return cc, nil
}

func (fileCredentialProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{cloud.JSONFileAuthType: {cloud.NamedCredentialAttr{
		Name: "file",
		CredentialAttr: cloud.CredentialAttr{
			FilePath: true,
		}},
	}}
}

func (f fileCredentialProvider) FinalizeCredential(_ environs.FinalizeCredentialContext, fp environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	*f.finalizedCredential = fp.Credential
	return &fp.Credential, nil
}

func (c cloudDetectorFunc) DetectCloud(name string) (cloud.Cloud, error) {
	clouds, err := c.DetectClouds()
	if err != nil {
		return cloud.Cloud{}, err
	}
	for _, aCloud := range clouds {
		if aCloud.Name == name {
			return aCloud, nil
		}
	}
	return cloud.Cloud{}, errors.NotFoundf("cloud %s", name)
}

func (c cloudDetectorFunc) DetectClouds() ([]cloud.Cloud, error) {
	return c()
}

type cloudRegionDetectorFunc func() ([]cloud.Region, error)

func (c cloudRegionDetectorFunc) DetectRegions() ([]cloud.Region, error) {
	return c()
}

type cloudFinalizerFunc func(environs.FinalizeCloudContext, cloud.Cloud) (cloud.Cloud, error)

func (c cloudFinalizerFunc) FinalizeCloud(ctx environs.FinalizeCloudContext, in cloud.Cloud) (cloud.Cloud, error) {
	return c(ctx, in)
}
