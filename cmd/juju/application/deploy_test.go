// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	csclientparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/charmstore.v5-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	jtesting "github.com/juju/juju/testing"
)

type DeploySuite struct {
	testing.RepoSuite
	coretesting.CmdBlockHelper
}

var _ = gc.Suite(&DeploySuite{})

func (s *DeploySuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

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
		args: []string{"charm-name", "application-name", "hotdog"},
		err:  `unrecognized args: \["hotdog"\]`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid application name "burble-1"`,
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
		args: []string{"charm", "application", "--force"},
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

func (s *DeploySuite) TestNoCharmOrBundle(c *gc.C) {
	err := runDeploy(c, c.MkDir())
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `charm or bundle at .*`)
}

func (s *DeploySuite) TestBlockDeploy(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockDeploy")
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "some-application-name", "--series", "quantal")
	s.AssertBlocked(c, err, ".*TestBlockDeploy.*")
}

func (s *DeploySuite) TestInvalidPath(c *gc.C) {
	err := runDeploy(c, "/home/nowhere")
	c.Assert(err, gc.ErrorMatches, `charm or bundle URL has invalid form: "/home/nowhere"`)
}

func (s *DeploySuite) TestInvalidFileFormat(c *gc.C) {
	path := filepath.Join(c.MkDir(), "bundle.yaml")
	err := ioutil.WriteFile(path, []byte(":"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, `invalid charm or bundle provided at ".*bundle.yaml"`)
}

func (s *DeploySuite) TestPathWithNoCharmOrBundle(c *gc.C) {
	err := runDeploy(c, c.MkDir())
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `charm or bundle at .*`)
}

func (s *DeploySuite) TestInvalidURL(c *gc.C) {
	err := runDeploy(c, "cs:craz~ness")
	c.Assert(err, gc.ErrorMatches, `URL has invalid charm or bundle name: "cs:craz~ness"`)
}

func (s *DeploySuite) TestCharmDir(c *gc.C) {
	ch := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathRelativeDir(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	wd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chdir(wd)
	err = os.Chdir(s.CharmsPath)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "multi-series")
	c.Assert(err, gc.ErrorMatches, ""+
		"The charm or bundle \"multi-series\" is ambiguous.\n"+
		"To deploy a local charm or bundle, run `juju deploy ./multi-series`.\n"+
		"To deploy a charm or bundle from the store, run `juju deploy cs:multi-series`.")
}

func (s *DeploySuite) TestDeployFromPathOldCharm(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	err := runDeploy(c, path, "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	err := runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, "series not specified and charm does not define any")
}

func (s *DeploySuite) TestDeployFromPathDefaultSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:precise/multi-series-1")
	s.AssertService(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPath(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertService(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, path, "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, `series "quantal" not supported by charm, supported series are: precise,trusty. Use --force to deploy the charm anyway.`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesForce(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, path, "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:quantal/multi-series-1")
	s.AssertService(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeCharmDir(c *gc.C) {
	// Add the charm, so the url will exist and a new revision will be
	// picked in application Deploy.
	dummyCharm := s.AddTestingCharm(c, "dummy")

	dirPath := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	err := runDeploy(c, dirPath, "--series", "quantal")
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "some-application-name", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-application-name", curl, 1, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "logging")
	err := runDeploy(c, ch, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/logging-1")
	s.AssertService(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) TestConfig(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, ch, "dummy-application", "--config", path, "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := application.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"skill-level": int64(9000),
		"username":    "admin001",
	})
}

func (s *DeploySuite) TestRelativeConfigPath(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	// Putting a config file in home is okay as $HOME is set to a tempdir
	setupConfigFile(c, utils.Home())
	err := runDeploy(c, ch, "dummy-application", "--config", "~/testconfig.yaml", "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigError(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, ch, "other-application", "--config", path, "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, `no settings found for "other-application"`)
	_, err = s.State.Application("other-application")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeploySuite) TestConstraints(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "--constraints", "mem=2G cpu-cores=2", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	application, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := application.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeploySuite) TestResources(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
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
	args := []string{ch, "--resource", res1, "--resource", res2, "--series", "quantal"}

	err = coretesting.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

// TODO(ericsnow) Add tests for charmstore-based resources once the
// endpoints are implemented.

// TODO(wallyworld) - add another test that deploy with storage fails for older environments
// (need deploy client to be refactored to use API stub)
func (s *DeploySuite) TestStorage(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "storage-block")
	err := runDeploy(c, ch, "--storage", "data=machinescoped,1G", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/storage-block-1")
	application, _ := s.AssertService(c, "storage-block", curl, 1, 0)

	cons, err := application.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  "machinescoped",
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

func (s *DeploySuite) TestPlacement(c *gc.C) {
	ch := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	// Add a machine that will be ignored due to placement directive.
	machine, err := s.State.AddMachine(series.LatestLts(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = runDeploy(c, ch, "-n", "1", "--to", "valid", "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)

	svc, err := s.State.Application("dummy")
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "logging")
	err := runDeploy(c, ch, "--constraints", "mem=1G", "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, "cannot use --constraints with subordinate application")
}

func (s *DeploySuite) TestNumUnits(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "-n", "13", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "logging")
	err := runDeploy(c, "--num-units", "3", ch, "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate application")
	_, err = s.State.Application("dummy")
	c.Assert(err, gc.ErrorMatches, `application "dummy" not found`)
}

func (s *DeploySuite) assertForceMachine(c *gc.C, machineId string) {
	svc, err := s.State.Application("portlandia")
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	machine, err := s.State.AddMachine(series.LatestLts(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", machine.Id(), ch, "portlandia", "--series", series.LatestLts())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	template := state.MachineTemplate{
		Series: series.LatestLts(),
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", container.Id(), ch, "portlandia", "--series", series.LatestLts())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, container.Id())
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	machine, err := s.State.AddMachine(series.LatestLts(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", "lxd:"+machine.Id(), ch, "portlandia", "--series", series.LatestLts())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id()+"/lxd/0")

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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, "--to", "42", ch, "portlandia", "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, `cannot deploy "portlandia" to machine 42: machine 42 not found`)
	_, err = s.State.Application("portlandia")
	c.Assert(err, gc.ErrorMatches, `application "portlandia" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *gc.C) {
	machine, err := s.State.AddMachine(series.LatestLts(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "logging")
	err = runDeploy(c, "--to", machine.Id(), ch, "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate application")
	_, err = s.State.Application("dummy")
	c.Assert(err, gc.ErrorMatches, `application "dummy" not found`)
}

func (s *DeploySuite) TestNonLocalCannotHostUnits(c *gc.C) {
	err := runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, gc.Not(gc.ErrorMatches), "machine 0 is the controller for a local model and cannot host units")
}

func (s *DeploySuite) TestDeployLocalWithTerms(c *gc.C) {
	ch := testcharms.Repo.ClonedDirPath(s.CharmsPath, "terms1")
	output, err := runDeployCommand(c, ch, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(output, gc.Equals, `Deploying charm "local:trusty/terms1-1".`)

	curl := charm.MustParseURL("local:trusty/terms1-1")
	s.AssertService(c, "terms1", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFlags(c *gc.C) {
	command := DeployCommand{}
	flagSet := gnuflag.NewFlagSet(command.Info().Name, gnuflag.ContinueOnError)
	command.SetFlags(flagSet)
	c.Assert(command.flagSet, jc.DeepEquals, flagSet)
	// Add to the slice below if a new flag is introduced which is valid for
	// both charms and bundles.
	charmAndBundleFlags := []string{"channel", "storage"}
	var allFlags []string
	flagSet.VisitAll(func(flag *gnuflag.Flag) {
		allFlags = append(allFlags, flag.Name)
	})
	declaredFlags := append(charmAndBundleFlags, charmOnlyFlags...)
	declaredFlags = append(declaredFlags, bundleOnlyFlags...)
	declaredFlags = append(declaredFlags, modelCommandBaseFlags...)
	sort.Strings(declaredFlags)
	c.Assert(declaredFlags, jc.DeepEquals, allFlags)
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
	content := []byte("dummy-application:\n  skill-level: 9000\n  username: admin001\n\n")
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
Located charm "cs:~bob/trusty/wordpress1-10".
Deploying charm "cs:~bob/trusty/wordpress1-10".`,
}, {
	about:     "public charm, fully resolved, success",
	uploadURL: "cs:~bob/trusty/wordpress2-10",
	deployURL: "cs:~bob/trusty/wordpress2-10",
	expectOutput: `
Located charm "cs:~bob/trusty/wordpress2-10".
Deploying charm "cs:~bob/trusty/wordpress2-10".`,
}, {
	about:        "non-public charm, success",
	uploadURL:    "cs:~bob/trusty/wordpress3-10",
	deployURL:    "cs:~bob/trusty/wordpress3",
	readPermUser: clientUserName,
	expectOutput: `
Located charm "cs:~bob/trusty/wordpress3-10".
Deploying charm "cs:~bob/trusty/wordpress3-10".`,
}, {
	about:        "non-public charm, fully resolved, success",
	uploadURL:    "cs:~bob/trusty/wordpress4-10",
	deployURL:    "cs:~bob/trusty/wordpress4-10",
	readPermUser: clientUserName,
	expectOutput: `
Located charm "cs:~bob/trusty/wordpress4-10".
Deploying charm "cs:~bob/trusty/wordpress4-10".`,
}, {
	about:        "non-public charm, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress5-10",
	deployURL:    "cs:~bob/trusty/wordpress5",
	readPermUser: "bob",
	expectError:  `cannot resolve (charm )?URL "cs:~bob/trusty/wordpress5": cannot get "/~bob/trusty/wordpress5/meta/any\?include=id&include=supported-series&include=published": unauthorized: access denied for user "client-username"`,
}, {
	about:        "non-public charm, fully resolved, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress6-47",
	deployURL:    "cs:~bob/trusty/wordpress6-47",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress6-47": cannot get "/~bob/trusty/wordpress6-47/meta/any\?include=id&include=supported-series&include=published": unauthorized: access denied for user "client-username"`,
}, {
	about:     "public bundle, success",
	uploadURL: "cs:~bob/bundle/wordpress-simple1-42",
	deployURL: "cs:~bob/bundle/wordpress-simple1",
	expectOutput: `
added charm cs:trusty/mysql-0
application mysql deployed (charm cs:trusty/mysql-0)
added charm cs:trusty/wordpress-1
application wordpress deployed (charm cs:trusty/wordpress-1)
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
reusing application mysql (charm: cs:trusty/mysql-0)
added charm cs:trusty/wordpress-1
reusing application wordpress (charm: cs:trusty/wordpress-1)
wordpress:db and mysql:server are already related
avoid adding new units to application mysql: 1 unit already present
avoid adding new units to application wordpress: 1 unit already present
deployment of bundle "cs:~bob/bundle/wordpress-simple2-0" completed`,
}, {
	about:        "non-public bundle, access denied",
	uploadURL:    "cs:~bob/bundle/wordpress-simple3-47",
	deployURL:    "cs:~bob/bundle/wordpress-simple3",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/bundle/wordpress-simple3": cannot get "/~bob/bundle/wordpress-simple3/meta/any\?include=id&include=supported-series&include=published": unauthorized: access denied for user "client-username"`,
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
Located charm "cs:trusty/terms1-1".
Deploying charm "cs:trusty/terms1-1".
Deployment under prior agreement to terms: term1/1 term3/1
`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUploaded(c, "cs:trusty/terms1-1")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
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

func (s *DeployCharmStoreSuite) TestDeployWithChannel(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "wordpress")
	id := charm.MustParseURL("cs:~client-username/precise/wordpress-0")
	err := s.client.UploadCharmWithRevision(id, ch, -1)
	c.Assert(err, gc.IsNil)

	err = s.client.Publish(id, []csclientparams.Channel{csclientparams.EdgeChannel}, nil)
	c.Assert(err, gc.IsNil)

	_, err = runDeployCommand(c, "--channel", "edge", "~client-username/wordpress")
	c.Assert(err, gc.IsNil)
	s.assertCharmsUploaded(c, "cs:~client-username/precise/wordpress-0")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"wordpress": {charm: "cs:~client-username/precise/wordpress-0"},
	})
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
	handler, err := charmstore.NewServer(db, nil, "", params, charmstore.V5)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.srv = httptest.NewServer(handler)
	c.Logf("started charmstore on %v", s.srv.URL)
	s.client = csclient.New(csclient.Params{
		URL:      s.srv.URL,
		User:     params.AuthUsername,
		Password: params.AuthPassword,
	})

	// Initialize the charm cache dir.
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())

	// Point the CLI to the charm store testing server.
	s.PatchValue(&newCharmStoreClient, func(client *httpbakery.Client) *csclient.Client {
		// Add a cookie so that the discharger can detect whether the
		// HTTP client is the juju environment or the juju client.
		lurl, err := url.Parse(s.discharger.Location())
		c.Assert(err, jc.ErrorIsNil)
		client.Jar.SetCookies(lurl, []*http.Cookie{{
			Name:  clientUserCookie,
			Value: clientUserName,
		}})
		return csclient.New(csclient.Params{
			URL:          s.srv.URL,
			BakeryClient: client,
		})
	})

	// Point the Juju API server to the charm store testing server.
	s.PatchValue(&csclient.ServerURL, s.srv.URL)
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

// assertCharmsUploaded checks that the given charm ids have been uploaded.
func (s *charmStoreSuite) assertCharmsUploaded(c *gc.C, ids ...string) {
	charms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	uploaded := make([]string, len(charms))
	for i, charm := range charms {
		uploaded[i] = charm.URL().String()
	}
	c.Assert(uploaded, jc.SameContents, ids)
}

// serviceInfo holds information about a deployed application.
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
	services, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)

	for _, application := range services {
		endpointBindings, err := application.EndpointBindings()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(endpointBindings, jc.DeepEquals, info[application.Name()].endpointBindings)
	}
}

// assertApplicationsDeployed checks that the given applications have been deployed.
func (s *charmStoreSuite) assertApplicationsDeployed(c *gc.C, info map[string]serviceInfo) {
	services, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	deployed := make(map[string]serviceInfo, len(services))
	for _, application := range services {
		charm, _ := application.CharmURL()
		config, err := application.ConfigSettings()
		c.Assert(err, jc.ErrorIsNil)
		if len(config) == 0 {
			config = nil
		}
		constraints, err := application.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		storage, err := application.StorageConstraints()
		c.Assert(err, jc.ErrorIsNil)
		if len(storage) == 0 {
			storage = nil
		}
		deployed[application.Name()] = serviceInfo{
			charm:       charm.String(),
			config:      config,
			constraints: constraints,
			exposed:     application.IsExposed(),
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
	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	charmDir := testcharms.Repo.CharmDir("metered")

	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    charmDir.Meta(),
				Metrics: charmDir.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: true},
			// Call to SetMetricCredentials
			params.ErrorResults{
				Results: []params.ErrorResult{{}},
			},
		},
	}

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1", "--plan", "someplan")
	c.Assert(err, jc.ErrorIsNil)

	fakeAPI.CheckCall(c, 9, "SetMetricCredentials",
		"metered",
		// `"hello registration"\n` (quotes and newline
		// from json encoding) is returned by the fake
		// http server. This is binary64 encoded before
		// the call into SetMetricCredentials.
		append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA),
	)

	stub.CheckCalls(c, []jujutesting.StubCall{{
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "metered",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "0",
		}},
	}})
}

func (s *DeployCharmStoreSuite) TestAddMetricCredentialsDefaultPlan(c *gc.C) {
	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	charmDir := testcharms.Repo.CharmDir("metered")

	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    charmDir.Meta(),
				Metrics: charmDir.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: true},
			// Call to SetMetricCredentials
			params.ErrorResults{
				Results: []params.ErrorResult{{}},
			},
		},
	}
	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1")
	c.Assert(err, jc.ErrorIsNil)

	fakeAPI.CheckCall(c, 9, "SetMetricCredentials",
		"metered",
		// `"hello registration"\n` (quotes and newline
		// from json encoding) is returned by the fake
		// http server. This is binary64 encoded before
		// the call into SetMetricCredentials.
		append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA),
	)

	c.Logf("KT: %v", fakeAPI.Calls())

	stub.CheckCalls(c, []jujutesting.StubCall{{
		"DefaultPlan", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "metered",
			PlanURL:         "thisplan",
			Budget:          "personal",
			Limit:           "0",
		}},
	}})
}

func (s *DeployCharmStoreSuite) TestSetMetricCredentialsNotCalledForUnmeteredCharm(c *gc.C) {
	charmDir := testcharms.Repo.CharmDir("dummy")
	testcharms.UploadCharm(c, s.client, "cs:quantal/dummy-1", "dummy")
	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    charmDir.Meta(),
				Metrics: charmDir.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: false},
		},
	}

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}

	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/dummy-1")
	c.Assert(err, jc.ErrorIsNil)
	fakeAPI.CheckCallNames(c,
		"BestFacadeVersion",
		"APICall",
		"BestFacadeVersion",
		"APICall",
		"AddCharm",
		"CharmInfo",
		"ModelUUID",
		"IsMetered",
		"Deploy",
		"Close",
	)
}

func (s *DeployCharmStoreSuite) TestAddMetricCredentialsNotNeededForOptionalPlan(c *gc.C) {
	metricsYAML := `
plan:
  required: false
metrics:
  pings:
    type: gauge
    description: ping pongs
`
	meteredMetaYAML := `
name: metered
description: metered charm
summary: summary
`
	url, ch := testcharms.UploadCharmWithMeta(c, s.client, "cs:~user/quantal/metered", meteredMetaYAML, metricsYAML, 1)
	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    ch.Meta(),
				Metrics: ch.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: true},
		},
	}

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()
	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}

	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), url.String())
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckNoCalls(c)
	fakeAPI.CheckCallNames(c,
		"BestFacadeVersion",
		"APICall",
		"BestFacadeVersion",
		"APICall",
		"AddCharm",
		"CharmInfo",
		"ModelUUID",
		"IsMetered",
		"Deploy",
		"Close",
	)
}

func (s *DeployCharmStoreSuite) TestSetMetricCredentialsCalledWhenPlanSpecifiedWhenOptional(c *gc.C) {
	metricsYAML := `
plan:
  required: false
metrics:
  pings:
    type: gauge
    description: ping pongs
`
	meteredMetaYAML := `
name: metered
description: metered charm
summary: summary
`
	url, ch := testcharms.UploadCharmWithMeta(c, s.client, "cs:~user/quantal/metered", meteredMetaYAML, metricsYAML, 1)
	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    ch.Meta(),
				Metrics: ch.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: true},
			// Call to SetMetricCredentials
			params.ErrorResults{
				Results: []params.ErrorResult{{}},
			},
		},
	}

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()
	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}

	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), url.String(), "--plan", "someplan")
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:~user/quantal/metered-0",
			ApplicationName: "metered",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "0",
		}},
	}})
	fakeAPI.CheckCallNames(c,
		"BestFacadeVersion",
		"APICall",
		"BestFacadeVersion",
		"APICall",
		"AddCharm",
		"CharmInfo",
		"ModelUUID",
		"IsMetered",
		"Deploy",
		"SetMetricCredentials",
		"Close",
	)
}

// FAILING
func (s *DeployCharmStoreSuite) TestDeployCharmWithSomeEndpointBindingsSpecifiedSuccess(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	testcharms.UploadCharm(c, s.client, "cs:quantal/wordpress-extra-bindings-1", "wordpress-extra-bindings")
	err = runDeploy(c, "cs:quantal/wordpress-extra-bindings-1", "--bind", "db=db db-client=db public admin-api=public")
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"wordpress-extra-bindings": {charm: "cs:quantal/wordpress-extra-bindings-1"},
	})
	s.assertDeployedServiceBindings(c, map[string]serviceInfo{
		"wordpress-extra-bindings": {
			endpointBindings: map[string]string{
				"cache":           "public",
				"url":             "public",
				"logging-dir":     "public",
				"monitoring-port": "public",
				"db":              "db",
				"db-client":       "db",
				"admin-api":       "public",
				"foo-bar":         "public",
				"cluster":         "public",
			},
		},
	})
}

func (s *DeployCharmStoreSuite) TestDeployCharmsEndpointNotImplemented(c *gc.C) {
	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	charmDir := testcharms.Repo.CharmDir("metered")

	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to ModelGet
			0,
			params.ModelConfigResults{
				Config: map[string]params.ConfigValue{
					"name": {Value: "name"},
					"uuid": {Value: "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
					"type": {Value: "foo"},
				},
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    charmDir.Meta(),
				Metrics: charmDir.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: true},
			// Call to SetMetricCredentials
			params.ErrorResults{
				Results: []params.ErrorResult{{}},
			},
		},
	}
	fakeAPI.Stub.SetErrors(errors.New("IsMetered"))

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{RegisterURL: server.URL, QueryURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}
	_, err := coretesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1", "--plan", "someplan")

	c.Check(err, gc.ErrorMatches, "cannot fetch model settings")
	c.Check(errors.Details(err), gc.Matches, ".*IsMetered.*")
}

type ParseBindSuite struct {
}

var _ = gc.Suite(&ParseBindSuite{})

func (s *ParseBindSuite) TestParseSuccessWithEmptyArgs(c *gc.C) {
	s.checkParseOKForArgs(c, "", nil)
}

func (s *ParseBindSuite) TestParseSuccessWithEndpointsOnly(c *gc.C) {
	s.checkParseOKForArgs(c, "foo=a bar=b", map[string]string{"foo": "a", "bar": "b"})
}

func (s *ParseBindSuite) TestParseSuccessWithServiceDefaultSpaceOnly(c *gc.C) {
	s.checkParseOKForArgs(c, "application-default", map[string]string{"": "application-default"})
}

func (s *ParseBindSuite) TestBindingsOrderForDefaultSpaceAndEndpointsDoesNotMatter(c *gc.C) {
	expectedBindings := map[string]string{
		"ep1": "sp1",
		"ep2": "sp2",
		"":    "sp3",
	}
	s.checkParseOKForArgs(c, "ep1=sp1 ep2=sp2 sp3", expectedBindings)
	s.checkParseOKForArgs(c, "ep1=sp1 sp3 ep2=sp2", expectedBindings)
	s.checkParseOKForArgs(c, "ep2=sp2 ep1=sp1 sp3", expectedBindings)
	s.checkParseOKForArgs(c, "ep2=sp2 sp3 ep1=sp1", expectedBindings)
	s.checkParseOKForArgs(c, "sp3 ep1=sp1 ep2=sp2", expectedBindings)
	s.checkParseOKForArgs(c, "sp3 ep2=sp2 ep1=sp1", expectedBindings)
}

func (s *ParseBindSuite) TestParseFailsWithSpaceNameButNoEndpoint(c *gc.C) {
	s.checkParseFailsForArgs(c, "=bad", "Found = without endpoint name. Use a lone space name to set the default.")
}

func (s *ParseBindSuite) TestParseFailsWithTooManyEqualsSignsInArgs(c *gc.C) {
	s.checkParseFailsForArgs(c, "foo=bar=baz", "Found multiple = in binding. Did you forget to space-separate the binding list?")
}

func (s *ParseBindSuite) TestParseFailsWithBadSpaceName(c *gc.C) {
	s.checkParseFailsForArgs(c, "rel1=spa#ce1", "Space name invalid.")
}

func (s *ParseBindSuite) runParseBindWithArgs(args string) (error, map[string]string) {
	deploy := &DeployCommand{BindToSpaces: args}
	return deploy.parseBind(), deploy.Bindings
}

func (s *ParseBindSuite) checkParseOKForArgs(c *gc.C, args string, expectedBindings map[string]string) {
	err, parsedBindings := s.runParseBindWithArgs(args)
	c.Check(err, jc.ErrorIsNil)
	c.Check(parsedBindings, jc.DeepEquals, expectedBindings)
}

func (s *ParseBindSuite) checkParseFailsForArgs(c *gc.C, args string, expectedErrorSuffix string) {
	err, parsedBindings := s.runParseBindWithArgs(args)
	c.Check(err.Error(), gc.Equals, parseBindErrorPrefix+expectedErrorSuffix)
	c.Check(parsedBindings, gc.IsNil)
}

type DeployUnitTestSuite struct {
	jujutesting.IsolationSuite
	DeployAPI
}

var _ = gc.Suite(&DeployUnitTestSuite{})

func (s *DeployUnitTestSuite) TestDeployLocalCharm_GivesCorrectUserMessage(c *gc.C) {
	// Copy dummy charm to path where we can deploy it from
	charmsPath := c.MkDir()
	charmDir := testcharms.Repo.ClonedDir(charmsPath, "dummy")

	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to AddLocalCharm
			&charm.URL{
				Schema:   "local",
				User:     "joe",
				Name:     "wordpress",
				Revision: -1,
				Series:   "trusty",
				Channel:  "development",
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    charmDir.Meta(),
				Metrics: charmDir.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: false},
		},
	}

	cmd := NewDeployCommandWithAPI(func() (DeployAPI, error) {
		return fakeAPI, nil
	})
	context, err := jtesting.RunCommand(c, cmd, charmDir.Path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(jtesting.Stderr(context), gc.Equals, `Deploying charm "local:~joe/development/trusty/wordpress".`+"\n")
}

func (s *DeployUnitTestSuite) TestAddMetricCredentialsDefaultForUnmeteredCharm(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.Repo.ClonedDir(charmsPath, "dummy")
	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to AddLocalCharm
			&charm.URL{
				Schema:   "local",
				User:     "joe",
				Name:     "dummy",
				Revision: -1,
				Series:   "trusty",
				Channel:  "development",
			},
			// Call to CharmInfo
			&charms.CharmInfo{
				Meta:    charmDir.Meta(),
				Metrics: charmDir.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d",
			true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: false},
		},
	}
	deployCmd := NewDeployCommandWithAPI(func() (DeployAPI, error) { return fakeAPI, nil })
	_, err := coretesting.RunCommand(c, deployCmd, charmDir.Path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)

	// We never attempt to set metric credentials
	for _, call := range fakeAPI.Calls() {
		if call.FuncName == "FacadeCall" {
			c.Assert(call.Args[0], gc.Not(gc.Matches), "SetMetricCredentials")
		}
	}
}

func (s *DeployUnitTestSuite) TestRedeployLocalCharm_SucceedsWhenDeployed(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.Repo.ClonedDir(charmsPath, "dummy")
	fakeAPI := &fakeDeployAPI{
		ReturnValues: []interface{}{
			// Call to CharmInfo
			&charms.CharmInfo{
				URL:     "local:trusty/dummy",
				Meta:    charmDir.Meta(),
				Metrics: charmDir.Metrics(),
			},
			// Call to ModelUUID
			"deadbeef-0bad-400d-8000-4b1d0d06f00d", true,
			// Call to IsMetered
			params.IsMeteredResult{Metered: false},
		},
	}
	deployCmd := NewDeployCommandWithAPI(func() (DeployAPI, error) { return fakeAPI, nil })
	context, err := jtesting.RunCommand(c, deployCmd, "local:trusty/dummy-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(jtesting.Stderr(context), gc.Equals,
		`Located charm "local:trusty/dummy-0".`+"\n"+
			`Deploying charm "local:trusty/dummy-0".`+"\n",
	)
}

// fakeDeployAPI is a mock of the API used by the deploy command. It's
// a little muddled at the moment, but as the DeployAPI interface is
// sharpened, this will become so as well.
type fakeDeployAPI struct {
	jujutesting.Stub
	DeployAPI

	// ReturnValues holds a set of things the various methods should
	// return.
	ReturnValues []interface{}
}

func (f *fakeDeployAPI) Close() error {
	f.AddCall("Close")
	return f.NextErr()
}

func (f *fakeDeployAPI) BestFacadeVersion(facade string) int {
	f.AddCall("BestFacadeVersion", facade)
	retVal := f.ReturnValues[0].(int)
	f.ReturnValues = f.ReturnValues[1:]
	return retVal
}

func (f *fakeDeployAPI) FacadeCall(request string, params, response interface{}) error {
	f.AddCall("FacadeCall", request, params, response)
	marshaled, err := json.Marshal(f.ReturnValues[0])
	if err != nil {
		panic(err)
	}
	json.Unmarshal(marshaled, &response)
	f.ReturnValues = f.ReturnValues[1:]
	return f.NextErr()
}

func (f *fakeDeployAPI) APICall(
	objType string,
	version int,
	id, request string,
	params, response interface{},
) error {
	f.AddCall("APICall", objType, version, id, request, params, response)
	marshaled, err := json.Marshal(f.ReturnValues[0])
	if err != nil {
		panic(err)
	}
	json.Unmarshal(marshaled, &response)
	f.ReturnValues = f.ReturnValues[1:]
	return f.NextErr()
}

func (f *fakeDeployAPI) Client() *api.Client {
	f.AddCall("Client")
	retVal := f.ReturnValues[0].(*api.Client)
	f.ReturnValues = f.ReturnValues[1:]
	return retVal
}

func (f *fakeDeployAPI) ModelUUID() (string, bool) {
	f.MethodCall(f, "ModelUUID")
	uuid := f.ReturnValues[0].(string)
	f.ReturnValues = f.ReturnValues[1:]

	ok := f.ReturnValues[0].(bool)
	f.ReturnValues = f.ReturnValues[1:]
	return uuid, ok
}

func (f *fakeDeployAPI) AddLocalCharm(url *charm.URL, ch charm.Charm) (*charm.URL, error) {
	f.MethodCall(f, "AddLocalCharm", url, ch)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	retVal := f.ReturnValues[0].(*charm.URL)
	f.ReturnValues = f.ReturnValues[1:]
	return retVal, nil
}

func (f *fakeDeployAPI) AddCharm(url *charm.URL, channel csclientparams.Channel) error {
	f.MethodCall(f, "AddCharm", url, channel)
	return f.NextErr()
}

func (f *fakeDeployAPI) AddCharmWithAuthorization(
	url *charm.URL,
	channel csclientparams.Channel,
	macaroon *macaroon.Macaroon,
) error {
	f.MethodCall(f, "AddCharmWithAuthorization", url, channel, macaroon)
	return f.NextErr()
}

func (f *fakeDeployAPI) CharmInfo(url string) (*charms.CharmInfo, error) {
	f.MethodCall(f, "CharmInfo", url)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	retVal := f.ReturnValues[0].(*charms.CharmInfo)
	f.ReturnValues = f.ReturnValues[1:]
	return retVal, nil
}

func (f *fakeDeployAPI) Deploy(args application.DeployArgs) error {
	f.MethodCall(f, "Deploy", args)
	return f.NextErr()
}

func (f *fakeDeployAPI) IsMetered(charmURL string) (bool, error) {
	f.MethodCall(f, "IsMetered", charmURL)
	retVal := f.ReturnValues[0].(params.IsMeteredResult)
	f.ReturnValues = f.ReturnValues[1:]
	return retVal.Metered, f.NextErr()

}
func (f *fakeDeployAPI) SetMetricCredentials(service string, credentials []byte) error {
	f.MethodCall(f, "SetMetricCredentials", service, credentials)
	return f.NextErr()
}
