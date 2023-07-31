// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v10"
	charmresource "github.com/juju/charm/v10/resource"
	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type deployerSuite struct {
	testing.IsolationSuite

	consumeDetails    *mocks.MockConsumeDetails
	resolver          *mocks.MockResolver
	deployerAPI       *mocks.MockDeployerAPI
	deployStep        *fakeDeployStep
	modelCommand      *mocks.MockModelCommand
	filesystem        *mocks.MockFilesystem
	bundle            *mocks.MockBundle
	modelConfigGetter *mocks.MockModelConfigGetter
	charmReader       *mocks.MockCharmReader
	charm             *mocks.MockCharm

	deployResourceIDs map[string]string
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) SetUpTest(_ *gc.C) {
	s.deployResourceIDs = make(map[string]string)

	// TODO: remove this patch once we removed all the old series from tests in current package.
	s.PatchValue(&SupportedJujuSeries,
		func(time.Time, string, string) (set.Strings, error) {
			return set.NewStrings(
				"centos7", "centos8", "centos9", "genericlinux", "kubernetes", "opensuseleap",
				"jammy", "focal", "bionic", "xenial",
			), nil
		},
	)
}

func (s *deployerSuite) TestGetDeployerPredeployedLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()
	s.expectModelType()

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
	cfg := s.basicDeployerConfig(corebase.MustParseBaseFromString("ubuntu@18.04"))
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

func (s *deployerSuite) TestGetDeployerLocalCharmError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	factory := s.newDeployerFactory().(*factory)
	factory.charmOrBundle = "./bad.charm"

	s.expectStat("./bad.charm", os.ErrNotExist)

	_, err := factory.maybePredeployedLocalCharm()
	c.Assert(err, gc.ErrorMatches, `no charm was found at "./bad.charm"`)
}

func (s *deployerSuite) TestGetDeployerCharmHubCharm(c *gc.C) {
	ch := charm.MustParseURL("ch:test-charm")
	s.testGetDeployerRepositoryCharm(c, ch)
}

func (s *deployerSuite) testGetDeployerRepositoryCharm(c *gc.C, ch *charm.URL) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()
	s.expectModelType()
	// NotValid ensures that maybeReadRepositoryBundle won't find
	// charmOrBundle is a bundle.
	s.expectResolveBundleURL(errors.NotValidf("not a bundle"), 1)

	cfg := s.basicDeployerConfig()
	s.expectStat(ch.String(), errors.NotFoundf("file"))
	cfg.CharmOrBundle = ch.String()

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy charm: %s", ch.String()))
}

func (s *deployerSuite) TestGetDeployerCharmHubCharmWithRevision(c *gc.C) {
	cfg := s.channelDeployerConfig()
	cfg.Revision = 42
	cfg.Channel, _ = charm.ParseChannel("stable")
	ch := charm.MustParseURL("ch:test-charm")
	deployer, err := s.testGetDeployerRepositoryCharmWithRevision(c, ch, cfg)
	c.Assert(err, jc.ErrorIsNil)
	str := fmt.Sprintf("deploy charm: %s with revision %d will refresh from channel %s", ch.String(), cfg.Revision, cfg.Channel.String())
	c.Assert(deployer.String(), gc.Equals, str)
}

func (s *deployerSuite) TestGetDeployerCharmHubCharmWithRevisionFail(c *gc.C) {
	cfg := s.basicDeployerConfig()
	cfg.Revision = 42
	ch := charm.MustParseURL("ch:test-charm")
	_, err := s.testGetDeployerRepositoryCharmWithRevision(c, ch, cfg)
	c.Assert(err, gc.ErrorMatches, "specifying a revision requires a channel for future upgrades. Please use --channel")
}

func (s *deployerSuite) testGetDeployerRepositoryCharmWithRevision(c *gc.C, ch *charm.URL, cfg DeployerConfig) (Deployer, error) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()
	s.expectModelType()
	// NotValid ensures that maybeReadRepositoryBundle won't find
	// charmOrBundle is a bundle.
	s.expectResolveBundleURL(errors.NotValidf("not a bundle"), 1)

	cfg.CharmOrBundle = ch.String()
	s.expectStat(cfg.CharmOrBundle, errors.NotFoundf("file"))

	factory := s.newDeployerFactory()
	return factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
}

func (s *deployerSuite) TestSeriesOverride(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()
	s.expectModelType()
	s.expectResolveBundleURL(errors.NotValidf("not a bundle"), 1)

	cfg := s.basicDeployerConfig()
	ch := charm.MustParseURL("ch:test-charm")
	s.expectStat(ch.String(), errors.NotFoundf("file"))
	cfg.CharmOrBundle = ch.String()

	factory := s.newDeployerFactory()
	deployer, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy charm: %s", ch.String()))

	charmDeployer := deployer.(*repositoryCharm)
	c.Assert(charmDeployer.id.Origin.Base.OS, gc.Equals, "ubuntu")
	c.Assert(charmDeployer.id.Origin.Base.Channel.String(), gc.Equals, "20.04/stable")
}

func (s *deployerSuite) TestGetDeployerLocalBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	cfg := s.basicDeployerConfig(corebase.MustParseBaseFromString("ubuntu@18.04"))
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

func (s *deployerSuite) TestGetDeployerCharmHubBundleWithChannel(c *gc.C) {
	bundle := charm.MustParseURL("ch:test-bundle")
	cfg := s.channelDeployerConfig()
	cfg.CharmOrBundle = bundle.String()

	deployer, err := s.testGetDeployerRepositoryBundle(c, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy bundle: %s from channel edge", bundle.String()))
}

func (s *deployerSuite) TestGetDeployerCharmHubBundleWithRevision(c *gc.C) {
	bundle := charm.MustParseURL("ch:test-bundle")
	cfg := s.basicDeployerConfig()
	cfg.Revision = 8
	cfg.CharmOrBundle = bundle.String()

	deployer, err := s.testGetDeployerRepositoryBundle(c, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deployer.String(), gc.Equals, fmt.Sprintf("deploy bundle: %s with revision 8", bundle.String()))
}

func (s *deployerSuite) testGetDeployerRepositoryBundle(c *gc.C, cfg DeployerConfig) (Deployer, error) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	s.expectResolveBundleURL(nil, 1)

	cfg.Base = corebase.MustParseBaseFromString("ubuntu@18.04")
	cfg.FlagSet = &gnuflag.FlagSet{}
	s.expectStat(cfg.CharmOrBundle, errors.NotFoundf("file"))
	s.expectModelType()
	s.expectGetBundle(nil)
	s.expectData()

	factory := s.newDeployerFactory()
	return factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
}

func (s *deployerSuite) TestGetDeployerCharmHubBundleWithRevisionURL(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	bundle := charm.MustParseURL("ch:test-bundle-8")
	cfg := s.basicDeployerConfig(corebase.MustParseBaseFromString("ubuntu@18.04"))
	cfg.CharmOrBundle = bundle.String()
	cfg.FlagSet = &gnuflag.FlagSet{}
	s.expectStat(cfg.CharmOrBundle, errors.NotFoundf("file"))
	s.expectModelType()

	factory := s.newDeployerFactory()
	_, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, gc.ErrorMatches, "cannot specify revision in a charm or bundle name. Please use --revision.")
}

func (s *deployerSuite) TestGetDeployerCharmHubBundleError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFilesystem()

	s.expectResolveBundleURL(nil, 1)

	bundle := charm.MustParseURL("ch:test-bundle")
	cfg := s.channelDeployerConfig(corebase.MustParseBaseFromString("ubuntu@18.04"))
	cfg.Revision = 42
	cfg.CharmOrBundle = bundle.String()
	cfg.FlagSet = &gnuflag.FlagSet{}
	s.expectStat(cfg.CharmOrBundle, errors.NotFoundf("file"))
	s.expectModelType()

	factory := s.newDeployerFactory()
	_, err := factory.GetDeployer(cfg, s.modelConfigGetter, s.resolver)
	c.Assert(err, gc.ErrorMatches, "revision and channel are mutually exclusive when deploying a bundle. Please choose one.")
}

func (s *deployerSuite) TestResolveCharmURL(c *gc.C) {
	tests := []struct {
		defaultSchema charm.Schema
		path          string
		url           *charm.URL
		err           error
	}{{
		defaultSchema: charm.CharmHub,
		path:          "wordpress",
		url:           &charm.URL{Schema: "ch", Name: "wordpress", Revision: -1},
	}, {
		defaultSchema: charm.CharmHub,
		path:          "ch:wordpress",
		url:           &charm.URL{Schema: "ch", Name: "wordpress", Revision: -1},
	}, {
		defaultSchema: charm.CharmHub,
		path:          "local:wordpress",
		url:           &charm.URL{Schema: "local", Name: "wordpress", Revision: -1},
	}}
	for i, test := range tests {
		c.Logf("%d %s", i, test.path)
		url, err := resolveCharmURL(test.path, test.defaultSchema)
		if test.err != nil {
			c.Assert(err, gc.ErrorMatches, test.err.Error())
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(url, gc.DeepEquals, test.url)
		}
	}
}

func (s *deployerSuite) TestValidateResourcesNeededForLocalDeployCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelCommand.EXPECT().ModelType().Return(model.CAAS, nil).AnyTimes()

	f := &factory{
		model: s.modelCommand,
	}

	err := f.validateResourcesNeededForLocalDeploy(&charm.Meta{
		Series: []string{corebase.Kubernetes.String()},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *deployerSuite) TestValidateResourcesNeededForLocalDeployIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelCommand.EXPECT().ModelType().Return(model.IAAS, nil).AnyTimes()

	f := &factory{
		model: s.modelCommand,
	}

	err := f.validateResourcesNeededForLocalDeploy(&charm.Meta{
		Series: []string{corebase.Kubernetes.String()},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *deployerSuite) TestMaybeReadLocalCharmErrorWithApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectModelGet(c)

	s.charmReader.EXPECT().ReadCharm("meshuggah").Return(s.charm, nil)
	s.charm.EXPECT().Manifest().Return(nil).AnyTimes()
	s.charm.EXPECT().Meta().Return(&charm.Meta{Series: []string{"focal"}}).AnyTimes()

	f := &factory{
		clock:           clock.WallClock,
		applicationName: "meshuggah",
		charmOrBundle:   "local:meshuggah",
		charmReader:     s.charmReader,
		model:           s.modelCommand,
	}

	_, err := f.maybeReadLocalCharm(s.modelConfigGetter)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *deployerSuite) TestMaybeReadLocalCharmErrorWithoutApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectModelGet(c)

	s.charmReader.EXPECT().ReadCharm("meshuggah").Return(s.charm, nil)
	s.charm.EXPECT().Manifest().Return(nil).AnyTimes()
	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "meshuggah", Series: []string{"focal"}}).AnyTimes()

	f := &factory{
		clock:         clock.WallClock,
		charmOrBundle: "local:meshuggah",
		charmReader:   s.charmReader,
		model:         s.modelCommand,
	}

	_, err := f.maybeReadLocalCharm(s.modelConfigGetter)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *deployerSuite) makeBundleDir(c *gc.C, content string) string {
	bundlePath := filepath.Join(c.MkDir(), "example")
	c.Assert(os.Mkdir(bundlePath, 0777), jc.ErrorIsNil)
	err := os.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return bundlePath
}

func (s *deployerSuite) newDeployerFactory() DeployerFactory {
	dep := DeployerDependencies{
		DeployResources: func(
			string,
			resources.CharmID,
			map[string]string,
			map[string]charmresource.Meta,
			base.APICallCloser,
			modelcmd.Filesystem,
		) (ids map[string]string, err error) {
			return s.deployResourceIDs, nil
		},
		Model:                s.modelCommand,
		NewConsumeDetailsAPI: func(url *charm.OfferURL) (ConsumeDetails, error) { return s.consumeDetails, nil },
		FileSystem:           s.filesystem,
		CharmReader:          fsCharmReader{},
		Steps:                []DeployStep{s.deployStep},
	}
	return NewDeployerFactory(dep)
}

func (s *deployerSuite) basicDeployerConfig(bases ...corebase.Base) DeployerConfig {
	var base corebase.Base
	if len(bases) == 0 {
		base = corebase.MustParseBaseFromString("ubuntu@20.04")
	} else {
		base = bases[0]
	}
	return DeployerConfig{
		Base:     base,
		Revision: -1,
	}
}

func (s *deployerSuite) channelDeployerConfig(bases ...corebase.Base) DeployerConfig {
	var base corebase.Base
	if len(bases) == 0 {
		base = corebase.MustParseBaseFromString("ubuntu@20.04")
	} else {
		base = bases[0]
	}
	return DeployerConfig{
		Base: base,
		Channel: charm.Channel{
			Risk: "edge",
		},
		Revision: -1,
	}
}

func (s *deployerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.consumeDetails = mocks.NewMockConsumeDetails(ctrl)
	s.resolver = mocks.NewMockResolver(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	s.modelCommand = mocks.NewMockModelCommand(ctrl)
	s.filesystem = mocks.NewMockFilesystem(ctrl)
	s.modelConfigGetter = mocks.NewMockModelConfigGetter(ctrl)
	s.deployStep = &fakeDeployStep{}
	s.charmReader = mocks.NewMockCharmReader(ctrl)
	s.charm = mocks.NewMockCharm(ctrl)
	return ctrl
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
		"secret-backend":  "auto",
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

// RunPre runs before the call is made to add the charm to the environment.
func (f *fakeDeployStep) RunPre(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo) error {
	return nil
}

// RunPost runs after the call is made to add the charm to the environment.
// The error parameter is used to notify the step of a previously occurred error.
func (f *fakeDeployStep) RunPost(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo, error) error {
	return nil
}

// TODO (stickupkid): Remove this in favour of better unit tests with mocks.
// Currently most of the tests are integration tests, pretending to be unit
// tests.
type fsCharmReader struct{}

// ReadCharm attempts to read a charm from a path on the filesystem.
func (fsCharmReader) ReadCharm(path string) (charm.Charm, error) {
	return charm.ReadCharm(path)
}
