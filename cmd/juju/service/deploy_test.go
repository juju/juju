// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmstore.v5-unstable"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type DeploySuite struct {
	testing.RepoSuite
	common.CmdBlockHelper
}

func (s *DeploySuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = common.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&DeploySuite{})

func runDeploy(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, NewDeployCommand(), args...)
	return err
}

var initErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: nil,
		err:  `no charm or bundle specified`,
	}, {
		args: []string{"charm-name", "service-name", "hotdog"},
		err:  `unrecognized args: \["hotdog"\]`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid service name "burble-1"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"craziness", "burble1", "--to", "#:foo"},
		err:  `invalid --to parameter "#:foo"`,
	}, {
		args: []string{"craziness", "burble1", "--constraints", "gibber=plop"},
		err:  `invalid value "gibber=plop" for flag --constraints: unknown constraint "gibber"`,
	}, {
		args: []string{"charm", "service", "--force"},
		err:  `--force is only used with --series`,
	},
}

func (s *DeploySuite) TestInitErrors(c *gc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(NewDeployCommand(), t.args)
		c.Assert(err, gc.ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestNoCharm(c *gc.C) {
	err := runDeploy(c, "local:unknown-123")
	c.Assert(err, gc.ErrorMatches, `.* entity not found in ".*": local:trusty/unknown-123`)
}

func (s *DeploySuite) TestBlockDeploy(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockDeploy")
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	s.AssertBlocked(c, err, ".*TestBlockDeploy.*")
}

func (s *DeploySuite) TestInvalidPath(c *gc.C) {
	err := runDeploy(c, "/home/nowhere")
	c.Assert(err, gc.ErrorMatches, `charm or bundle URL has invalid form: "/home/nowhere"`)
}

func (s *DeploySuite) TestPathWithNoCharm(c *gc.C) {
	err := runDeploy(c, c.MkDir())
	c.Assert(err, gc.ErrorMatches, `no charm or bundle found at .*`)
}

func (s *DeploySuite) TestInvalidURL(c *gc.C) {
	err := runDeploy(c, "cs:craz~ness")
	c.Assert(err, gc.ErrorMatches, `URL has invalid charm or bundle name: "cs:craz~ness"`)
}

func (s *DeploySuite) TestCharmDir(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathRelativeDir(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "multi-series")
	wd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chdir(wd)
	err = os.Chdir(s.SeriesPath)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "multi-series")
	c.Assert(err, gc.ErrorMatches, `.*path "multi-series" can not be a relative path`)
}

func (s *DeploySuite) TestDeployFromPathOldCharm(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, path, "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, "series not specified and charm does not define any")
}

func (s *DeploySuite) TestDeployFromPathDefaultSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.SeriesPath, "multi-series")
	err := runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:precise/multi-series-1")
	s.AssertService(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPath(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.SeriesPath, "multi-series")
	err := runDeploy(c, path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertService(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.SeriesPath, "multi-series")
	err := runDeploy(c, path, "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, `series "quantal" not supported by charm, supported series are: precise,trusty. Use --force to deploy the charm anyway.`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesForce(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.SeriesPath, "multi-series")
	err := runDeploy(c, path, "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:quantal/multi-series-1")
	s.AssertService(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeReportsDeprecated(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	ctx, err := coretesting.RunCommand(c, NewDeployCommand(), "local:dummy", "-u")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
	output := strings.Split(coretesting.Stderr(ctx), "\n")
	c.Check(output[0], gc.Matches, `Added charm ".*" to the model.`)
	c.Check(output[1], gc.Matches, `Deploying charm .*`)
	c.Check(output[2], gc.Equals, "--upgrade (or -u) is deprecated and ignored; charms are always deployed with a unique revision.")
}

func (s *DeploySuite) TestUpgradeCharmDir(c *gc.C) {
	// Add the charm, so the url will exist and a new revision will be
	// picked in service Deploy.
	dummyCharm := s.AddTestingCharm(c, "dummy")

	dirPath := testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:quantal/dummy")
	c.Assert(err, jc.ErrorIsNil)
	upgradedRev := dummyCharm.Revision() + 1
	curl := dummyCharm.URL().WithRevision(upgradedRev)
	s.AssertService(c, "dummy", curl, 1, 0)
	// Check the charm dir was left untouched.
	ch, err := charm.ReadCharmDir(dirPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
}

func (s *DeploySuite) TestCharmBundle(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/logging-1")
	s.AssertService(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) TestConfig(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "dummy-service", "--config", path)
	c.Assert(err, jc.ErrorIsNil)
	service, err := s.State.Service("dummy-service")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"skill-level": int64(9000),
		"username":    "admin001",
	})
}

func (s *DeploySuite) TestRelativeConfigPath(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	// Putting a config file in home is okay as $HOME is set to a tempdir
	setupConfigFile(c, utils.Home())
	err := runDeploy(c, "local:dummy", "dummy-service", "--config", "~/testconfig.yaml")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigError(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "other-service", "--config", path)
	c.Assert(err, gc.ErrorMatches, `no settings found for "other-service"`)
	_, err = s.State.Service("other-service")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeploySuite) TestConstraints(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--constraints", "mem=2G cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeploySuite) TestResources(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	dir := c.MkDir()

	foopath := path.Join(dir, "foo")
	barpath := path.Join(dir, "bar")
	err := ioutil.WriteFile(foopath, []byte("foo"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(barpath, []byte("bar"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	res1 := fmt.Sprintf("foo=%s", foopath)
	res2 := fmt.Sprintf("bar=%s", barpath)

	d := DeployCommand{}
	args := []string{"local:dummy", "--resource", res1, "--resource", res2}

	err = coretesting.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *DeploySuite) TestNetworksIsDeprecated(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--networks", ", net1, net2 , ", "--constraints", "mem=2G cpu-cores=2 networks=net1,net0,^net3,^net4")
	c.Assert(err, gc.ErrorMatches, "use of --networks is deprecated. Please use spaces")
}

// TODO(wallyworld) - add another test that deploy with storage fails for older environments
// (need deploy client to be refactored to use API stub)
func (s *DeploySuite) TestStorage(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	testcharms.Repo.CharmArchivePath(s.SeriesPath, "storage-block")
	err = runDeploy(c, "local:storage-block", "--storage", "data=loop-pool,1G")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/storage-block-1")
	service, _ := s.AssertService(c, "storage-block", curl, 1, 0)

	cons, err := service.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  "loop-pool",
			Count: 1,
			Size:  1024,
		},
		"allecto": {
			Pool:  "loop",
			Count: 0,
			Size:  1024,
		},
	})
}

// TODO(wallyworld) - add another test that deploy with placement fails for older environments
// (need deploy client to be refactored to use API stub)
func (s *DeploySuite) TestPlacement(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	// Add a machine that will be ignored due to placement directive.
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = runDeploy(c, "local:dummy", "-n", "1", "--to", "valid")
	c.Assert(err, jc.ErrorIsNil)

	svc, err := s.State.Service("dummy")
	c.Assert(err, jc.ErrorIsNil)

	// manually run staged assignments
	errs, err := s.APIState.UnitAssigner().AssignUnits([]names.UnitTag{names.NewUnitTag("dummy/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Not(gc.Equals), machine.Id())
}

func (s *DeploySuite) TestSubordinateConstraints(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging", "--constraints", "mem=1G")
	c.Assert(err, gc.ErrorMatches, "cannot use --constraints with subordinate service")
}

func (s *DeploySuite) TestNumUnits(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-n", "13")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err := runDeploy(c, "--num-units", "3", "local:logging")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, gc.ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) assertForceMachine(c *gc.C, machineId string) {
	svc, err := s.State.Service("portlandia")
	c.Assert(err, jc.ErrorIsNil)

	// manually run staged assignments
	errs, err := s.APIState.UnitAssigner().AssignUnits([]names.UnitTag{names.NewUnitTag("portlandia/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machineId)
}

func (s *DeploySuite) TestForceMachine(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", machine.Id(), "local:dummy", "portlandia")
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", container.Id(), "local:dummy", "portlandia")
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, container.Id())
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", "lxc:"+machine.Id(), "local:dummy", "portlandia")
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id()+"/lxc/0")

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		machines, err := s.State.AllMachines()
		c.Assert(err, jc.ErrorIsNil)
		if !a.HasNext() {
			c.Assert(machines, gc.HasLen, 2)
			break
		}
		if len(machines) == 2 {
			break
		}
	}
}

func (s *DeploySuite) TestForceMachineNotFound(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "--to", "42", "local:dummy", "portlandia")
	c.Assert(err, gc.ErrorMatches, `cannot deploy "portlandia" to machine 42: machine 42 not found`)
	_, err = s.State.Service("portlandia")
	c.Assert(err, gc.ErrorMatches, `service "portlandia" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *gc.C) {
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err = runDeploy(c, "--to", machine.Id(), "local:logging")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, gc.ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) TestNonLocalCannotHostUnits(c *gc.C) {
	err := runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, gc.Not(gc.ErrorMatches), "machine 0 is the controller for a local model and cannot host units")
}

func (s *DeploySuite) TestCharmSeries(c *gc.C) {
	deploySeriesTests := []struct {
		requestedSeries string
		force           bool
		seriesFromCharm string
		supportedSeries []string
		modelSeries     string
		ltsSeries       string
		expectedSeries  string
		message         string
		err             string
	}{{
		ltsSeries:       "precise",
		modelSeries:     "wily",
		supportedSeries: []string{"trusty", "precise"},
		expectedSeries:  "trusty",
		message:         "with the default charm metadata series %q",
	}, {
		requestedSeries: "trusty",
		seriesFromCharm: "trusty",
		expectedSeries:  "trusty",
		message:         "with the user specified series %q",
	}, {
		requestedSeries: "wily",
		seriesFromCharm: "trusty",
		err:             `series "wily" not supported by charm, supported series are: trusty`,
	}, {
		requestedSeries: "wily",
		supportedSeries: []string{"trusty", "precise"},
		err:             `series "wily" not supported by charm, supported series are: trusty,precise`,
	}, {
		ltsSeries: config.LatestLtsSeries(),
		err:       `series .* not supported by charm, supported series are: .*`,
	}, {
		modelSeries: "xenial",
		err:         `series "xenial" not supported by charm, supported series are: .*`,
	}, {
		requestedSeries: "wily",
		seriesFromCharm: "trusty",
		expectedSeries:  "wily",
		message:         "with the user specified series %q",
		force:           true,
	}, {
		requestedSeries: "wily",
		supportedSeries: []string{"trusty", "precise"},
		expectedSeries:  "wily",
		message:         "with the user specified series %q",
		force:           true,
	}, {
		ltsSeries:      config.LatestLtsSeries(),
		force:          true,
		expectedSeries: config.LatestLtsSeries(),
		message:        "with the latest LTS series %q",
	}, {
		ltsSeries:      "precise",
		modelSeries:    "xenial",
		force:          true,
		expectedSeries: "xenial",
		message:        "with the configured model default series %q",
	}}

	for i, test := range deploySeriesTests {
		c.Logf("test %d", i)
		cfg, err := config.New(config.UseDefaults, map[string]interface{}{
			"name":            "test",
			"type":            "dummy",
			"uuid":            coretesting.ModelTag.Id(),
			"ca-cert":         coretesting.CACert,
			"ca-private-key":  coretesting.CAKey,
			"authorized-keys": coretesting.FakeAuthKeys,
			"default-series":  test.modelSeries,
		})
		c.Assert(err, jc.ErrorIsNil)
		series, msg, err := charmSeries(test.requestedSeries, test.seriesFromCharm, test.supportedSeries, test.force, cfg)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Check(series, gc.Equals, test.expectedSeries)
		c.Check(msg, gc.Matches, test.message)
	}
}

type DeployLocalSuite struct {
	testing.RepoSuite
}

var _ = gc.Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
}

// setupConfigFile creates a configuration file for testing set
// with the --config argument specifying a configuration file.
func setupConfigFile(c *gc.C, dir string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte("dummy-service:\n  skill-level: 9000\n  username: admin001\n\n")
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

type DeployCharmStoreSuite struct {
	charmStoreSuite
}

var _ = gc.Suite(&DeployCharmStoreSuite{})

var deployAuthorizationTests = []struct {
	about        string
	uploadURL    string
	deployURL    string
	readPermUser string
	expectError  string
	expectOutput string
}{{
	about:     "public charm, success",
	uploadURL: "cs:~bob/trusty/wordpress1-10",
	deployURL: "cs:~bob/trusty/wordpress1",
	expectOutput: `
Added charm "cs:~bob/trusty/wordpress1-10" to the model.
Deploying charm "cs:~bob/trusty/wordpress1-10" with the charm series "trusty".`,
}, {
	about:     "public charm, fully resolved, success",
	uploadURL: "cs:~bob/trusty/wordpress2-10",
	deployURL: "cs:~bob/trusty/wordpress2-10",
	expectOutput: `
Added charm "cs:~bob/trusty/wordpress2-10" to the model.
Deploying charm "cs:~bob/trusty/wordpress2-10" with the charm series "trusty".`,
}, {
	about:        "non-public charm, success",
	uploadURL:    "cs:~bob/trusty/wordpress3-10",
	deployURL:    "cs:~bob/trusty/wordpress3",
	readPermUser: clientUserName,
	expectOutput: `
Added charm "cs:~bob/trusty/wordpress3-10" to the model.
Deploying charm "cs:~bob/trusty/wordpress3-10" with the charm series "trusty".`,
}, {
	about:        "non-public charm, fully resolved, success",
	uploadURL:    "cs:~bob/trusty/wordpress4-10",
	deployURL:    "cs:~bob/trusty/wordpress4-10",
	readPermUser: clientUserName,
	expectOutput: `
Added charm "cs:~bob/trusty/wordpress4-10" to the model.
Deploying charm "cs:~bob/trusty/wordpress4-10" with the charm series "trusty".`,
}, {
	about:        "non-public charm, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress5-10",
	deployURL:    "cs:~bob/trusty/wordpress5",
	readPermUser: "bob",
	expectError:  `cannot resolve (charm )?URL "cs:~bob/trusty/wordpress5": cannot get "/~bob/trusty/wordpress5/meta/any\?include=id&include=supported-series": unauthorized: access denied for user "client-username"`,
}, {
	about:        "non-public charm, fully resolved, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress6-47",
	deployURL:    "cs:~bob/trusty/wordpress6-47",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress6-47": cannot get "/~bob/trusty/wordpress6-47/meta/any\?include=id&include=supported-series": unauthorized: access denied for user "client-username"`,
}, {
	about:     "public bundle, success",
	uploadURL: "cs:~bob/bundle/wordpress-simple1-42",
	deployURL: "cs:~bob/bundle/wordpress-simple1",
	expectOutput: `
added charm cs:trusty/mysql-0
service mysql deployed (charm: cs:trusty/mysql-0)
added charm cs:trusty/wordpress-1
service wordpress deployed (charm: cs:trusty/wordpress-1)
related wordpress:db and mysql:server
added mysql/0 unit to new machine
added wordpress/0 unit to new machine
deployment of bundle "cs:~bob/bundle/wordpress-simple1-42" completed`,
}, {
	about:        "non-public bundle, success",
	uploadURL:    "cs:~bob/bundle/wordpress-simple2-0",
	deployURL:    "cs:~bob/bundle/wordpress-simple2-0",
	readPermUser: clientUserName,
	expectOutput: `
added charm cs:trusty/mysql-0
reusing service mysql (charm: cs:trusty/mysql-0)
added charm cs:trusty/wordpress-1
reusing service wordpress (charm: cs:trusty/wordpress-1)
wordpress:db and mysql:server are already related
avoid adding new units to service mysql: 1 unit already present
avoid adding new units to service wordpress: 1 unit already present
deployment of bundle "cs:~bob/bundle/wordpress-simple2-0" completed`,
}, {
	about:        "non-public bundle, access denied",
	uploadURL:    "cs:~bob/bundle/wordpress-simple3-47",
	deployURL:    "cs:~bob/bundle/wordpress-simple3",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/bundle/wordpress-simple3": cannot get "/~bob/bundle/wordpress-simple3/meta/any\?include=id&include=supported-series": unauthorized: access denied for user "client-username"`,
}}

func (s *DeployCharmStoreSuite) TestDeployAuthorization(c *gc.C) {
	// Upload the two charms required to upload the bundle.
	testcharms.UploadCharm(c, s.client, "trusty/mysql-0", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-1", "wordpress")

	// Run the tests.
	for i, test := range deployAuthorizationTests {
		c.Logf("test %d: %s", i, test.about)

		// Upload the charm or bundle under test.
		url := charm.MustParseURL(test.uploadURL)
		if url.Series == "bundle" {
			url, _ = testcharms.UploadBundle(c, s.client, test.uploadURL, "wordpress-simple")
		} else {
			url, _ = testcharms.UploadCharm(c, s.client, test.uploadURL, "wordpress")
		}

		// Change the ACL of the uploaded entity if required in this case.
		if test.readPermUser != "" {
			s.changeReadPerm(c, url, test.readPermUser)
		}
		ctx, err := coretesting.RunCommand(c, NewDeployCommand(), test.deployURL, fmt.Sprintf("wordpress%d", i))
		if test.expectError != "" {
			c.Check(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		output := strings.Trim(coretesting.Stderr(ctx), "\n")
		c.Check(output, gc.Equals, strings.TrimSpace(test.expectOutput))
	}
}

func (s *DeployCharmStoreSuite) TestDeployWithTermsSuccess(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/terms1-1", "terms1")
	output, err := runDeployCommand(c, "trusty/terms1")
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
Added charm "cs:trusty/terms1-1" to the model.
Deploying charm "cs:trusty/terms1-1" with the charm series "trusty".
Deployment under prior agreement to terms: term1/1 term3/1
`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:trusty/terms1-1")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"terms1": {charm: "cs:trusty/terms1-1"},
	})
	_, err = s.State.Unit("terms1/0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployCharmStoreSuite) TestDeployWithTermsNotSigned(c *gc.C) {
	s.termsDischargerError = &httpbakery.Error{
		Message: "term agreement required: term/1 term/2",
		Code:    "term agreement required",
	}
	testcharms.UploadCharm(c, s.client, "quantal/terms1-1", "terms1")
	_, err := runDeployCommand(c, "quantal/terms1")
	expectedError := `Declined: please agree to the following terms term/1 term/2. Try: "juju agree term/1 term/2"`
	c.Assert(err, gc.ErrorMatches, expectedError)
}

const (
	// clientUserCookie is the name of the cookie which is
	// used to signal to the charmStoreSuite macaroon discharger
	// that the client is a juju client rather than the juju environment.
	clientUserCookie = "client"

	// clientUserName is the name chosen for the juju client
	// when it has authorized.
	clientUserName = "client-username"
)

// charmStoreSuite is a suite fixture that puts the machinery in
// place to allow testing code that calls addCharmViaAPI.
type charmStoreSuite struct {
	testing.JujuConnSuite
	handler              charmstore.HTTPCloseHandler
	srv                  *httptest.Server
	client               *csclient.Client
	discharger           *bakerytest.Discharger
	termsDischarger      *bakerytest.Discharger
	termsDischargerError error
	termsString          string
}

func (s *charmStoreSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Set up the third party discharger.
	s.discharger = bakerytest.NewDischarger(nil, func(req *http.Request, cond string, arg string) ([]checkers.Caveat, error) {
		cookie, err := req.Cookie(clientUserCookie)
		if err != nil {
			return nil, errors.Annotate(err, "discharge denied to non-clients")
		}
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", cookie.Value),
		}, nil
	})

	s.termsDischargerError = nil
	// Set up the third party terms discharger.
	s.termsDischarger = bakerytest.NewDischarger(nil, func(req *http.Request, cond string, arg string) ([]checkers.Caveat, error) {
		s.termsString = arg
		return nil, s.termsDischargerError
	})
	s.termsString = ""

	keyring := bakery.NewPublicKeyRing()

	pk, err := httpbakery.PublicKeyForLocation(http.DefaultClient, s.discharger.Location())
	c.Assert(err, gc.IsNil)
	err = keyring.AddPublicKeyForLocation(s.discharger.Location(), true, pk)
	c.Assert(err, gc.IsNil)

	pk, err = httpbakery.PublicKeyForLocation(http.DefaultClient, s.termsDischarger.Location())
	c.Assert(err, gc.IsNil)
	err = keyring.AddPublicKeyForLocation(s.termsDischarger.Location(), true, pk)
	c.Assert(err, gc.IsNil)

	// Set up the charm store testing server.
	db := s.Session.DB("juju-testing")
	params := charmstore.ServerParams{
		AuthUsername:     "test-user",
		AuthPassword:     "test-password",
		IdentityLocation: s.discharger.Location(),
		PublicKeyLocator: keyring,
		TermsLocation:    s.termsDischarger.Location(),
	}
	handler, err := charmstore.NewServer(db, nil, "", params, charmstore.V4)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.srv = httptest.NewServer(handler)
	s.client = csclient.New(csclient.Params{
		URL:      s.srv.URL,
		User:     params.AuthUsername,
		Password: params.AuthPassword,
	})

	// Initialize the charm cache dir.
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())

	// Point the CLI to the charm store testing server.
	original := newCharmStoreClient
	s.PatchValue(&newCharmStoreClient, func(httpClient *http.Client) *csClient {
		csclient := original(httpClient)
		csclient.params.URL = s.srv.URL
		// Add a cookie so that the discharger can detect whether the
		// HTTP client is the juju environment or the juju client.
		lurl, err := url.Parse(s.discharger.Location())
		c.Assert(err, jc.ErrorIsNil)
		csclient.params.HTTPClient.Jar.SetCookies(lurl, []*http.Cookie{{
			Name:  clientUserCookie,
			Value: clientUserName,
		}})
		return csclient
	})

	// Point the Juju API server to the charm store testing server.
	s.PatchValue(&csclient.ServerURL, s.srv.URL)

	s.PatchValue(&getApiClient, func(*http.Client) (apiClient, error) { return &mockBudgetAPIClient{&jujutesting.Stub{}}, nil })
}

func (s *charmStoreSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.handler.Close()
	s.srv.Close()
	s.JujuConnSuite.TearDownTest(c)
}

// changeReadPerm changes the read permission of the given charm URL.
// The charm must be present in the testing charm store.
func (s *charmStoreSuite) changeReadPerm(c *gc.C, url *charm.URL, perms ...string) {
	err := s.client.Put("/"+url.Path()+"/meta/perm/read", perms)
	c.Assert(err, jc.ErrorIsNil)
}

// assertCharmsUplodaded checks that the given charm ids have been uploaded.
func (s *charmStoreSuite) assertCharmsUplodaded(c *gc.C, ids ...string) {
	charms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	uploaded := make([]string, len(charms))
	for i, charm := range charms {
		uploaded[i] = charm.URL().String()
	}
	c.Assert(uploaded, jc.SameContents, ids)
}

// serviceInfo holds information about a deployed service.
type serviceInfo struct {
	charm            string
	config           charm.Settings
	constraints      constraints.Value
	exposed          bool
	storage          map[string]state.StorageConstraints
	endpointBindings map[string]string
}

// assertDeployedServiceBindings checks that services were deployed into the
// expected spaces. It is separate to assertServicesDeployed because it is only
// relevant to a couple of tests.
func (s *charmStoreSuite) assertDeployedServiceBindings(c *gc.C, info map[string]serviceInfo) {
	services, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)

	for _, service := range services {
		endpointBindings, err := service.EndpointBindings()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(endpointBindings, jc.DeepEquals, info[service.Name()].endpointBindings)
	}
}

// assertServicesDeployed checks that the given services have been deployed.
func (s *charmStoreSuite) assertServicesDeployed(c *gc.C, info map[string]serviceInfo) {
	services, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	deployed := make(map[string]serviceInfo, len(services))
	for _, service := range services {
		charm, _ := service.CharmURL()
		config, err := service.ConfigSettings()
		c.Assert(err, jc.ErrorIsNil)
		if len(config) == 0 {
			config = nil
		}
		constraints, err := service.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		storage, err := service.StorageConstraints()
		c.Assert(err, jc.ErrorIsNil)
		if len(storage) == 0 {
			storage = nil
		}
		deployed[service.Name()] = serviceInfo{
			charm:       charm.String(),
			config:      config,
			constraints: constraints,
			exposed:     service.IsExposed(),
			storage:     storage,
		}
	}
	c.Assert(deployed, jc.DeepEquals, info)
}

// assertRelationsEstablished checks that the given relations have been set.
func (s *charmStoreSuite) assertRelationsEstablished(c *gc.C, relations ...string) {
	rs, err := s.State.AllRelations()
	c.Assert(err, jc.ErrorIsNil)
	established := make([]string, len(rs))
	for i, r := range rs {
		established[i] = r.String()
	}
	c.Assert(established, jc.SameContents, relations)
}

// assertUnitsCreated checks that the given units have been created. The
// expectedUnits argument maps unit names to machine names.
func (s *charmStoreSuite) assertUnitsCreated(c *gc.C, expectedUnits map[string]string) {
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	created := make(map[string]string)
	for _, m := range machines {
		id := m.Id()
		units, err := s.State.UnitsFor(id)
		c.Assert(err, jc.ErrorIsNil)
		for _, u := range units {
			created[u.Name()] = id
		}
	}
	c.Assert(created, jc.DeepEquals, expectedUnits)
}

type testMetricCredentialsSetter struct {
	assert func(string, []byte)
	err    error
}

func (t *testMetricCredentialsSetter) SetMetricCredentials(serviceName string, data []byte) error {
	t.assert(serviceName, data)
	return t.err
}

func (t *testMetricCredentialsSetter) Close() error {
	return nil
}

func (s *DeployCharmStoreSuite) TestAddMetricCredentials(c *gc.C) {
	var called bool
	setter := &testMetricCredentialsSetter{
		assert: func(serviceName string, data []byte) {
			called = true
			c.Assert(serviceName, gc.DeepEquals, "metered")
			var b []byte
			err := json.Unmarshal(data, &b)
			c.Assert(err, gc.IsNil)
			c.Assert(string(b), gc.Equals, "hello registration")
		},
	}

	cleanup := jujutesting.PatchValue(&getMetricCredentialsAPI, func(_ api.Connection) (metricCredentialsAPI, error) {
		return setter, nil
	})
	defer cleanup()

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	deploy := &DeployCommand{Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}}}
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1", "--plan", "someplan")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("cs:quantal/metered-1")
	svc, err := s.State.Service("metered")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := svc.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, curl)
	c.Assert(called, jc.IsTrue)
	modelUUID, _ := s.Environ.Config().UUID()
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:   modelUUID,
			CharmURL:    "cs:quantal/metered-1",
			ServiceName: "metered",
			PlanURL:     "someplan",
			Budget:      "personal",
			Limit:       "0",
		}},
	}})
}

func (s *DeployCharmStoreSuite) TestAddMetricCredentialsDefaultPlan(c *gc.C) {
	var called bool
	setter := &testMetricCredentialsSetter{
		assert: func(serviceName string, data []byte) {
			called = true
			c.Assert(serviceName, gc.DeepEquals, "metered")
			var b []byte
			err := json.Unmarshal(data, &b)
			c.Assert(err, gc.IsNil)
			c.Assert(string(b), gc.Equals, "hello registration")
		},
	}

	cleanup := jujutesting.PatchValue(&getMetricCredentialsAPI, func(_ api.Connection) (metricCredentialsAPI, error) {
		return setter, nil
	})
	defer cleanup()

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	deploy := &DeployCommand{Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}}}
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("cs:quantal/metered-1")
	svc, err := s.State.Service("metered")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := svc.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, curl)
	c.Assert(called, jc.IsTrue)
	modelUUID, _ := s.Environ.Config().UUID()
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"DefaultPlan", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:   modelUUID,
			CharmURL:    "cs:quantal/metered-1",
			ServiceName: "metered",
			PlanURL:     "thisplan",
			Budget:      "personal",
			Limit:       "0",
		}},
	}})
}

func (s *DeploySuite) TestAddMetricCredentialsDefaultForUnmeteredCharm(c *gc.C) {
	var called bool
	setter := &testMetricCredentialsSetter{
		assert: func(serviceName string, data []byte) {
			called = true
			c.Assert(serviceName, gc.DeepEquals, "dummy")
			c.Assert(data, gc.DeepEquals, []byte{})
		},
	}

	cleanup := jujutesting.PatchValue(&getMetricCredentialsAPI, func(_ api.Connection) (metricCredentialsAPI, error) {
		return setter, nil
	})
	defer cleanup()

	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")

	deploy := &DeployCommand{Steps: []DeployStep{&RegisterMeteredCharm{}}}
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "local:dummy")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
	c.Assert(called, jc.IsFalse)
}

func (s *DeploySuite) TestDeployFlags(c *gc.C) {
	command := DeployCommand{}
	flagSet := gnuflag.NewFlagSet(command.Info().Name, gnuflag.ContinueOnError)
	command.SetFlags(flagSet)
	c.Assert(command.flagSet, jc.DeepEquals, flagSet)
	// Add to the slice below if a new flag is introduced which is valid for
	// both charms and bundles.
	charmAndBundleFlags := []string{"repository", "storage"}
	var allFlags []string
	flagSet.VisitAll(func(flag *gnuflag.Flag) {
		allFlags = append(allFlags, flag.Name)
	})
	declaredFlags := append(charmAndBundleFlags, charmOnlyFlags...)
	declaredFlags = append(declaredFlags, bundleOnlyFlags...)
	sort.Strings(declaredFlags)
	c.Assert(declaredFlags, jc.DeepEquals, allFlags)
}

func (s *DeployCharmStoreSuite) TestDeployCharmWithSomeEndpointBindingsSpecifiedSuccess(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	testcharms.UploadCharm(c, s.client, "cs:quantal/wordpress-1", "wordpress")
	err = runDeploy(c, "cs:quantal/wordpress-1", "--bind", "db=db public")
	c.Assert(err, jc.ErrorIsNil)
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"wordpress": {charm: "cs:quantal/wordpress-1"},
	})
	s.assertDeployedServiceBindings(c, map[string]serviceInfo{
		"wordpress": {
			endpointBindings: map[string]string{
				"cache":           "public",
				"url":             "public",
				"logging-dir":     "public",
				"monitoring-port": "public",
				"db":              "db",
			},
		},
	})
}

func (s *DeployCharmStoreSuite) TestDeployCharmsEndpointNotImplemented(c *gc.C) {
	setter := &testMetricCredentialsSetter{
		assert: func(serviceName string, data []byte) {},
		err: &params.Error{
			Message: "IsMetered",
			Code:    params.CodeNotImplemented,
		},
	}
	cleanup := jujutesting.PatchValue(&getMetricCredentialsAPI, func(_ api.Connection) (metricCredentialsAPI, error) {
		return setter, nil
	})
	defer cleanup()

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	deploy := &DeployCommand{Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}}}
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1", "--plan", "someplan")

	c.Assert(err, gc.ErrorMatches, "IsMetered")
}

type ParseBindSuite struct {
}

var _ = gc.Suite(&ParseBindSuite{})

func (s *ParseBindSuite) TestBindParseEmpty(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: ""}
	err := deploy.parseBind()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deploy.Bindings, gc.IsNil)
}

func (s *ParseBindSuite) TestBindParseOK(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "foo=a bar=b"}
	err := deploy.parseBind()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deploy.Bindings, jc.DeepEquals, map[string]string{"foo": "a", "bar": "b"})
}

func (s *ParseBindSuite) TestBindParseServiceDefault(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "service-default"}
	err := deploy.parseBind()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deploy.Bindings, jc.DeepEquals, map[string]string{"": "service-default"})
}

func (s *ParseBindSuite) TestBindParseNoEndpoint(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "=bad"}
	err := deploy.parseBind()
	c.Assert(err.Error(), gc.Equals, parseBindErrorPrefix+"Found = without relation name. Use a lone space name to set the default.")
	c.Assert(deploy.Bindings, gc.IsNil)
}

func (s *ParseBindSuite) TestBindParseBadList(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "foo=bar=baz"}
	err := deploy.parseBind()
	c.Assert(err.Error(), gc.Equals, parseBindErrorPrefix+"Found multiple = in binding. Did you forget to space-separate the binding list?")
	c.Assert(deploy.Bindings, gc.IsNil)
}

func (s *ParseBindSuite) TestBindParseDefaultAndEndpoints(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "rel1=space1  rel2=space2 space3"}
	err := deploy.parseBind()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deploy.Bindings, jc.DeepEquals, map[string]string{"rel1": "space1", "rel2": "space2", "": "space3"})
}

func (s *ParseBindSuite) TestBindParseDefaultAndEndpoints2(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "rel1=space1  space3 rel2=space2"}
	err := deploy.parseBind()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deploy.Bindings, jc.DeepEquals, map[string]string{"rel1": "space1", "rel2": "space2", "": "space3"})
}

func (s *ParseBindSuite) TestBindParseDefaultAndEndpoints3(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "space3  rel1=space1 rel2=space2"}
	err := deploy.parseBind()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deploy.Bindings, jc.DeepEquals, map[string]string{"rel1": "space1", "rel2": "space2", "": "space3"})
}

func (s *ParseBindSuite) TestBindParseBadSpace(c *gc.C) {
	deploy := &DeployCommand{BindToSpaces: "rel1=spa#ce1"}
	err := deploy.parseBind()
	c.Assert(err.Error(), gc.Equals, parseBindErrorPrefix+"Space name invalid.")
	c.Assert(deploy.Bindings, gc.IsNil)
}
