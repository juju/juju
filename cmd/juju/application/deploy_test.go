// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
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
	"github.com/juju/juju/api/client/client"
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
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	jjtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	apiparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var defaultBase = corebase.MustParseBaseFromString("ubuntu@22.04")

func resourceHash(content string) charmresource.Fingerprint {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	if err != nil {
		panic(err)
	}
	return fp
}

type DeploySuiteBase struct {
	jjtesting.RepoSuite
	coretesting.CmdBlockHelper
	DeployResources deployer.DeployResourcesFunc

	fakeAPI *fakeDeployAPI
}

// deployCommand returns a deploy command that stubs out the
// charm repository and the controller deploy API.
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
// charm repository but writes data to the juju database.
func (s *DeploySuiteBase) deployCommandForState() *DeployCommand {
	deploy := newDeployCommand()
	deploy.DeployResources = s.DeployResources
	deploy.NewResolver = func(charmsAPI store.CharmsAPI, downloadFn store.DownloadBundleClientFunc) deployer.Resolver {
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
	deploy.DeployResources = s.DeployResources
	deploy.NewResolver = func(charmsAPI store.CharmsAPI, downloadFn store.DownloadBundleClientFunc) deployer.Resolver {
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

	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
	s.DeployResources = func(applicationID string,
		chID resources.CharmID,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		conn base.APICallCloser,
		filesystem modelcmd.Filesystem,
	) (ids map[string]string, err error) {
		return deployResources(s.State, applicationID, resources)
	}
	cfgAttrs := map[string]interface{}{
		"name":           "name",
		"uuid":           coretesting.ModelTag.Id(),
		"type":           "foo",
		"secret-backend": "auto",
	}
	s.fakeAPI = vanillaFakeModelAPI(cfgAttrs)
	s.fakeAPI.deployerFactoryFunc = deployer.NewDeployerFactory
}

// deployResources does what would be expected when a charm with
// resources is deployed (ie write the pending and actual resources
// to state), but it does not upload (fake data from the store is used).
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
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "some-application-name", defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	// Block operation
	s.BlockAllChanges(c, "TestBlockDeploy")
	err := s.runDeployForState(c, charmDir.Path, "some-application-name", "--base", "ubuntu@22.04")
	s.AssertBlocked(c, err, ".*TestBlockDeploy.*")
}

func (s *DeploySuite) TestInvalidPath(c *gc.C) {
	err := s.runDeploy(c, "/home/nowhere")
	c.Assert(err, gc.ErrorMatches, `no charm was found at "/home/nowhere"`)
}

func (s *DeploySuite) TestInvalidFileFormat(c *gc.C) {
	path := filepath.Join(c.MkDir(), "bundle.yaml")
	err := os.WriteFile(path, []byte(":"), 0600)
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
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "some-application-name", defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl.String(), 1, 0)
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
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@20.04", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "dummy", curl.String(), 1, 0)
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeries(c *gc.C) {
	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "dummy-fail-no-os")
	err := s.runDeploy(c, path, "--base", "ubuntu@20.04")
	c.Assert(err, gc.ErrorMatches, "charm does not define any bases, not valid")
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeriesNoBase(c *gc.C) {
	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "dummy-fail-no-os")
	err := s.runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, "charm does not define any bases, not valid")
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeriesUseDefaultSeries(c *gc.C) {
	updateAttrs := map[string]interface{}{"default-base": version.DefaultSupportedLTSBase().String()}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err = s.runDeployForState(c, charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "dummy", curl.String(), 1, 0)
}

func (s *DeploySuite) TestDeployFromPathDefaultSeries(c *gc.C) {
	// multi-series/metadata.yaml provides "focal" as its default base
	// and yet, here, the model defaults to the base "ubuntu@22.04". This test
	// asserts that the model's default takes precedence.
	updateAttrs := map[string]interface{}{"default-base": "ubuntu@22.04"}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err = s.runDeployForState(c, charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl.String(), 1, 0)
}

func (s *DeploySuite) TestDeployFromPath(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl.String(), 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesHaveOverlap(c *gc.C) {
	// Do not remove this because we want to test: bases supported by the charm and bases supported by Juju have overlap.
	s.PatchValue(&deployer.SupportedJujuBases, func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
		return transform.SliceOrErr([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@12.10"}, corebase.ParseBaseFromString)
	})

	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "multi-series")
	err := s.runDeploy(c, path, "--base", "ubuntu@12.10")
	c.Assert(err, gc.ErrorMatches, `base "ubuntu@12.10/stable" is not supported, supported bases are: .*`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedBaseHaveNoOverlap(c *gc.C) {
	// Do not remove this because we want to test: bases supported by the charm and bases supported by Juju have NO overlap.
	s.PatchValue(&deployer.SupportedJujuBases,
		func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
			return []corebase.Base{corebase.MustParseBaseFromString("ubuntu@22.10")}, nil
		},
	)

	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "multi-series")
	err := s.runDeploy(c, path)
	c.Assert(err, gc.ErrorMatches, `the charm defined bases ".*" not supported`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesForce(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	// TODO remove this patch once we removed all the old bases from tests in current package.
	s.PatchValue(&deployer.SupportedJujuBases, func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
		return transform.SliceOrErr([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@18.04", "ubuntu@12.10"}, corebase.ParseBaseFromString)
	})

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@12.10", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl.String(), 1, 0)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedLXDProfileForce(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("quantal").ClonedDir(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL("local:lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, true, 1, nil, nil)

	// TODO remove this patch once we removed all the old bases from tests in current package.
	s.PatchValue(&deployer.SupportedJujuBases, func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
		return transform.SliceOrErr([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@18.04", "ubuntu@12.10"}, corebase.ParseBaseFromString)
	})

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@12.10", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "lxd-profile-fail", curl.String(), 1, 0)
}

func (s *DeploySuite) TestUpgradeCharmDir(c *gc.C) {
	// Add the charm, so the url will exist and a new revision will be
	// picked in application Deploy.
	repo := testcharms.RepoForSeries("bionic")
	ch := repo.CharmDir("dummy")
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL(fmt.Sprintf("local:%s", ident))
	dummyCharm, err := jjtesting.PutCharm(s.State, curl, ch)
	c.Assert(err, jc.ErrorIsNil)

	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	deployURL := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, deployURL, charmDir, false)
	withCharmDeployable(s.fakeAPI, deployURL, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err = s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	upgradedRev := dummyCharm.Revision() + 1
	curl = charm.MustParseURL(dummyCharm.URL()).WithRevision(upgradedRev)
	s.AssertApplication(c, "dummy", curl.String(), 1, 0)
	// Check the charm dir was left untouched.
	ch, err = charm.ReadCharmDir(charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
}

func (s *DeploySuite) TestCharmBundle(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, charmURL, "some-application-name", defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "some-application-name", "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:multi-series-1")
	s.AssertApplication(c, "some-application-name", curl.String(), 1, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "logging", curl.String(), 0, 0)
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
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "dummy-application", corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	path := setupConfigFile(c, c.MkDir())
	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", path, "--base", "ubuntu@20.04")
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
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	// Putting a config file in home is okay as $HOME is set to a tempdir
	setupConfigFile(c, utils.Home())
	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", "~/testconfig.yaml")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigValues(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "dummy-name", defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	confPath := filepath.Join(c.MkDir(), "include.txt")
	c.Assert(os.WriteFile(confPath, []byte("lorem\nipsum"), os.ModePerm), jc.ErrorIsNil)

	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", "skill-level=9000", "--config", "outlook=good", "--config", "title=@"+confPath, "--base", "ubuntu@22.04")
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
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	path := setupConfigFile(c, c.MkDir())
	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", path, "--config", "outlook=good", "--config", "skill-level=8000", "--base", "ubuntu@22.04")
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
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "dummy-application", "--config", "one", "--config", "another", "--base", "ubuntu@22.04")
	c.Assert(err, gc.ErrorMatches, "only a single config YAML file can be specified, got 2")
}

func (s *DeploySuite) TestConfigError(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, charmURL, "some-application-name", defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	path := setupConfigFile(c, c.MkDir())
	err := s.runDeployForState(c, charmDir.Path, "other-application", "--config", path)
	c.Assert(err, gc.ErrorMatches, `no settings found for "other-application"`)
	_, err = s.State.Application("other-application")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeploySuite) TestConstraints(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withCharmDeployable(s.fakeAPI, charmURL, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--constraints", "mem=2G", "--constraints", "cores=2", "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:multi-series-1")
	app, _ := s.AssertApplication(c, "multi-series", curl.String(), 1, 0)
	cons, err := app.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cores=2 arch=amd64"))
}

func (s *DeploySuite) TestResources(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	foopath := "/test/path/foo"
	barpath := "/test/path/var"

	res1 := fmt.Sprintf("foo=%s", foopath)
	res2 := fmt.Sprintf("bar=%s", barpath)

	d := DeployCommand{}
	args := []string{charmDir.Path, "--resource", res1, "--resource", res2, "--base", "ubuntu@22.04"}

	err := cmdtesting.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *DeploySuite) TestLXDProfileLocalCharm(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL("local:lxd-profile-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "lxd-profile", curl.String(), 1, 0)
}

func (s *DeploySuite) TestLXDProfileLocalCharmFails(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL("local:lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `invalid lxd-profile.yaml: contains device type "unix-disk"`)
}

func (s *DeploySuite) TestStorage(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "storage-block")
	curl := charm.MustParseURL("local:storage-block-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployableWithStorage(
		s.fakeAPI, curl, "storage-block", defaultBase,
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

	err := s.runDeployForState(c, charmDir.Path, "--storage", "data=machinescoped,1G", "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	app, _ := s.AssertApplication(c, "storage-block", curl.String(), 1, 0)

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
	ubURL := charm.MustParseURL("ch:ubuntu")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "")

	withCharmDeployable(
		s.fakeAPI, ubURL, defaultBase,
		&charm.Meta{Name: "ubuntu"},
		nil, false, false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu",
		NumUnits:        1,
	}).Returns([]string{"ubuntu/0"}, error(nil))

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	deploy := s.deployCommand()
	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "basic")
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), bundlePath, "--channel", "edge")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployBundlesRequiringTrust(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("ch:aws-integrator")
	withCharmRepoResolvable(s.fakeAPI, inURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	// The aws-integrator charm requires trust and since the operator passes
	// --trust we expect to see a "trust: true" config value in the yaml
	// config passed to deploy.
	//
	// As withCharmDeployable does not support passing a "ConfigYAML"
	// it's easier to just invoke it to set up all other calls and then
	// explicitly Deploy here.
	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "aws-integrator", Series: []string{"jammy"}},
		nil, false, false, 0, nil, nil,
	)

	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: arch.DefaultArchitecture,
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	deployURL := *inURL

	dArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    deployURL.String(),
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: inURL.Name,
		ConfigYAML:      "aws-integrator:\n  trust: \"true\"\n",
	}

	s.fakeAPI.Call("Deploy", dArgs).Returns(error(nil))
	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "aws-integrator",
		NumUnits:        1,
	}).Returns([]string{"aws-integrator/0"}, error(nil))

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	// The second charm from the bundle does not require trust so no
	// additional configuration should be injected
	ubURL := charm.MustParseURL("ch:ubuntu")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "")
	withCharmDeployable(
		s.fakeAPI, ubURL, defaultBase,
		&charm.Meta{Name: "ubuntu", Series: []string{"jammy"}},
		nil, false, false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu",
		NumUnits:        1,
	}).Returns([]string{"ubuntu/0"}, error(nil))

	deploy := s.deployCommand()
	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "aws-integrator-trust-single")
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), bundlePath, "--trust")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployBundleWithOffers(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("ch:apache2")
	withCharmRepoResolvable(s.fakeAPI, inURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "apache2", Series: []string{"jammy"}},
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
	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "apache2-with-offers-legacy")
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

	inURL := charm.MustParseURL("ch:wordpress")
	withCharmRepoResolvable(s.fakeAPI, inURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "wordpress", Series: []string{"jammy"}},
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
		Offer: &params.ApplicationOfferDetailsV5{
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
			Offer: params.ApplicationOfferDetailsV5{
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
	coretesting.JujuOSEnvSuite
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
	s.deployer.EXPECT().PrepareAndDeploy(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

func (s *CAASDeploySuiteBase) SetUpTest(c *gc.C) {
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
		"secret-backend":   "auto",
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
		NewResolver: func(charmsAPI store.CharmsAPI, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
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
		fakeAPI, curl, corebase.LegacyKubernetesBase(),
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
	cfg.Base = corebase.LegacyKubernetesBase()
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	curl := charm.MustParseURL("local:kubernetes/bitcoin-miner-1")
	withLocalCharmDeployable(fakeAPI, curl, charmDir, false)
	withCharmDeployableWithDevices(
		fakeAPI, curl, curl.Name, corebase.LegacyKubernetesBase(),
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
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		conn base.APICallCloser,
		filesystem modelcmd.Filesystem,
	) (ids map[string]string, err error) {
		fakeAPI.AddCall("DeployResources", applicationID, chID, filesAndRevisions, resources, conn)
		return nil, fakeAPI.NextErr()
	}

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "-m", "caas-model", "--device", "bitcoinminer=10,nvidia.com/gpu", "--series", "kubernetes")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CAASDeploySuite) TestDeployAttachStorage(c *gc.C) {
	s.SetFeatureFlags(feature.K8SAttachStorage)
	defer func() {
		// Unset feature flag
		os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
		featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	}()

	repo := testcharms.RepoWithSeries("kubernetes")
	charmDir := repo.ClonedDir(s.CharmsPath, "gitlab")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	cfg.AttachStorage = []string{"foo/0", "bar/0"}
	cfg.Base = corebase.LegacyKubernetesBase()
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	curl := charm.MustParseURL("local:kubernetes/gitlab-0")
	withLocalCharmDeployable(fakeAPI, curl, charmDir, false)
	withCharmDeployable(
		fakeAPI, curl, corebase.LegacyKubernetesBase(),
		charmDir.Meta(),
		charmDir.Metrics(),
		true, false, 1, nil, nil,
	)
	_, err := s.runDeploy(
		c, fakeAPI, charmDir.Path,
		"-m", "caas-model", "--series", "kubernetes", "--attach-storage", "foo/0,bar/0",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployStorageFailContainer(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@22.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container := "lxd:" + machine.Id()
	err = s.runDeploy(c, charmDir.Path, "--to", container, "--storage", "data=machinescoped,1G")
	c.Assert(err, gc.ErrorMatches, `adding storage of type "machinescoped" to lxd container not supported`)
}

func (s *DeploySuite) TestPlacement(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)
	// Add a machine that will be ignored due to placement directive.
	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = s.runDeployForState(c, charmDir.Path, "-n", "1", "--to", "valid", "--base", "ubuntu@20.04")
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
	curl := charm.MustParseURL("local:logging")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--constraints", "mem=1G", "--base", "ubuntu@22.04")
	c.Assert(err, gc.ErrorMatches, "cannot use --constraints with subordinate application")
}

func (s *DeploySuite) TestNumUnits(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "-n", "13", "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "multi-series", curl.String(), 13, 0)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:logging")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, "--num-units", "3", charmDir.Path, "--base", "ubuntu@22.04")
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
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	machine, err := s.State.AddMachine(state.UbuntuBase("24.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeployForState(c, "--to", machine.Id(), charmDir.Path, "portlandia", "--base", version.DefaultSupportedLTSBase().String())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:jammy/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	template := state.MachineTemplate{
		Base: state.DefaultLTSBase(),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeployForState(c, "--to", container.Id(), charmDir.Path, "portlandia", "--base", version.DefaultSupportedLTSBase().String())
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, container.Id())
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:jammy/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	ltsseries := version.DefaultSupportedLTSBase()
	withCharmDeployable(s.fakeAPI, curl, ltsseries, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeployForState(c, "--to", "lxd:"+machine.Id(), charmDir.Path, "portlandia", "--base", "ubuntu@22.04")
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
	curl := charm.MustParseURL("local:jammy/multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, "--to", "42", charmDir.Path, "portlandia", "--base", "ubuntu@20.04")
	c.Assert(err, gc.ErrorMatches, `cannot deploy "portlandia" to machine 42: machine 42 not found`)
	_, err = s.State.Application("portlandia")
	c.Assert(err, gc.ErrorMatches, `application "portlandia" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *gc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err = s.runDeployForState(c, "--to", machine.Id(), charmDir.Path, "--base", "ubuntu@22.04")

	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate application")
	_, err = s.State.Application("dummy")
	c.Assert(err, gc.ErrorMatches, `application "dummy" not found`)
}

func (s *DeploySuite) TestNonLocalCannotHostUnits(c *gc.C) {
	s.fakeAPI.Call("CharmInfo", "local:dummy").Returns(
		&apicommoncharms.CharmInfo{
			URL:  "local:dummy",
			Meta: &charm.Meta{Name: "dummy", Series: []string{"jammy"}},
		},
		error(nil),
	)
	err := s.runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, gc.Not(gc.ErrorMatches), "machine 0 is the controller for a local model and cannot host units")
}

func (s *DeploySuite) TestDeployLocalWithTerms(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "terms1")
	curl := charm.MustParseURL("local:terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "terms1", curl.String(), 1, 0)
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
	curl := charm.MustParseURL("local:terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@12.10"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	_, _, err := s.runDeployWithOutput(c, charmDir.Path, "--base", "ubuntu@12.10")

	c.Check(err, gc.ErrorMatches, `terms1 is not available on the following base: ubuntu@12.10/stable`)
}

func (s *DeploySuite) TestDeployLocalWithSeriesAndForce(c *gc.C) {
	// TODO remove this patch once we removed all the old bases from tests in current package.
	s.PatchValue(&deployer.SupportedJujuBases, func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
		return transform.SliceOrErr([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@18.04", "ubuntu@12.10"}, corebase.ParseBaseFromString)
	})

	charmDir := testcharms.RepoWithSeries("quantal").ClonedDir(c.MkDir(), "terms1")
	curl := charm.MustParseURL("local:terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, true)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, true, 1, nil, nil)

	err := s.runDeployForState(c, charmDir.Path, "--base", "ubuntu@12.10", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertApplication(c, "terms1", curl.String(), 1, 0)
}

// TODO (stickupkid): Remove this test once we remove series in 3.2. This is only
// here to test legacy behaviour.
func (s *DeploySuite) setupNonESMBase(c *gc.C) (corebase.Base, string) {
	supported := set.NewStrings(corebase.SupportedJujuWorkloadSeries()...)
	// Allowing kubernetes as an option, can lead to an unrelated failure:
	// 		series "kubernetes" in a non container model not valid
	supported.Remove("kubernetes")
	supportedNotEMS := supported.Difference(set.NewStrings(corebase.ESMSupportedJujuSeries()...))
	c.Assert(supportedNotEMS.Size(), jc.GreaterThan, 0)

	// TODO remove this patch once we removed all the old bases from tests in current package.
	s.PatchValue(&deployer.SupportedJujuBases, func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
		return transform.SliceOrErr([]string{"centos@7", "centos@9", "ubuntu@22.04", "ubuntu@20.04", "ubuntu@16.04"}, corebase.ParseBaseFromString)
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

	curl := charm.MustParseURL("local:logging-1")
	ch, err := charm.ReadCharm(loggingPath)
	c.Assert(err, jc.ErrorIsNil)
	withLocalCharmDeployable(s.fakeAPI, curl, ch, false)

	nonEMSBase, err := corebase.GetBaseFromSeries(nonEMSSeries)
	c.Assert(err, jc.ErrorIsNil)
	withAliasedCharmDeployable(s.fakeAPI, curl, "logging", nonEMSBase, ch.Meta(), ch.Metrics(), false, false, 1, nil, nil)

	return nonEMSBase, loggingPath
}

// TODO (stickupkid): Remove this test once we remove series in 3.2
func (s *DeploySuite) TestDeployLocalWithSupportedNonESMSeries(c *gc.C) {
	nonEMSBase, loggingPath := s.setupNonESMBase(c)
	err := s.runDeploy(c, loggingPath, "--base", nonEMSBase.String())
	c.Logf("%+v", s.fakeAPI.Calls())
	c.Assert(err, jc.ErrorIsNil)
}

// TODO (stickupkid): Remove this test once we remove series in 3.2
func (s *DeploySuite) TestDeployLocalWithNotSupportedNonESMSeries(c *gc.C) {
	_, loggingPath := s.setupNonESMBase(c)
	err := s.runDeploy(c, loggingPath, "--base", "ubuntu@17.10")
	c.Assert(err, gc.ErrorMatches, "logging is not available on the following base: ubuntu@17.10/stable")
}

// setupConfigFile creates a configuration file for testing set
// with the --config argument specifying a configuration file.
func setupConfigFile(c *gc.C, dir string) string {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte("dummy-application:\n  skill-level: 9000\n  username: admin001\n\n")
	err := os.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

func (s *DeploySuite) TestDeployWithChannel(c *gc.C) {
	curl := charm.MustParseURL("ch:jammy/dummy")
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: arch.DefaultArchitecture,
		Risk:         "beta",
	}
	originWithSeries := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: arch.DefaultArchitecture,
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
		Risk:         "beta",
	}
	s.fakeAPI.Call("ResolveCharm", curl, origin, false).Returns(
		curl,
		origin,
		[]corebase.Base{corebase.MustParseBaseFromString("ubuntu@22.04")}, // Supported bases
		error(nil),
	)
	s.fakeAPI.Call("ResolveCharm", curl, originWithSeries, false).Returns(
		curl,
		originWithSeries,
		[]corebase.Base{corebase.MustParseBaseFromString("ubuntu@22.04")}, // Supported bases
		error(nil),
	)
	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID: application.CharmID{
			URL:    curl.String(),
			Origin: originWithSeries,
		},
		CharmOrigin:     originWithSeries,
		ApplicationName: curl.Name,
		NumUnits:        1,
	}).Returns(error(nil))
	s.fakeAPI.Call("AddCharm", curl, originWithSeries, false).Returns(originWithSeries, error(nil))
	withCharmDeployable(
		s.fakeAPI, curl, defaultBase,
		&charm.Meta{Name: "dummy", Series: []string{"jammy"}},
		nil, false, false, 0, nil, nil,
	)
	deploy := s.deployCommand()

	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "ch:jammy/dummy", "--channel", "beta")
	c.Assert(err, jc.ErrorIsNil)
}

type FakeStoreStateSuite struct {
	DeploySuiteBase
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
			var base corebase.Base
			if serie != "" {
				var err error
				base, err = corebase.GetBaseFromSeries(serie)
				c.Assert(err, jc.ErrorIsNil)
			}
			for _, a := range []string{"", arc, arch.DefaultArchitecture} {
				platform := corecharm.Platform{
					Architecture: a,
					OS:           base.OS,
					Channel:      base.Channel.Track,
				}
				origin, err := apputils.MakeOrigin(charm.Schema(url.Schema), url.Revision, charm.Channel{}, platform)
				c.Assert(err, jc.ErrorIsNil)

				abase, err := corebase.GetBaseFromSeries(aseries)
				c.Assert(err, jc.ErrorIsNil)
				s.fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
					resolveURL,
					origin,
					[]corebase.Base{abase},
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
		var base corebase.Base
		if serie != "" {
			var err error
			base, err = corebase.GetBaseFromSeries(serie)
			c.Assert(err, jc.ErrorIsNil)
		}
		origin, err := apputils.MakeOrigin(charm.Schema(bundleResolveURL.Schema), bundleResolveURL.Revision, charm.Channel{}, corecharm.Platform{
			OS: base.OS, Channel: base.Channel.Track})
		c.Assert(err, jc.ErrorIsNil)
		s.fakeAPI.Call("ResolveBundleURL", &baseURL, origin).Returns(
			bundleResolveURL,
			origin,
			error(nil),
		)
		s.fakeAPI.Call("GetBundle", bundleResolveURL).Returns(bundleDir, error(nil))
	}
}

// assertCharmsUploaded checks that the given charm ids have been uploaded.
func (s *FakeStoreStateSuite) assertCharmsUploaded(c *gc.C, ids ...string) {
	allCharms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	uploaded := make([]string, len(allCharms))
	for i, ch := range allCharms {
		uploaded[i] = ch.URL()
	}
	c.Assert(uploaded, jc.SameContents, ids)
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
	charm       string
	config      charm.Settings
	constraints constraints.Value
	scale       int
	exposed     bool
	storage     map[string]state.StorageConstraints
	devices     map[string]state.DeviceConstraints
}

func (s *DeploySuite) TestDeployCharmWithSomeEndpointBindingsSpecifiedSuccess(c *gc.C) {
	curl := charm.MustParseURL("ch:jammy/wordpress-extra-bindings")
	charmDir := testcharms.RepoWithSeries("bionic").CharmDir("wordpress-extra-bindings")
	withCharmRepoResolvable(s.fakeAPI, curl, "")
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: arch.DefaultArchitecture,
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmID := application.CharmID{
		URL:    curl.String(),
		Origin: origin,
	}

	s.fakeAPI.Call("Deploy", application.DeployArgs{
		CharmID:         charmID,
		CharmOrigin:     origin,
		ApplicationName: curl.Name,
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
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "ch:jammy/wordpress-extra-bindings", "--bind", "db=db db-client=db public admin-api=public")
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

	cookiesFile := filepath.Join(c.MkDir(), ".go-cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookiesFile)
}

func (s *DeployUnitTestSuite) cfgAttrs() map[string]interface{} {
	return map[string]interface{}{
		"name":           "name",
		"uuid":           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type":           "foo",
		"secret-backend": "auto",
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
	cfg := basicDeployerConfig("local:dummy-0")
	opt := bytes.NewBufferString("foo: bar")
	err := cfg.ConfigOptions.SetAttrsFromReader(opt)
	c.Assert(err, jc.ErrorIsNil)
	s.expectDeployer(c, cfg)

	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "dummy")
	fakeAPI := s.fakeAPI()

	dummyURL := charm.MustParseURL("local:dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI,
		dummyURL,
		defaultBase,
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
	cfg.Base = corebase.MustParseBaseFromString("ubuntu@22.04")
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()

	multiSeriesURL := charm.MustParseURL("local:multi-series-1")

	withLocalCharmDeployable(fakeAPI, multiSeriesURL, charmDir, false)
	withCharmDeployable(fakeAPI, multiSeriesURL, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "--base", "ubuntu@22.04")
	c.Check(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestAddMetricCredentialsDefaultForUnmeteredCharm(c *gc.C) {
	charmDir := s.makeCharmDir(c, "multi-series")
	multiSeriesURL := charm.MustParseURL("local:multi-series-1")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	cfg.Base = corebase.MustParseBaseFromString("ubuntu@22.04")
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	withLocalCharmDeployable(fakeAPI, multiSeriesURL, charmDir, false)
	withCharmDeployable(fakeAPI, multiSeriesURL, defaultBase, charmDir.Meta(), charmDir.Metrics(), true, false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "--base", "ubuntu@22.04")
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
	cfg := basicDeployerConfig("local:dummy-0")
	s.expectDeployer(c, cfg)
	charmDir := s.makeCharmDir(c, "dummy")
	fakeAPI := s.fakeAPI()
	dummyURL := charm.MustParseURL("local:dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(fakeAPI, dummyURL, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, dummyURL.String())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployAttachStorage(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "dummy")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:dummy-0")
	cfg.AttachStorage = []string{"foo/0", "bar/1", "baz/2"}
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()

	dummyURL := charm.MustParseURL("local:dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
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
	cfg := basicDeployerConfig("local:dummy-0")
	cfg.AttachStorage = []string{"foo/0"}
	cfg.PlacementSpec = "lxd"
	cfg.Placement = []*instance.Placement{
		{Scope: "lxd"},
	}
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	dummyURL := charm.MustParseURL("local:dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, defaultBase, charmDir.Meta(), charmDir.Metrics(), false, false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
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
	s.deployer.EXPECT().PrepareAndDeploy(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
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
			filesAndRevisions map[string]string,
			resources map[string]charmresource.Meta,
			conn base.APICallCloser,
			filesystem modelcmd.Filesystem,
		) (ids map[string]string, err error) {
			return nil, nil
		},
	}
	if fakeAPI == nil {
		deployCmd.NewDeployAPI = func() (deployer.DeployerAPI, error) {
			apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			localCharmClient, err := apicharms.NewLocalCharmClient(apiRoot)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return &deployAPIAdapter{
				Connection:        apiRoot,
				legacyClient:      apiclient.NewClient(apiRoot, coretesting.NoopLogger{}),
				charmsClient:      apicharms.NewClient(apiRoot),
				localCharmsClient: localCharmClient,
				applicationClient: application.NewClient(apiRoot),
				modelConfigClient: modelconfig.NewClient(apiRoot),
				annotationsClient: annotations.NewClient(apiRoot),
			}, nil
		}
		deployCmd.NewResolver = func(charmsAPI store.CharmsAPI, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
			return store.NewCharmAdaptor(charmsAPI, downloadClientFn)
		}
		deployCmd.NewDeployerFactory = deployer.NewDeployerFactory
	} else {
		deployCmd.NewDeployerFactory = fakeAPI.deployerFactoryFunc
		deployCmd.NewResolver = func(charmsAPI store.CharmsAPI, downloadClientFn store.DownloadBundleClientFunc) deployer.Resolver {
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
	*jujutesting.CallMocker
	deployerFactoryFunc func(dep deployer.DeployerDependencies) deployer.DeployerFactory
	modelCons           constraints.Value
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
	[]corebase.Base,
	error,
) {
	results := f.MethodCall(f, "ResolveCharm", url, preferredChannel, switchCharm)
	if results == nil {
		if url.Schema == "ch" {
			return nil, commoncharm.Origin{}, nil, errors.Errorf(
				"cannot resolve charm or bundle %q: charm or bundle not found", url.Name)
		}
		return nil, commoncharm.Origin{}, nil, errors.Errorf(
			"unknown schema for charm URL %q", url)
	}
	return results[0].(*charm.URL),
		results[1].(commoncharm.Origin),
		results[2].([]corebase.Base),
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
	return results[0].(*charm.URL),
		results[1].(commoncharm.Origin),
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

func (f *fakeDeployAPI) Status(args *apiclient.StatusArgs) (*params.FullStatus, error) {
	results := f.MethodCall(f, "Status", args)
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

func (f *fakeDeployAPI) SetConstraints(application string, constraints constraints.Value) error {
	results := f.MethodCall(f, "SetConstraints", application, constraints)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) AddMachines(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	results := f.MethodCall(f, "AddMachines", machineParams)
	return results[0].([]params.AddMachinesResult), jujutesting.TypeAssertError(results[0])
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
	fakeAPI.Call("BestFacadeVersion", "Application").Returns(13)
	fakeAPI.Call("BestFacadeVersion", "Charms").Returns(4)

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
	base corebase.Base,
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
		base,
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
	base corebase.Base,
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
		base,
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
	base corebase.Base,
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
		base,
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
	base corebase.Base,
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
		base,
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
	base corebase.Base,
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
	fallbackCons := constraints.MustParse("arch=amd64")
	platform := apputils.MakePlatform(constraints.Value{}, base, fallbackCons)
	origin, _ := apputils.MakeOrigin(charm.Schema(url.Schema), url.Revision, charm.Channel{}, platform)
	fakeAPI.Call("AddCharm", &deployURL, origin, force).Returns(origin, error(nil))
	fakeAPI.Call("CharmInfo", deployURL.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:     url.String(),
			Meta:    meta,
			Metrics: metrics,
		},
		error(nil),
	)
	deployArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    deployURL.String(),
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: appName,
		NumUnits:        numUnits,
		AttachStorage:   attachStorage,
		Config:          config,
		Storage:         storage,
		Devices:         devices,
	}

	fakeAPI.Call("Deploy", deployArgs).Returns(error(nil))

	stableArgs := deployArgs
	stableOrigin := stableArgs.CharmOrigin
	stableOrigin.Risk = "stable"
	fakeAPI.Call("AddCharm", &deployURL, stableOrigin, force).Returns(origin, error(nil))
	fakeAPI.Call("Deploy", stableArgs).Returns(error(nil))
}

func withCharmRepoResolvable(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	aseries string,
) {
	base, _ := corebase.GetBaseFromSeries(aseries)

	for _, risk := range []string{"", "stable"} {
		origin := commoncharm.Origin{
			Source:       commoncharm.OriginCharmHub,
			Architecture: arch.DefaultArchitecture,
			Base:         base,
			Risk:         risk,
		}
		logger.Criticalf("mocking ResolveCharm -- url : %v -- base : %v -- switch : %v", url, origin, false)
		fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
			url,
			origin,
			[]corebase.Base{corebase.MustParseBaseFromString("ubuntu@22.04")}, // Supported bases
			error(nil),
		)
	}

}

func withLocalBundleCharmDeployable(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	base corebase.Base,
	meta *charm.Meta,
	manifest *charm.Manifest,
	force bool,
) {
	fakeAPI.Call("CharmInfo", url.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:      url.String(),
			Meta:     meta,
			Manifest: manifest,
		},
		error(nil),
	)
	fakeAPI.Call("ListSpaces").Returns([]apiparams.Space{}, error(nil))
	deployArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    url.String(),
			Origin: commoncharm.Origin{Source: "local"},
		},
		CharmOrigin:     commoncharm.Origin{Source: "local", Base: base},
		ApplicationName: url.Name,
		NumUnits:        0,
		Force:           force,
	}
	fakeAPI.Call("Deploy", deployArgs).Returns(error(nil))
	fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: url.Name,
		NumUnits:        1,
	}).Returns([]string{url.Name + "/0"}, error(nil))
}

func withAllWatcher(fakeAPI *fakeDeployAPI) {
	id := "0"
	fakeAPI.Call("WatchAll").Returns(api.NewAllWatcher(fakeAPI, &id), error(nil))

	fakeAPI.Call("BestFacadeVersion", "AllWatcher").Returns(0)
	fakeAPI.Call("APICall", "AllWatcher", 0, "0", "Stop", nil, nil).Returns(error(nil))
	fakeAPI.Call("Status", (*client.StatusArgs)(nil)).Returns(&params.FullStatus{}, error(nil))
}
