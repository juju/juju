// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	apicharms "github.com/juju/juju/api/client/charms"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/cmd/juju/application/store"
	apputils "github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
)

var defaultBase = corebase.MustParseBaseFromString("ubuntu@22.04")

type DeploySuiteBase struct {
	jujutesting.FakeHomeSuite

	fakeAPI *fakeDeployAPI
}

func (s *DeploySuiteBase) runDeploy(c *tc.C, args ...string) error {
	deployCmd := newDeployCommandForTest(s.fakeAPI)
	_, err := cmdtesting.RunCommand(c, deployCmd, args...)
	return err
}

func minimalModelConfig() map[string]interface{} {
	return map[string]interface{}{
		"name":           "name",
		"uuid":           coretesting.ModelTag.Id(),
		"type":           "foo",
		"secret-backend": "auto",
	}
}

func (s *DeploySuiteBase) SetUpTest(c *tc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.fakeAPI = vanillaFakeModelAPI(minimalModelConfig())
	s.fakeAPI.deployerFactoryFunc = deployer.NewDeployerFactory
}

type DeploySuite struct {
	DeploySuiteBase
}

var _ = tc.Suite(&DeploySuite{})

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

func (s *DeploySuite) TestInitErrors(c *tc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		deployCmd := newDeployCommandForTest(s.fakeAPI)
		err := cmdtesting.InitCommand(deployCmd, t.args)
		c.Check(err, tc.ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestBlockDeploy(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, curl, "some-application-name", defaultBase, charmDir.Meta(), false, 1, nil, nil)

	s.fakeAPI.SetErrors(apiservererrors.OperationBlockedError("deploy"))

	err := s.runDeploy(c, charmDir.Path, "some-application-name", "--base", "ubuntu@22.04")
	c.Assert(err, tc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), tc.IsTrue)
}

func (s *DeploySuite) TestInvalidPath(c *tc.C) {
	err := s.runDeploy(c, "/home/nowhere")
	c.Assert(err, tc.ErrorMatches, `no charm was found at "/home/nowhere"`)
}

func (s *DeploySuite) TestInvalidFileFormat(c *tc.C) {
	path := filepath.Join(c.MkDir(), "bundle.yaml")
	err := os.WriteFile(path, []byte(":"), 0600)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runDeploy(c, path)
	c.Assert(err, tc.ErrorMatches, `cannot deploy bundle: cannot unmarshal bundle contents:.* yaml:.*`)
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeries(c *tc.C) {
	path := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "dummy-no-series")
	err := s.runDeploy(c, path)
	c.Assert(err, tc.ErrorMatches, ".*charm metadata without bases in manifest not valid")
}

func (s *DeploySuite) TestDeployFromPathRelativeDir(c *tc.C) {
	dir := c.MkDir()
	path := testcharms.RepoWithSeries("bionic").CharmArchivePath(dir, "multi-series")

	wd, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = os.Chdir(wd) }()
	err = os.Chdir(dir)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runDeploy(c, filepath.Base(path))
	c.Assert(err, tc.ErrorMatches, ""+
		"The charm or bundle .* is ambiguous.\n"+
		"To deploy a local charm or bundle, run `juju deploy .*`.\n"+
		"To deploy a charm or bundle from CharmHub, run `juju deploy ch:.*`.")
}

func (s *DeploySuite) TestDeployFromPathOldCharm(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, true)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), true, 1, nil, nil)
	err := s.runDeploy(c, charmDir.Path, "--base", "ubuntu@20.04", "--force")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployFromPathOldCharmMissingSeriesUseDefaultBase(c *tc.C) {
	cfg := minimalModelConfig()
	cfg["default-base"] = version.DefaultSupportedLTSBase().String()
	s.fakeAPI.Call("ModelGet").Returns(cfg, error(nil))
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@24.04"), charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployFromPathDefaultBase(c *tc.C) {
	cfg := minimalModelConfig()
	cfg["default-base"] = "ubuntu@22.04"
	s.fakeAPI.Call("ModelGet").Returns(cfg, error(nil))
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesForce(c *tc.C) {
	// Do not remove this because we want to test: bases supported by the charm and bases supported by Juju have overlap.
	s.PatchValue(&deployer.SupportedJujuBases, func() []corebase.Base {
		return transform.Slice([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@12.10"}, corebase.MustParseBaseFromString)
	})
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, true)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@12.10"), charmDir.Meta(), true, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "--base", "ubuntu@12.10", "--force")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedSeriesHaveOverlap(c *tc.C) {
	// Do not remove this because we want to test: bases supported by the charm and bases supported by Juju have overlap.
	s.PatchValue(&deployer.SupportedJujuBases, func() []corebase.Base {
		return transform.Slice([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@12.10"}, corebase.MustParseBaseFromString)
	})

	path := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "multi-series")
	err := s.runDeploy(c, path, "--base", "ubuntu@12.10")
	c.Assert(err, tc.ErrorMatches, `base "ubuntu@12.10/stable" is not supported, supported bases are: .*`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedBaseHaveNoOverlap(c *tc.C) {
	// Do not remove this because we want to test: bases supported by the charm and bases supported by Juju have NO overlap.
	s.PatchValue(&deployer.SupportedJujuBases,
		func() []corebase.Base {
			return []corebase.Base{corebase.MustParseBaseFromString("ubuntu@22.10")}
		},
	)

	path := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "multi-series")
	err := s.runDeploy(c, path)
	c.Assert(err, tc.ErrorMatches, `the charm defined bases ".*" not supported`)
}

func (s *DeploySuite) TestDeployFromPathUnsupportedLXDProfileForce(c *tc.C) {
	// TODO remove this patch once we removed all the old bases from tests in current package.
	s.PatchValue(&deployer.SupportedJujuBases, func() []corebase.Base {
		return transform.Slice([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@18.04", "ubuntu@12.10"}, corebase.MustParseBaseFromString)
	})

	charmDir := testcharms.RepoWithSeries("quantal").CharmArchive(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL("local:lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, true)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@12.10"), charmDir.Meta(), true, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "--base", "ubuntu@12.10", "--force")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestCharmDeployAlias(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withAliasedCharmDeployable(s.fakeAPI, charmURL, "some-application-name", defaultBase, charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "some-application-name", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestSubordinateCharm(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 0, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestSingleConfigFile(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)

	path, content := setupConfigFile(c, c.MkDir())
	withCharmDeployableWithYAMLConfig(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), false, 1, nil, content, nil)

	err := s.runDeploy(c, charmDir.Path, "--config", path, "--base", "ubuntu@20.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestRelativeConfigPath(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)

	// Putting a config file in home is okay as $HOME is set to a tempdir
	_, content := setupConfigFile(c, jujutesting.HomePath())
	withCharmDeployableWithYAMLConfig(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@24.04"), charmDir.Meta(), false, 1, nil, content, nil)

	err := s.runDeploy(c, charmDir.Path, "multi-series", "--config", "~/testconfig.yaml")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigValues(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)

	confPath := filepath.Join(c.MkDir(), "include.txt")
	c.Assert(os.WriteFile(confPath, []byte("lorem\nipsum"), os.ModePerm), tc.ErrorIsNil)

	cfg := map[string]string{
		"outlook":     "good",
		"skill-level": "9000",
		"title":       "lorem\nipsum",
	}
	withAliasedCharmDeployable(s.fakeAPI, curl, "dummy-application", defaultBase, charmDir.Meta(), false, 1, nil, cfg)

	err := s.runDeploy(c, charmDir.Path, "dummy-application", "--config", "skill-level=9000", "--config", "outlook=good", "--config", "title=@"+confPath, "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigValuesWithFile(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)

	cfg := map[string]string{
		"outlook":     "good",
		"skill-level": "8000",
	}
	path, content := setupConfigFile(c, c.MkDir())
	withCharmDeployableWithYAMLConfig(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, content, cfg)

	err := s.runDeploy(c, charmDir.Path, "--config", path, "--config", "outlook=good", "--config", "skill-level=8000", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestSingleConfigMoreThanOneFile(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "dummy-application", "--config", "one", "--config", "another", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorMatches, "only a single config YAML file can be specified, got 2")
}

func (s *DeploySuite) TestConstraints(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	charmURL := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, charmURL, charmDir, false)
	withCharmDeployableWithConstraints(s.fakeAPI, charmURL, defaultBase, charmDir.Meta(), false, 1, constraints.MustParse("mem=2G cores=2"))

	err := s.runDeploy(c, charmDir.Path, "--constraints", "mem=2G", "--constraints", "cores=2", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestResources(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	foopath := "/test/path/foo"
	barpath := "/test/path/var"

	res1 := fmt.Sprintf("foo=%s", foopath)
	res2 := fmt.Sprintf("bar=%s", barpath)

	d := DeployCommand{}
	args := []string{charmDir.Path, "--resource", res1, "--resource", res2, "--base", "ubuntu@22.04"}

	cmd := modelcmd.Wrap(&d)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	err := cmdtesting.InitCommand(cmd, args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(d.Resources, tc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *DeploySuite) TestLXDProfileLocalCharm(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL("local:lxd-profile-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@24.04"), charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestStorage(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "storage-block")
	curl := charm.MustParseURL("local:storage-block-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployableWithStorage(
		s.fakeAPI, curl, "storage-block", defaultBase,
		charmDir.Meta(),

		false, 1, nil, "", nil,
		map[string]storage.Directive{
			"data": {
				Pool:  "machinescoped",
				Size:  1024,
				Count: 1,
			},
		},
	)

	err := s.runDeploy(c, charmDir.Path, "--storage", "data=machinescoped,1G", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
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

func (s *CAASDeploySuiteBase) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.factory = mocks.NewMockDeployerFactory(ctrl)
	s.deployer = mocks.NewMockDeployer(ctrl)
	return ctrl
}

func (s *CAASDeploySuiteBase) expectDeployer(c *tc.C, cfg deployer.DeployerConfig) {
	match := deployerConfigMatcher{
		c:        c,
		expected: cfg,
	}
	s.factory.EXPECT().GetDeployer(gomock.Any(), match, gomock.Any(), gomock.Any()).Return(s.deployer, nil)
	s.deployer.EXPECT().PrepareAndDeploy(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

func (s *CAASDeploySuiteBase) SetUpTest(c *tc.C) {
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
		"workload-storage": "k8s-storage",
		"secret-backend":   "auto",
	}
	fakeAPI := vanillaFakeModelAPI(cfgAttrs)
	fakeAPI.deployerFactoryFunc = func(dep deployer.DeployerDependencies) deployer.DeployerFactory {
		return s.factory
	}
	return fakeAPI
}

func (s *CAASDeploySuiteBase) runDeploy(c *tc.C, fakeAPI *fakeDeployAPI, args ...string) (*cmd.Context, error) {
	deployCmd := &DeployCommand{
		NewDeployAPI: func(ctx context.Context) (deployer.DeployerAPI, error) {
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

var _ = tc.Suite(&CAASDeploySuite{})

func (s *CAASDeploySuite) TestInitErrorsCaasModel(c *tc.C) {
	for i, t := range caasTests {
		deployCmd := NewDeployCommand()
		deployCmd.SetClientStore(s.Store)
		c.Logf("Running %d with args %v", i, t.args)
		err := cmdtesting.InitCommand(deployCmd, t.args)
		c.Assert(err, tc.ErrorMatches, t.message)
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

func (s *CAASDeploySuite) TestCaasModelValidatedAtRun(c *tc.C) {
	for i, t := range caasTests {
		c.Logf("Running %d with args %v", i, t.args)
		s.Store = jujuclienttesting.MinimalStore()
		mycmd := NewDeployCommand()
		mycmd.SetClientStore(s.Store)
		err := cmdtesting.InitCommand(mycmd, t.args)
		c.Assert(err, tc.ErrorIsNil)

		m := s.Store.Models["arthur"].Models["king/sword"]
		m.ModelType = model.CAAS
		s.Store.Models["arthur"].Models["king/caas-model"] = m
		ctx := cmdtesting.Context(c)
		err = mycmd.Run(ctx)
		c.Assert(err, tc.ErrorMatches, t.message)
	}
}

func (s *CAASDeploySuite) TestLocalCharmNeedsResources(c *tc.C) {
	repo := testcharms.RepoWithSeries("focal")
	charmDir := repo.CharmArchive(s.CharmsPath, "mariadb-k8s")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	curl := charm.MustParseURL("local:mariadb-k8s-0")
	withLocalCharmDeployable(fakeAPI, curl, charmDir, false)
	withCharmDeployable(
		fakeAPI, curl, defaultBase,
		charmDir.Meta(),

		false, 1, nil, nil,
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CAASDeploySuite) TestDevices(c *tc.C) {
	repo := testcharms.RepoWithSeries("focal")
	charmDir := repo.CharmArchive(s.CharmsPath, "bitcoin-miner")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	cfg.Devices = map[string]devices.Constraints{
		"bitcoinminer": {
			Type:  "nvidia.com/gpu",
			Count: 10,
		},
	}
	cfg.Base = defaultBase
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()
	curl := charm.MustParseURL("local:bitcoin-miner-1")
	withLocalCharmDeployable(fakeAPI, curl, charmDir, false)
	withCharmDeployableWithDevices(
		fakeAPI, curl, curl.Name, cfg.Base,
		charmDir.Meta(),

		true, 1, nil, "", nil,
		map[string]devices.Constraints{
			"bitcoinminer": {Type: "nvidia.com/gpu", Count: 10},
		},
	)
	s.DeployResources = func(
		_ context.Context,
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

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "-m", "caas-model", "--device", "bitcoinminer=10,nvidia.com/gpu", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployStorageFailContainer(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@22.04"), charmDir.Meta(), false, 1, nil, nil)

	container := "lxd:0"
	err := s.runDeploy(c, charmDir.Path, "--to", container, "--storage", "data=machinescoped,1G")
	c.Assert(err, tc.ErrorMatches, `adding storage of type "machinescoped" to lxd container not supported`)
}

func (s *DeploySuite) TestPlacement(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	p := instance.MustParsePlacement("model-uuid:valid")
	withCharmDeployableWithPlacement(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@20.04"), charmDir.Meta(), false, 1, p)

	err := s.runDeploy(c, charmDir.Path, "-n", "1", "--to", "valid", "--base", "ubuntu@20.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestSubordinateConstraints(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "--constraints", "mem=1G", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorMatches, "cannot use --constraints with subordinate application")
}

func (s *DeploySuite) TestNumUnits(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "multi-series")
	curl := charm.MustParseURL("local:multi-series-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 13, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "-n", "13", "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, "--num-units", "3", charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorMatches, "cannot use --num-units or --to with subordinate application")
}

func (s *DeploySuite) TestForceMachine(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployableWithPlacement(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, instance.MustParsePlacement("1"))

	err := s.runDeploy(c, "--to", "1", charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "logging")
	curl := charm.MustParseURL("local:logging-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, "--to", "1", charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorMatches, "cannot use --num-units or --to with subordinate application")
}

func (s *DeploySuite) TestNonLocalCannotHostUnits(c *tc.C) {
	s.fakeAPI.Call("CharmInfo", "local:dummy").Returns(
		&apicommoncharms.CharmInfo{
			URL:  "local:dummy",
			Meta: &charm.Meta{Name: "dummy"},
		},
		error(nil),
	)
	err := s.runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, tc.Not(tc.ErrorMatches), "machine 0 is the controller for a local model and cannot host units")
}

func (s *DeploySuite) TestDeployLocalWithTerms(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "terms1")
	curl := charm.MustParseURL("local:terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "--base", "ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployFlags(c *tc.C) {
	// TODO: (2020-06-03)
	// Move to deployer package for testing, then BundleOnlyFlags and
	// CharmOnlyFlags can be private again.
	command := DeployCommand{}
	flagSet := gnuflag.NewFlagSetWithFlagKnownAs(command.Info().Name, gnuflag.ContinueOnError, "option")
	command.SetFlags(flagSet)
	c.Assert(command.flagSet, tc.DeepEquals, flagSet)
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
	c.Assert(declaredFlags, tc.DeepEquals, allFlags)
}

func (s *DeploySuite) TestDeployLocalWithSeriesMismatchReturnsError(c *tc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(c.MkDir(), "terms1")
	curl := charm.MustParseURL("local:terms1-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@12.10"), charmDir.Meta(), false, 1, nil, nil)

	err := s.runDeploy(c, charmDir.Path, "--base", "ubuntu@12.10")

	c.Check(err, tc.ErrorMatches, `terms1 is not available on the following base: ubuntu@12.10/stable not supported`)
}

// setupConfigFile creates a configuration file for testing set
// with the --config argument specifying a configuration file.
func setupConfigFile(c *tc.C, dir string) (string, string) {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte("dummy-application:\n  skill-level: 9000\n  username: admin001\n\n")
	err := os.WriteFile(path, content, 0666)
	c.Assert(err, tc.ErrorIsNil)
	return path, string(content)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *DeploySuite) TestDeployWithChannel(c *tc.C) {
	curl := charm.MustParseURL("ch:dummy")
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
	s.fakeAPI.Call("DeployFromRepository", application.DeployFromRepositoryArg{
		CharmName:  "dummy",
		Channel:    ptr("beta"),
		ConfigYAML: "dummy: {}\n",
		NumUnits:   ptr(1),
	}).Returns(application.DeployInfo{
		Architecture: "amd64",
		Base:         corebase.Base{OS: "ubuntu", Channel: corebase.Channel{Track: "22.04"}},
		Channel:      "beta",
		Name:         "dummy",
		Revision:     666,
	}, []application.PendingResourceUpload(nil), nil)
	s.fakeAPI.Call("AddCharm", curl, originWithSeries, false).Returns(originWithSeries, error(nil))
	withCharmDeployable(
		s.fakeAPI, curl, defaultBase,
		&charm.Meta{Name: "dummy"},
		false, 0, nil, nil,
	)

	err := s.runDeploy(c, "ch:dummy", "--channel", "beta")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeploySuite) TestDeployCharmWithSomeEndpointBindingsSpecifiedSuccess(c *tc.C) {
	curl := charm.MustParseURL("ch:wordpress-extra-bindings")
	charmDir := testcharms.RepoWithSeries("bionic").CharmDir("wordpress-extra-bindings")
	withCharmRepoResolvable(s.fakeAPI, curl, corebase.Base{})
	withCharmDeployable(s.fakeAPI, curl, defaultBase, charmDir.Meta(), false, 1, nil, nil)

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
	s.fakeAPI.Call("DeployFromRepository", application.DeployFromRepositoryArg{
		CharmName:  "wordpress-extra-bindings",
		ConfigYAML: "wordpress-extra-bindings: {}\n",
		NumUnits:   ptr(1),
		EndpointBindings: map[string]string{
			"db":        "db",
			"db-client": "db",
			"admin-api": "public",
			"":          "public",
		},
	}).Returns(application.DeployInfo{
		Architecture: "amd64",
		Base:         corebase.Base{OS: "ubuntu", Channel: corebase.Channel{Track: "22.04"}},
		Name:         "wordpress-extra-bindings",
		Revision:     666,
	}, []application.PendingResourceUpload(nil), nil)
	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{
		{
			Id:   "0",
			Name: "db",
		}, {
			Id:   "1",
			Name: "public",
		},
	}, error(nil))
	err := s.runDeploy(c, "ch:wordpress-extra-bindings", "--bind", "db=db db-client=db public admin-api=public")
	c.Assert(err, tc.ErrorIsNil)
}

type ParseMachineMapSuite struct{}

var _ = tc.Suite(&ParseMachineMapSuite{})

func (s *ParseMachineMapSuite) TestEmptyString(c *tc.C) {
	existing, mapping, err := parseMachineMap("")
	c.Check(err, tc.ErrorIsNil)
	c.Check(existing, tc.IsFalse)
	c.Check(mapping, tc.HasLen, 0)
}

func (s *ParseMachineMapSuite) TestExisting(c *tc.C) {
	existing, mapping, err := parseMachineMap("existing")
	c.Check(err, tc.ErrorIsNil)
	c.Check(existing, tc.IsTrue)
	c.Check(mapping, tc.HasLen, 0)
}

func (s *ParseMachineMapSuite) TestMapping(c *tc.C) {
	existing, mapping, err := parseMachineMap("1=2,3=4")
	c.Check(err, tc.ErrorIsNil)
	c.Check(existing, tc.IsFalse)
	c.Check(mapping, tc.DeepEquals, map[string]string{
		"1": "2", "3": "4",
	})
}

func (s *ParseMachineMapSuite) TestMappingWithExisting(c *tc.C) {
	existing, mapping, err := parseMachineMap("1=2,3=4,existing")
	c.Check(err, tc.ErrorIsNil)
	c.Check(existing, tc.IsTrue)
	c.Check(mapping, tc.DeepEquals, map[string]string{
		"1": "2", "3": "4",
	})
}

func (s *ParseMachineMapSuite) TestErrors(c *tc.C) {
	checkErr := func(value, expect string) {
		_, _, err := parseMachineMap(value)
		c.Check(err, tc.ErrorMatches, expect)
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

var _ = tc.Suite(&DeployUnitTestSuite{})

func (s *DeployUnitTestSuite) SetUpTest(c *tc.C) {
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

func (s *DeployUnitTestSuite) makeCharmDir(c *tc.C, cloneCharm string) *charm.CharmArchive {
	charmsPath := c.MkDir()
	return testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, cloneCharm)
}

func (s *DeployUnitTestSuite) runDeploy(c *tc.C, fakeAPI *fakeDeployAPI, args ...string) (*cmd.Context, error) {
	deployCmd := newDeployCommandForTest(fakeAPI)
	return cmdtesting.RunCommand(c, deployCmd, args...)
}

func (s *DeployUnitTestSuite) TestDeployApplicationConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:dummy-0")
	opt := bytes.NewBufferString("foo: bar")
	err := cfg.ConfigOptions.SetAttrsFromReader(opt)
	c.Assert(err, tc.ErrorIsNil)
	s.expectDeployer(c, cfg)

	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "dummy")

	fakeAPI := s.fakeAPI()
	dummyURL := charm.MustParseURL("local:dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI,
		dummyURL,
		defaultBase,
		charmDir.Meta(),

		false,
		1,
		nil,
		map[string]string{"foo": "bar"},
	)

	_, err = s.runDeploy(c, fakeAPI, dummyURL.String(),
		"--config", "foo=bar",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployLocalCharm(c *tc.C) {
	// Copy multi-series charm to path where we can deploy it from
	charmDir := s.makeCharmDir(c, "multi-series")
	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig(charmDir.Path)
	cfg.Base = defaultBase
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()

	multiSeriesURL := charm.MustParseURL("local:multi-series-1")

	withLocalCharmDeployable(fakeAPI, multiSeriesURL, charmDir, false)
	withCharmDeployable(fakeAPI, multiSeriesURL, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, charmDir.Path, "--base", "ubuntu@22.04")
	c.Check(err, tc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestRedeployLocalCharmSucceedsWhenDeployed(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:dummy-0")
	s.expectDeployer(c, cfg)
	charmDir := s.makeCharmDir(c, "dummy")
	fakeAPI := s.fakeAPI()
	dummyURL := charm.MustParseURL("local:dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(fakeAPI, dummyURL, defaultBase, charmDir.Meta(), false, 1, nil, nil)

	_, err := s.runDeploy(c, fakeAPI, dummyURL.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployAttachStorage(c *tc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "dummy")

	defer s.setupMocks(c).Finish()
	cfg := basicDeployerConfig("local:dummy-0")
	cfg.AttachStorage = []string{"foo/0", "bar/1", "baz/2"}
	s.expectDeployer(c, cfg)

	fakeAPI := s.fakeAPI()

	dummyURL := charm.MustParseURL("local:dummy-0")
	withLocalCharmDeployable(fakeAPI, dummyURL, charmDir, false)
	withCharmDeployable(
		fakeAPI, dummyURL, defaultBase, charmDir.Meta(), false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
	)

	deployCmd := newDeployCommandForTest(fakeAPI)
	_, err := cmdtesting.RunCommand(c, deployCmd, dummyURL.String(),
		"--attach-storage", "foo/0",
		"--attach-storage", "bar/1,baz/2",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeployUnitTestSuite) TestDeployAttachStorageContainer(c *tc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "dummy")

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
		fakeAPI, dummyURL, defaultBase, charmDir.Meta(), false, 1, []string{"foo/0", "bar/1", "baz/2"}, nil,
	)

	deployCmd := newDeployCommandForTest(fakeAPI)
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

func (s *DeployUnitTestSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.factory = mocks.NewMockDeployerFactory(ctrl)
	s.deployer = mocks.NewMockDeployer(ctrl)
	return ctrl
}

func (s *DeployUnitTestSuite) expectDeployer(c *tc.C, cfg deployer.DeployerConfig) {
	match := deployerConfigMatcher{
		c:        c,
		expected: cfg,
	}
	s.factory.EXPECT().GetDeployer(gomock.Any(), match, gomock.Any(), gomock.Any()).Return(s.deployer, nil)
	s.deployer.EXPECT().PrepareAndDeploy(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

type deployerConfigMatcher struct {
	c        *tc.C
	expected deployer.DeployerConfig
}

func (m deployerConfigMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(deployer.DeployerConfig)
	m.c.Assert(ok, tc.IsTrue)
	if !ok {
		return false
	}
	// FlagSet validation is not required for these tests.
	obtained.FlagSet = nil
	m.c.Assert(obtained, tc.DeepEquals, m.expected)
	return true
}

func (m deployerConfigMatcher) String() string {
	return "match deployer DeployerConfig"
}

// newDeployCommandForTest returns a command to deploy applications.
func newDeployCommandForTest(fakeAPI *fakeDeployAPI) modelcmd.ModelCommand {
	deployCmd := &DeployCommand{
		NewDeployAPI: func(ctx context.Context) (deployer.DeployerAPI, error) {
			return fakeAPI, nil
		},
		DeployResources: func(
			_ context.Context,
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
	cmd := modelcmd.Wrap(deployCmd)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	if fakeAPI == nil {
		return cmd
	}
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
	deployCmd.NewConsumeDetailsAPI = func(ctx context.Context, url *charm.OfferURL) (deployer.ConsumeDetails, error) {
		return fakeAPI, nil
	}
	return cmd
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

func (f *fakeDeployAPI) Sequences(ctx context.Context) (map[string]int, error) {
	return nil, nil
}

func (f *fakeDeployAPI) ModelGet(ctx context.Context) (map[string]interface{}, error) {
	results := f.MethodCall(f, "ModelGet")
	return results[0].(map[string]interface{}), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) ResolveCharm(ctx context.Context, url *charm.URL, preferredChannel commoncharm.Origin, switchCharm bool) (
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

func (f *fakeDeployAPI) ResolveBundleURL(ctx context.Context, url *charm.URL, preferredChannel commoncharm.Origin) (
	*charm.URL,
	commoncharm.Origin,
	error,
) {
	results := f.MethodCall(f, "ResolveBundleURL", url, preferredChannel)
	if results == nil {
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

func (f *fakeDeployAPI) APICall(ctx context.Context, objType string, version int, id, request string, params, response interface{}) error {
	results := f.MethodCall(f, "APICall", objType, version, id, request, params, response)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) Client(ctx context.Context) *apiclient.Client {
	results := f.MethodCall(f, "Client")
	return results[0].(*apiclient.Client)
}

func (f *fakeDeployAPI) ModelUUID() (string, bool) {
	results := f.MethodCall(f, "ModelUUID")
	return results[0].(string), results[1].(bool)
}

func (f *fakeDeployAPI) AddLocalCharm(ctx context.Context, url *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	results := f.MethodCall(f, "AddLocalCharm", url, ch, force)
	if results == nil {
		return nil, errors.NotFoundf("registered API call AddLocalCharm %v", url)
	}
	return results[0].(*charm.URL), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddCharm(ctx context.Context, url *charm.URL, origin commoncharm.Origin, force bool) (commoncharm.Origin, error) {
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

func (f *fakeDeployAPI) CharmInfo(ctx context.Context, url string) (*apicommoncharms.CharmInfo, error) {
	results := f.MethodCall(f, "CharmInfo", url)
	return results[0].(*apicommoncharms.CharmInfo), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Get(ctx context.Context, endpoint string, extra interface{}) error {
	return nil
}

func (f *fakeDeployAPI) Deploy(ctx context.Context, args application.DeployArgs) error {
	results := f.MethodCall(f, "Deploy", args)
	if len(results) != 1 {
		return errors.Errorf("expected 1 result, got %d: %v", len(results), results)
	}
	if err := f.NextErr(); err != nil {
		return err
	}
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) DeployFromRepository(ctx context.Context, arg application.DeployFromRepositoryArg) (application.DeployInfo, []application.PendingResourceUpload, []error) {
	results := f.MethodCall(f, "DeployFromRepository", arg)
	if err := f.NextErr(); err != nil {
		return application.DeployInfo{}, nil, []error{err}
	}
	return results[0].(application.DeployInfo), results[1].([]application.PendingResourceUpload), nil
}

func (f *fakeDeployAPI) ListSpaces(ctx context.Context) ([]params.Space, error) {
	results := f.MethodCall(f, "ListSpaces")
	return results[0].([]params.Space), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) GetAnnotations(context.Context, []string) ([]params.AnnotationsGetResult, error) {
	return nil, nil
}

func (f *fakeDeployAPI) GetConfig(context.Context, ...string) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeDeployAPI) GetConstraints(context.Context, ...string) ([]constraints.Value, error) {
	return nil, nil
}

func (f *fakeDeployAPI) GetModelConstraints(ctx context.Context) (constraints.Value, error) {
	f.MethodCall(f, "GetModelConstraints")
	return f.modelCons, nil
}

func (f *fakeDeployAPI) GetBundle(_ context.Context, url *charm.URL, _ commoncharm.Origin, _ string) (charm.Bundle, error) {
	results := f.MethodCall(f, "GetBundle", url)
	if results == nil {
		return nil, errors.NotFoundf("bundle %v", url)
	}
	return results[0].(charm.Bundle), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Status(ctx context.Context, args *apiclient.StatusArgs) (*params.FullStatus, error) {
	results := f.MethodCall(f, "Status", args)
	return results[0].(*params.FullStatus), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddRelation(ctx context.Context, endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	results := f.MethodCall(f, "AddRelation", stringToInterface(endpoints), stringToInterface(viaCIDRs))
	return results[0].(*params.AddRelationResults), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) AddUnits(ctx context.Context, args application.AddUnitsParams) ([]string, error) {
	results := f.MethodCall(f, "AddUnits", args)
	return results[0].([]string), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Expose(ctx context.Context, application string, exposedEndpoints map[string]params.ExposedEndpoint) error {
	results := f.MethodCall(f, "Expose", application, exposedEndpoints)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) SetAnnotation(ctx context.Context, annotations map[string]map[string]string) ([]params.ErrorResult, error) {
	results := f.MethodCall(f, "SetAnnotation", annotations)
	return results[0].([]params.ErrorResult), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) SetCharm(ctx context.Context, cfg application.SetCharmConfig) error {
	results := f.MethodCall(f, "SetCharm", cfg)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) SetConstraints(ctx context.Context, application string, constraints constraints.Value) error {
	results := f.MethodCall(f, "SetConstraints", application, constraints)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) AddMachines(ctx context.Context, machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	results := f.MethodCall(f, "AddMachines", machineParams)
	return results[0].([]params.AddMachinesResult), jujutesting.TypeAssertError(results[0])
}

func (f *fakeDeployAPI) ScaleApplication(ctx context.Context, p application.ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	return params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: p.Scale},
	}, nil
}

func (f *fakeDeployAPI) Offer(ctx context.Context, modelUUID, application string, endpoints []string, owner, offerName, descr string) ([]params.ErrorResult, error) {
	results := f.MethodCall(f, "Offer", modelUUID, application, endpoints, owner, offerName, descr)
	return results[0].([]params.ErrorResult), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) GetConsumeDetails(ctx context.Context, offerURL string) (params.ConsumeOfferDetails, error) {
	results := f.MethodCall(f, "GetConsumeDetails", offerURL)
	return results[0].(params.ConsumeOfferDetails), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) Consume(ctx context.Context, arg crossmodel.ConsumeApplicationArgs) (string, error) {
	results := f.MethodCall(f, "Consume", arg)
	return results[0].(string), jujutesting.TypeAssertError(results[1])
}

func (f *fakeDeployAPI) GrantOffer(ctx context.Context, user, access string, offerURLs ...string) error {
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
	fakeAPI.Call("BestFacadeVersion", "Charms").Returns(4)
	fakeAPI.Call("BestFacadeVersion", "Resources").Returns(3)

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
		force,
		numUnits,
		attachStorage,
		"",
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
	force bool,
	numUnits int,
	attachStorage []string,
	config map[string]string,
) {
	withCharmDeployableWithDevices(
		fakeAPI,
		url,
		appName,
		base,
		meta,
		force,
		numUnits,
		attachStorage,
		"",
		config,
		nil,
	)
}

func withCharmDeployableWithYAMLConfig(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	base corebase.Base,
	meta *charm.Meta,
	force bool,
	numUnits int,
	attachStorage []string,
	configYAML string,
	config map[string]string,
) {
	withCharmDeployableWithDevices(
		fakeAPI,
		url,
		url.Name,
		base,
		meta,
		force,
		numUnits,
		attachStorage,
		configYAML,
		config,
		nil,
	)
}

func withCharmDeployableWithDevices(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	appName string,
	base corebase.Base,
	meta *charm.Meta,
	force bool,
	numUnits int,
	attachStorage []string,
	configYAML string,
	config map[string]string,
	devices map[string]devices.Constraints,
) {
	withCharmDeployableWithDevicesAndStorage(
		fakeAPI,
		url,
		appName,
		base,
		meta,
		constraints.Value{},
		nil,
		force,
		numUnits,
		attachStorage,
		configYAML,
		config,
		nil,
		devices,
	)
}

func withCharmDeployableWithConstraints(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	base corebase.Base,
	meta *charm.Meta,
	force bool,
	numUnits int,
	cons constraints.Value,
) {
	withCharmDeployableWithDevicesAndStorage(
		fakeAPI,
		url,
		url.Name,
		base,
		meta,
		cons,
		nil,
		force,
		numUnits,
		nil,
		"",
		nil,
		nil,
		nil,
	)
}

func withCharmDeployableWithPlacement(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	base corebase.Base,
	meta *charm.Meta,
	force bool,
	numUnits int,
	p *instance.Placement,
) {
	withCharmDeployableWithDevicesAndStorage(
		fakeAPI,
		url,
		url.Name,
		base,
		meta,
		constraints.Value{},
		p,
		force,
		numUnits,
		nil,
		"",
		nil,
		nil,
		nil,
	)
}

func withCharmDeployableWithStorage(
	fakeAPI *fakeDeployAPI,
	url *charm.URL,
	appName string,
	base corebase.Base,
	meta *charm.Meta,
	force bool,
	numUnits int,
	attachStorage []string,
	configYAML string,
	config map[string]string,
	storage map[string]storage.Directive,
) {
	withCharmDeployableWithDevicesAndStorage(
		fakeAPI,
		url,
		appName,
		base,
		meta,
		constraints.Value{},
		nil,
		force,
		numUnits,
		attachStorage,
		configYAML,
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
	cons constraints.Value,
	p *instance.Placement,
	force bool,
	numUnits int,
	attachStorage []string,
	configYAML string,
	config map[string]string,
	storage map[string]storage.Directive,
	devices map[string]devices.Constraints,
) {
	deployURL := *url
	fallbackCons := constraints.MustParse("arch=amd64")
	platform := apputils.MakePlatform(constraints.Value{}, base, fallbackCons)
	origin, _ := apputils.MakeOrigin(charm.Schema(url.Schema), url.Revision, charm.Channel{}, platform)
	fakeAPI.Call("AddCharm", &deployURL, origin, force).Returns(origin, error(nil))
	fakeAPI.Call("CharmInfo", deployURL.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:  url.String(),
			Meta: meta,
		},
		error(nil),
	)
	var placement []*instance.Placement
	if p != nil {
		placement = []*instance.Placement{p}
	}
	deployArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    deployURL.String(),
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: appName,
		NumUnits:        numUnits,
		AttachStorage:   attachStorage,
		Cons:            cons,
		Placement:       placement,
		Config:          config,
		ConfigYAML:      configYAML,
		Storage:         storage,
		Devices:         devices,
		Force:           force,
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
	base corebase.Base,
) {
	for _, risk := range []string{"", "stable"} {
		origin := commoncharm.Origin{
			Source:       commoncharm.OriginCharmHub,
			Architecture: arch.DefaultArchitecture,
			Base:         base,
			Risk:         risk,
		}
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
	fakeAPI.Call("ListSpaces").Returns([]params.Space{}, error(nil))
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
