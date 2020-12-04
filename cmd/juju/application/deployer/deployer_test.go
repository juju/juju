// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/resources/client"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type deployerSuite struct {
	consumeDetails    *mocks.MockConsumeDetails
	resolver          *mocks.MockResolver
	deployerAPI       *mocks.MockDeployerAPI
	deployStep        *fakeDeployStep
	macaroonGetter    *mocks.MockMacaroonGetter
	modelCommand      *mocks.MockModelCommand
	filesystem        *mocks.MockFilesystem
	bundle            *mocks.MockBundle
	modelConfigGetter *mocks.MockModelConfigGetter

	deployResourceIDs map[string]string
	output            *bytes.Buffer
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) SetUpTest(_ *gc.C) {
	s.deployResourceIDs = make(map[string]string)
	s.output = bytes.NewBuffer([]byte{})
}

func (s *deployerSuite) TestGetDeployerPredeployedLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	cfg := s.basicDeployerConfig()
	ch := charm.MustParseURL("local:test-charm")
	s.expectStat(ch.String(), errors.NotFoundf("file"))
	cfg.CharmOrBundle = ch.String()

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy predeployed local charm: %s", ch.String()))
}

func (s *deployerSuite) TestGetDeployerLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()
	s.expectModelGet(c)
	cfg := s.basicDeployerConfig()
	cfg.Series = "bionic"
	s.expectModelType()

	dir := c.MkDir()
	charmPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(dir, "multi-series")

	s.expectStat(charmPath, nil)
	cfg.CharmOrBundle = charmPath

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	ch := charm.MustParseURL("local:bionic/multi-series-1")
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy local charm: %s", ch.String()))
}

func (s *deployerSuite) TestGetDeployerCharmStoreCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()
	// NotValid ensures that maybeReadCharmstoreBundle won't find
	// charmOrBundle is a bundle.
	s.expectResolveBundleURL(errors.NotValidf("not a bundle"), 1)

	cfg := s.basicDeployerConfig()
	ch := charm.MustParseURL("cs:test-charm")
	s.expectStat(ch.String(), errors.NotFoundf("file"))
	cfg.CharmOrBundle = ch.String()

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy charm store charm: %s", ch.String()))
}

func (s *deployerSuite) TestCharmStoreSeriesOverride(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()
	s.expectResolveBundleURL(errors.NotValidf("not a bundle"), 1)

	cfg := s.basicDeployerConfig()
	cfg.Series = "bionic" // Override the default series (as if --series was specified)
	ch := charm.MustParseURL("cs:test-charm")
	s.expectStat(ch.String(), errors.NotFoundf("file"))
	cfg.CharmOrBundle = ch.String()

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy charm store charm: %s", ch.String()))

	charmStoreDeployer := deployer.(*charmStoreCharm)
	c.Assert(charmStoreDeployer.series, gc.Equals, "bionic")
}

func (s *deployerSuite) TestGetDeployerLocalBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	cfg := s.basicDeployerConfig()
	cfg.Series = "bionic"
	cfg.FlagSet = &gnuflag.FlagSet{}
	s.expectModelType()

	content := `
      series: xenial
      applications:
          wordpress:
              charm: wordpress
              num_units: 1
          mysql:
              charm: mysql
              num_units: 2
      relations:
          - ["wordpress:db", "mysql:server"]
`
	bundlePath := s.makeBundleDir(c, content)
	s.expectStat(bundlePath, nil)
	cfg.CharmOrBundle = bundlePath

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy local bundle from: %s", bundlePath))
}

func (s *deployerSuite) TestGetDeployerCharmStoreBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	s.expectResolveBundleURL(nil, 1)

	bundle := charm.MustParseURL("cs:test-bundle")
	cfg := s.basicDeployerConfig()
	cfg.Series = "bionic"
	cfg.FlagSet = &gnuflag.FlagSet{}
	cfg.CharmOrBundle = bundle.String()
	s.expectStat(bundle.String(), errors.NotFoundf("file"))
	s.expectModelType()
	s.expectGetBundle(nil)
	s.expectData()

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy charm store bundle: %s", bundle.String()))
}

func (s *deployerSuite) TestGetDeployerCharmStoreBundleWithChannel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	s.expectResolveBundleURL(nil, 1)

	bundle := charm.MustParseURL("cs:test-bundle")
	cfg := s.channelDeployerConfig()
	cfg.Series = "bionic"
	cfg.FlagSet = &gnuflag.FlagSet{}
	cfg.CharmOrBundle = bundle.String()
	s.expectStat(bundle.String(), errors.NotFoundf("file"))
	s.expectModelType()
	s.expectGetBundle(nil)
	s.expectData()

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy charm store bundle: %s", bundle.String()))
}

func (s *deployerSuite) TestResolveCharmURL(c *gc.C) {
	tests := []struct {
		path string
		url  *charm.URL
		err  error
	}{{
		path: "wordpress",
		url:  &charm.URL{Schema: "cs", Name: "wordpress", Revision: -1},
	}, {
		path: "cs:wordpress",
		url:  &charm.URL{Schema: "cs", Name: "wordpress", Revision: -1},
	}, {
		path: "local:wordpress",
		url:  &charm.URL{Schema: "local", Name: "wordpress", Revision: -1},
	}, {
		path: "cs:~user/series/name",
		url:  &charm.URL{Schema: "cs", User: "user", Name: "name", Series: "series", Revision: -1},
	}, {
		path: "~user/series/name",
		url:  &charm.URL{Schema: "cs", User: "user", Name: "name", Series: "series", Revision: -1},
	}, {
		path: "ch:~user/series/name",
		err:  errors.Errorf(`unexpected charm schema: cannot parse URL "ch:~user/series/name": schema "ch" not valid`),
	}}
	for i, test := range tests {
		c.Logf("%d %s", i, test.path)
		url, err := resolveCharmURL(test.path)
		if test.err != nil {
			c.Assert(err, gc.ErrorMatches, test.err.Error())
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(url, gc.DeepEquals, test.url)
		}
	}
}

func (s *deployerSuite) TestResolveCharmURLCharmHubIntegration(c *gc.C) {
	setFeatureFlags(feature.CharmHubIntegration)
	defer setFeatureFlags("")

	tests := []struct {
		path string
		url  *charm.URL
		err  error
	}{{
		path: "wordpress",
		url:  &charm.URL{Schema: "ch", Name: "wordpress", Revision: -1},
	}, {
		path: "ch:wordpress-42",
		url:  &charm.URL{Schema: "ch", Name: "wordpress", Revision: 42},
	}, {
		path: "cs:wordpress",
		url:  &charm.URL{Schema: "cs", Name: "wordpress", Revision: -1},
	}, {
		path: "local:wordpress",
		url:  &charm.URL{Schema: "local", Name: "wordpress", Revision: -1},
	}, {
		path: "cs:~user/series/name",
		url:  &charm.URL{Schema: "cs", User: "user", Name: "name", Series: "series", Revision: -1},
	}}
	for i, test := range tests {
		c.Logf("%d %s", i, test.path)
		url, err := resolveCharmURL(test.path)
		if test.err != nil {
			c.Assert(err, gc.ErrorMatches, test.err.Error())
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(url, gc.DeepEquals, test.url)
		}
	}
}

func setFeatureFlags(flags string) {
	if err := os.Setenv(osenv.JujuFeatureFlagEnvKey, flags); err != nil {
		panic(err)
	}
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *deployerSuite) makeBundleDir(c *gc.C, content string) string {
	bundlePath := filepath.Join(c.MkDir(), "example")
	c.Assert(os.Mkdir(bundlePath, 0777), jc.ErrorIsNil)
	err := ioutil.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return bundlePath
}

func (s *deployerSuite) newDeployerFactory() DeployerFactory {
	dep := DeployerDependencies{
		DeployResources: func(
			string,
			client.CharmID,
			*macaroon.Macaroon,
			map[string]string,
			map[string]charmresource.Meta,
			base.APICallCloser,
			modelcmd.Filesystem,
		) (ids map[string]string, err error) {
			return s.deployResourceIDs, nil
		},
		Model:                s.modelCommand,
		NewConsumeDetailsAPI: func(url *charm.OfferURL) (ConsumeDetails, error) { return s.consumeDetails, nil },
		Steps:                []DeployStep{s.deployStep},
	}
	return NewDeployerFactory(dep)
}

func (s *deployerSuite) basicDeployerConfig() DeployerConfig {
	return DeployerConfig{
		Series: "focal",
	}
}

func (s *deployerSuite) channelDeployerConfig() DeployerConfig {
	return DeployerConfig{
		Series: "focal",
		Channel: corecharm.Channel{
			Risk: "edge",
		},
	}
}

func (s *deployerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.consumeDetails = mocks.NewMockConsumeDetails(ctrl)
	s.resolver = mocks.NewMockResolver(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	s.macaroonGetter = mocks.NewMockMacaroonGetter(ctrl)
	s.modelCommand = mocks.NewMockModelCommand(ctrl)
	s.filesystem = mocks.NewMockFilesystem(ctrl)
	s.modelConfigGetter = mocks.NewMockModelConfigGetter(ctrl)
	s.deployStep = &fakeDeployStep{}
	return ctrl
}

func (s *deployerSuite) expectResolveCharm(err error, times int) {
	s.resolver.EXPECT().ResolveCharm(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin) (*charm.URL, commoncharm.Origin, []string, error) {
			return curl, origin, []string{"bionic", "focal", "xenial"}, err
		}).Times(times)
}

func (s *deployerSuite) expectResolveBundleURL(err error, times int) {
	s.resolver.EXPECT().ResolveBundleURL(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin) (*charm.URL, commoncharm.Origin, error) {
			return curl, origin, err
		}).Times(times)
}

func (s *deployerSuite) expectFilesystem() {
	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem)
}

func (s *deployerSuite) expectStat(name string, err error) {
	s.filesystem.EXPECT().Stat(name).Return(nil, err)
}

func (s *deployerSuite) expectModelGet(c *gc.C) {
	minimal := map[string]interface{}{
		"name":            "test",
		"type":            "manual",
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ControllerTag.Id(),
		"firewall-mode":   "instance",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
	cfg, err := config.New(true, minimal)
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigGetter.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil)
}

func (s *deployerSuite) expectModelType() {
	s.modelCommand.EXPECT().ModelType().Return(model.IAAS, nil).AnyTimes()
}

func (s *deployerSuite) expectGetBundle(err error) {
	s.resolver.EXPECT().GetBundle(gomock.AssignableToTypeOf(&charm.URL{}), gomock.Any(), gomock.Any()).Return(s.bundle, err)
}

func (s *deployerSuite) expectData() {
	s.bundle.EXPECT().Data().Return(&charm.BundleData{})
}

// fakeDeployStep implements the DeployStep interface.  Using gomock
// creates an import cycle.
type fakeDeployStep struct {
}

func (f *fakeDeployStep) SetFlags(*gnuflag.FlagSet) {}

// SetPlanURL sets the plan URL prefix.
func (f *fakeDeployStep) SetPlanURL(string) {}

// RunPre runs before the call is made to add the charm to the environment.
func (f *fakeDeployStep) RunPre(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo) error {
	return nil
}

// RunPost runs after the call is made to add the charm to the environment.
// The error parameter is used to notify the step of a previously occurred error.
func (f *fakeDeployStep) RunPost(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo, error) error {
	return nil
}
