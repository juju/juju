// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type DeploySuite struct {
	testing.RepoSuite
	CmdBlockHelper
}

func (s *DeploySuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&DeploySuite{})

func runDeploy(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), args...)
	return err
}

var initErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: nil,
		err:  `no charm specified`,
	}, {
		args: []string{"charm-name", "service-name", "hotdog"},
		err:  `unrecognized args: \["hotdog"\]`,
	}, {
		args: []string{"craz~ness"},
		err:  `invalid charm name "craz~ness"`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid service name "burble-1"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"craziness", "burble1", "--to", "bigglesplop"},
		err:  `invalid --to parameter "bigglesplop"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "2", "--to", "123"},
		err:  `cannot use --num-units > 1 with --to`,
	}, {
		args: []string{"craziness", "burble1", "--constraints", "gibber=plop"},
		err:  `invalid value "gibber=plop" for flag --constraints: unknown constraint "gibber"`,
	},
}

func (s *DeploySuite) TestInitErrors(c *gc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(envcmd.Wrap(&DeployCommand{}), t.args)
		c.Assert(err, gc.ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestNoCharm(c *gc.C) {
	err := runDeploy(c, "local:unknown-123")
	c.Assert(err, gc.ErrorMatches, `charm not found in ".*": local:trusty/unknown-123`)
}

func (s *DeploySuite) TestBlockDeploy(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockDeploy")
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	s.AssertBlocked(c, err, ".*TestBlockDeploy.*")
}

func (s *DeploySuite) TestCharmDir(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeReportsDeprecated(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), "local:dummy", "-u")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
	output := strings.Split(coretesting.Stderr(ctx), "\n")
	c.Check(output[0], gc.Matches, `Added charm ".*" to the environment.`)
	c.Check(output[1], gc.Equals, "--upgrade (or -u) is deprecated and ignored; charms are always deployed with a unique revision.")
}

func (s *DeploySuite) TestUpgradeCharmDir(c *gc.C) {
	// Add the charm, so the url will exist and a new revision will be
	// picked in ServiceDeploy.
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
	err := runDeploy(c, "local:dummy", "--constraints", "mem=2G cpu-cores=2 networks=net1,^net2")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cpu-cores=2 networks=net1,^net2"))
}

func (s *DeploySuite) TestNetworks(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--networks", ", net1, net2 , ", "--constraints", "mem=2G cpu-cores=2 networks=net1,net0,^net3,^net4")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	networks, err := service.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, jc.DeepEquals, []string{"net1", "net2"})
	cons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cpu-cores=2 networks=net1,net0,^net3,^net4"))
}

func (s *DeploySuite) TestStorageWithoutFeatureFlag(c *gc.C) {
	err := runDeploy(c, "local:storage-block", "--storage", "data=1G")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: --storage")
}

func (s *DeploySuite) TestStorage(c *gc.C) {
	s.SetFeatureFlags(feature.Storage)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
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
	})
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
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
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
	c.Assert(err, gc.Not(gc.ErrorMatches), "machine 0 is the state server for a local environment and cannot host units")
}

type DeployLocalSuite struct {
	testing.RepoSuite
}

var _ = gc.Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)

	// override provider type
	s.PatchValue(&getClientConfig, func(client *api.Client) (*config.Config, error) {
		attrs, err := client.EnvironmentGet()
		if err != nil {
			return nil, err
		}
		attrs["type"] = "local"

		return config.New(config.NoDefaults, attrs)
	})
}

func (s *DeployLocalSuite) TestLocalCannotHostUnits(c *gc.C) {
	err := runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, gc.ErrorMatches, "machine 0 is the state server for a local environment and cannot host units")
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
