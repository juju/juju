// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"
	csclientparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/charmstore.v5"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2-unstable/bakerytest"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon.v2-unstable"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	jjcharmstore "github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/juju/version"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type DeploySuiteBase struct {
	testing.RepoSuite
	coretesting.CmdBlockHelper
}

func (s *DeploySuiteBase) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

type DeploySuite struct {
	DeploySuiteBase
}

var _ = gc.Suite(&DeploySuite{})

func (s *DeploySuite) SetUpTest(c *gc.C) {
	s.DeploySuiteBase.SetUpTest(c)

	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.LXDProfile)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

// runDeploy executes the deploy command in order to deploy the given
// charm or bundle. The deployment stderr output and error are returned.
func runDeployWithOutput(c *gc.C, args ...string) (string, string, error) {
	ctx, err := cmdtesting.RunCommand(c, NewDeployCommand(), args...)
	return strings.Trim(cmdtesting.Stdout(ctx), "\n"),
		strings.Trim(cmdtesting.Stderr(ctx), "\n"),
		err
}

// runDeploy executes the deploy command in order to deploy the given
// charm or bundle. The deployment stderr output and error are returned.
func runDeploy(c *gc.C, args ...string) error {
	_, _, err := runDeployWithOutput(c, args...)
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
		args: []string{"charm", "--attach-storage", "foo/0", "-n", "2"},
		err:  `--attach-storage cannot be used with -n`,
	}, {
		args: []string{"bundle", "--map-machines", "foo"},
		err:  `error in --map-machines: expected "existing" or "<bundle-id>=<machine-id>", got "foo"`,
	},
}

func (s *DeploySuite) TestInitErrors(c *gc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := cmdtesting.InitCommand(NewDeployCommand(), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "some-application-name", "--series", "precise")

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

func (s *DeploySuite) TestCharmDir(c *gc.C) {
	ch := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "multi-series", curl, 1, 0)
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
	err := runDeploy(c, path, "--series", "precise", "--force")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertApplication(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeries(c *gc.C) {
	// Update the model default series to be unset.
	updateAttrs := map[string]interface{}{"default-series": ""}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	err = runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, "series not specified and charm does not define any")
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeriesUseDefaultSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	err := runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL(fmt.Sprintf("local:%s/dummy-1", version.SupportedLTS()))
	s.AssertApplication(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathDefaultSeries(c *gc.C) {
	// multi-series/metadata.yaml provides "precise" as its default series
	// and yet, here, the model defaults to the series "trusty". This test
	// asserts that the model's default takes precedence.
	updateAttrs := map[string]interface{}{"default-series": "trusty"}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err = runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPath(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeries(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, path, "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, `series "quantal" not supported by charm, supported series are: precise,trusty,xenial,yakkety`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesForce(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "multi-series")
	err := runDeploy(c, path, "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:quantal/multi-series-1")
	s.AssertApplication(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedLXDProfileForce(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "lxd-profile-fail")
	err := runDeploy(c, path, "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:quantal/lxd-profile-fail-0")
	s.AssertApplication(c, "lxd-profile-fail", curl, 1, 0)
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
	s.AssertApplication(c, "dummy", curl, 1, 0)
	// Check the charm dir was left untouched.
	ch, err := charm.ReadCharmDir(dirPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
}

func (s *DeploySuite) TestCharmBundle(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "some-application-name", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "some-application-name", curl, 1, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "logging")
	err := runDeploy(c, ch, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/logging-1")
	s.AssertApplication(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) combinedSettings(ch charm.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *DeploySuite) TestSingleConfigFile(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, ch, "dummy-application", "--config", path, "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	appCh, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(appCh, charm.Settings{
		"skill-level": int64(9000),
		"username":    "admin001",
	}))
}

func (s *DeploySuite) TestRelativeConfigPath(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	// Putting a config file in home is okay as $HOME is set to a tempdir
	setupConfigFile(c, utils.Home())
	err := runDeploy(c, ch, "dummy-application", "--config", "~/testconfig.yaml")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigValues(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "dummy-application", "--config", "skill-level=9000", "--config", "outlook=good", "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	appCh, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(appCh, charm.Settings{
		"outlook":     "good",
		"skill-level": int64(9000),
		"username":    "admin001",
	}))
}

func (s *DeploySuite) TestConfigValuesWithFile(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, ch, "dummy-application", "--config", path, "--config", "outlook=good", "--config", "skill-level=8000", "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	appCh, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(appCh, charm.Settings{
		"outlook":     "good",
		"skill-level": int64(8000),
		"username":    "admin001",
	}))
}

func (s *DeploySuite) TestSingleConfigMoreThanOneFile(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "dummy-application", "--config", "one", "--config", "another", "--series", "precise")
	c.Assert(err, gc.ErrorMatches, "only a single config YAML file can be specified, got 2")
}

func (s *DeploySuite) TestConfigError(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, ch, "other-application", "--config", path)
	c.Assert(err, gc.ErrorMatches, `no settings found for "other-application"`)
	_, err = s.State.Application("other-application")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeploySuite) TestConstraints(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "--constraints", "mem=2G cores=2", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	application, _ := s.AssertApplication(c, "multi-series", curl, 1, 0)
	cons, err := application.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cores=2"))
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

	err = cmdtesting.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *DeploySuite) TestLXDProfileLocalCharm(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "lxd-profile")
	err := runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:bionic/lxd-profile-0")
	s.AssertApplication(c, "lxd-profile", curl, 1, 0)
}

func (s *DeploySuite) TestLXDProfileLocalCharmFails(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(s.CharmsPath, "lxd-profile-fail")
	err := runDeploy(c, path)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `invalid lxd-profile.yaml: contains device type "unix-disk"`)
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
	application, _ := s.AssertApplication(c, "storage-block", curl, 1, 0)

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

type CAASDeploySuiteBase struct {
	charmStoreSuite

	series     string
	CharmsPath string
}

func (s *CAASDeploySuiteBase) SetUpTest(c *gc.C) {
	s.series = "kubernetes"
	s.CharmsPath = c.MkDir()

	s.charmStoreSuite.SetUpTest(c)

	// Set up a CAAS model to replace the IAAS one.
	st := s.Factory.MakeCAASModel(c, nil)
	s.CleanupSuite.AddCleanup(func(*gc.C) { st.Close() })
	// Close the state pool before the state object itself.
	s.StatePool.Close()
	s.StatePool = nil
	err := s.State.Close()
	c.Assert(err, jc.ErrorIsNil)
	s.State = st
}

// assertUnitsCreated checks that the given units have been created. The
// expectedUnits argument maps unit names to machine names.
func (s *CAASDeploySuiteBase) assertUnitsCreated(c *gc.C, expectedUnits map[string]string) {
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	created := make(map[string]string)
	for _, application := range applications {
		var units []*state.Unit
		units, err = application.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		for _, unit := range units {
			created[unit.Name()] = "" // caas unit does not have a machineID here currently
		}
	}
	c.Assert(created, jc.DeepEquals, expectedUnits)
}

type CAASDeploySuite struct {
	CAASDeploySuiteBase
}

var _ = gc.Suite(&CAASDeploySuite{})

func (s *CAASDeploySuite) TestInitErrorsCaasModel(c *gc.C) {
	otherModels := map[string]jujuclient.ModelDetails{
		"admin/caas-model": {ModelUUID: "test.caas.model.uuid", ModelType: model.CAAS},
	}
	err := s.ControllerStore.SetModels("kontroll", otherModels)
	c.Assert(err, jc.ErrorIsNil)
	args := []string{"-m", "caas-model", "some-application-name", "--attach-storage", "foo/0"}
	err = cmdtesting.InitCommand(NewDeployCommand(), args)
	c.Assert(err, gc.ErrorMatches, "--attach-storage cannot be used on kubernetes models")

	args = []string{"-m", "caas-model", "some-application-name", "--to", "a=b,c=d"}
	err = cmdtesting.InitCommand(NewDeployCommand(), args)
	c.Assert(err, gc.ErrorMatches, "only 1 placement directive is supported, got 2")

	args = []string{"-m", "caas-model", "some-application-name", "--to", "#:2"}
	err = cmdtesting.InitCommand(NewDeployCommand(), args)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`placement directive "#:2" not supported`))
}

func (s *CAASDeploySuite) TestPlacement(c *gc.C) {
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(s.State)
	c.Assert(err, jc.ErrorIsNil)
	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(broker)
	storagePoolManager := poolmanager.New(state.NewStateSettings(s.State), storageProviderRegistry)
	_, err = storagePoolManager.Create("operator-storage", provider.K8s_ProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	otherModels := map[string]jujuclient.ModelDetails{
		"admin/" + m.Name(): {ModelUUID: m.UUID(), ModelType: model.CAAS},
	}
	err = s.ControllerStore.SetModels("kontroll", otherModels)
	c.Assert(err, jc.ErrorIsNil)

	_, ch := testcharms.UploadCharmWithSeries(c, s.client, "kubernetes/gitlab-1", "gitlab", "kubernetes")
	err = runDeploy(c, "gitlab", "-m", m.Name(), "--to", "a=b")
	c.Assert(err, jc.ErrorIsNil)

	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"gitlab": {
			charm:     "cs:kubernetes/gitlab-1",
			config:    ch.Config().DefaultSettings(),
			placement: "a=b",
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"gitlab/0": "",
	})
}

func (s *CAASDeploySuite) TestDevices(c *gc.C) {
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(s.State)
	c.Assert(err, jc.ErrorIsNil)
	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(broker)
	storagePoolManager := poolmanager.New(state.NewStateSettings(s.State), storageProviderRegistry)
	_, err = storagePoolManager.Create("operator-storage", provider.K8s_ProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	otherModels := map[string]jujuclient.ModelDetails{
		"admin/" + m.Name(): {ModelUUID: m.UUID(), ModelType: model.CAAS},
	}
	err = s.ControllerStore.SetModels("kontroll", otherModels)
	c.Assert(err, jc.ErrorIsNil)

	_, ch := testcharms.UploadCharmWithSeries(c, s.client, "kubernetes/bitcoin-miner-1", "bitcoin-miner", "kubernetes")
	err = runDeploy(c, "bitcoin-miner", "-m", m.Name(), "--device", "bitcoinminer=10,nvidia.com/gpu", "--series", "kubernetes")
	c.Assert(err, jc.ErrorIsNil)

	s.assertCharmsUploaded(c, "cs:kubernetes/bitcoin-miner-1")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"bitcoin-miner": {
			charm:  "cs:kubernetes/bitcoin-miner-1",
			config: ch.Config().DefaultSettings(),
			devices: map[string]state.DeviceConstraints{
				"bitcoinminer": {Type: "nvidia.com/gpu", Count: 10, Attributes: map[string]string{}},
			},
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"bitcoin-miner/0": "",
	})
}

func (s *DeploySuite) TestDeployStorageFailContainer(c *gc.C) {
	ch := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	machine, err := s.State.AddMachine(version.SupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container := "lxd:" + machine.Id()
	err = runDeploy(c, ch, "--to", container, "--storage", "data=machinescoped,1G")
	c.Assert(err, gc.ErrorMatches, "adding storage to lxd container not supported")
}

func (s *DeploySuite) TestPlacement(c *gc.C) {
	ch := testcharms.Repo.ClonedDirPath(s.CharmsPath, "dummy")
	// Add a machine that will be ignored due to placement directive.
	machine, err := s.State.AddMachine(version.SupportedLTS(), state.JobHostUnits)
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "-n", "13", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "multi-series", curl, 13, 0)
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
	machine, err := s.State.AddMachine(version.SupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", machine.Id(), ch, "portlandia", "--series", version.SupportedLTS())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestInvalidSeriesForModel(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, ch, "portlandia", "--series", "kubernetes")
	c.Assert(err, gc.ErrorMatches, `series "kubernetes" in a non container model not valid`)
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	template := state.MachineTemplate{
		Series: version.SupportedLTS(),
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", container.Id(), ch, "portlandia", "--series", version.SupportedLTS())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, container.Id())
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	machine, err := s.State.AddMachine(version.SupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", "lxd:"+machine.Id(), ch, "portlandia", "--series", version.SupportedLTS())
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, "--to", "42", ch, "portlandia", "--series", "precise")
	c.Assert(err, gc.ErrorMatches, `cannot deploy "portlandia" to machine 42: machine 42 not found`)
	_, err = s.State.Application("portlandia")
	c.Assert(err, gc.ErrorMatches, `application "portlandia" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *gc.C) {
	machine, err := s.State.AddMachine(version.SupportedLTS(), state.JobHostUnits)
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
	_, stdErr, err := runDeployWithOutput(c, ch, "--series", "trusty")

	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdErr, gc.Equals, `Deploying charm "local:trusty/terms1-1".`)

	curl := charm.MustParseURL("local:trusty/terms1-1")
	s.AssertApplication(c, "terms1", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFlags(c *gc.C) {
	command := DeployCommand{}
	flagSet := gnuflag.NewFlagSet(command.Info().Name, gnuflag.ContinueOnError)
	command.SetFlags(flagSet)
	c.Assert(command.flagSet, jc.DeepEquals, flagSet)
	// Add to the slice below if a new flag is introduced which is valid for
	// both charms and bundles.
	charmAndBundleFlags := []string{"channel", "storage", "device"}
	var allFlags []string
	flagSet.VisitAll(func(flag *gnuflag.Flag) {
		allFlags = append(allFlags, flag.Name)
	})
	declaredFlags := append(charmAndBundleFlags, charmOnlyFlags()...)
	declaredFlags = append(declaredFlags, bundleOnlyFlags...)
	declaredFlags = append(declaredFlags, "B", "no-browser-login")
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
	ctx := cmdtesting.ContextForDir(c, dir)
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
}, {
	about:     "public charm, fully resolved, success",
	uploadURL: "cs:~bob/trusty/wordpress2-10",
	deployURL: "cs:~bob/trusty/wordpress2-10",
}, {
	about:        "non-public charm, success",
	uploadURL:    "cs:~bob/trusty/wordpress3-10",
	deployURL:    "cs:~bob/trusty/wordpress3",
	readPermUser: clientUserName,
}, {
	about:        "non-public charm, fully resolved, success",
	uploadURL:    "cs:~bob/trusty/wordpress4-10",
	deployURL:    "cs:~bob/trusty/wordpress4-10",
	readPermUser: clientUserName,
}, {
	about:        "non-public charm, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress5-10",
	deployURL:    "cs:~bob/trusty/wordpress5",
	readPermUser: "bob",
	expectError:  `cannot resolve (charm )?URL "cs:~bob/trusty/wordpress5": cannot get "/~bob/trusty/wordpress5/meta/any\?include=id&include=supported-series&include=published": access denied for user "client-username"`,
}, {
	about:        "non-public charm, fully resolved, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress6-47",
	deployURL:    "cs:~bob/trusty/wordpress6-47",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress6-47": cannot get "/~bob/trusty/wordpress6-47/meta/any\?include=id&include=supported-series&include=published": access denied for user "client-username"`,
}, {
	about:     "public bundle, success",
	uploadURL: "cs:~bob/bundle/wordpress-simple1-42",
	deployURL: "cs:~bob/bundle/wordpress-simple1",
}, {
	about:        "non-public bundle, success",
	uploadURL:    "cs:~bob/bundle/wordpress-simple2-0",
	deployURL:    "cs:~bob/bundle/wordpress-simple2-0",
	readPermUser: clientUserName,
}, {
	about:        "non-public bundle, access denied",
	uploadURL:    "cs:~bob/bundle/wordpress-simple3-47",
	deployURL:    "cs:~bob/bundle/wordpress-simple3",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/bundle/wordpress-simple3": cannot get "/~bob/bundle/wordpress-simple3/meta/any\?include=id&include=supported-series&include=published": access denied for user "client-username"`,
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
		_, err := cmdtesting.RunCommand(c, NewDeployCommand(), test.deployURL, fmt.Sprintf("wordpress%d", i))
		if test.expectError != "" {
			c.Check(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *DeployCharmStoreSuite) TestDeployWithTermsSuccess(c *gc.C) {
	_, ch := testcharms.UploadCharm(c, s.client, "trusty/terms1-1", "terms1")
	_, stdErr, err := runDeployWithOutput(c, "trusty/terms1")
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
Located charm "cs:trusty/terms1-1".
Deploying charm "cs:trusty/terms1-1".
Deployment under prior agreement to terms: term1/1 term3/1
`
	c.Assert(stdErr, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUploaded(c, "cs:trusty/terms1-1")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"terms1": {charm: "cs:trusty/terms1-1", config: ch.Config().DefaultSettings()},
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
	err := runDeploy(c, "quantal/terms1")
	expectedError := `Declined: some terms require agreement. Try: "juju agree term/1 term/2"`
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *DeployCharmStoreSuite) TestDeployWithChannel(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "wordpress")
	id := charm.MustParseURL("cs:~client-username/precise/wordpress-0")
	err := s.client.UploadCharmWithRevision(id, ch, -1)
	c.Assert(err, gc.IsNil)

	err = s.client.Publish(id, []csclientparams.Channel{csclientparams.EdgeChannel}, nil)
	c.Assert(err, gc.IsNil)

	err = runDeploy(c, "--channel", "edge", "~client-username/wordpress")
	c.Assert(err, gc.IsNil)
	s.assertCharmsUploaded(c, "cs:~client-username/precise/wordpress-0")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"wordpress": {charm: "cs:~client-username/precise/wordpress-0", config: ch.Config().DefaultSettings()},
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
	srvSession           *mgo.Session
	client               *csclient.Client
	discharger           *bakerytest.Discharger
	termsDischarger      *bakerytest.Discharger
	termsDischargerError error
	termsString          string
}

func (s *charmStoreSuite) SetUpTest(c *gc.C) {
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

	// Grab a db session to setup the charmstore with (so we can grab the
	// URL to use for the controller config.)
	srvSession, err := jujutesting.MgoServer.Dial()
	c.Assert(err, gc.IsNil)
	s.srvSession = srvSession

	// Set up the testing charm store server.
	db := s.srvSession.DB("juju-testing")
	keyring := bakery.NewPublicKeyRing()
	pk, err := httpbakery.PublicKeyForLocation(http.DefaultClient, s.discharger.Location())
	c.Assert(err, gc.IsNil)
	err = keyring.AddPublicKeyForLocation(s.discharger.Location(), true, pk)
	c.Assert(err, gc.IsNil)

	pk, err = httpbakery.PublicKeyForLocation(http.DefaultClient, s.termsDischarger.Location())
	c.Assert(err, gc.IsNil)
	err = keyring.AddPublicKeyForLocation(s.termsDischarger.Location(), true, pk)
	c.Assert(err, gc.IsNil)

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

	// Set charmstore URL config so the config is set during bootstrap
	if s.ControllerConfigAttrs == nil {
		s.ControllerConfigAttrs = make(map[string]interface{})
	}
	s.JujuConnSuite.ControllerConfigAttrs[controller.CharmStoreURL] = s.srv.URL
	s.JujuConnSuite.SetUpTest(c)

	// Initialize the charm cache dir.
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())

	// Point the CLI to the charm store testing server, injecting a cookie of our choosing.
	actualNewCharmStoreClient := newCharmStoreClient
	s.PatchValue(&newCharmStoreClient, func(client *httpbakery.Client, _ string) *csclient.Client {
		// Add a cookie so that the discharger can detect whether the
		// HTTP client is the juju environment or the juju client.
		lurl, err := url.Parse(s.discharger.Location())
		c.Assert(err, jc.ErrorIsNil)
		client.Jar.SetCookies(lurl, []*http.Cookie{{
			Name:  clientUserCookie,
			Value: clientUserName,
		}})
		return actualNewCharmStoreClient(client, s.srv.URL)
	})

	// Point the Juju API server to the charm store testing server.
	s.PatchValue(&csclient.ServerURL, s.srv.URL)
}

func (s *charmStoreSuite) TearDownTest(c *gc.C) {
	// We have to close all of these things before the connsuite tear down due to the
	// dirty socket detection in the base mgo suite.
	s.srv.Close()
	s.handler.Close()
	s.srvSession.Close()
	s.discharger.Close()
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

// applicationInfo holds information about a deployed application.
type applicationInfo struct {
	charm            string
	config           charm.Settings
	constraints      constraints.Value
	placement        string
	exposed          bool
	storage          map[string]state.StorageConstraints
	devices          map[string]state.DeviceConstraints
	endpointBindings map[string]string
}

// assertDeployedApplicationBindings checks that applications were deployed into the
// expected spaces. It is separate to assertApplicationsDeployed because it is only
// relevant to a couple of tests.
func (s *charmStoreSuite) assertDeployedApplicationBindings(c *gc.C, info map[string]applicationInfo) {
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)

	for _, application := range applications {
		endpointBindings, err := application.EndpointBindings()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(endpointBindings, jc.DeepEquals, info[application.Name()].endpointBindings)
	}
}

func (s *charmStoreSuite) combinedSettings(ch charm.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

// assertApplicationsDeployed checks that the given applications have been deployed.
func (s *charmStoreSuite) assertApplicationsDeployed(c *gc.C, info map[string]applicationInfo) {
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	deployed := make(map[string]applicationInfo, len(applications))
	for _, application := range applications {
		curl, _ := application.CharmURL()
		c.Assert(err, jc.ErrorIsNil)
		config, err := application.CharmConfig()
		c.Assert(err, jc.ErrorIsNil)
		constraints, err := application.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		storage, err := application.StorageConstraints()
		c.Assert(err, jc.ErrorIsNil)
		if len(storage) == 0 {
			storage = nil
		}
		devices, err := application.DeviceConstraints()
		c.Assert(err, jc.ErrorIsNil)
		if len(devices) == 0 {
			devices = nil
		}
		deployed[application.Name()] = applicationInfo{
			charm:       curl.String(),
			config:      config,
			constraints: constraints,
			exposed:     application.IsExposed(),
			storage:     storage,
			devices:     devices,
			placement:   application.GetPlacement(),
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

func (t *testMetricCredentialsSetter) SetMetricCredentials(applicationName string, data []byte) error {
	t.assert(applicationName, data)
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

	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	}
	meteredURL := charm.MustParseURL("cs:quantal/metered-1")
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)
	fakeAPI.planURL = server.URL
	withCharmDeployable(fakeAPI, meteredURL, "quantal", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	cfg, err := config.New(config.NoDefaults, cfgAttrs)
	c.Assert(err, jc.ErrorIsNil)
	withCharmRepoResolvable(fakeAPI, meteredURL, cfg)

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	setMetricCredentialsCall := fakeAPI.Call("SetMetricCredentials", meteredURL.Name, creds).Returns(error(nil))

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{PlanURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1", "--plan", "someplan")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(setMetricCredentialsCall(), gc.Equals, 1)

	stub.CheckCalls(c, []jujutesting.StubCall{{
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "metered",
			PlanURL:         "someplan",
			IncreaseBudget:  0,
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

	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	}
	meteredURL := charm.MustParseURL("cs:quantal/metered-1")
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)
	fakeAPI.planURL = server.URL
	withCharmDeployable(fakeAPI, meteredURL, "quantal", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	cfg, err := config.New(config.NoDefaults, cfgAttrs)
	c.Assert(err, jc.ErrorIsNil)
	withCharmRepoResolvable(fakeAPI, meteredURL, cfg)

	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	setMetricCredentialsCall := fakeAPI.Call("SetMetricCredentials", meteredURL.Name, creds).Returns(error(nil))

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{PlanURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(setMetricCredentialsCall(), gc.Equals, 1)
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"DefaultPlan", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "metered",
			PlanURL:         "thisplan",
			IncreaseBudget:  0,
		}},
	}})
}

func (s *DeployCharmStoreSuite) TestSetMetricCredentialsNotCalledForUnmeteredCharm(c *gc.C) {
	charmDir := testcharms.Repo.CharmDir("dummy")
	testcharms.UploadCharm(c, s.client, "cs:quantal/dummy-1", "dummy")

	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	}
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)

	charmURL := charm.MustParseURL("cs:quantal/dummy-1")
	cfg, err := config.New(config.NoDefaults, cfgAttrs)
	c.Assert(err, jc.ErrorIsNil)
	withCharmRepoResolvable(fakeAPI, charmURL, cfg)
	withCharmDeployable(fakeAPI, charmURL, "quantal", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}

	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/dummy-1")
	c.Assert(err, jc.ErrorIsNil)

	for _, call := range fakeAPI.Calls() {
		if call.FuncName == "SetMetricCredentials" {
			c.Fatal("call to SetMetricCredentials was not supposed to happen")
		}
	}
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

	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	}
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)

	cfg, err := config.New(config.NoDefaults, cfgAttrs)
	c.Assert(err, jc.ErrorIsNil)
	withCharmRepoResolvable(fakeAPI, url, cfg)
	withCharmDeployable(fakeAPI, url, "quantal", ch.Meta(), ch.Metrics(), true, false, 1, nil, nil)

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()
	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{PlanURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}

	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), url.String())
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckNoCalls(c)
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

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	}
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)
	fakeAPI.planURL = server.URL

	cfg, err := config.New(config.NoDefaults, cfgAttrs)
	c.Assert(err, jc.ErrorIsNil)
	withCharmRepoResolvable(fakeAPI, url, cfg)
	withCharmDeployable(fakeAPI, url, "quantal", ch.Meta(), ch.Metrics(), true, false, 1, nil, nil)

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{PlanURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}

	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), url.String(), "--plan", "someplan")
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:~user/quantal/metered-0",
			ApplicationName: "metered",
			PlanURL:         "someplan",
			IncreaseBudget:  0,
		}},
	}})
}

func (s *DeployCharmStoreSuite) TestDeployCharmWithSomeEndpointBindingsSpecifiedSuccess(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	_, ch := testcharms.UploadCharm(c, s.client, "cs:quantal/wordpress-extra-bindings-1", "wordpress-extra-bindings")
	err = runDeploy(c, "cs:quantal/wordpress-extra-bindings-1", "--bind", "db=db db-client=db public admin-api=public")
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"wordpress-extra-bindings": {charm: "cs:quantal/wordpress-extra-bindings-1", config: ch.Config().DefaultSettings()},
	})
	s.assertDeployedApplicationBindings(c, map[string]applicationInfo{
		"wordpress-extra-bindings": {
			endpointBindings: map[string]string{
				"":                "public",
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

	meteredCharmURL := charm.MustParseURL("cs:quantal/metered-1")
	testcharms.UploadCharm(c, s.client, meteredCharmURL.String(), "metered")
	charmDir := testcharms.Repo.CharmDir("metered")

	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	}
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)
	fakeAPI.planURL = server.URL

	cfg, err := config.New(config.NoDefaults, cfgAttrs)
	c.Assert(err, jc.ErrorIsNil)
	withCharmRepoResolvable(fakeAPI, meteredCharmURL, cfg)
	withCharmDeployable(fakeAPI, meteredCharmURL, "quantal", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	fakeAPI.Call("SetMetricCredentials", meteredCharmURL.Name, creds).Returns(errors.New("IsMetered"))

	deploy := &DeployCommand{
		Steps: []DeployStep{&RegisterMeteredCharm{PlanURL: server.URL}},
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeAPI, nil
		},
	}
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:quantal/metered-1", "--plan", "someplan")

	c.Check(err, gc.ErrorMatches, "IsMetered")
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

func (s *ParseBindSuite) TestParseSuccessWithApplicationDefaultSpaceOnly(c *gc.C) {
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

type ParseMachineMapSuite struct{}

var _ = gc.Suite(&ParseMachineMapSuite{})

func (s *ParseMachineMapSuite) TestEmptyString(c *gc.C) {
	existing, mapping, err := parseMachineMap("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(existing, jc.IsFalse)
	c.Check(mapping, gc.HasLen, 0)
}

func (s *ParseMachineMapSuite) TestExisting(c *gc.C) {
	existing, mapping, err := parseMachineMap("existing")
	c.Check(err, jc.ErrorIsNil)
	c.Check(existing, jc.IsTrue)
	c.Check(mapping, gc.HasLen, 0)
}

func (s *ParseMachineMapSuite) TestMapping(c *gc.C) {
	existing, mapping, err := parseMachineMap("1=2,3=4")
	c.Check(err, jc.ErrorIsNil)
	c.Check(existing, jc.IsFalse)
	c.Check(mapping, jc.DeepEquals, map[string]string{
		"1": "2", "3": "4",
	})
}

func (s *ParseMachineMapSuite) TestMappingWithExisting(c *gc.C) {
	existing, mapping, err := parseMachineMap("1=2,3=4,existing")
	c.Check(err, jc.ErrorIsNil)
	c.Check(existing, jc.IsTrue)
	c.Check(mapping, jc.DeepEquals, map[string]string{
		"1": "2", "3": "4",
	})
}

func (s *ParseMachineMapSuite) TestSpaces(c *gc.C) {
	existing, mapping, err := parseMachineMap("1=2, 3=4, existing")
	c.Check(err, jc.ErrorIsNil)
	c.Check(existing, jc.IsTrue)
	c.Check(mapping, jc.DeepEquals, map[string]string{
		"1": "2", "3": "4",
	})
}

func (s *ParseMachineMapSuite) TestErrors(c *gc.C) {
	checkErr := func(value, expect string) {
		_, _, err := parseMachineMap(value)
		c.Check(err, gc.ErrorMatches, expect)
	}

	checkErr("blah", `expected "existing" or "<bundle-id>=<machine-id>", got "blah"`)
	checkErr("1=2=3", `expected "existing" or "<bundle-id>=<machine-id>", got "1=2=3"`)
	checkErr("1=-1", `machine-id "-1" is not a top level machine id`)
	checkErr("-1=1", `bundle-id "-1" is not a top level machine id`)
}

type DeployUnitTestSuite struct {
	jujutesting.IsolationSuite
	DeployAPI
}

var _ = gc.Suite(&DeployUnitTestSuite{})

func (s *DeployUnitTestSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	cookiesFile := filepath.Join(c.MkDir(), ".go-cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookiesFile)
}

func (s *DeployUnitTestSuite) cfgAttrs() map[string]interface{} {
	return map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	}
}

func (s *DeployUnitTestSuite) fakeAPI() *fakeDeployAPI {
	return vanillaFakeModelAPI(s.cfgAttrs())
}

func (s *DeployUnitTestSuite) makeCharmDir(c *gc.C, cloneCharm string) *charm.CharmDir {
	charmsPath := c.MkDir()
	return testcharms.Repo.ClonedDir(charmsPath, cloneCharm)
}

func (s *DeployUnitTestSuite) runDeploy(c *gc.C, fakeAPI *fakeDeployAPI, args ...string) (*cmd.Context, error) {
	cmd := NewDeployCommandForTest(func() (DeployAPI, error) {
		return fakeAPI, nil
	}, nil)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *DeployUnitTestSuite) TestDeployApplicationConfig(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.Repo.ClonedDir(charmsPath, "dummy")

	fakeAPI := vanillaFakeModelAPI(map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	})

	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI,
		dummyURL,
		"trusty",
		charmDir.Meta(),
		charmDir.Metrics(),
		false,
		false,
		1,
		nil,
		map[string]string{"foo": "bar"},
	)

	cmd := NewDeployCommandForTest(func() (DeployAPI, error) { return fakeAPI, nil }, nil)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, dummyURL.String(),
		"--config", "foo=bar",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployLocalWithBundleOverlay(c *gc.C) {
	charmDir := s.makeCharmDir(c, "multi-series")
	fakeAPI := s.fakeAPI()

	multiSeriesURL := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(fakeAPI, multiSeriesURL, charmDir, false)
	withCharmDeployable(fakeAPI, multiSeriesURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "--overlay", "somefile")
	c.Check(err, gc.ErrorMatches, "flags provided but not supported when deploying a charm: --overlay")
}

func (s *DeployUnitTestSuite) TestDeployLocalCharmGivesCorrectUserMessage(c *gc.C) {
	// Copy multi-series charm to path where we can deploy it from
	charmDir := s.makeCharmDir(c, "multi-series")
	fakeAPI := s.fakeAPI()

	multiSeriesURL := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(fakeAPI, multiSeriesURL, charmDir, false)
	withCharmDeployable(fakeAPI, multiSeriesURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	context, err := s.runDeploy(c, fakeAPI, charmDir.Path, "--series", "trusty")
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(context), gc.Equals, `Deploying charm "local:trusty/multi-series-1".`+"\n")
}

func (s *DeployUnitTestSuite) TestAddMetricCredentialsDefaultForUnmeteredCharm(c *gc.C) {
	charmDir := s.makeCharmDir(c, "multi-series")
	multiSeriesURL := charm.MustParseURL("local:trusty/multi-series-1")
	fakeAPI := s.fakeAPI()
	withLocalCharmDeployable(fakeAPI, multiSeriesURL, charmDir, false)
	withCharmDeployable(fakeAPI, multiSeriesURL, "trusty", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)

	// We never attempt to set metric credentials
	for _, call := range fakeAPI.Calls() {
		if call.FuncName == "FacadeCall" {
			c.Assert(call.Args[0], gc.Not(gc.Matches), "SetMetricCredentials")
		}
	}
}

func (s *DeployUnitTestSuite) TestRedeployLocalCharmSucceedsWhenDeployed(c *gc.C) {
	charmDir := s.makeCharmDir(c, "dummy")
	fakeAPI := s.fakeAPI()
	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(fakeAPI, dummyURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	context, err := s.runDeploy(c, fakeAPI, dummyURL.String())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(context), gc.Equals, ""+
		`Located charm "local:trusty/dummy-0".`+"\n"+
		`Deploying charm "local:trusty/dummy-0".`+"\n",
	)
}

func (s *DeployUnitTestSuite) TestDeployBundle_OutputsCorrectMessage(c *gc.C) {
	bundleDir := testcharms.Repo.BundleArchive(c.MkDir(), "wordpress-simple")

	fakeAPI := s.fakeAPI()
	withAllWatcher(fakeAPI)

	fakeBundleURL := charm.MustParseURL("cs:bundle/wordpress-simple")
	cfg, err := config.New(config.NoDefaults, s.cfgAttrs())
	c.Assert(err, jc.ErrorIsNil)
	withCharmRepoResolvable(fakeAPI, fakeBundleURL, cfg)
	fakeAPI.Call("GetBundle", fakeBundleURL).Returns(bundleDir, error(nil))

	mysqlURL := charm.MustParseURL("cs:mysql")
	withCharmRepoResolvable(fakeAPI, mysqlURL, cfg)
	withCharmDeployable(
		fakeAPI,
		mysqlURL,
		"quantal",
		&charm.Meta{Series: []string{"quantal"}},
		&charm.Metrics{},
		false,
		false,
		0,
		nil,
		nil,
	)
	fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "mysql",
		NumUnits:        1,
	}).Returns([]string{"mysql/0"}, error(nil))

	wordpressURL := charm.MustParseURL("cs:wordpress")
	withCharmRepoResolvable(fakeAPI, wordpressURL, cfg)
	withCharmDeployable(
		fakeAPI,
		wordpressURL,
		"quantal",
		&charm.Meta{Series: []string{"quantal"}},
		&charm.Metrics{},
		false,
		false,
		0,
		nil,
		nil,
	)
	fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "wordpress",
		NumUnits:        1,
	}).Returns([]string{"wordpress/0"}, error(nil))

	fakeAPI.Call("AddRelation", []interface{}{"wordpress:db", "mysql:server"}, []interface{}{}).Returns(
		&params.AddRelationResults{},
		error(nil),
	)

	fakeAPI.Call("SetAnnotation", map[string]map[string]string{"application-wordpress": {"bundleURL": "cs:bundle/wordpress-simple"}}).Returns(
		[]params.ErrorResult{},
		error(nil),
	)

	fakeAPI.Call("SetAnnotation", map[string]map[string]string{"application-mysql": {"bundleURL": "cs:bundle/wordpress-simple"}}).Returns(
		[]params.ErrorResult{},
		error(nil),
	)

	deployCmd := NewDeployCommandForTest(func() (DeployAPI, error) {
		return fakeAPI, nil
	}, nil)
	deployCmd.SetClientStore(jujuclienttesting.MinimalStore())
	context, err := cmdtesting.RunCommand(c, deployCmd, "cs:bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(context), gc.Equals, ""+
		`Located bundle "cs:bundle/wordpress-simple"`+"\n"+
		"Resolving charm: mysql\n"+
		"Resolving charm: wordpress\n"+
		`Deploy of bundle completed.`+
		"\n",
	)
	c.Check(cmdtesting.Stdout(context), gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:mysql\n"+
		"- deploy application mysql using cs:mysql\n"+
		"- set annotations for mysql\n"+
		"- upload charm cs:wordpress\n"+
		"- deploy application wordpress using cs:wordpress\n"+
		"- set annotations for wordpress\n"+
		"- add relation wordpress:db - mysql:server\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1\n",
	)
}

func (s *DeployUnitTestSuite) TestDeployAttachStorage(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.Repo.ClonedDir(charmsPath, "dummy")

	fakeAPI := vanillaFakeModelAPI(map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	})

	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
	)

	cmd := NewDeployCommandForTest(func() (DeployAPI, error) { return fakeAPI, nil }, nil)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, dummyURL.String(),
		"--attach-storage", "foo/0",
		"--attach-storage", "bar/1,baz/2",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployAttachStorageFailContainer(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.Repo.ClonedDir(charmsPath, "dummy")

	fakeAPI := vanillaFakeModelAPI(map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	})

	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
	)

	cmd := NewDeployCommandForTest(func() (DeployAPI, error) { return fakeAPI, nil }, nil)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, dummyURL.String(),
		"--attach-storage", "foo/0", "--to", "lxd",
	)
	c.Assert(err, gc.ErrorMatches, "adding storage to lxd container not supported")
}

func (s *DeployUnitTestSuite) TestDeployAttachStorageNotSupported(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.Repo.ClonedDir(charmsPath, "dummy")

	fakeAPI := vanillaFakeModelAPI(map[string]interface{}{
		"name": "name",
		"uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "foo",
	})
	fakeAPI.Call("BestFacadeVersion", "Application").Returns(4) // v4 doesn't support attach-storage
	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
	)

	cmd := NewDeployCommandForTest(func() (DeployAPI, error) { return fakeAPI, nil }, nil)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, dummyURL.String(), "--attach-storage", "foo/0")
	c.Assert(err, gc.ErrorMatches, "this juju controller does not support --attach-storage")
}

// fakeDeployAPI is a mock of the API used by the deploy command. It's
// a little muddled at the moment, but as the DeployAPI interface is
// sharpened, this will become so as well.
type fakeDeployAPI struct {
	DeployAPI
	*jujutesting.CallMocker
	planURL string
}

func (f *fakeDeployAPI) IsMetered(charmURL string) (bool, error) {
	results := f.MethodCall(f, "IsMetered", charmURL)
	return results[0].(bool), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) SetMetricCredentials(application string, credentials []byte) error {
	results := f.MethodCall(f, "SetMetricCredentials", application, credentials)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) Close() error {
	results := f.MethodCall(f, "Close")
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) Sequences() (map[string]int, error) {
	return nil, nil
}

func (f *fakeDeployAPI) ModelGet() (map[string]interface{}, error) {
	results := f.MethodCall(f, "ModelGet")
	return results[0].(map[string]interface{}), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Resolve(cfg *config.Config, url *charm.URL) (
	*charm.URL,
	csclientparams.Channel,
	[]string,
	error,
) {
	results := f.MethodCall(f, "Resolve", cfg, url)

	return results[0].(*charm.URL),
		results[1].(csclientparams.Channel),
		results[2].([]string),
		jujutesting.TypeAssertError(results[3])
}

func (f *fakeDeployAPI) BestFacadeVersion(facade string) int {
	results := f.MethodCall(f, "BestFacadeVersion", facade)
	return results[0].(int)
}

func (f *fakeDeployAPI) APICall(objType string, version int, id, request string, params, response interface{}) error {
	results := f.MethodCall(f, "APICall", objType, version, id, request, params, response)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) Client() *api.Client {
	results := f.MethodCall(f, "Client")
	return results[0].(*api.Client)
}

func (f *fakeDeployAPI) ModelUUID() (string, bool) {
	results := f.MethodCall(f, "ModelUUID")
	return results[0].(string), results[1].(bool)
}

func (f *fakeDeployAPI) AddLocalCharm(url *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	results := f.MethodCall(f, "AddLocalCharm", url, ch, force)
	return results[0].(*charm.URL), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddCharm(url *charm.URL, channel csclientparams.Channel, force bool) error {
	results := f.MethodCall(f, "AddCharm", url, channel, force)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) AddCharmWithAuthorization(
	url *charm.URL,
	channel csclientparams.Channel,
	macaroon *macaroon.Macaroon,
	force bool,
) error {
	results := f.MethodCall(f, "AddCharmWithAuthorization", url, channel, macaroon, force)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) CharmInfo(url string) (*charms.CharmInfo, error) {
	results := f.MethodCall(f, "CharmInfo", url)
	return results[0].(*charms.CharmInfo), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Deploy(args application.DeployArgs) error {
	results := f.MethodCall(f, "Deploy", args)
	if len(results) != 1 {
		return errors.Errorf("expected 1 result, got %d: %v", len(results), results)
	}
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) GetAnnotations(tags []string) ([]params.AnnotationsGetResult, error) {
	return nil, nil
}
func (f *fakeDeployAPI) GetConfig(appNames ...string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeDeployAPI) GetConstraints(appNames ...string) ([]constraints.Value, error) {
	return nil, nil
}

func (f *fakeDeployAPI) GetBundle(url *charm.URL) (charm.Bundle, error) {
	results := f.MethodCall(f, "GetBundle", url)
	return results[0].(charm.Bundle), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Status(patterns []string) (*params.FullStatus, error) {
	results := f.MethodCall(f, "Status", patterns)
	return results[0].(*params.FullStatus), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) WatchAll() (*api.AllWatcher, error) {
	results := f.MethodCall(f, "WatchAll")
	return results[0].(*api.AllWatcher), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddRelation(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	results := f.MethodCall(f, "AddRelation", stringToInterface(endpoints), stringToInterface(viaCIDRs))
	return results[0].(*params.AddRelationResults), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddUnits(args application.AddUnitsParams) ([]string, error) {
	results := f.MethodCall(f, "AddUnits", args)
	return results[0].([]string), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Expose(application string) error {
	results := f.MethodCall(f, "Expose", application)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) SetAnnotation(annotations map[string]map[string]string) ([]params.ErrorResult, error) {
	results := f.MethodCall(f, "SetAnnotation", annotations)
	return results[0].([]params.ErrorResult), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) GetCharmURL(applicationName string) (*charm.URL, error) {
	results := f.MethodCall(f, "GetCharmURL", applicationName)
	return results[0].(*charm.URL), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) SetCharm(cfg application.SetCharmConfig) error {
	results := f.MethodCall(f, "SetCharm", cfg)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) Update(args params.ApplicationUpdate) error {
	results := f.MethodCall(f, "Update", args)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) SetConstraints(application string, constraints constraints.Value) error {
	results := f.MethodCall(f, "SetConstraints", application, constraints)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) AddMachines(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	results := f.MethodCall(f, "AddMachines", machineParams)
	return results[0].([]params.AddMachinesResult), jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) PlanURL() string {
	return f.planURL
}

func stringToInterface(args []string) []interface{} {
	interfaceArgs := make([]interface{}, len(args))
	for i, a := range args {
		interfaceArgs[i] = a
	}
	return interfaceArgs
}

func vanillaFakeModelAPI(cfgAttrs map[string]interface{}) *fakeDeployAPI {
	var logger loggo.Logger
	fakeAPI := &fakeDeployAPI{CallMocker: jujutesting.NewCallMocker(logger)}

	fakeAPI.Call("Close").Returns(error(nil))
	fakeAPI.Call("ModelGet").Returns(cfgAttrs, error(nil))
	fakeAPI.Call("ModelUUID").Returns("deadbeef-0bad-400d-8000-4b1d0d06f00d", true)
	fakeAPI.Call("BestFacadeVersion", "Application").Returns(6)

	return fakeAPI
}

func withLocalCharmDeployable(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	c charm.Charm,
	force bool,
) {
	fakeAPI.Call("AddLocalCharm", url, c, force).Returns(url, error(nil))
}

func withCharmDeployable(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	series string,
	meta *charm.Meta,
	metrics *charm.Metrics,
	metered bool,
	force bool,
	numUnits int,
	attachStorage []string,
	config map[string]string,
) {
	fakeAPI.Call("AddCharm", url, csclientparams.Channel(""), force).Returns(error(nil))
	fakeAPI.Call("CharmInfo", url.String()).Returns(
		&charms.CharmInfo{
			URL:     url.String(),
			Meta:    meta,
			Metrics: metrics,
		},
		error(nil),
	)
	fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID:         jjcharmstore.CharmID{URL: url},
		ApplicationName: url.Name,
		Series:          series,
		NumUnits:        numUnits,
		AttachStorage:   attachStorage,
		Config:          config,
	}).Returns(error(nil))
	fakeAPI.Call("IsMetered", url.String()).Returns(metered, error(nil))

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	fakeAPI.Call("SetMetricCredentials", url.Name, creds).Returns(error(nil))
}

func withCharmRepoResolvable(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	cfg *config.Config,
) {
	fakeAPI.Call("Resolve", cfg, url).Returns(
		url,
		csclientparams.Channel(""),
		[]string{"quantal"}, // Supported series
		error(nil),
	)
}

func withAllWatcher(fakeAPI *fakeDeployAPI) {
	id := "0"
	fakeAPI.Call("WatchAll").Returns(api.NewAllWatcher(fakeAPI, &id), error(nil))

	fakeAPI.Call("BestFacadeVersion", "Application").Returns(0)
	fakeAPI.Call("BestFacadeVersion", "Annotations").Returns(0)
	fakeAPI.Call("BestFacadeVersion", "AllWatcher").Returns(0)
	fakeAPI.Call("BestFacadeVersion", "Charms").Returns(0)
	fakeAPI.Call("APICall", "AllWatcher", 0, "0", "Stop", nil, nil).Returns(error(nil))
	fakeAPI.Call("Status", []string(nil)).Returns(&params.FullStatus{}, error(nil))
}
