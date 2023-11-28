// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/charmrepo/v6"
	csclientparams "github.com/juju/charmrepo/v6/csclient/params"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/fs"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	unitassignerapi "github.com/juju/juju/api/agent/unitassigner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/annotations"
	"github.com/juju/juju/api/client/application"
	apicharms "github.com/juju/juju/api/client/charms"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/cmd/juju/application/store"
	apputils "github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/juju/testing"
	jjtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

func resourceHash(content string) charmresource.Fingerprint {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	if err != nil {
		panic(err)
	}
	return fp
}

// defaultSupportedJujuSeries is used to return canned information about what
// juju supports in terms of the release cycle
// see juju/os and documentation https://www.ubuntu.com/about/release-cycle
var defaultSupportedJujuSeries = set.NewStrings("focal", "bionic", "xenial", "trusty", testing.KubernetesSeriesName)

var defaultLocalOrigin = commoncharm.Origin{
	Source:       commoncharm.OriginLocal,
	Architecture: arch.DefaultArchitecture,
	Series:       "bionic",
}

type DeploySuiteBase struct {
	testing.RepoSuite
	coretesting.CmdBlockHelper
	DeployResources deployer.DeployResourcesFunc

	fakeAPI *fakeDeployAPI
}

// deployCommand returns a deploy command that stubs out the
// charm store and the controller deploy API.
func (s *DeploySuiteBase) deployCommand() *DeployCommand {
	deploy := s.deployCommandForState()
	deploy.NewDeployAPI = func() (deployer.DeployerAPI, error) {
		return s.fakeAPI, nil
	}
	deploy.NewModelConfigAPI = func(api base.APICallCloser) ModelConfigGetter {
		return s.fakeAPI
	}
	deploy.NewCharmsAPI = func(api base.APICallCloser) CharmsAPI {
		return apicharms.NewClient(s.fakeAPI)
	}
	return deploy
}

// deployCommandForState returns a deploy command that stubs out the
// charm store but writes data to the juju database.
func (s *DeploySuiteBase) deployCommandForState() *DeployCommand {
	deploy := newDeployCommand()
	deploy.Steps = nil
	deploy.DeployResources = s.DeployResources
	deploy.NewCharmRepo = func() (*store.CharmStoreAdaptor, error) {
		return s.fakeAPI.CharmStoreAdaptor, nil
	}
	deploy.NewResolver = func(charmsAPI store.CharmsAPI, charmRepoFn store.CharmStoreRepoFunc, downloadFn store.DownloadBundleClientFunc) deployer.Resolver {
		return s.fakeAPI
	}
	deploy.NewConsumeDetailsAPI = func(url *charm.OfferURL) (deployer.ConsumeDetails, error) {
		return s.fakeAPI, nil
	}
	return deploy
}

func (s *DeploySuiteBase) runDeploy(c *gc.C, args ...string) error {
	_, _, err := s.runDeployWithOutput(c, args...)
	return err
}

func (s *DeploySuiteBase) runDeployForState(c *gc.C, args ...string) error {
	deploy := newDeployCommand()
	deploy.Steps = nil
	deploy.DeployResources = s.DeployResources
	deploy.NewCharmRepo = func() (*store.CharmStoreAdaptor, error) {
		return s.fakeAPI.CharmStoreAdaptor, nil
	}
	deploy.NewResolver = func(charmsAPI store.CharmsAPI, charmRepoFn store.CharmStoreRepoFunc, downloadFn store.DownloadBundleClientFunc) deployer.Resolver {
		return s.fakeAPI
	}
	deploy.NewConsumeDetailsAPI = func(url *charm.OfferURL) (deployer.ConsumeDetails, error) {
		return s.fakeAPI, nil
	}
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), args...)
	return err
}

func (s *DeploySuiteBase) runDeployWithOutput(c *gc.C, args ...string) (string, string, error) {
	deployCmd := newWrappedDeployCommandForTest(s.fakeAPI)
	ctx, err := cmdtesting.RunCommand(c, deployCmd, args...)
	return strings.Trim(cmdtesting.Stdout(ctx), "\n"),
		strings.Trim(cmdtesting.Stderr(ctx), "\n"),
		err
}

func (s *DeploySuiteBase) SetUpTest(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.RepoSuite.SetUpTest(c)
	s.PatchValue(&supportedJujuSeries, func(time.Time, string, string) (set.Strings, error) {
		return defaultSupportedJujuSeries, nil
	})
	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
	s.DeployResources = func(applicationID string,
		chID resources.CharmID,
		csMac *macaroon.Macaroon,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		conn base.APICallCloser,
		filesystem modelcmd.Filesystem,
	) (ids map[string]string, err error) {
		return deployResources(s.State, applicationID, resources)
	}
	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": coretesting.ModelTag.Id(),
		"type": "foo",
	}
	s.fakeAPI = vanillaFakeModelAPI(cfgAttrs)
	s.fakeAPI.CharmStoreAdaptor = &store.CharmStoreAdaptor{
		CharmrepoForDeploy: &fakeCharmStoreAPI{
			fakeDeployAPI: s.fakeAPI,
		},
		MacaroonGetter: &noopMacaroonGetter{},
	}
	s.fakeAPI.deployerFactoryFunc = deployer.NewDeployerFactory
}

// deployResources does what would be expected when a charm with
// resources is deployed (ie write the pending and actual resources
// to state), but it does not upload or otherwise use the charmstore
// (fake data from the store is used).
func deployResources(
	st *state.State,
	applicationID string,
	resources map[string]charmresource.Meta,
) (ids map[string]string, err error) {
	if len(resources) == 0 {
		return nil, nil
	}
	stRes := st.Resources()
	ids = make(map[string]string)
	for _, res := range resources {
		content := res.Name + " content"
		origin := charmresource.OriginStore
		user := ""
		if res.Name == "upload-resource" {
			content = "some-data"
			origin = charmresource.OriginUpload
			user = "admin"
		}
		chRes := charmresource.Resource{
			Meta:        res,
			Origin:      origin,
			Fingerprint: resourceHash(content),
			Size:        int64(len(content)),
		}
		pendingID, err := stRes.AddPendingResource(applicationID, user, chRes)
		if err != nil {
			return nil, err
		}
		ids[res.Name] = pendingID
		if origin == charmresource.OriginUpload {
			_, err := stRes.UpdatePendingResource(applicationID, pendingID, user, chRes, strings.NewReader(content))
			if err != nil {
				// We don't bother aggregating errors since a partial
				// completion is disruptive and a retry of this endpoint
				// is not expensive.
				return nil, err
			}
		}
	}
	return ids, nil
}

type DeploySuite struct {
	DeploySuiteBase
}

var _ = gc.Suite(&DeploySuite{})

// runDeploy executes the deploy command in order to deploy the given
// charm or bundle. The deployment stderr output and error are returned.
func runDeployWithOutput(c *gc.C, cmd cmd.Command, args ...string) (string, string, error) {
	ctx, err := cmdtesting.RunCommand(c, cmd, args...)
	return strings.Trim(cmdtesting.Stdout(ctx), "\n"),
		strings.Trim(cmdtesting.Stderr(ctx), "\n"),
		err
}

// runDeploy executes the deploy command in order to deploy the given
// charm or bundle. The deployment stderr output and error are returned.
func runDeploy(c *gc.C, args ...string) error {
	_, _, err := runDeployWithOutput(c, NewDeployCommand(), args...)
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
		err:  `invalid application name "burble-1", unexpected number\(s\) found after last hyphen`,
	}, {
		args: []string{"craziness", "Burble-1"},
		err:  `invalid application name "Burble-1", unexpected uppercase character`,
	}, {
		args: []string{"craziness", "bu£rble"},
		err:  `invalid application name "bu£rble", unexpected character £`,
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
		deployCmd := newWrappedDeployCommandForTest(s.fakeAPI)
		err := cmdtesting.InitCommand(deployCmd, t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestNoCharmOrBundle(c *gc.C) {
	err := s.runDeploy(c, c.MkDir())
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `charm or bundle at .*`)
}

func (s *DeploySuite) TestBlockDeploy(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:bionic/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "some-application-name", "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	// Block operation
	s.BlockAllChanges(c, "TestBlockDeploy")
	err := s.runDeployForState(c, charmDir.Path, "some-application-name", "--series", "bionic")
	s.AssertBlocked(c, err, ".*TestBlockDeploy.*")
}

func (s *DeploySuite) TestInvalidPath(c *gc.C) {
	err := s.runDeploy(c, "/home/nowhere")
	c.Assert(err, gc.ErrorMatches, `cannot resolve charm or bundle "nowhere": charm or bundle not found`)
}

func (s *DeploySuite) TestInvalidFileFormat(c *gc.C) {
	path := filepath.Join(c.MkDir(), "bundle.yaml")
	err := ioutil.WriteFile(path, []byte(":"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot unmarshal bundle contents:.* yaml:.*`)
}

func (s *DeploySuite) TestPathWithNoCharmOrBundle(c *gc.C) {
	err := s.runDeploy(c, c.MkDir())
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `charm or bundle at .*`)
}

func (s *DeploySuite) TestCharmDir(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "some-application-name", "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathRelativeDir(c *gc.C) {
	dir := c.MkDir()
	testcharms.RepoWithSeries("bionic").ClonedDirPath(dir, "multi-series")
	wd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = os.Chdir(wd) }()
	err = os.Chdir(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeploy(c, "multi-series")
	c.Assert(err, gc.ErrorMatches, ""+
		"The charm or bundle \"multi-series\" is ambiguous.\n"+
		"To deploy a local charm or bundle, run `juju deploy ./multi-series`.\n"+
		"To deploy a charm or bundle from CharmHub, run `juju deploy ch:multi-series`.")
}

func (s *DeploySuite) TestDeployFromPathOldCharm(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:precise/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "precise", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeries(c *gc.C) {
	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "dummy")
	err := s.runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, "series not specified and charm does not define any")
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeriesUseDefaultSeries(c *gc.C) {
	updateAttrs := map[string]interface{}{"default-series": version.DefaultSupportedLTS()}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL(fmt.Sprintf("local:%s/dummy-1", version.DefaultSupportedLTS()))
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "focal", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err = s.runDeployForState(c, charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathDefaultSeries(c *gc.C) {
	// multi-series/metadata.yaml provides "precise" as its default series
	// and yet, here, the model defaults to the series "trusty". This test
	// asserts that the model's default takes precedence.
	updateAttrs := map[string]interface{}{"default-series": "trusty"}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err = s.runDeployForState(c, charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPath(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeries(c *gc.C) {
	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "multi-series")
	err := s.runDeploy(c, path, "--series", "quantal")
	c.Assert(err, gc.ErrorMatches, `series "quantal" not supported by charm, supported series are: precise, trusty, xenial, yakkety, bionic`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesForce(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:quantal/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedLXDProfileForce(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("quantal").ClonedDir(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL("local:quantal/lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, true, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "lxd-profile-fail", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeCharmDir(c *gc.C) {
	// Add the charm, so the url will exist and a new revision will be
	// picked in application Deploy.
	dummyCharm := s.AddTestingCharmForSeries(c, "dummy", "bionic")

	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	deployURL := charm.MustParseURL("local:bionic/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, deployURL, charmDir, false)
	withCharmDeployable(s.fakeAPI, deployURL, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	upgradedRev := dummyCharm.Revision() + 1
	curl := dummyCharm.URL().WithRevision(upgradedRev)
	s.AssertApplication(c, "dummy", curl, 1, 0)
	// Check the charm dir was left untouched.
	ch, err := charm.ReadCharmDir(charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
}

func (s *DeploySuite) TestCharmBundle(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, charmURL, "some-application-name", "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "some-application-name", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.AssertApplication(c, "some-application-name", curl, 1, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:trusty/logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
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
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "dummy-application", "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	path := setupConfigFile(c, c.MkDir())
	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", path, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.Application("dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	appCh, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(appCh, charm.Settings{
		"skill-level": int64(9000),
		"username":    "admin001",
	}))
}

func (s *DeploySuite) TestRelativeConfigPath(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	// Putting a config file in home is okay as $HOME is set to a tempdir
	setupConfigFile(c, utils.Home())
	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", "~/testconfig.yaml")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigValues(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:bionic/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "dummy-name", "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	confPath := filepath.Join(c.MkDir(), "include.txt")
	c.Assert(ioutil.WriteFile(confPath, []byte("lorem\nipsum"), os.ModePerm), jc.ErrorIsNil)

	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", "skill-level=9000", "--config", "outlook=good", "--config", "title=@"+confPath, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.Application("dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	appCh, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(appCh, charm.Settings{
		"outlook":     "good",
		"skill-level": int64(9000),
		"username":    "admin001",
		"title":       "lorem\nipsum",
	}))
}

func (s *DeploySuite) TestConfigValuesWithFile(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:bionic/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	path := setupConfigFile(c, c.MkDir())
	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", path, "--config", "outlook=good", "--config", "skill-level=8000", "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.Application("dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	appCh, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(appCh, charm.Settings{
		"outlook":     "good",
		"skill-level": int64(8000),
		"username":    "admin001",
	}))
}

func (s *DeploySuite) TestSingleConfigMoreThanOneFile(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:bionic/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", "one", "--config", "another", "--series", "bionic")
	c.Assert(err, gc.ErrorMatches, "only a single config YAML file can be specified, got 2")
}

func (s *DeploySuite) TestConfigError(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:bionic/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, charmURL, "some-application-name", "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	path := setupConfigFile(c, c.MkDir())
	err := s.runDeployForState(c, charmDir.Path, "other-application", "--config", path)
	c.Assert(err, gc.ErrorMatches, `no settings found for "other-application"`)
	_, err = s.State.Application("other-application")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeploySuite) TestConstraints(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:bionic/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withCharmDeployable(s.fakeAPI, charmURL, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--constraints", "mem=2G cores=2", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	app, _ := s.AssertApplication(c, "multi-series", curl, 1, 0)
	cons, err := app.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cores=2 arch=amd64"))
}

func (s *DeploySuite) TestResources(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:bionic/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	foopath := "/test/path/foo"
	barpath := "/test/path/var"

	res1 := fmt.Sprintf("foo=%s", foopath)
	res2 := fmt.Sprintf("bar=%s", barpath)

	d := DeployCommand{}
	args := []string{charmDir.Path, "--resource", res1, "--resource", res2, "--series", "quantal"}

	err := cmdtesting.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *DeploySuite) TestLXDProfileLocalCharm(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL("local:bionic/lxd-profile-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "lxd-profile", curl, 1, 0)
}

func (s *DeploySuite) TestLXDProfileLocalCharmFails(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL("local:bionic/lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `invalid lxd-profile.yaml: contains device type "unix-disk"`)
}

// TODO(ericsnow) Add tests for charmstore-based resources once the
// endpoints are implemented.

// TODO(wallyworld) - add another test that deploy with storage fails for older environments
// (need deploy client to be refactored to use API stub)
func (s *DeploySuite) TestStorage(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "storage-block")
	curl := charm.MustParseURL("local:trusty/storage-block-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployableWithStorage(
		s.fakeAPI, curl, "storage-block", "trusty",
		charmDir.Meta(),
		charmDir.Metrics(),
		false, false, 1, nil, nil,
		map[string]storage.Constraints{
			"data": {
				Pool:  "machinescoped",
				Size:  1024,
				Count: 1,
			},
		},
	)

	err := s.runDeployForState(c, charmDir.Path, "--storage", "data=machinescoped,1G", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	app, _ := s.AssertApplication(c, "storage-block", curl, 1, 0)

	cons, err := app.StorageConstraints()
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

func (s *DeploySuite) TestErrorDeployingBundlesRequiringTrust(c *gc.C) {
	specs := []struct {
		descr      string
		bundle     string
		expAppList []string
	}{
		{
			descr:      "bundle with a single app with the trust field set to true",
			bundle:     "aws-integrator-trust-single",
			expAppList: []string{"aws-integrator"},
		},
		{
			descr:      "bundle with a multiple apps with the trust field set to true",
			bundle:     "aws-integrator-trust-multi",
			expAppList: []string{"aws-integrator", "gcp-integrator"},
		},
		{
			descr:      "bundle with a single app with a 'trust: true' config option",
			bundle:     "aws-integrator-trust-conf-param",
			expAppList: []string{"aws-integrator"},
		},
	}

	for specIndex, spec := range specs {
		c.Logf("[spec %d] %s", specIndex, spec.descr)

		expErr := fmt.Sprintf(`Bundle cannot be deployed without trusting applications with your cloud credentials.
Please repeat the deploy command with the --trust argument if you consent to trust the following application(s):
  - %s`, strings.Join(spec.expAppList, "\n  - "))

		bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), spec.bundle)
		err := s.runDeploy(c, bundlePath)
		c.Assert(err, gc.Not(gc.IsNil))
		c.Assert(err.Error(), gc.Equals, expErr)
	}
}

func (s *DeploySuite) TestDeployBundleWithChannel(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	// The second charm from the bundle does not require trust so no
	// additional configuration should be injected
	ubURL := charm.MustParseURL("cs:~jameinel/ubuntu-lite-7")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "bionic")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "")
	withCharmDeployable(
		s.fakeAPI, ubURL, "bionic",
		&charm.Meta{Name: "ubuntu-lite", Series: []string{"bionic"}},
		nil, false, false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu-lite",
		NumUnits:        1,
	}).Returns([]string{"ubuntu-lite/0"}, error(nil))

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	deploy := s.deployCommand()
	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "basic")
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), bundlePath, "--channel", "edge")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployBundlesRequiringTrust(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("cs:~containers/aws-integrator")
	withCharmRepoResolvable(s.fakeAPI, inURL, "bionic")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	// The aws-integrator charm requires trust and since the operator passes
	// --trust we expect to see a "trust: true" config value in the yaml
	// config passed to deploy.
	//
	// As withCharmDeployable does not support passing a "ConfigYAML"
	// it's easier to just invoke it to set up all other calls and then
	// explicitly Deploy here.
	withCharmDeployable(
		s.fakeAPI, inURL, "bionic",
		&charm.Meta{Name: "aws-integrator", Series: []string{"bionic"}},
		nil, false, false, 0, nil, nil,
	)

	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Architecture: arch.DefaultArchitecture,
		Series:       "bionic",
		Base:         series.MakeDefaultBase("ubuntu", "18.04"),
	}

	deployURL := *inURL
	deployURL.Revision = 1
	deployURL.Series = "bionic"
	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    &deployURL,
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: inURL.Name,
		Series:          "bionic",
		ConfigYAML:      "aws-integrator:\n  trust: \"true\"\n",
	}).Returns(error(nil))
	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    &deployURL,
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: inURL.Name,
		Series:          "bionic",
	}).Returns(errors.New("expected Deploy for aws-integrator to be called with 'trust: true'"))

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "aws-integrator",
		NumUnits:        1,
	}).Returns([]string{"aws-integrator/0"}, error(nil))

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	// The second charm from the bundle does not require trust so no
	// additional configuration should be injected
	ubURL := charm.MustParseURL("cs:~jameinel/ubuntu-lite-7")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "bionic")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "")
	withCharmDeployable(
		s.fakeAPI, ubURL, "bionic",
		&charm.Meta{Name: "ubuntu-lite", Series: []string{"bionic"}},
		nil, false, false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu-lite",
		NumUnits:        1,
	}).Returns([]string{"ubuntu-lite/0"}, error(nil))

	deploy := s.deployCommand()
	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "aws-integrator-trust-single")
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), bundlePath, "--trust")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployBundleWithOffers(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("cs:apache2-26")
	withCharmRepoResolvable(s.fakeAPI, inURL, "bionic")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	withCharmDeployable(
		s.fakeAPI, inURL, "bionic",
		&charm.Meta{Name: "apache2", Series: []string{"bionic"}},
		nil, false, false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "apache2",
		NumUnits:        1,
	}).Returns([]string{"apache2/0"}, error(nil))

	s.fakeAPI.Call("Offer",
		"deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"apache2",
		[]string{"apache-website", "website-cache"},
		"admin",
		"my-offer",
		"",
	).Returns([]params.ErrorResult{}, nil)

	s.fakeAPI.Call("Offer",
		"deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"apache2",
		[]string{"apache-website"},
		"admin",
		"my-other-offer",
		"",
	).Returns([]params.ErrorResult{}, nil)

	s.fakeAPI.Call("GrantOffer",
		"admin",
		"admin",
		[]string{"controller.my-offer"},
	).Returns(errors.New(`cannot grant admin access to user admin on offer admin/controller.my-offer: user already has "admin" access or greater`))
	s.fakeAPI.Call("GrantOffer",
		"bar",
		"consume",
		[]string{"controller.my-offer"},
	).Returns(nil)

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	deploy := s.deployCommand()
	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "apache2-with-offers")
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), bundlePath)
	c.Assert(err, jc.ErrorIsNil)

	var offerCallCount int
	var grantOfferCallCount int
	for _, call := range s.fakeAPI.Calls() {
		switch call.FuncName {
		case "Offer":
			offerCallCount++
		case "GrantOffer":
			grantOfferCallCount++
		}
	}
	c.Assert(offerCallCount, gc.Equals, 2)
	c.Assert(grantOfferCallCount, gc.Equals, 2)
}

func (s *DeploySuite) TestDeployBundleWithSAAS(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("cs:wordpress")
	withCharmRepoResolvable(s.fakeAPI, inURL, "bionic")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	withCharmDeployable(
		s.fakeAPI, inURL, "bionic",
		&charm.Meta{Name: "wordpress", Series: []string{"bionic"}},
		nil, false, false, 0, nil, nil,
	)

	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "wordpress",
		NumUnits:        1,
	}).Returns([]string{"wordpress/0"}, error(nil))

	s.fakeAPI.Call("GetConsumeDetails",
		"admin/default.mysql",
	).Returns(params.ConsumeOfferDetails{
		Offer: &params.ApplicationOfferDetails{
			OfferName: "mysql",
			OfferURL:  "admin/default.mysql",
		},
		Macaroon: mac,
		ControllerInfo: &params.ExternalControllerInfo{
			ControllerTag: coretesting.ControllerTag.String(),
			Addrs:         []string{"192.168.1.0"},
			Alias:         "controller-alias",
			CACert:        coretesting.CACert,
		},
	}, nil)

	s.fakeAPI.Call("Consume",
		crossmodel.ConsumeApplicationArgs{
			Offer: params.ApplicationOfferDetails{
				OfferName: "mysql",
				OfferURL:  "test:admin/default.mysql",
			},
			ApplicationAlias: "mysql",
			Macaroon:         mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerTag: coretesting.ControllerTag,
				Alias:         "controller-alias",
				Addrs:         []string{"192.168.1.0"},
				CACert:        coretesting.CACert,
			},
		},
	).Returns("mysql", nil)

	s.fakeAPI.Call("AddRelation",
		[]interface{}{"wordpress:db", "mysql:db"}, []interface{}{},
	).Returns(
		&params.AddRelationResults{},
		error(nil),
	)

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	deploy := s.deployCommand()
	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "wordpress-with-saas")
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), bundlePath)
	c.Assert(err, jc.ErrorIsNil)
}

type CAASDeploySuiteBase struct {
	jujutesting.IsolationSuite
	deployer.DeployerAPI
	Store           *jujuclient.MemStore
	DeployResources deployer.DeployResourcesFunc

	CharmsPath string
	factory    *mocks.MockDeployerFactory
	deployer   *mocks.MockDeployer
}

func (s *CAASDeploySuiteBase) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.factory = mocks.NewMockDeployerFactory(ctrl)
	s.deployer = mocks.NewMockDeployer(ctrl)
	return ctrl
}

func (s *CAASDeploySuiteBase) expectDeployer(c *gc.C, cfg deployer.DeployerConfig) {
	match := deployerConfigMatcher{
		c:        c,
		expected: cfg,
	}
	s.factory.EXPECT().GetDeployer(match, gomock.Any(), gomock.Any()).Return(s.deployer, nil)
	s.deployer.EXPECT().PrepareAndDeploy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

func (s *CAASDeploySuiteBase) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(&supportedJujuSeries, func(time.Time, string, string) (set.Strings, error) {
		return defaultSupportedJujuSeries, nil
	})
	cookiesFile := filepath.Join(c.MkDir(), ".go-cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookiesFile)

	s.Store = jujuclienttesting.MinimalStore()
	m := s.Store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.Store.Models["arthur"].Models["king/caas-model"] = m

	s.CharmsPath = c.MkDir()
}

func (s *CAASDeploySuiteBase) fakeAPI() *fakeDeployAPI {
	cfgAttrs := map[string]interface{}{
		"name":             "sword",
		"uuid":             coretesting.ModelTag.Id(),
		"type":             model.CAAS,
		"operator-storage": "k8s-storage",
	}
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)
	fakeAPI.deployerFactoryFunc = func(dep deployer.DeployerDependencies) deployer.DeployerFactory {
		return s.factory
	}
	return fakeAPI
}

func (s *CAASDeploySuiteBase) runDeploy(c *gc.C, fakeAPI *fakeDeployAPI, args ...string) (*cmd.Context, error) {
	deployCmd := &DeployCommand{
		NewDeployAPI: func() (deployer.DeployerAPI, error) {
			return fakeAPI, nil
		},
		DeployResources: s.DeployResources,
		NewCharmRepo: func() (*store.CharmStoreAdaptor, error) {
			return &store.CharmStoreAdaptor{MacaroonGetter: &noopMacaroonGetter{}}, nil
		},
		NewResolver: func(charmsAPI store.CharmsAPI, charmRepoFn store.CharmStoreRepoFunc, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
			return fakeAPI
		},
		NewDeployerFactory: fakeAPI.deployerFactoryFunc,
		NewModelConfigAPI: func(api base.APICallCloser) ModelConfigGetter {
			return fakeAPI
		},
		NewCharmsAPI: func(api base.APICallCloser) CharmsAPI {
			return apicharms.NewClient(fakeAPI)
		},
	}
	deployCmd.SetClientStore(s.Store)
	return cmdtesting.RunCommand(c, modelcmd.Wrap(deployCmd), args...)
}

type CAASDeploySuite struct {
	CAASDeploySuiteBase
}

var _ = gc.Suite(&CAASDeploySuite{})

func (s *CAASDeploySuite) TestInitErrorsCaasModel(c *gc.C) {
	for i, t := range caasTests {
		deployCmd := NewDeployCommand()
		deployCmd.SetClientStore(s.Store)
		c.Logf("Running %d with args %v", i, t.args)
		err := cmdtesting.InitCommand(deployCmd, t.args)
		c.Assert(err, gc.ErrorMatches, t.message)
	}
}

var caasTests = []struct {
	args    []string
	message string
}{
	{[]string{"-m", "caas-model", "some-application-name", "--attach-storage", "foo/0"},
		"--attach-storage cannot be used on k8s models"},
	{[]string{"-m", "caas-model", "some-application-name", "--to", "a=b"},
		regexp.QuoteMeta(`--to cannot be used on k8s models`)},
}

func (s *CAASDeploySuite) TestCaasModelValidatedAtRun(c *gc.C) {
	for i, t := range caasTests {
		c.Logf("Running %d with args %v", i, t.args)
		s.Store = jujuclienttesting.MinimalStore()
		mycmd := NewDeployCommand()
		mycmd.SetClientStore(s.Store)
		err := cmdtesting.InitCommand(mycmd, t.args)
		c.Assert(err, jc.ErrorIsNil)

		m := s.Store.Models["arthur"].Models["king/sword"]
		m.ModelType = model.CAAS
		s.Store.Models["arthur"].Models["king/caas-model"] = m
		ctx := cmdtesting.Context(c)
		err = mycmd.Run(ctx)
		c.Assert(err, gc.ErrorMatches, t.message)
	}
}

func (s *CAASDeploySuite) TestLocalCharmNeedsResources(c *gc.C) {
	repo := testcharms.RepoWithSeries("kubernetes")
	charmDir := repo.ClonedDir(s.CharmsPath, "mariadb")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	curl := charm.MustParseURL("local:kubernetes/mariadb-0")
	withLocalCharmDeployable(fakeAPI, curl, charmDir, false)
	withCharmDeployable(
		fakeAPI, curl, "kubernetes",
		charmDir.Meta(),
		charmDir.Metrics(),
		true, false, 1, nil, nil,
	)

	// This error is from a different package, ensure we setup correctly for it.
	// "local charm missing OCI images for: .... "
	_, _ = s.runDeploy(c, fakeAPI, charmDir.Path, "-m", "caas-model")

	cfg.Resources = map[string]string{"mysql_image": "abc"}
	s.expectDeployer(c, cfg)
	// This error is from a different package, ensure we setup correctly for it.
	// "local charm missing OCI images for: another_image"
	_, _ = s.runDeploy(c, fakeAPI, charmDir.Path, "-m", "caas-model", "--resource", "mysql_image=abc")

	cfg.Resources = map[string]string{"another_image": "zxc", "mysql_image": "abc"}
	s.expectDeployer(c, cfg)
	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "-m", "caas-model", "--resource", "mysql_image=abc", "--resource", "another_image=zxc")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CAASDeploySuite) TestDevices(c *gc.C) {
	repo := testcharms.RepoWithSeries("kubernetes")
	charmDir := repo.ClonedDir(s.CharmsPath, "bitcoin-miner")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	cfg.Devices = map[string]devices.Constraints{
		"bitcoinminer": {
			Type:  "nvidia.com/gpu",
			Count: 10,
		},
	}
	cfg.Series = "kubernetes"
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	curl := charm.MustParseURL("local:kubernetes/bitcoin-miner-1")
	withLocalCharmDeployable(fakeAPI, curl, charmDir, false)
	withCharmDeployableWithDevices(
		fakeAPI, curl, curl.Name, "kubernetes",
		charmDir.Meta(),
		charmDir.Metrics(),
		true, false, 1, nil, nil,
		map[string]devices.Constraints{
			"bitcoinminer": {Type: "nvidia.com/gpu", Count: 10},
		},
	)
	s.DeployResources = func(
		applicationID string,
		chID resources.CharmID,
		csMac *macaroon.Macaroon,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		conn base.APICallCloser,
		filesystem modelcmd.Filesystem,
	) (ids map[string]string, err error) {
		fakeAPI.AddCall("DeployResources", applicationID, chID, csMac, filesAndRevisions, resources, conn)
		return nil, fakeAPI.NextErr()
	}

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "-m", "caas-model", "--device", "bitcoinminer=10,nvidia.com/gpu", "--series", "kubernetes")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployStorageFailContainer(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	machine, err := s.State.AddMachine(version.DefaultSupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container := "lxd:" + machine.Id()
	err = s.runDeploy(c, charmDir.Path, "--to", container, "--storage", "data=machinescoped,1G")
	c.Assert(err, gc.ErrorMatches, "adding storage to lxd container not supported")
}

func (s *DeploySuite) TestPlacement(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)
	// Add a machine that will be ignored due to placement directive.
	machine, err := s.State.AddMachine(version.DefaultSupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = s.runDeployForState(c, charmDir.Path, "-n", "1", "--to", "valid", "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)

	svc, err := s.State.Application("dummy")
	c.Assert(err, jc.ErrorIsNil)

	// manually run staged assignments
	errs, err := unitassignerapi.New(s.APIState).AssignUnits([]names.UnitTag{names.NewUnitTag("dummy/0")})
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
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:bionic/logging")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--constraints", "mem=1G", "--series", "bionic")
	c.Assert(err, gc.ErrorMatches, "cannot use --constraints with subordinate application")
}

func (s *DeploySuite) TestNumUnits(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "-n", "13", "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl, 13, 0)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:bionic/logging")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, "--num-units", "3", charmDir.Path, "--series", "bionic")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate application")
	_, err = s.State.Application("dummy")
	c.Assert(err, gc.ErrorMatches, `application "dummy" not found`)
}

func (s *DeploySuite) assertForceMachine(c *gc.C, machineId string) {
	svc, err := s.State.Application("portlandia")
	c.Assert(err, jc.ErrorIsNil)

	// manually run staged assignments
	errs, err := unitassignerapi.New(s.APIState).AssignUnits([]names.UnitTag{names.NewUnitTag("portlandia/0")})
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
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:bionic/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	machine, err := s.State.AddMachine(version.DefaultSupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeployForState(c, "--to", machine.Id(), charmDir.Path, "portlandia", "--series", version.DefaultSupportedLTS())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:bionic/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	template := state.MachineTemplate{
		Series: version.DefaultSupportedLTS(),
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeployForState(c, "--to", container.Id(), charmDir.Path, "portlandia", "--series", version.DefaultSupportedLTS())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, container.Id())
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:bionic/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	ltsseries := version.DefaultSupportedLTS()
	withCharmDeployable(s.fakeAPI, curl, ltsseries, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	machine, err := s.State.AddMachine(version.DefaultSupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeployForState(c, "--to", "lxd:"+machine.Id(), charmDir.Path, "portlandia", "--series", "bionic")
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
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:trusty/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, "--to", "42", charmDir.Path, "portlandia", "--series", "bionic")
	c.Assert(err, gc.ErrorMatches, `cannot deploy "portlandia" to machine 42: machine 42 not found`)
	_, err = s.State.Application("portlandia")
	c.Assert(err, gc.ErrorMatches, `application "portlandia" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *gc.C) {
	machine, err := s.State.AddMachine(version.DefaultSupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:bionic/logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err = s.runDeployForState(c, "--to", machine.Id(), charmDir.Path, "--series", "bionic")

	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate application")
	_, err = s.State.Application("dummy")
	c.Assert(err, gc.ErrorMatches, `application "dummy" not found`)
}

func (s *DeploySuite) TestNonLocalCannotHostUnits(c *gc.C) {
	s.fakeAPI.Call("CharmInfo", "local:dummy").Returns(
		&apicommoncharms.CharmInfo{
			URL:  "local:dummy",
			Meta: &charm.Meta{Name: "dummy", Series: []string{"bionic"}},
		},
		error(nil),
	)
	err := s.runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, gc.Not(gc.ErrorMatches), "machine 0 is the controller for a local model and cannot host units")
}

func (s *DeploySuite) TestDeployLocalWithTerms(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "terms1")
	curl := charm.MustParseURL("local:trusty/terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "terms1", curl, 1, 0)
}

func (s *DeploySuite) TestDeployFlags(c *gc.C) {
	// TODO: (2020-06-03)
	// Move to deployer package for testing, then BundleOnlyFlags and
	// CharmOnlyFlags can be private again.
	command := DeployCommand{}
	flagSet := gnuflag.NewFlagSetWithFlagKnownAs(command.Info().Name, gnuflag.ContinueOnError, "option")
	command.SetFlags(flagSet)
	c.Assert(command.flagSet, jc.DeepEquals, flagSet)
	// Add to the slice below if a new flag is introduced which is valid for
	// both charms and bundles.
	charmAndBundleFlags := []string{"channel", "storage", "device", "dry-run", "force", "trust", "revision"}
	var allFlags []string
	flagSet.VisitAll(func(flag *gnuflag.Flag) {
		allFlags = append(allFlags, flag.Name)
	})
	declaredFlags := append(charmAndBundleFlags, deployer.CharmOnlyFlags()...)
	declaredFlags = append(declaredFlags, deployer.BundleOnlyFlags...)
	declaredFlags = append(declaredFlags, "B", "no-browser-login")
	sort.Strings(declaredFlags)
	c.Assert(declaredFlags, jc.DeepEquals, allFlags)
}

func (s *DeploySuite) TestDeployLocalWithSeriesMismatchReturnsError(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "terms1")
	curl := charm.MustParseURL("local:trusty/terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "quantal", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	_, _, err := s.runDeployWithOutput(c, charmDir.Path, "--series", "quantal")

	c.Check(err, gc.ErrorMatches, `terms1 is not available on the following series: quantal not supported`)
}

func (s *DeploySuite) TestDeployLocalWithSeriesAndForce(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("quantal").ClonedDir(c.MkDir(), "terms1")
	curl := charm.MustParseURL("local:quantal/terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, true)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, true, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "terms1", curl, 1, 0)
}

func (s *DeploySuite) setupNonESMSeries(c *gc.C) (string, string) {
	supported := set.NewStrings(series.SupportedJujuWorkloadSeries()...)
	// Allowing kubernetes as an option, can lead to an unrelated failure:
	// 		series "kubernetes" in a non container model not valid
	supported.Remove("kubernetes")
	supportedNotEMS := supported.Difference(set.NewStrings(series.ESMSupportedJujuSeries()...))
	c.Assert(supportedNotEMS.Size(), jc.GreaterThan, 0)

	s.PatchValue(&supportedJujuSeries, func(time.Time, string, string) (set.Strings, error) {
		return supported, nil
	})

	nonEMSSeries := supportedNotEMS.SortedValues()[0]

	loggingPath := filepath.Join(c.MkDir(), "series-logging")
	repo := testcharms.RepoWithSeries("bionic")
	charmDir := repo.CharmDir("logging")
	err := fs.Copy(charmDir.Path, loggingPath)
	c.Assert(err, jc.ErrorIsNil)
	metadataPath := filepath.Join(loggingPath, "metadata.yaml")
	file, err := os.OpenFile(metadataPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open metadata.yaml"))
	}
	defer func() { _ = file.Close() }()

	// Overwrite the metadata.yaml to contain a non EMS series.
	newMetadata := strings.Join([]string{`name: logging`, `summary: ""`, `description: ""`, `series: `, `  - ` + nonEMSSeries, `  - artful`}, "\n")
	if _, err := file.WriteString(newMetadata); err != nil {
		c.Fatal("cannot write to metadata.yaml")
	}

	curl := charm.MustParseURL(fmt.Sprintf("local:%s/series-logging-1", nonEMSSeries))
	ch, err := charm.ReadCharm(loggingPath)
	c.Assert(err, jc.ErrorIsNil)
	withLocalCharmDeployable(s.fakeAPI, curl, ch, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "logging", nonEMSSeries, ch.Meta(), ch.Metrics(), false, false, 1, nil, nil)

	return nonEMSSeries, loggingPath
}

func (s *DeploySuite) TestDeployLocalWithSupportedNonESMSeries(c *gc.C) {
	nonEMSSeries, loggingPath := s.setupNonESMSeries(c)
	err := s.runDeploy(c, loggingPath, "--series", nonEMSSeries)
	c.Logf("%+v", s.fakeAPI.Calls())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployLocalWithNotSupportedNonESMSeries(c *gc.C) {
	_, loggingPath := s.setupNonESMSeries(c)
	err := s.runDeploy(c, loggingPath, "--series", "artful")
	c.Assert(err, gc.ErrorMatches, "logging is not available on the following series: artful not supported")
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

func (s *DeploySuite) TestDeployWithTermsNotSigned(c *gc.C) {
	termsRequiredError := &common.TermsRequiredError{Terms: []string{"term/1", "term/2"}}
	curl := charm.MustParseURL("cs:bionic/terms1")
	withCharmRepoResolvable(s.fakeAPI, curl, "")
	deployURL := *curl
	deployURL.Revision = 1
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Architecture: arch.DefaultArchitecture,
		Series:       "bionic",
		Base:         series.MakeDefaultBase("ubuntu", "18.04"),
	}
	s.fakeAPI.Call("AddCharm", &deployURL, origin, false).Returns(origin, error(termsRequiredError))
	s.fakeAPI.Call("CharmInfo", deployURL.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:  deployURL.String(),
			Meta: &charm.Meta{Name: "dummy", Series: []string{"bionic"}},
		},
		error(nil),
	)
	deploy := s.deployCommand()

	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bionic/terms1")
	expectedError := `Declined: some terms require agreement. Try: "juju agree term/1 term/2"`
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *DeploySuite) TestDeployWithChannel(c *gc.C) {
	curl := charm.MustParseURL("cs:bionic/dummy-1")
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Architecture: arch.DefaultArchitecture,
		Risk:         "beta",
	}
	originWithSeries := commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Architecture: arch.DefaultArchitecture,
		Series:       "bionic",
		Base:         series.MakeDefaultBase("ubuntu", "18.04"),
		Risk:         "beta",
	}
	s.fakeAPI.Call("ResolveCharm", curl, origin, false).Returns(
		curl,
		origin,
		[]string{"bionic"}, // Supported series
		error(nil),
	)
	s.fakeAPI.Call("ResolveCharm", curl, originWithSeries, false).Returns(
		curl,
		originWithSeries,
		[]string{"bionic"}, // Supported series
		error(nil),
	)
	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    curl,
			Origin: originWithSeries,
		},
		CharmOrigin:     originWithSeries,
		ApplicationName: curl.Name,
		Series:          "bionic",
		NumUnits:        1,
	}).Returns(error(nil))
	s.fakeAPI.Call("AddCharm", curl, originWithSeries, false).Returns(originWithSeries, error(nil))
	withCharmDeployable(
		s.fakeAPI, curl, "bionic",
		&charm.Meta{Name: "dummy", Series: []string{"bionic"}},
		nil, false, false, 0, nil, nil,
	)
	deploy := s.deployCommand()

	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bionic/dummy-1", "--channel", "beta")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployCharmsEndpointNotImplemented(c *gc.C) {
	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	meteredCharmURL := charm.MustParseURL("cs:bionic/metered-1")
	charmDir := testcharms.CharmRepo().CharmDir("metered")

	s.fakeAPI.planURL = server.URL
	withCharmRepoResolvable(s.fakeAPI, meteredCharmURL, "")

	series := "bionic"

	fallbackCons := constraints.MustParse("arch=amd64")
	platform, _ := apputils.DeducePlatform(constraints.Value{}, "bionic", fallbackCons)
	origin, _ := apputils.DeduceOrigin(meteredCharmURL, charm.Channel{}, platform)
	s.fakeAPI.Call("AddCharm", meteredCharmURL, origin, false).Returns(origin, error(nil))
	s.fakeAPI.Call("CharmInfo", meteredCharmURL.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:     meteredCharmURL.String(),
			Meta:    charmDir.Meta(),
			Metrics: charmDir.Metrics(),
		},
		error(nil),
	)
	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    meteredCharmURL,
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: "metered",
		Series:          series,
		NumUnits:        1,
	}).Returns(error(nil))
	s.fakeAPI.Call("IsMetered", meteredCharmURL.String()).Returns(true, error(nil))

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	s.fakeAPI.Call("SetMetricCredentials", meteredCharmURL.Name, creds).Returns(errors.New("IsMetered"))

	deploy := s.deployCommand()
	deploy.Steps = []deployer.DeployStep{&deployer.RegisterMeteredCharm{PlanURL: server.URL}}
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bionic/metered-1", "--plan", "someplan")

	c.Check(err, gc.ErrorMatches, "IsMetered")
}

func (s *DeploySuite) TestAddMetricCredentials(c *gc.C) {
	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	charmDir := testcharms.CharmRepo().CharmDir("metered")
	meteredURL := charm.MustParseURL("cs:bionic/metered-1")
	s.fakeAPI.planURL = server.URL
	withCharmDeployable(s.fakeAPI, meteredURL, "bionic", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)
	withCharmRepoResolvable(s.fakeAPI, meteredURL, "")

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	setMetricCredentialsCall := s.fakeAPI.Call("SetMetricCredentials", meteredURL.Name, creds).Returns(error(nil))

	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL: meteredURL,
			Origin: commoncharm.Origin{
				Source:       commoncharm.OriginCharmStore,
				Architecture: arch.DefaultArchitecture,
			},
		},
		CharmOrigin: commoncharm.Origin{
			Source:       commoncharm.OriginCharmStore,
			Architecture: arch.DefaultArchitecture,
		},
		ApplicationName: meteredURL.Name,
		Series:          "bionic",
		NumUnits:        1,
	}).Returns(error(nil))

	deploy := s.deployCommand()
	deploy.Steps = []deployer.DeployStep{&deployer.RegisterMeteredCharm{PlanURL: server.URL}}
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bionic/metered-1", "--plan", "someplan")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(setMetricCredentialsCall(), gc.Equals, 1)

	stub.CheckCalls(c, []jujutesting.StubCall{{
		"Authorize", []interface{}{deployer.MetricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:bionic/metered-1",
			ApplicationName: "metered",
			PlanURL:         "someplan",
			IncreaseBudget:  0,
		}},
	}})
}

func (s *DeploySuite) TestAddMetricCredentialsDefaultPlan(c *gc.C) {
	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	charmDir := testcharms.CharmRepo().CharmDir("metered")

	meteredURL := charm.MustParseURL("cs:bionic/metered-1")
	s.fakeAPI.planURL = server.URL
	withCharmDeployable(s.fakeAPI, meteredURL, "bionic", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)
	withCharmRepoResolvable(s.fakeAPI, meteredURL, "")

	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	setMetricCredentialsCall := s.fakeAPI.Call("SetMetricCredentials", meteredURL.Name, creds).Returns(error(nil))

	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL: meteredURL,
			Origin: commoncharm.Origin{
				Source:       commoncharm.OriginCharmStore,
				Architecture: arch.DefaultArchitecture,
			},
		},
		CharmOrigin: commoncharm.Origin{
			Source:       commoncharm.OriginCharmStore,
			Architecture: arch.DefaultArchitecture,
		},
		ApplicationName: meteredURL.Name,
		Series:          "bionic",
		NumUnits:        1,
	}).Returns(error(nil))

	deploy := s.deployCommand()
	deploy.Steps = []deployer.DeployStep{&deployer.RegisterMeteredCharm{PlanURL: server.URL}}
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bionic/metered-1")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(setMetricCredentialsCall(), gc.Equals, 1)
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"DefaultPlan", []interface{}{"cs:bionic/metered-1"},
	}, {
		"Authorize", []interface{}{deployer.MetricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:bionic/metered-1",
			ApplicationName: "metered",
			PlanURL:         "thisplan",
			IncreaseBudget:  0,
		}},
	}})
}

func (s *DeploySuite) TestSetMetricCredentialsNotCalledForUnmeteredCharm(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmDir("dummy")
	charmURL := charm.MustParseURL("cs:bionic/dummy-1")
	withCharmRepoResolvable(s.fakeAPI, charmURL, "")
	withCharmDeployable(s.fakeAPI, charmURL, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL: charmURL,
			Origin: commoncharm.Origin{
				Source:       commoncharm.OriginCharmStore,
				Architecture: arch.DefaultArchitecture,
			},
		},
		CharmOrigin: commoncharm.Origin{
			Source:       commoncharm.OriginCharmStore,
			Architecture: arch.DefaultArchitecture,
		},
		ApplicationName: charmURL.Name,
		Series:          charmURL.Series,
		NumUnits:        1,
	}).Returns(error(nil))

	deploy := s.deployCommand()
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bionic/dummy-1")
	c.Assert(err, jc.ErrorIsNil)

	for _, call := range s.fakeAPI.Calls() {
		if call.FuncName == "SetMetricCredentials" {
			c.Fatal("call to SetMetricCredentials was not supposed to happen")
		}
	}
}

func (s *DeploySuite) TestAddMetricCredentialsNotNeededForOptionalPlan(c *gc.C) {
	metricsYAML := `		
plan:		
required: false		
metrics:		
pings:		
  type: gauge		
  description: ping pongs		
`
	charmDir := testcharms.CharmRepo().ClonedDir(c.MkDir(), "metered")
	metadataPath := filepath.Join(charmDir.Path, "metrics.yaml")
	file, err := os.OpenFile(metadataPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open metrics.yaml"))
	}
	defer func() { _ = file.Close() }()

	// Overwrite the metrics.yaml to contain an optional plan.
	if _, err := file.WriteString(metricsYAML); err != nil {
		c.Fatal("cannot write to metrics.yaml")
	}

	curl := charm.MustParseURL("local:bionic/metered")

	withCharmRepoResolvable(s.fakeAPI, curl, "")
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    curl,
			Origin: defaultLocalOrigin,
		},
		CharmOrigin:     defaultLocalOrigin,
		ApplicationName: curl.Name,
		Series:          "bionic",
		NumUnits:        1,
	}).Returns(error(nil))

	deploy := s.deployCommand()
	deploy.Steps = []deployer.DeployStep{&deployer.RegisterMeteredCharm{PlanURL: server.URL}}
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), curl.String())
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckNoCalls(c)
}

func (s *DeploySuite) TestSetMetricCredentialsCalledWhenPlanSpecifiedWhenOptional(c *gc.C) {
	metricsYAML := `		
plan:		
required: false		
metrics:		
pings:		
  type: gauge		
  description: ping pongs		
`
	charmDir := testcharms.CharmRepo().ClonedDir(c.MkDir(), "metered")
	metadataPath := filepath.Join(charmDir.Path, "metrics.yaml")
	file, err := os.OpenFile(metadataPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open metrics.yaml"))
	}
	defer func() { _ = file.Close() }()

	// Overwrite the metrics.yaml to contain an optional plan.
	if _, err := file.WriteString(metricsYAML); err != nil {
		c.Fatal("cannot write to metrics.yaml")
	}

	curl := charm.MustParseURL("local:bionic/metered")

	stub := &jujutesting.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: stub}
	server := httptest.NewServer(handler)
	defer server.Close()

	s.fakeAPI.planURL = server.URL
	withCharmRepoResolvable(s.fakeAPI, curl, "")
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    curl,
			Origin: defaultLocalOrigin,
		},
		CharmOrigin:     defaultLocalOrigin,
		ApplicationName: curl.Name,
		Series:          curl.Series,
		NumUnits:        1,
	}).Returns(error(nil))

	deploy := s.deployCommand()
	deploy.Steps = []deployer.DeployStep{&deployer.RegisterMeteredCharm{PlanURL: server.URL}}
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), curl.String(), "--plan", "someplan")
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"Authorize", []interface{}{deployer.MetricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "local:bionic/metered",
			ApplicationName: "metered",
			PlanURL:         "someplan",
			IncreaseBudget:  0,
		}},
	}})
}

type availablePlanURL struct {
	URL string `json:"url"`
}

// testMetricsRegistrationHandler duplicated from deployer/register_test.go
// for MetricCredentials tests
type testMetricsRegistrationHandler struct {
	*jujutesting.Stub
	availablePlans []availablePlanURL
}

type respErr struct {
	Error string `json:"error"`
}

// ServeHTTP implements http.Handler.
func (c *testMetricsRegistrationHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		var registrationPost deployer.MetricRegistrationPost
		decoder := json.NewDecoder(req.Body)
		err := decoder.Decode(&registrationPost)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		c.AddCall("Authorize", registrationPost)
		rErr := c.NextErr()
		if rErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(respErr{Error: rErr.Error()})
			if err != nil {
				panic(err)
			}
			return
		}
		err = json.NewEncoder(w).Encode([]byte("hello registration"))
		if err != nil {
			panic(err)
		}
	} else if req.Method == "GET" {
		if req.URL.Path == "/default" {
			cURL := req.URL.Query().Get("charm-url")
			c.AddCall("DefaultPlan", cURL)
			rErr := c.NextErr()
			if rErr != nil {
				if errors.IsNotFound(rErr) {
					http.Error(w, rErr.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, rErr.Error(), http.StatusInternalServerError)
				return
			}
			result := struct {
				URL string `json:"url"`
			}{"thisplan"}
			err := json.NewEncoder(w).Encode(result)
			if err != nil {
				panic(err)
			}
			return
		}
		cURL := req.URL.Query().Get("charm-url")
		c.AddCall("ListPlans", cURL)
		rErr := c.NextErr()
		if rErr != nil {
			http.Error(w, rErr.Error(), http.StatusInternalServerError)
			return
		}
		err := json.NewEncoder(w).Encode(c.availablePlans)
		if err != nil {
			panic(err)
		}
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

type FakeStoreStateSuite struct {
	DeploySuiteBase
}

func (s *FakeStoreStateSuite) runDeploy(c *gc.C, args ...string) error {
	_, _, err := s.runDeployWithOutput(c, args...)
	return err
}

func (s *FakeStoreStateSuite) runDeployWithOutput(c *gc.C, args ...string) (string, string, error) {
	deploy := s.deployCommandForState()
	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), args...)
	return strings.Trim(cmdtesting.Stdout(ctx), "\n"),
		strings.Trim(cmdtesting.Stderr(ctx), "\n"),
		err
}

func (s *FakeStoreStateSuite) setupCharm(c *gc.C, url, name, series string) charm.Charm {
	return s.setupCharmMaybeAdd(c, url, name, series, true)
}

func (s *FakeStoreStateSuite) setupCharmWithArch(c *gc.C, url, name, series, arch string) charm.Charm {
	return s.setupCharmMaybeAddForce(c, url, name, series, arch, false, true)
}

func (s *FakeStoreStateSuite) setupCharmMaybeAdd(c *gc.C, url, name, series string, addToState bool) charm.Charm {
	return s.setupCharmMaybeAddForce(c, url, name, series, arch.DefaultArchitecture, false, addToState)
}

func (s *FakeStoreStateSuite) setupCharmMaybeAddForce(c *gc.C, url, name, aseries, arc string, force, addToState bool) charm.Charm {
	baseURL := charm.MustParseURL(url)
	baseURL.Series = ""
	deployURL := charm.MustParseURL(url)
	resolveURL := charm.MustParseURL(url)
	if resolveURL.Revision < 0 {
		resolveURL.Revision = 1
	}
	noRevisionURL := charm.MustParseURL(url)
	noRevisionURL.Series = resolveURL.Series
	noRevisionURL.Revision = -1
	charmStoreURL := charm.MustParseURL(fmt.Sprintf("cs:%s", baseURL.Name))
	seriesURL := charm.MustParseURL(url)
	seriesURL.Series = aseries
	// In order to replicate what the charmstore does in terms of matching, we
	// brute force (badly) the various types of charm urls.
	// TODO (stickupkid): This is terrible, the fact that you're bruteforcing
	// a mock to replicate a charm store, means your test isn't unit testing
	// at any level. Instead we should have tests that know exactly the url
	// is and pass that. The new mocking tests do this.
	charmURLs := []*charm.URL{
		baseURL,
		resolveURL,
		noRevisionURL,
		deployURL,
		charmStoreURL,
		seriesURL,
	}
	for _, url := range charmURLs {
		for _, serie := range []string{"", url.Series, aseries} {
			channel := ""
			if serie != "" {
				var err error
				channel, err = series.SeriesVersion(serie)
				c.Assert(err, jc.ErrorIsNil)
			}
			for _, a := range []string{"", arc, arch.DefaultArchitecture} {
				platform := corecharm.Platform{
					Architecture: a,
					Channel:      channel,
				}
				origin, err := apputils.DeduceOrigin(url, charm.Channel{}, platform)
				c.Assert(err, jc.ErrorIsNil)

				s.fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
					resolveURL,
					origin,
					[]string{aseries},
					error(nil),
				)
				s.fakeAPI.Call("AddCharm", resolveURL, origin, force).Returns(origin, error(nil))
			}
		}
	}

	var chDir charm.Charm
	chDir, err := charm.ReadCharmDir(testcharms.RepoWithSeries(aseries).CharmDirPath(name))
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			c.Fatal(err)
			return nil
		}
		chDir = testcharms.RepoForSeries(aseries).CharmArchive(c.MkDir(), "dummy")
	}
	if addToState {
		_, err = jjtesting.AddCharm(s.State, resolveURL, chDir, force)
		c.Assert(err, jc.ErrorIsNil)
	}
	return chDir
}

func (s *FakeStoreStateSuite) setupBundle(c *gc.C, url, name string, allSeries ...string) {
	bundleResolveURL := charm.MustParseURL(url)
	baseURL := *bundleResolveURL
	baseURL.Revision = -1
	withCharmRepoResolvable(s.fakeAPI, &baseURL, "")
	bundleDir := testcharms.RepoWithSeries(allSeries[0]).BundleArchive(c.MkDir(), name)

	// Resolve a bundle with no revision and return a url with a version.  Ensure
	// GetBundle expects the url with revision.
	for _, serie := range append([]string{"", baseURL.Series}, allSeries...) {
		channel := ""
		if serie != "" {
			var err error
			channel, err = series.SeriesVersion(serie)
			c.Assert(err, jc.ErrorIsNil)
		}
		origin, err := apputils.DeduceOrigin(bundleResolveURL, charm.Channel{}, corecharm.Platform{Channel: channel})
		c.Assert(err, jc.ErrorIsNil)
		s.fakeAPI.Call("ResolveBundleURL", &baseURL, origin).Returns(
			bundleResolveURL,
			origin,
			error(nil),
		)
		s.fakeAPI.Call("GetBundle", bundleResolveURL).Returns(bundleDir, error(nil))
	}
}

func (s *FakeStoreStateSuite) combinedSettings(ch charm.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

// assertCharmsUploaded checks that the given charm ids have been uploaded.
func (s *FakeStoreStateSuite) assertCharmsUploaded(c *gc.C, ids ...string) {
	allCharms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	uploaded := make([]string, len(allCharms))
	for i, ch := range allCharms {
		uploaded[i] = ch.URL().String()
	}
	c.Assert(uploaded, jc.SameContents, ids)
}

// assertDeployedApplicationBindings checks that applications were deployed into the
// expected spaces. It is separate to assertApplicationsDeployed because it is only
// relevant to a couple of tests.
func (s *FakeStoreStateSuite) assertDeployedApplicationBindings(c *gc.C, info map[string]applicationInfo) {
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)

	for _, app := range applications {
		endpointBindings, err := app.EndpointBindings()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(endpointBindings.Map(), jc.DeepEquals, info[app.Name()].endpointBindings)
	}
}

// assertApplicationsDeployed checks that the given applications have been deployed.
func (s *FakeStoreStateSuite) assertApplicationsDeployed(c *gc.C, info map[string]applicationInfo) {
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	deployed := make(map[string]applicationInfo, len(applications))
	for _, app := range applications {
		curl, _ := app.CharmURL()
		c.Assert(err, jc.ErrorIsNil)
		config, err := app.CharmConfig(model.GenerationMaster)
		c.Assert(err, jc.ErrorIsNil)
		constr, err := app.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		stor, err := app.StorageConstraints()
		c.Assert(err, jc.ErrorIsNil)
		if len(stor) == 0 {
			stor = nil
		}
		deviceConstraints, err := app.DeviceConstraints()
		c.Assert(err, jc.ErrorIsNil)
		if len(deviceConstraints) == 0 {
			deviceConstraints = nil
		}
		deployed[app.Name()] = applicationInfo{
			charm:       *curl,
			config:      config,
			constraints: constr,
			exposed:     app.IsExposed(),
			scale:       app.GetScale(),
			storage:     stor,
			devices:     deviceConstraints,
		}
	}
	c.Assert(deployed, jc.DeepEquals, info)
}

// assertRelationsEstablished checks that the given relations have been set.
func (s *FakeStoreStateSuite) assertRelationsEstablished(c *gc.C, relations ...string) {
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
func (s *FakeStoreStateSuite) assertUnitsCreated(c *gc.C, expectedUnits map[string]string) {
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

// applicationInfo holds information about a deployed application.
type applicationInfo struct {
	charm            string
	config           charm.Settings
	constraints      constraints.Value
	scale            int
	exposed          bool
	storage          map[string]state.StorageConstraints
	devices          map[string]state.DeviceConstraints
	endpointBindings map[string]string
}

func (s *DeploySuite) TestDeployCharmWithSomeEndpointBindingsSpecifiedSuccess(c *gc.C) {
	curl := charm.MustParseURL("cs:bionic/wordpress-extra-bindings-1")
	charmDir := testcharms.RepoWithSeries("bionic").CharmDir("wordpress-extra-bindings")
	withCharmRepoResolvable(s.fakeAPI, curl, "")
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)
	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL: curl,
			Origin: commoncharm.Origin{
				Source:       commoncharm.OriginCharmStore,
				Architecture: arch.DefaultArchitecture,
				Base:         series.MakeDefaultBase("ubuntu", "18.04"),
				Series:       "bionic",
			},
		},
		CharmOrigin: commoncharm.Origin{
			Source:       commoncharm.OriginCharmStore,
			Architecture: arch.DefaultArchitecture,
			Base:         series.MakeDefaultBase("ubuntu", "18.04"),
			Series:       "bionic",
		},
		ApplicationName: curl.Name,
		Series:          "bionic",
		NumUnits:        1,
		EndpointBindings: map[string]string{
			"db":        "db",
			"db-client": "db",
			"admin-api": "public",
			"":          "public",
		},
	}).Returns(error(nil))
	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{
		{
			Id:   "0",
			Name: "db",
		}, {
			Id:   "1",
			Name: "public",
		},
	}, error(nil))
	deploy := s.deployCommand()
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bionic/wordpress-extra-bindings-1", "--bind", "db=db db-client=db public admin-api=public")
	c.Assert(err, jc.ErrorIsNil)
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
	deployer.DeployerAPI
	deployer *mocks.MockDeployer
	factory  *mocks.MockDeployerFactory
}

var _ = gc.Suite(&DeployUnitTestSuite{})

func (s *DeployUnitTestSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(&supportedJujuSeries, func(time.Time, string, string) (set.Strings, error) {
		return defaultSupportedJujuSeries, nil
	})
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
	fakeAPI := vanillaFakeModelAPI(s.cfgAttrs())
	fakeAPI.deployerFactoryFunc = func(dep deployer.DeployerDependencies) deployer.DeployerFactory {
		return s.factory
	}
	return fakeAPI
}

func (s *DeployUnitTestSuite) makeCharmDir(c *gc.C, cloneCharm string) *charm.CharmDir {
	charmsPath := c.MkDir()
	return testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, cloneCharm)
}

func (s *DeployUnitTestSuite) runDeploy(c *gc.C, fakeAPI *fakeDeployAPI, args ...string) (*cmd.Context, error) {
	deployCmd := newWrappedDeployCommandForTest(fakeAPI)
	deployCmd.SetClientStore(jujuclienttesting.MinimalStore())

	return cmdtesting.RunCommand(c, deployCmd, args...)
}

func (s *DeployUnitTestSuite) TestDeployApplicationConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:trusty/dummy-0")
	opt := bytes.NewBufferString("foo: bar")
	err := cfg.ConfigOptions.SetAttrsFromReader(opt)
	c.Assert(err, jc.ErrorIsNil)
	s.expectDeployer(c, cfg)

	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "dummy")
	fakeAPI := s.fakeAPI()

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

	deployCmd := newWrappedDeployCommandForTest(fakeAPI)
	deployCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err = cmdtesting.RunCommand(c, deployCmd, dummyURL.String(),
		"--config", "foo=bar",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployLocalCharmGivesCorrectUserMessage(c *gc.C) {
	// Copy multi-series charm to path where we can deploy it from
	charmDir := s.makeCharmDir(c, "multi-series")
	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	cfg.Series = "trusty"
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()

	multiSeriesURL := charm.MustParseURL("local:trusty/multi-series-1")

	withLocalCharmDeployable(fakeAPI, multiSeriesURL, charmDir, false)
	withCharmDeployable(fakeAPI, multiSeriesURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "--series", "trusty")
	c.Check(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestAddMetricCredentialsDefaultForUnmeteredCharm(c *gc.C) {
	charmDir := s.makeCharmDir(c, "multi-series")
	multiSeriesURL := charm.MustParseURL("local:trusty/multi-series-1")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	cfg.Series = "trusty"
	s.expectDeployer(c, cfg)

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
	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:trusty/dummy-0")
	s.expectDeployer(c, cfg)
	charmDir := s.makeCharmDir(c, "dummy")
	fakeAPI := s.fakeAPI()
	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(fakeAPI, dummyURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, dummyURL.String())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployAttachStorage(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "dummy")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:trusty/dummy-0")
	cfg.AttachStorage = []string{"foo/0", "bar/1", "baz/2"}
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()

	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
	)

	deployCmd := newWrappedDeployCommandForTest(fakeAPI)
	deployCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, deployCmd, dummyURL.String(),
		"--attach-storage", "foo/0",
		"--attach-storage", "bar/1,baz/2",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployAttachStorageContainer(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "dummy")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:trusty/dummy-0")
	cfg.AttachStorage = []string{"foo/0"}
	cfg.PlacementSpec = "lxd"
	cfg.Placement = []*instance.Placement{
		{Scope: "lxd"},
	}
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	dummyURL := charm.MustParseURL("local:trusty/dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, "trusty", charmDir.Meta(), charmDir.Metrics(), false, false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
	)

	deployCmd := newWrappedDeployCommandForTest(fakeAPI)
	deployCmd.SetClientStore(jujuclienttesting.MinimalStore())
	// Failure expected here comes from a part that is mocked for
	// deploy tests. deployer charm: "adding storage to lxd container not supported"
	// Checking we setup the scenario correctly in the factory config.
	_, _ = cmdtesting.RunCommand(c, deployCmd, dummyURL.String(),
		"--attach-storage", "foo/0", "--to", "lxd",
	)

}

func basicDeployerConfig(charmOrBundle string) deployer.DeployerConfig {
	cfgOps := common.ConfigFlag{}
	return deployer.DeployerConfig{
		BundleMachines:     map[string]string{},
		CharmOrBundle:      charmOrBundle,
		ConfigOptions:      cfgOps,
		Constraints:        constraints.Value{},
		NumUnits:           1,
		DefaultCharmSchema: charm.CharmHub,
		Revision:           -1,
	}
}

func (s *DeployUnitTestSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.factory = mocks.NewMockDeployerFactory(ctrl)
	s.deployer = mocks.NewMockDeployer(ctrl)
	return ctrl
}

func (s *DeployUnitTestSuite) expectDeployer(c *gc.C, cfg deployer.DeployerConfig) {
	match := deployerConfigMatcher{
		c:        c,
		expected: cfg,
	}
	s.factory.EXPECT().GetDeployer(match, gomock.Any(), gomock.Any()).Return(s.deployer, nil)
	s.deployer.EXPECT().PrepareAndDeploy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

type deployerConfigMatcher struct {
	c        *gc.C
	expected deployer.DeployerConfig
}

func (m deployerConfigMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(deployer.DeployerConfig)
	m.c.Assert(ok, jc.IsTrue)
	if !ok {
		return false
	}
	// FlagSet validation is not required for these tests.
	obtained.FlagSet = nil
	m.c.Assert(obtained, jc.DeepEquals, m.expected)
	return true
}

func (m deployerConfigMatcher) String() string {
	return "match deployer DeployerConfig"
}

// newWrappedDeployCommandForTest returns a command, wrapped by model command,
// to deploy applications.
func newWrappedDeployCommandForTest(fakeApi *fakeDeployAPI) modelcmd.ModelCommand {
	return modelcmd.Wrap(newDeployCommandForTest(fakeApi))
}

// newDeployCommandForTest returns a command to deploy applications.
func newDeployCommandForTest(fakeAPI *fakeDeployAPI) *DeployCommand {
	deployCmd := &DeployCommand{
		NewDeployAPI: func() (deployer.DeployerAPI, error) {
			return fakeAPI, nil
		},
		DeployResources: func(
			applicationID string,
			chID resources.CharmID,
			csMac *macaroon.Macaroon,
			filesAndRevisions map[string]string,
			resources map[string]charmresource.Meta,
			conn base.APICallCloser,
			filesystem modelcmd.Filesystem,
		) (ids map[string]string, err error) {
			return nil, nil
		},
		NewCharmRepo: func() (*store.CharmStoreAdaptor, error) {
			return fakeAPI.CharmStoreAdaptor, nil
		},
	}
	if fakeAPI == nil {
		deployCmd.NewDeployAPI = func() (deployer.DeployerAPI, error) {
			apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			controllerAPIRoot, err := deployCmd.NewControllerAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			mURL, err := deployCmd.getMeteringAPIURL(controllerAPIRoot)
			if err != nil {
				return nil, errors.Trace(err)
			}

			return &deployAPIAdapter{
				Connection:        apiRoot,
				legacyClient:      &apiClient{Client: apiclient.NewClient(apiRoot, coretesting.NoopLogger{})},
				charmsClient:      &charmsClient{Client: apicharms.NewClient(apiRoot)},
				applicationClient: &applicationClient{Client: application.NewClient(apiRoot)},
				modelConfigClient: &modelConfigClient{Client: modelconfig.NewClient(apiRoot)},
				annotationsClient: &annotationsClient{Client: annotations.NewClient(apiRoot)},
				plansClient:       &plansClient{planURL: mURL},
			}, nil
		}
		deployCmd.NewCharmRepo = func() (*store.CharmStoreAdaptor, error) {
			controllerAPIRoot, err := deployCmd.NewControllerAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			bakeryClient, err := deployCmd.BakeryClient()
			if err != nil {
				return nil, errors.Trace(err)
			}
			csURL, err := getCharmStoreAPIURL(controllerAPIRoot)
			if err != nil {
				return nil, errors.Trace(err)
			}
			risk := csclientparams.Channel(deployCmd.Channel.Risk)
			cstoreClient := store.NewCharmStoreClient(bakeryClient, csURL).WithChannel(risk)
			return &store.CharmStoreAdaptor{
				MacaroonGetter:     cstoreClient,
				CharmrepoForDeploy: charmrepo.NewCharmStoreFromClient(cstoreClient),
			}, nil
		}
		deployCmd.NewResolver = func(charmsAPI store.CharmsAPI, charmRepoFn store.CharmStoreRepoFunc, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
			return store.NewCharmAdaptor(charmsAPI, charmRepoFn, downloadClientFn)
		}
		deployCmd.NewDeployerFactory = deployer.NewDeployerFactory
	} else {
		deployCmd.NewDeployerFactory = fakeAPI.deployerFactoryFunc
		deployCmd.NewCharmRepo = fakeAPI.charmRepoFunc
		deployCmd.NewResolver = func(charmsAPI store.CharmsAPI, charmRepoFn store.CharmStoreRepoFunc, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
			return fakeAPI
		}
		deployCmd.NewModelConfigAPI = func(api base.APICallCloser) ModelConfigGetter {
			return fakeAPI
		}
		deployCmd.NewCharmsAPI = func(api base.APICallCloser) CharmsAPI {
			return apicharms.NewClient(fakeAPI)
		}
	}
	return deployCmd
}

// fakeDeployAPI is a mock of the API used by the deploy command. It's
// a little muddled at the moment, but as the deployer.DeployerAPI interface is
// sharpened, this will become so as well.
type fakeDeployAPI struct {
	deployer.DeployerAPI
	*store.CharmStoreAdaptor
	*jujutesting.CallMocker
	planURL             string
	deployerFactoryFunc func(dep deployer.DeployerDependencies) deployer.DeployerFactory
	charmRepoFunc       func() (*store.CharmStoreAdaptor, error)
	modelCons           constraints.Value
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

func (f *fakeDeployAPI) ResolveCharm(url *charm.URL, preferredChannel commoncharm.Origin, switchCharm bool) (
	*charm.URL,
	commoncharm.Origin,
	[]string,
	error,
) {
	results := f.MethodCall(f, "ResolveCharm", url, preferredChannel, switchCharm)
	if results == nil {
		if url.Schema == "cs" || url.Schema == "ch" {
			return nil, commoncharm.Origin{}, nil, errors.Errorf(
				"cannot resolve charm or bundle %q: charm or bundle not found", url.Name)
		}
		return nil, commoncharm.Origin{}, nil, errors.Errorf(
			"unknown schema for charm URL %q", url)
	}
	origin := results[1].(commoncharm.Origin)
	return results[0].(*charm.URL),
		origin,
		results[2].([]string),
		jujutesting.TypeAssertError(results[3])
}

func (f *fakeDeployAPI) ResolveBundleURL(url *charm.URL, preferredChannel commoncharm.Origin) (
	*charm.URL,
	commoncharm.Origin,
	error,
) {
	results := f.MethodCall(f, "ResolveBundleURL", url, preferredChannel)
	if results == nil {
		if url.Series == "bundle" {
			return nil, commoncharm.Origin{}, errors.Errorf(
				"cannot resolve URL %q: bundle not found", url)
		}
		return nil, commoncharm.Origin{}, errors.NotValidf("charmstore bundle %q", url)
	}
	origin := results[1].(commoncharm.Origin)
	return results[0].(*charm.URL),
		origin,
		jujutesting.TypeAssertError(results[2])
}

func (f *fakeDeployAPI) BestFacadeVersion(facade string) int {
	results := f.MethodCall(f, "BestFacadeVersion", facade)
	return results[0].(int)
}

func (f *fakeDeployAPI) APICall(objType string, version int, id, request string, params, response interface{}) error {
	results := f.MethodCall(f, "APICall", objType, version, id, request, params, response)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) Client() *apiclient.Client {
	results := f.MethodCall(f, "Client")
	return results[0].(*apiclient.Client)
}

func (f *fakeDeployAPI) ModelUUID() (string, bool) {
	results := f.MethodCall(f, "ModelUUID")
	return results[0].(string), results[1].(bool)
}

func (f *fakeDeployAPI) AddLocalCharm(url *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	results := f.MethodCall(f, "AddLocalCharm", url, ch, force)
	if results == nil {
		return nil, errors.NotFoundf("registered API call AddLocalCharm %v", url)
	}
	return results[0].(*charm.URL), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddCharm(url *charm.URL, origin commoncharm.Origin, force bool) (commoncharm.Origin, error) {
	results := f.MethodCall(f, "AddCharm", url, origin, force)
	return results[0].(commoncharm.Origin), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddCharmWithAuthorization(
	url *charm.URL,
	origin commoncharm.Origin,
	macaroon *macaroon.Macaroon,
	force bool,
) (commoncharm.Origin, error) {
	results := f.MethodCall(f, "AddCharmWithAuthorization", url, origin, macaroon, force)
	return results[0].(commoncharm.Origin), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) CharmInfo(url string) (*apicommoncharms.CharmInfo, error) {
	results := f.MethodCall(f, "CharmInfo", url)
	return results[0].(*apicommoncharms.CharmInfo), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Get(endpoint string, extra interface{}) error {
	return nil
}

func (f *fakeDeployAPI) Deploy(args application.DeployArgs) error {
	results := f.MethodCall(f, "Deploy", args)
	if len(results) != 1 {
		return errors.Errorf("expected 1 result, got %d: %v", len(results), results)
	}
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) ListSpaces() ([]params.Space, error) {
	results := f.MethodCall(f, "ListSpaces")
	return results[0].([]params.Space), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) GetAnnotations(_ []string) ([]params.AnnotationsGetResult, error) {
	return nil, nil
}

func (f *fakeDeployAPI) GetConfig(_ string, _ ...string) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeDeployAPI) GetConstraints(_ ...string) ([]constraints.Value, error) {
	return nil, nil
}

func (f *fakeDeployAPI) GetModelConstraints() (constraints.Value, error) {
	f.MethodCall(f, "GetModelConstraints")
	return f.modelCons, nil
}

func (f *fakeDeployAPI) GetBundle(url *charm.URL, _ commoncharm.Origin, _ string) (charm.Bundle, error) {
	results := f.MethodCall(f, "GetBundle", url)
	if results == nil {
		return nil, errors.NotFoundf("bundle %v", url)
	}
	return results[0].(charm.Bundle), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Status(patterns []string) (*params.FullStatus, error) {
	results := f.MethodCall(f, "Status", patterns)
	return results[0].(*params.FullStatus), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) WatchAll() (api.AllWatch, error) {
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

func (f *fakeDeployAPI) Expose(application string, exposedEndpoints map[string]params.ExposedEndpoint) error {
	results := f.MethodCall(f, "Expose", application, exposedEndpoints)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) SetAnnotation(annotations map[string]map[string]string) ([]params.ErrorResult, error) {
	results := f.MethodCall(f, "SetAnnotation", annotations)
	return results[0].([]params.ErrorResult), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) SetCharm(branchName string, cfg application.SetCharmConfig) error {
	results := f.MethodCall(f, "SetCharm", branchName, cfg)
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

func (f *fakeDeployAPI) ScaleApplication(p application.ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	return params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: p.Scale},
	}, nil
}

func (f *fakeDeployAPI) Offer(modelUUID, application string, endpoints []string, owner, offerName, descr string) ([]params.ErrorResult, error) {
	results := f.MethodCall(f, "Offer", modelUUID, application, endpoints, owner, offerName, descr)
	return results[0].([]params.ErrorResult), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) GetConsumeDetails(offerURL string) (params.ConsumeOfferDetails, error) {
	results := f.MethodCall(f, "GetConsumeDetails", offerURL)
	return results[0].(params.ConsumeOfferDetails), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Consume(arg crossmodel.ConsumeApplicationArgs) (string, error) {
	results := f.MethodCall(f, "Consume", arg)
	return results[0].(string), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) GrantOffer(user, access string, offerURLs ...string) error {
	res := f.MethodCall(f, "GrantOffer", user, access, offerURLs)
	return jujutesting.TypeAssertError(res[0])
}

func (f *fakeDeployAPI) ResolveWithPreferredChannel(url *charm.URL, risk csparams.Channel) (*charm.URL, csparams.Channel, []string, error) {
	results := f.MethodCall(f, "ResolveWithPreferredChannel", url)
	return results[0].(*charm.URL), results[1].(csparams.Channel), results[2].([]string), results[3].(error)
}

type fakeCharmStoreAPI struct {
	*fakeDeployAPI
}

func (f *fakeCharmStoreAPI) GetBundle(url *charm.URL, _ string) (charm.Bundle, error) {
	results := f.MethodCall(f, "GetBundle", url)
	if results == nil {
		return nil, errors.NotFoundf("bundle %v", url)
	}
	return results[0].(charm.Bundle), jujutesting.TypeAssertError(results[1])
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
	fakeAPI.charmRepoFunc = func() (*store.CharmStoreAdaptor, error) {
		return &store.CharmStoreAdaptor{
			MacaroonGetter: &noopMacaroonGetter{},
		}, nil
	}

	fakeAPI.Call("Close").Returns(error(nil))
	fakeAPI.Call("ModelGet").Returns(cfgAttrs, error(nil))
	fakeAPI.Call("ModelUUID").Returns("deadbeef-0bad-400d-8000-4b1d0d06f00d", true)
	fakeAPI.Call("BestFacadeVersion", "Application").Returns(6)
	fakeAPI.Call("BestFacadeVersion", "Charms").Returns(3)

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
	withCharmDeployableWithDevices(
		fakeAPI,
		url,
		url.Name,
		series,
		meta,
		metrics,
		metered,
		force,
		numUnits,
		attachStorage,
		config,
		nil,
	)
}

func withAliasedCharmDeployable(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	appName string,
	series string,
	meta *charm.Meta,
	metrics *charm.Metrics,
	metered bool,
	force bool,
	numUnits int,
	attachStorage []string,
	config map[string]string,
) {
	withCharmDeployableWithDevicesAndStorage(
		fakeAPI,
		url,
		appName,
		series,
		meta,
		metrics,
		metered,
		force,
		numUnits,
		attachStorage,
		config,
		nil,
		nil,
	)
}

func withCharmDeployableWithDevices(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	appName string,
	series string,
	meta *charm.Meta,
	metrics *charm.Metrics,
	metered bool,
	force bool,
	numUnits int,
	attachStorage []string,
	config map[string]string,
	devices map[string]devices.Constraints,
) {
	withCharmDeployableWithDevicesAndStorage(
		fakeAPI,
		url,
		appName,
		series,
		meta,
		metrics,
		metered,
		force,
		numUnits,
		attachStorage,
		config,
		nil,
		devices,
	)
}

func withCharmDeployableWithStorage(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	appName string,
	series string,
	meta *charm.Meta,
	metrics *charm.Metrics,
	metered bool,
	force bool,
	numUnits int,
	attachStorage []string,
	config map[string]string,
	storage map[string]storage.Constraints,
) {
	withCharmDeployableWithDevicesAndStorage(
		fakeAPI,
		url,
		appName,
		series,
		meta,
		metrics,
		metered,
		force,
		numUnits,
		attachStorage,
		config,
		storage,
		nil,
	)
}

func withCharmDeployableWithDevicesAndStorage(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	appName string,
	series string,
	meta *charm.Meta,
	metrics *charm.Metrics,
	metered bool,
	force bool,
	numUnits int,
	attachStorage []string,
	config map[string]string,
	storage map[string]storage.Constraints,
	devices map[string]devices.Constraints,
) {
	deployURL := *url
	if deployURL.Series == "" {
		deployURL.Series = "bionic"
		if deployURL.Revision < 0 {
			deployURL.Revision = 1
		}
	}
	fallbackCons := constraints.MustParse("arch=amd64")
	platform, _ := apputils.DeducePlatform(constraints.Value{}, series, fallbackCons)
	origin, _ := apputils.DeduceOrigin(url, charm.Channel{}, platform)
	fakeAPI.Call("AddCharm", &deployURL, origin, force).Returns(origin, error(nil))
	fakeAPI.Call("CharmInfo", deployURL.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:     url.String(),
			Meta:    meta,
			Metrics: metrics,
		},
		error(nil),
	)
	fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    &deployURL,
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: appName,
		Series:          series,
		NumUnits:        numUnits,
		AttachStorage:   attachStorage,
		Config:          config,
		Storage:         storage,
		Devices:         devices,
	}).Returns(error(nil))
	fakeAPI.Call("IsMetered", deployURL.String()).Returns(metered, error(nil))

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	fakeAPI.Call("SetMetricCredentials", deployURL.Name, creds).Returns(error(nil))
}

func withCharmRepoResolvable(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	aseries string,
) {
	// We have to handle all possible variations on the supplied URL.
	// The real store can be queried with a base URL like "cs:foo" and
	// resolve that to the real URL, it it may be queried with the fully
	// qualified URL, or one without series set etc.
	resultURL := *url
	if resultURL.Revision < 0 {
		resultURL.Revision = 1
	}
	if resultURL.Series == "" {
		resultURL.Series = "bionic"
	}
	resolveURLs := []*charm.URL{url}
	if url.Revision < 0 || url.Series == "" {
		inURL := *url
		if inURL.Revision < 0 {
			inURL.Revision = 1
		}
		if inURL.Series == "" {
			inURL.Series = "bionic"
		}
		resolveURLs = append(resolveURLs, &inURL)
	}
	base, _ := series.GetBaseFromSeries(aseries)
	for _, url := range resolveURLs {
		for _, arch := range []string{"", arch.DefaultArchitecture} {
			origin := commoncharm.Origin{
				Source:       commoncharm.OriginCharmStore,
				Architecture: arch,
				Series:       aseries,
				Base:         base,
			}
			resolvedOrigin := origin
			fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
				&resultURL,
				resolvedOrigin,
				[]string{"bionic"}, // Supported series
				error(nil),
			)
		}
	}
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

type noopMacaroonGetter struct{}

func (*noopMacaroonGetter) Get(string, interface{}) error {
	return nil
}
