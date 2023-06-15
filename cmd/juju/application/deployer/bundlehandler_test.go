// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	bundlechanges "github.com/juju/juju/core/bundle/changes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
)

type BundleDeployRepositorySuite struct {
	testing.IsolationSuite

	allWatcher     *mocks.MockAllWatch
	bundleResolver *mocks.MockResolver
	deployerAPI    *mocks.MockDeployerAPI
	stdOut         *mocks.MockWriter
	stdErr         *mocks.MockWriter

	deployArgs map[string]application.DeployArgs
	output     *bytes.Buffer
}

var _ = gc.Suite(&BundleDeployRepositorySuite{})

func (s *BundleDeployRepositorySuite) SetUpTest(_ *gc.C) {
	s.deployArgs = make(map[string]application.DeployArgs)
	s.output = bytes.NewBuffer([]byte{})

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

func (s *BundleDeployRepositorySuite) TearDownTest(_ *gc.C) {
	s.output.Reset()
}

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

func (s *BundleDeployRepositorySuite) TestDeployBundleNotFoundCharmHub(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)

	// bundleHandler.addCharm():
	curl := charm.MustParseURL("ch:bundle/no-such")
	s.expectResolveCharm(errors.NotFoundf("bundle"))
	bundleData := &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"no-such": {
				Charm: curl.String(),
			},
		},
	}

	err := bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, `cannot resolve charm or bundle "no-such": bundle not found`)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("ch:mysql")
	wordpressCurl := charm.MustParseURL("ch:wordpress")
	chUnits := []charmUnit{
		{
			curl:                 mysqlCurl,
			charmMetaSeries:      []string{"bionic", "xenial"},
			machine:              "0",
			machineUbuntuVersion: "16.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 wordpressCurl,
			machine:              "1",
			machineUbuntuVersion: "16.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	s.runDeploy(c, wordpressBundle)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "ubuntu", "16.04")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "ubuntu", "16.04")
	s.assertDeployArgsConfig(c, "mysql", map[string]interface{}{"foo": "bar"})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-hub, channel stable\n"+
		"Located charm \"wordpress\" in charm-hub, channel stable\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-hub for series xenial with revision 42 with architecture=amd64\n"+
		"- deploy application mysql from charm-hub on xenial with stable\n"+
		"- upload charm wordpress from charm-hub for series xenial with revision 47 with architecture=amd64\n"+
		"- deploy application wordpress from charm-hub on xenial with stable\n"+
		"- add new machine 0\n"+
		"- add new machine 1\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleSuccessWithModelConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl, err := charm.ParseURL("mysql")
	c.Assert(err, jc.ErrorIsNil)
	wordpressCurl, err := charm.ParseURL("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:                 mysqlCurl,
			charmMetaSeries:      []string{"bionic", "xenial"},
			machine:              "0",
			machineUbuntuVersion: "16.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 wordpressCurl,
			machine:              "1",
			machineUbuntuVersion: "16.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundle))
	c.Assert(err, jc.ErrorIsNil)

	err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpecWithConstraints(constraints.MustParse("arch=arm64")))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "ubuntu", "16.04")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "ubuntu", "16.04")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-hub, channel stable\n"+
		"Located charm \"wordpress\" in charm-hub, channel stable\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-hub for series xenial with revision 42 with architecture=arm64\n"+
		"- deploy application mysql from charm-hub on xenial with stable\n"+
		"- upload charm wordpress from charm-hub for series xenial with revision 47 with architecture=arm64\n"+
		"- deploy application wordpress from charm-hub on xenial with stable\n"+
		"- add new machine 0\n"+
		"- add new machine 1\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"Deploy of bundle completed.\n")
}

const wordpressBundle = `
series: bionic
applications:
  mysql:
    charm: ch:mysql
    revision: 42
    channel: stable
    series: xenial
    num_units: 1
    options:
      foo: bar
    to:
    - "0"
  wordpress:
    charm: ch:wordpress
    revision: 47
    channel: stable
    series: xenial
    num_units: 1
    to:
    - "1"
machines:
  "0":
    series: xenial
  "1":
    series: xenial
relations:
- - wordpress:db
  - mysql:db
`

const multiApplicationBundle = `
name: istio
bundle: kubernetes
applications:
  istio-ingressgateway:
    charm: istio-gateway
    channel: latest/edge
    revision: 74
  istio-pilot:
    charm: istio-pilot
    channel: latest/edge
    revision: 95
  training-operator:
    charm: training-operator
    channel: 1.3/edge
`

func (s *BundleDeployRepositorySuite) TestDeployAddCharmHasSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	fullGatewayURL := s.expectK8sCharmByRevision(charm.MustParseURL("ch:istio-gateway"), 74)
	fullTrainingURL := s.expectK8sCharm(charm.MustParseURL("ch:training-operator"), 12)
	fullPilotURL := s.expectK8sCharmByRevision(charm.MustParseURL("ch:istio-pilot"), 95)

	bundleData, err := charm.ReadBundleData(strings.NewReader(multiApplicationBundle))
	c.Assert(err, jc.ErrorIsNil)

	err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 3)
	s.assertDeployArgs(c, fullGatewayURL.String(), "istio-ingressgateway", "ubuntu", "20.04")
	s.assertDeployArgs(c, fullTrainingURL.String(), "training-operator", "ubuntu", "20.04")
	s.assertDeployArgs(c, fullPilotURL.String(), "istio-pilot", "ubuntu", "20.04")
}

func (s *BundleDeployRepositorySuite) TestDeployKubernetesBundleSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mariadbCurl := charm.MustParseURL("ch:bionic/mariadb-k8s")
	gitlabCurl := charm.MustParseURL("ch:bionic/gitlab-k8s")
	chUnits := []charmUnit{
		{
			curl:                 mariadbCurl,
			charmMetaSeries:      []string{"kubernetes"},
			machineUbuntuVersion: "kubernetes",
		},
		{
			charmMetaSeries:      []string{"kubernetes"},
			curl:                 gitlabCurl,
			machineUbuntuVersion: "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"gitlab:mysql", "mariadb:server"})

	s.runDeploy(c, kubernetesGitlabBundle)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, gitlabCurl.String(), "gitlab", "ubuntu", "18.04")
	s.assertDeployArgs(c, mariadbCurl.String(), "mariadb", "ubuntu", "18.04")
	s.assertDeployArgsStorage(c, "mariadb", map[string]storage.Constraints{"database": {Pool: "mariadb-pv", Size: 0x14, Count: 0x1}})
	s.assertDeployArgsConfig(c, "mariadb", map[string]interface{}{"dataset-size": "70%"})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"gitlab-k8s\" in charm-hub\n"+
		"Located charm \"mariadb-k8s\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm gitlab-k8s from charm-hub with architecture=amd64\n"+
		"- deploy application gitlab from charm-hub with 1 unit using gitlab-k8s\n"+
		"- upload charm mariadb-k8s from charm-hub with architecture=amd64\n"+
		"- deploy application mariadb from charm-hub with 2 units using mariadb-k8s\n"+
		"- add relation gitlab:mysql - mariadb:server\n"+
		"Deploy of bundle completed.\n")
}

const kubernetesGitlabBundle = `
bundle: kubernetes
applications:
  mariadb:
    charm: ch:mariadb-k8s
    scale: 2
    constraints: mem=1G
    options:
      dataset-size: 70%
    storage:
      database: mariadb-pv,20M
  gitlab:
    charm: ch:gitlab-k8s
    placement: foo=bar
    scale: 1
relations:
  - - gitlab:mysql
    - mariadb:server
`

func (s *BundleDeployRepositorySuite) TestDeployKubernetesBundleSuccessWithCharmhub(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	fullGitlabCurl := s.expectK8sCharm(charm.MustParseURL("gitlab-k8s"), 4)
	fullMariadbCurl := s.expectK8sCharm(charm.MustParseURL("mariadb-k8s"), 7)
	s.expectAddRelation([]string{"gitlab:mysql", "mariadb:server"})

	s.runDeploy(c, kubernetesCharmhubGitlabBundle)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, fullGitlabCurl.String(), "gitlab", "ubuntu", "20.04")
	s.assertDeployArgs(c, fullMariadbCurl.String(), "mariadb", "ubuntu", "20.04")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"gitlab-k8s\" in charm-hub, channel new/edge\n"+
		"Located charm \"mariadb-k8s\" in charm-hub, channel old/stable\n"+
		"Executing changes:\n"+
		"- upload charm gitlab-k8s from charm-hub from channel new/edge with architecture=amd64\n"+
		"- deploy application gitlab from charm-hub with 1 unit with new/edge using gitlab-k8s\n"+
		"- upload charm mariadb-k8s from charm-hub from channel old/stable with architecture=amd64\n"+
		"- deploy application mariadb from charm-hub with 2 units with old/stable using mariadb-k8s\n"+
		"- add relation gitlab:mysql - mariadb:server\n"+
		"Deploy of bundle completed.\n")
}

const kubernetesCharmhubGitlabBundle = `
bundle: kubernetes
applications:
  mariadb:
    charm: mariadb-k8s
    scale: 2
    channel: old/stable
  gitlab:
    charm: gitlab-k8s
    scale: 1
    channel: new/edge
relations:
  - - gitlab:mysql
    - mariadb:server
`

func (s *BundleDeployRepositorySuite) expectK8sCharm(curl *charm.URL, rev int) *charm.URL {
	// Called from resolveCharmsAndEndpoints & resolveCharmChannelAndRevision && addCharm
	s.bundleResolver.EXPECT().ResolveCharm(
		curl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []string, error) {
			curl = curl.WithRevision(rev).WithSeries("focal").WithArchitecture("amd64")
			origin.Base = coreseries.MakeDefaultBase("ubuntu", "20.04")
			origin.Revision = &rev
			origin.Type = "charm"
			return curl, origin, []string{"focal"}, nil
		}).Times(3)

	fullCurl := curl.WithSeries("focal").WithRevision(rev).WithArchitecture("amd64")
	s.deployerAPI.EXPECT().AddCharm(
		fullCurl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		func(_ *charm.URL, origin commoncharm.Origin, _ bool) (commoncharm.Origin, error) {
			return origin, nil
		})

	charmInfo := &apicharms.CharmInfo{
		Revision: fullCurl.Revision,
		URL:      fullCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"kubernetes"},
		},
		Manifest: &charm.Manifest{
			Bases: []charm.Base{
				{
					Name:          "ubuntu",
					Channel:       charm.Channel{Track: "20.04", Risk: "stable", Branch: ""},
					Architectures: []string{"amd64"},
				},
			},
		},
	}
	s.expectCharmInfo(fullCurl.String(), charmInfo)
	s.expectDeploy()
	return fullCurl
}

const kubernetesCharmhubGitlabBundleWithRevision = `
bundle: kubernetes
applications:
  mariadb:
    charm: mariadb-k8s
    revision: 4
    scale: 2
    channel: old/stable
  gitlab:
    charm: gitlab-k8s
    revision: 7
    scale: 1
    channel: new/edge
relations:
  - - gitlab:mysql
    - mariadb:server
`

func (s *BundleDeployRepositorySuite) TestDeployKubernetesBundleSuccessWithRevisionCharmhub(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	fullGitlabCurl := s.expectK8sCharmByRevision(charm.MustParseURL("gitlab-k8s"), 7)
	fullMariadbCurl := s.expectK8sCharmByRevision(charm.MustParseURL("mariadb-k8s"), 4)
	s.expectAddRelation([]string{"gitlab:mysql", "mariadb:server"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(kubernetesCharmhubGitlabBundleWithRevision))
	c.Assert(err, jc.ErrorIsNil)

	err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, fullGitlabCurl.String(), "gitlab", "ubuntu", "20.04")
	s.assertDeployArgs(c, fullMariadbCurl.String(), "mariadb", "ubuntu", "20.04")

	str := s.output.String()
	c.Check(strings.Contains(str, "Located charm \"gitlab-k8s\" in charm-hub, channel new/edge\n"), jc.IsTrue)
	c.Check(strings.Contains(str, "Located charm \"mariadb-k8s\" in charm-hub, channel old/stable\n"), jc.IsTrue)
	c.Check(strings.Contains(str, "- upload charm mariadb-k8s from charm-hub with revision 4 with architecture=amd64\n"), jc.IsTrue)
}

func (s *BundleDeployRepositorySuite) expectK8sCharmByRevision(curl *charm.URL, rev int) *charm.URL {
	// Called from resolveCharmsAndEndpoints & resolveCharmChannelAndRevision && addCharm
	s.bundleResolver.EXPECT().ResolveCharm(
		curl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []string, error) {
			curl = curl.WithRevision(rev)
			curl = curl.WithSeries("focal")
			curl = curl.WithArchitecture("amd64")
			origin.Base = coreseries.MakeDefaultBase("ubuntu", "20.04")
			origin.Revision = &rev
			origin.Type = "charm"
			return curl, origin, []string{"focal"}, nil
		}).Times(2)

	fullCurl := curl.WithSeries("focal").WithRevision(rev).WithArchitecture("amd64")
	s.deployerAPI.EXPECT().AddCharm(
		fullCurl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		func(_ *charm.URL, origin commoncharm.Origin, _ bool) (commoncharm.Origin, error) {
			return origin, nil
		})

	charmInfo := &apicharms.CharmInfo{
		Revision: fullCurl.Revision,
		URL:      fullCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"kubernetes"},
		},
		Manifest: &charm.Manifest{
			Bases: []charm.Base{
				{
					Name:          "ubuntu",
					Channel:       charm.Channel{Track: "20.04", Risk: "stable", Branch: ""},
					Architectures: []string{"amd64"},
				},
			},
		},
	}
	s.expectCharmInfo(fullCurl.String(), charmInfo)
	s.expectDeploy()
	return fullCurl
}

func (s *BundleDeployRepositorySuite) TestDeployBundleStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("ch:mysql")
	wordpressCurl := charm.MustParseURL("ch:wordpress")
	chUnits := []charmUnit{
		{
			curl:                 mysqlCurl,
			charmMetaSeries:      []string{"bionic", "xenial"},
			machine:              "0",
			machineUbuntuVersion: "18.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 wordpressCurl,
			machine:              "1",
			machineUbuntuVersion: "18.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	s.runDeploy(c, wordpressBundleWithStorage)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "ubuntu", "18.04")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "ubuntu", "18.04")
	s.assertDeployArgsStorage(c, "mysql", map[string]storage.Constraints{"database": {Pool: "mysql-pv", Size: 0x14, Count: 0x1}})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-hub, channel stable\n"+
		"Located charm \"wordpress\" in charm-hub, channel stable\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-hub for series bionic with revision 42 with architecture=amd64\n"+
		"- deploy application mysql from charm-hub on bionic with stable\n"+
		"- upload charm wordpress from charm-hub for series bionic with revision 47 with architecture=amd64\n"+
		"- deploy application wordpress from charm-hub on bionic with stable\n"+
		"- add new machine 0\n"+
		"- add new machine 1\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"Deploy of bundle completed.\n")
}

const wordpressBundleWithStorage = `
series: bionic
applications:
  mysql:
    charm: ch:mysql
    revision: 42
    channel: stable
    num_units: 1
    storage:
      database: mysql-pv,20M
    to:
    - "0"
  wordpress:
    charm: ch:wordpress
    revision: 47
    channel: stable
    num_units: 1
    to:
    - "1"
machines:
  "0": {}
  "1": {}
relations:
- - wordpress:db
  - mysql:db
`

func (s *BundleDeployRepositorySuite) TestDeployBundleDevices(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	bitcoinCurl := s.expectCharmhubK8sCharm(charm.MustParseURL("ch:bitcoin-miner"))
	dashboardCurl := s.expectCharmhubK8sCharm(charm.MustParseURL("ch:dashboard4miner"))
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})
	s.expectAddOneUnit("bitcoin-miner", "", "1")
	s.expectAddOneUnit("dashboard4miner", "", "1")

	spec := s.bundleDeploySpec()
	devConstraints := map[string]devices.Constraints{
		"bitcoinminer": {
			Count: 10, Type: "nvidia.com/gpu",
		},
	}
	spec.bundleDevices = map[string]map[string]devices.Constraints{
		"bitcoin-miner": devConstraints,
	}
	s.runDeployWithSpec(c, kubernetesBitcoinBundle, spec)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "kubernetes", "kubernetes")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "kubernetes", "kubernetes")
	s.assertDeployArgsDevices(c, bitcoinCurl.Name, devConstraints)

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-hub\n"+
		"Located charm \"dashboard4miner\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm bitcoin-miner from charm-hub with architecture=amd64\n"+
		"- deploy application bitcoin-miner from charm-hub\n"+
		"- upload charm dashboard4miner from charm-hub with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-hub\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"- add unit bitcoin-miner/0 to new machine 0\n"+
		"- add unit dashboard4miner/0 to new machine 1\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) expectCharmhubK8sCharm(curl *charm.URL) *charm.URL {
	fullCurl := curl.WithSeries("kubernetes")
	// Called from resolveCharmsAndEndpoints & resolveCharmChannelAndRevision && addCharm
	s.bundleResolver.EXPECT().ResolveCharm(
		curl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []string, error) {
			origin.Type = "charm"
			return fullCurl, origin, []string{"kubernetes"}, nil
		}).Times(3)

	s.deployerAPI.EXPECT().AddCharm(
		fullCurl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		func(_ *charm.URL, origin commoncharm.Origin, _ bool) (commoncharm.Origin, error) {
			origin.Base = coreseries.MakeDefaultBase("kubernetes", "kubernetes")
			return origin, nil
		})

	charmInfo := &apicharms.CharmInfo{
		Revision: fullCurl.Revision,
		URL:      fullCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"kubernetes"},
		},
	}
	s.expectCharmInfo(fullCurl.String(), charmInfo)
	s.expectDeploy()
	return fullCurl
}

const kubernetesBitcoinBundle = `
applications:
    dashboard4miner:
        charm: ch:dashboard4miner
        num_units: 1
    bitcoin-miner:
        charm: ch:bitcoin-miner
        num_units: 1
        devices:
            bitcoinminer: 1,nvidia.com/gpu
relations:
    - ["dashboard4miner:miner", "bitcoin-miner:miner"]
`

func (s *BundleDeployRepositorySuite) TestDeployKubernetesBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	bitcoinCurl := charm.MustParseURL("ch:bitcoin-miner")
	dashboardCurl := charm.MustParseURL("ch:dashboard4miner")
	chUnits := []charmUnit{
		{
			curl:                 bitcoinCurl,
			machineUbuntuVersion: "kubernetes",
		},
		{
			curl:                 dashboardCurl,
			machineUbuntuVersion: "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})

	s.runDeploy(c, kubernetesBitcoinBundleWithoutDevices)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "ubuntu", "20.04")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "ubuntu", "20.04")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-hub\n"+
		"Located charm \"dashboard4miner\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm bitcoin-miner from charm-hub for series focal with architecture=amd64\n"+
		"- deploy application bitcoin-miner from charm-hub with 1 unit on focal\n"+
		"- upload charm dashboard4miner from charm-hub for series focal with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-hub with 1 unit on focal\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"Deploy of bundle completed.\n")
}

const kubernetesBitcoinBundleWithoutDevices = `
bundle: kubernetes
applications:
    dashboard4miner:
        charm: dashboard4miner
        num_units: 1
        series: focal
    bitcoin-miner:
        charm: bitcoin-miner
        num_units: 1
        series: focal
relations:
    - ["dashboard4miner:miner", "bitcoin-miner:miner"]
`

func (s *BundleDeployRepositorySuite) TestExistingModelIdempotent(c *gc.C) {
	s.testExistingModel(c, false)
}

func (s *BundleDeployRepositorySuite) TestDryRunExistingModel(c *gc.C) {
	s.testExistingModel(c, true)
}

func (s *BundleDeployRepositorySuite) testExistingModel(c *gc.C, dryRun bool) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("ch:mysql")
	wordpressCurl := charm.MustParseURL("ch:wordpress")
	chUnits := []charmUnit{
		{
			curl:                 mysqlCurl,
			charmMetaSeries:      []string{"bionic", "xenial"},
			machine:              "0",
			machineUbuntuVersion: "18.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 wordpressCurl,
			machine:              "1",
			machineUbuntuVersion: "18.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})
	s.expectResolveCharm(nil)

	if !dryRun {
		s.expectAddCharm(false)
		s.expectCharmInfo("ch:mysql", &apicharms.CharmInfo{URL: mysqlCurl.String(), Meta: &charm.Meta{}})
		s.expectSetCharm(c, "mysql")
		s.expectAddCharm(false)
		s.expectCharmInfo("ch:wordpress", &apicharms.CharmInfo{URL: wordpressCurl.String(), Meta: &charm.Meta{}})
		s.expectSetCharm(c, "wordpress")
	}

	spec := s.bundleDeploySpec()
	s.runDeployWithSpec(c, wordpressBundleWithStorage, spec)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "ubuntu", "18.04")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "ubuntu", "18.04")

	expectedOutput := "" +
		"Located charm \"mysql\" in charm-hub, channel stable\n" +
		"Located charm \"wordpress\" in charm-hub, channel stable\n" +
		"Executing changes:\n" +
		"- upload charm mysql from charm-hub for series bionic with revision 42 with architecture=amd64\n" +
		"- deploy application mysql from charm-hub on bionic with stable\n" +
		"- upload charm wordpress from charm-hub for series bionic with revision 47 with architecture=amd64\n" +
		"- deploy application wordpress from charm-hub on bionic with stable\n" +
		"- add new machine 0\n" +
		"- add new machine 1\n" +
		"- add relation wordpress:db - mysql:db\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit wordpress/0 to new machine 1\n" +
		"Deploy of bundle completed.\n"

	changeOutput := "" +
		"Located charm \"mysql\" in charm-hub, channel stable\n" +
		"Located charm \"wordpress\" in charm-hub, channel stable\n" +
		"Executing changes:\n" +
		"- upload charm mysql from charm-hub for series bionic with revision 42 with architecture=amd64\n" +
		"- upgrade mysql from charm-hub using charm mysql for series bionic from channel stable\n" +
		"- upload charm wordpress from charm-hub for series bionic with revision 47 with architecture=amd64\n" +
		"- upgrade wordpress from charm-hub using charm wordpress for series bionic from channel stable\n" +
		"Deploy of bundle completed.\n"

	dryRunOutput := "" +
		"Located charm \"mysql\" in charm-hub, channel stable\n" +
		"Located charm \"wordpress\" in charm-hub, channel stable\n" +
		"Changes to deploy bundle:\n" +
		"- upload charm mysql from charm-hub for series bionic with revision 42 with architecture=amd64\n" +
		"- upgrade mysql from charm-hub using charm mysql for series bionic from channel stable\n" +
		"- upload charm wordpress from charm-hub for series bionic with revision 47 with architecture=amd64\n" +
		"- upgrade wordpress from charm-hub using charm wordpress for series bionic from channel stable\n"
	c.Check(s.output.String(), gc.Equals, expectedOutput)

	// Setup to run with --dry-run, no changes
	s.expectDeployerAPIStatusWordpressBundle()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()

	if dryRun {
		changeOutput = dryRunOutput
	}
	spec.dryRun = dryRun
	spec.useExistingMachines = true
	spec.bundleMachines = map[string]string{}
	s.runDeployWithSpec(c, wordpressBundleWithStorage, spec)
	c.Check(s.output.String(), gc.Equals, expectedOutput+changeOutput)
}

const charmWithResourcesBundle = `
       applications:
           django:
               charm: ch:django
               series: xenial
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleResources(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	djangoCurl := charm.MustParseURL("ch:django")
	charmInfo := &apicharms.CharmInfo{
		Revision: djangoCurl.Revision,
		URL:      djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
			Resources: map[string]charmresource.Meta{
				"one": {Type: charmresource.TypeFile},
				"two": {Type: charmresource.TypeFile},
			},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo)
	s.expectDeploy()

	spec := s.bundleDeploySpec()
	spec.deployResources = func(
		_ string,
		_ resources.CharmID,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		_ base.APICallCloser,
		_ modelcmd.Filesystem,
	) (ids map[string]string, err error) {
		c.Assert(resources, gc.DeepEquals, charmInfo.Meta.Resources)
		results := make(map[string]string, len(resources))
		for k := range resources {
			results[k] = "1"
		}
		return results, nil
	}

	s.runDeployWithSpec(c, charmWithResourcesBundle, spec)
	c.Assert(strings.Contains(s.output.String(), "added resource one"), jc.IsTrue)
	c.Assert(strings.Contains(s.output.String(), "added resource two"), jc.IsTrue)
}

const specifyResourcesBundle = `
       applications:
           django:
               charm: ch:django
               series: xenial
               resources:
                   one: 4
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleSpecifyResources(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	djangoCurl := charm.MustParseURL("ch:django")
	charmInfo := &apicharms.CharmInfo{
		Revision: djangoCurl.Revision,
		URL:      djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
			Resources: map[string]charmresource.Meta{
				"one": {Type: charmresource.TypeFile},
				"two": {Type: charmresource.TypeFile},
			},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo)
	s.expectDeploy()

	spec := s.bundleDeploySpec()
	spec.deployResources = func(
		_ string,
		_ resources.CharmID,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		_ base.APICallCloser,
		_ modelcmd.Filesystem,
	) (ids map[string]string, err error) {
		c.Assert(resources, gc.DeepEquals, charmInfo.Meta.Resources)
		c.Assert(filesAndRevisions, gc.DeepEquals, map[string]string{"one": "4"})
		results := make(map[string]string, len(resources))
		for k := range resources {
			results[k] = "1"
		}
		return results, nil
	}

	s.runDeployWithSpec(c, specifyResourcesBundle, spec)
	c.Assert(strings.Contains(s.output.String(), "added resource one"), jc.IsTrue)
	c.Assert(strings.Contains(s.output.String(), "added resource two"), jc.IsTrue)
}

const wordpressBundleWithStorageUpgradeConstraints = `
series: bionic
applications:
  wordpress:
    charm: ch:wordpress
    revision: 52
    channel: stable
    num_units: 1
    options:
      blog-title: new title
    constraints: spaces=new cores=8
    to:
    - "1"
  mysql:
    charm: ch:mysql
    revision: 42
    channel: stable
    num_units: 1
    storage:
      database: mysql-pv,20M
    to:
    - "0"
machines:
  "0": {}
  "1": {}
relations:
- - wordpress:db
  - mysql:db
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleApplicationUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectDeployerAPIStatusWordpressBundle()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()
	s.expectResolveCharm(nil)
	s.expectResolveCharm(nil)

	mysqlCurl := charm.MustParseURL("ch:mysql")
	s.expectAddCharm(false)
	s.expectSetCharm(c, "mysql")
	charmInfo := &apicharms.CharmInfo{
		URL: mysqlCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(mysqlCurl.String(), charmInfo)

	wordpressCurl := charm.MustParseURL("ch:wordpress")
	s.expectAddCharm(false)
	s.expectSetCharm(c, "wordpress")
	wpCharmInfo := &apicharms.CharmInfo{
		URL: wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), wpCharmInfo)

	s.expectSetConfig(c, "wordpress", map[string]interface{}{"blog-title": "new title"})
	s.expectSetConstraints("wordpress", "spaces=new cores=8")

	s.runDeploy(c, wordpressBundleWithStorageUpgradeConstraints)

	c.Assert(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-hub, channel stable\n"+
		"Located charm \"wordpress\" in charm-hub, channel stable\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-hub for series bionic with revision 42 with architecture=amd64\n"+
		"- upgrade mysql from charm-hub using charm mysql for series bionic from channel stable\n"+
		"- upload charm wordpress from charm-hub for series bionic with revision 52 with architecture=amd64\n"+
		"- upgrade wordpress from charm-hub using charm wordpress for series bionic from channel stable\n"+
		"- set application options for wordpress\n"+
		"- set constraints for wordpress to \"spaces=new cores=8\"\n"+
		"Deploy of bundle completed.\n",
	)
}

const wordpressBundleWithStorageUpgradeRelations = `
series: bionic
applications:
  mysql:
    charm: ch:mysql
    num_units: 1
    storage:
      database: mysql-pv,20M
    to:
    - "0"
  wordpress:
    charm: ch:wordpress
    num_units: 1
    to:
    - "1"
  varnish:
    charm: ch:varnish
    num_units: 1
    to: 
    - "2"
machines:
  "0": {}
  "1": {}
  "2": {}
relations:
- ["wordpress:db", "mysql:db"]
- ["varnish:webcache", "wordpress:cache"]
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleNewRelations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectDeployerAPIStatusWordpressBundle()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()
	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	s.expectAddCharm(false)
	s.expectAddCharm(false)
	s.expectCharmInfo("ch:mysql", &apicharms.CharmInfo{Meta: &charm.Meta{}})
	s.expectCharmInfo("ch:varnish", &apicharms.CharmInfo{Meta: &charm.Meta{}})
	s.expectCharmInfo("ch:wordpress", &apicharms.CharmInfo{Meta: &charm.Meta{}})
	s.expectSetCharm(c, "mysql")
	s.expectSetCharm(c, "wordpress")
	s.expectDeploy()
	s.expectAddMachine("0", "18.04")
	s.expectAddOneUnit("varnish", "0", "0")

	s.expectAddRelation([]string{"varnish:webcache", "wordpress:cache"})

	s.runDeploy(c, wordpressBundleWithStorageUpgradeRelations)

	c.Assert(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-hub\n"+
		"Located charm \"varnish\" in charm-hub\n"+
		"Located charm \"wordpress\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-hub for series bionic with architecture=amd64\n"+
		"- upgrade mysql from charm-hub using charm mysql for series bionic\n"+
		"- upload charm varnish from charm-hub for series bionic with architecture=amd64\n"+
		"- deploy application varnish from charm-hub on bionic\n"+
		"- upload charm wordpress from charm-hub for series bionic with architecture=amd64\n"+
		"- upgrade wordpress from charm-hub using charm wordpress for series bionic\n"+
		"- add new machine 2\n"+
		"- add relation varnish:webcache - wordpress:cache\n"+
		"- add unit varnish/0 to new machine 2\n"+
		"Deploy of bundle completed.\n",
	)
}

const machineUnitPlacementBundle = `
      applications:
          wordpress:
              charm: ch:xenial/wordpress
              num_units: 2
              to:
                  - 1
                  - lxd:2
              options:
                  blog-title: these are the voyages
          mysql:
              charm: ch:xenial/mysql
              num_units: 2
              to:
                  - lxd:wordpress/0
                  - new
      machines:
          1:
              series: xenial
          2:
              series: xenial
  `

func (s *BundleDeployRepositorySuite) TestDeployBundleMachinesUnitsPlacement(c *gc.C) {
	c.Skip("Won't work until LP:1940558 is fixed.")
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	s.expectAddMachine("0", "16.04")
	s.expectAddMachine("1", "16.04")
	s.expectAddMachine("2", "16.04")
	s.expectAddContainer("0", "0/lxd/0", "16.04", "lxd")
	s.expectAddContainer("1", "1/lxd/0", "16.04", "lxd")

	wordpressCurl := charm.MustParseURL("ch:wordpress")
	s.expectResolveCharm(nil)
	charmInfo := &apicharms.CharmInfo{
		Revision: wordpressCurl.Revision,
		URL:      wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), charmInfo)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("wordpress", "0", "0")
	s.expectAddOneUnit("wordpress", "1/lxd/0", "1")

	mysqlCurl := charm.MustParseURL("ch:mysql")
	s.expectResolveCharm(nil)
	charmInfo2 := &apicharms.CharmInfo{
		Revision: mysqlCurl.Revision,
		URL:      mysqlCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(mysqlCurl.String(), charmInfo2)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("mysql", "0/lxd/0", "0")
	s.expectAddOneUnit("mysql", "2", "1")

	s.runDeploy(c, machineUnitPlacementBundle)
}

const machineAttributesBundle = `
       applications:
           django:
               charm: ch:django
               series: xenial
               num_units: 2
               to:
                   - 1
                   - new
       machines:
           1:
               series: xenial
               constraints: "cores=4 mem=4G"
               annotations:
                   foo: bar
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleMachineAttributes(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	djangoCurl := charm.MustParseURL("ch:django")
	charmInfo := &apicharms.CharmInfo{
		Revision: djangoCurl.Revision,
		URL:      djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo)
	args := []params.AddMachineParams{
		{
			Constraints:   constraints.MustParse("cores=4 mem=4G"),
			ContainerType: instance.ContainerType(""),
			Jobs:          []model.MachineJob{model.JobHostUnits},
			Base:          &params.Base{Name: "ubuntu", Channel: "16.04/stable"},
		},
	}
	results := []params.AddMachinesResult{
		{Machine: "0"},
	}
	s.deployerAPI.EXPECT().AddMachines(args).Return(results, nil)
	s.expectAddMachine("1", "16.04")
	s.expectDeploy()
	s.expectAddOneUnit("django", "0", "0")
	s.expectAddOneUnit("django", "1", "1")
	s.expectSetAnnotation("machine-0", map[string]string{"foo": "bar"})

	s.runDeploy(c, machineAttributesBundle)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleTwiceScaleUp(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()
	s.expectResolveCharm(nil)

	djangoCurl := charm.MustParseURL("ch:django")
	s.expectResolveCharmWithSeries([]string{"ubuntu@18.04", "ubuntu@16.04"}, nil)
	s.expectAddCharm(false)
	s.expectAddCharm(false)
	charmInfo := &apicharms.CharmInfo{
		URL: djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo)
	s.expectCharmInfo(djangoCurl.String(), charmInfo)
	s.expectSetCharm(c, "django")
	s.expectDeploy()
	s.expectAddOneUnit("django", "", "0")
	s.expectAddOneUnit("django", "", "1")

	s.runDeploy(c, `
       applications:
           django:
               charm: ch:django
               series: xenial
               num_units: 2
   `)

	// Setup for scaling by 3 units
	s.expectDeployerAPIStatusDjango2Units()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()
	s.expectAddOneUnit("django", "", "2")
	s.expectAddOneUnit("django", "", "3")
	s.expectAddOneUnit("django", "", "4")

	s.runDeploy(c, `
       applications:
           django:
               charm: ch:django
               series: xenial
               num_units: 5
   `)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleUnitPlacedInApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()
	s.expectResolveCharm(nil)

	wordpressCurl := charm.MustParseURL("ch:bionic/wordpress")
	s.expectResolveCharm(nil)
	charmInfo := &apicharms.CharmInfo{
		Revision: wordpressCurl.Revision,
		URL:      wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), charmInfo)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("wordpress", "", "0")
	s.expectAddOneUnit("wordpress", "", "1")
	s.expectAddOneUnit("wordpress", "", "2")

	s.allWatcher.EXPECT().Next().Return([]params.Delta{
		{Entity: &params.UnitInfo{Name: "wordpress/0", MachineId: "0"}},
		{Entity: &params.UnitInfo{Name: "wordpress/1", MachineId: "1"}},
	}, nil)

	djangoCurl := charm.MustParseURL("ch:bionic/django")
	s.expectResolveCharm(nil)
	charmInfo2 := &apicharms.CharmInfo{
		Revision: djangoCurl.Revision,
		URL:      djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo2)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("django", "0", "0")
	s.expectAddOneUnit("django", "1", "1")

	s.runDeploy(c, `
       applications:
           wordpress:
               charm: ch:wordpress
               num_units: 3
           django:
               charm: ch:django
               num_units: 2
               to: [wordpress]
   `)
}

const peerContainerBundle = `
       applications:
           wordpress:
               charm: ch:wordpress
               num_units: 2
               to: ["lxd:new"]
           django:
               charm: ch:django
               num_units: 2
               to: ["lxd:wordpress"]
   `

func (s *BundleDeployRepositorySuite) TestDeployBundlePeerContainer(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	// Order is important here, to ensure containers get the correct machineID.
	s.expectAddContainer("", "0/lxd/0", "", "lxd")
	s.expectAddContainer("", "1/lxd/0", "", "lxd")
	s.expectAddContainer("0", "0/lxd/1", "", "lxd")
	s.expectAddContainer("1", "1/lxd/1", "", "lxd")

	wordpressCurl := charm.MustParseURL("ch:bionic/wordpress")
	s.expectResolveCharm(nil)
	charmInfo := &apicharms.CharmInfo{
		URL: wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), charmInfo)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("wordpress", "0/lxd/0", "0")
	s.expectAddOneUnit("wordpress", "1/lxd/0", "1")

	djangoCurl := charm.MustParseURL("ch:bionic/django")
	s.expectResolveCharm(nil)
	charmInfo2 := &apicharms.CharmInfo{
		URL: djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo2)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("django", "0/lxd/1", "0")
	s.expectAddOneUnit("django", "1/lxd/1", "1")

	s.runDeploy(c, peerContainerBundle)

	c.Assert(strings.Contains(s.output.String(), "add unit django/0 to 0/lxd/1 to satisfy [lxd:wordpress]"), jc.IsTrue)
	c.Assert(strings.Contains(s.output.String(), "add unit django/1 to 1/lxd/1 to satisfy [lxd:wordpress]"), jc.IsTrue)
}

const unitColocationWithUnitBundle = `
       applications:
           mem:
               charm: ch:mem
               revision: 47
               channel: stable
               series: xenial
               num_units: 3
               to: [1, new]
           django:
               charm: ch:django
               revision: 42
               channel: stable
               series: xenial
               num_units: 5
               to:
                   - mem/0
                   - lxd:mem/1
                   - lxd:mem/2
                   - kvm:ror
           ror:
               charm: ch:rails
               series: xenial
               num_units: 2
               to:
                   - new
                   - 1
       machines:
           1:
               series: xenial
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleUnitColocationWithUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	// Setup Machines and Containers
	s.expectAddMachine("0", "16.04")
	s.expectAddMachine("1", "16.04")
	s.expectAddMachine("2", "16.04")
	s.expectAddMachine("3", "16.04")
	s.expectAddContainer("1", "1/lxd/0", "16.04", "lxd")
	s.expectAddContainer("2", "2/lxd/0", "16.04", "lxd")
	s.expectAddContainer("3", "3/kvm/0", "16.04", "kvm")
	s.expectAddContainer("0", "0/kvm/0", "16.04", "kvm")

	// Setup for mem charm
	memCurl := charm.MustParseURL("ch:mem")
	s.expectResolveCharm(nil)
	charmInfo := &apicharms.CharmInfo{
		Revision: memCurl.Revision,
		URL:      memCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(memCurl.String(), charmInfo)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("mem", "0", "0")
	s.expectAddOneUnit("mem", "1", "1")
	s.expectAddOneUnit("mem", "2", "2")

	// Setup for django charm
	djangoCurl := charm.MustParseURL("ch:django")
	s.expectResolveCharm(nil)
	charmInfo2 := &apicharms.CharmInfo{
		Revision: djangoCurl.Revision,
		URL:      djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo2)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("django", "0", "0")
	s.expectAddOneUnit("django", "1/lxd/0", "1")
	s.expectAddOneUnit("django", "2/lxd/0", "2")
	s.expectAddOneUnit("django", "3/kvm/0", "3")
	s.expectAddOneUnit("django", "0/kvm/0", "4")

	// Setup for rails charm
	railsCurl := charm.MustParseURL("ch:rails")
	s.expectResolveCharm(nil)
	charmInfo3 := &apicharms.CharmInfo{
		Revision: railsCurl.Revision,
		URL:      railsCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(railsCurl.String(), charmInfo3)
	s.expectAddCharm(false)
	s.expectDeploy()
	s.expectAddOneUnit("ror", "0", "0")
	s.expectAddOneUnit("ror", "3", "1")

	// Run the bundle
	s.runDeploy(c, unitColocationWithUnitBundle)
}

const switchBundle = `
       applications:
           django:
               charm: ch:django
               series: bionic
               num_units: 1
           rails:
               charm: ch:rails
               series: bionic
               num_units: 1
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleSwitch(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectDeployerAPIStatusDjangoMemBundle()
	s.expectEmptyModelRepresentationNotAnnotations()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()

	s.expectGetAnnotationsEmpty()

	djangoCurl := charm.MustParseURL("ch:django")
	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	charmInfo := &apicharms.CharmInfo{
		URL: djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo)
	s.expectSetCharm(c, "django")
	s.expectDeploy()

	railsCurl := charm.MustParseURL("ch:rails")
	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	rCharmInfo := &apicharms.CharmInfo{
		Revision: railsCurl.Revision,
		URL:      railsCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(railsCurl.String(), rCharmInfo)
	s.expectAddOneUnit(railsCurl.Name, "", "0")

	// Redeploy a very similar bundle with another application unit. The new unit
	// is placed on the first unit of memcached. Due to ordering of the applications
	// there is no deterministic way to determine "least crowded" in a meaningful way.
	s.runDeploy(c, switchBundle)
}

const annotationsBundle = `
        applications:
            django:
                charm: ch:django
                num_units: 1
                annotations:
                    key1: value1
                    key2: value2
                to: [1]
            mem:
                charm: ch:mem
                num_units: 1
                to: [0]
        machines:
            0:
                series: bionic
            1:
                annotations: {foo: bar}
                series: bionic
    `

func (s *BundleDeployRepositorySuite) TestDeployBundleAnnotations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	djangoCurl := charm.MustParseURL("ch:bionic/django")
	memCurl := charm.MustParseURL("ch:bionic/mem")
	chUnits := []charmUnit{
		{
			curl:                 memCurl,
			charmMetaSeries:      []string{"bionic", "focal"},
			machine:              "0",
			machineUbuntuVersion: "18.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 djangoCurl,
			machine:              "1",
			machineUbuntuVersion: "18.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectSetAnnotation("application-django", map[string]string{"key1": "value1", "key2": "value2"})
	s.expectSetAnnotation("machine-1", map[string]string{"foo": "bar"})

	s.runDeploy(c, annotationsBundle)
}

const annotationsChangeBundle = `
        applications:
            django:
                charm: ch:django
                num_units: 1
                annotations:
                    key1: new value!
                    key2: value2
                to: [1]
        machines:
            1:
                annotations: {answer: 42}
                series: bionic
    `

func (s *BundleDeployRepositorySuite) TestDeployBundleAnnotationsChanges(c *gc.C) {
	// Follow on to TestDeployBundleAnnotations
	defer s.setupMocks(c).Finish()
	s.expectDeployerAPIStatusDjangoBundle()

	tags := []string{"machine-1", "application-django"}
	annotations := []map[string]string{
		{"foo": "bar"},
		{"key1": "value1", "key2": "value2"},
	}
	s.expectGetAnnotations(tags, annotations)

	s.expectEmptyModelRepresentationNotAnnotations()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()
	s.expectResolveCharmWithSeries([]string{"ubuntu@18.04", "ubuntu@16.04"}, nil)
	s.expectAddCharm(false)
	s.expectCharmInfo("ch:bionic/django", &apicharms.CharmInfo{
		URL:  "ch:bionic/django",
		Meta: &charm.Meta{},
	})

	s.expectSetCharm(c, "django")
	s.expectSetAnnotation("application-django", map[string]string{"key1": "new value!"})
	s.expectSetAnnotation("machine-1", map[string]string{"answer": "42"})

	s.runDeploy(c, annotationsChangeBundle)
}

func (s *BundleDeployRepositorySuite) expectSetAnnotation(entity string, annotations map[string]string) {
	s.deployerAPI.EXPECT().SetAnnotation(map[string]map[string]string{entity: annotations}).Return(nil, nil)
}

func (s *BundleDeployRepositorySuite) expectGetAnnotations(args []string, annotations []map[string]string) {
	results := make([]params.AnnotationsGetResult, len(args))
	for i, tag := range args {
		results[i] = params.AnnotationsGetResult{EntityTag: tag, Annotations: annotations[i]}
	}
	s.deployerAPI.EXPECT().GetAnnotations(args).Return(results, nil)
}

func (s *BundleDeployRepositorySuite) expectGetAnnotationsEmpty() {
	s.deployerAPI.EXPECT().GetAnnotations(gomock.Any()).DoAndReturn(
		func(tags []string) ([]params.AnnotationsGetResult, error) {
			results := make([]params.AnnotationsGetResult, len(tags))
			for i, tag := range tags {
				results[i] = params.AnnotationsGetResult{EntityTag: tag, Annotations: map[string]string{}}
			}
			return results, nil
		})
}

func (s *BundleDeployRepositorySuite) TestDeployBundleInvalidMachineContainerType(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl := charm.MustParseURL("ch:wordpress")
	s.expectAddCharm(false)
	s.expectResolveCharm(nil)
	charmInfo := &apicharms.CharmInfo{
		Revision: wordpressCurl.Revision,
		URL:      wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), charmInfo)
	s.expectDeploy()
	s.expectAddMachine("1", "18.04")

	quickBundle := `
       series: bionic
       applications:
           wp:
               charm: ch:wordpress
               num_units: 1
               to: ["bad:1"]
       machines:
           1:
   `

	bundleData, err := charm.ReadBundleData(strings.NewReader(quickBundle))
	c.Assert(err, jc.ErrorIsNil)
	err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, `cannot create machine for holding wp unit: invalid container type "bad"`)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleUnitPlacedToMachines(c *gc.C) {
	s.testDeployBundleUnitPlacedToMachines(c)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleUnitPlacedToMachinesDebug(c *gc.C) {
	level := logger.EffectiveLogLevel()
	logger.SetLogLevel(loggo.DEBUG)
	s.testDeployBundleUnitPlacedToMachines(c)
	logger.SetLogLevel(level)
	loggo.ResetLogging()
}

func (s *BundleDeployRepositorySuite) testDeployBundleUnitPlacedToMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl := charm.MustParseURL("ch:wordpress")
	s.expectAddCharm(false)
	s.expectResolveCharm(nil)
	charmInfo := &apicharms.CharmInfo{
		Revision: wordpressCurl.Revision,
		URL:      wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), charmInfo)
	s.expectDeploy()
	s.expectAddMachine("0", "18.04")
	s.expectAddContainer("0", "0/lxd/0", "18.04", "lxd")
	s.expectAddMachine("1", "18.04")
	s.expectAddContainer("1", "1/kvm/0", "18.04", "kvm")
	s.expectAddMachine("2", "18.04")
	s.expectAddContainer("", "3/lxd/0", "18.04", "lxd")
	s.expectAddContainer("", "4/lxd/0", "18.04", "lxd")
	s.expectAddContainer("", "5/lxd/0", "18.04", "lxd")
	s.expectAddOneUnit("wp", "2", "0")
	s.expectAddOneUnit("wp", "0", "1")
	s.expectAddOneUnit("wp", "1/kvm/0", "2")
	s.expectAddOneUnit("wp", "0/lxd/0", "3")
	s.expectAddOneUnit("wp", "3/lxd/0", "4")
	s.expectAddOneUnit("wp", "4/lxd/0", "5")
	s.expectAddOneUnit("wp", "5/lxd/0", "6")

	quickBundle := `
       series: bionic
       applications:
           wp:
               charm: ch:wordpress
               num_units: 7
               to:
                   - new
                   - 4
                   - kvm:8
                   - lxd:4
                   - lxd:new
       machines:
           4:
           8:
   `

	s.runDeploy(c, quickBundle)

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-hub for series bionic with architecture=amd64\n"+
		"- deploy application wp from charm-hub on bionic using wordpress\n"+
		"- add new machine 0 (bundle machine 4)\n"+
		"- add new machine 1 (bundle machine 8)\n"+
		"- add new machine 2\n"+
		"- add kvm container 1/kvm/0 on new machine 1\n"+
		"- add lxd container 0/lxd/0 on new machine 0\n"+
		"- add lxd container 3/lxd/0 on new machine 3\n"+
		"- add lxd container 4/lxd/0 on new machine 4\n"+
		"- add lxd container 5/lxd/0 on new machine 5\n"+
		"- add unit wp/0 to new machine 2\n"+
		"- add unit wp/1 to new machine 0\n"+
		"- add unit wp/2 to 1/kvm/0\n"+
		"- add unit wp/3 to 0/lxd/0\n"+
		"- add unit wp/4 to 3/lxd/0\n"+
		"- add unit wp/5 to 4/lxd/0\n"+
		"- add unit wp/6 to 5/lxd/0\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleExpose(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl := charm.MustParseURL("ch:bionic/wordpress")
	chUnits := []charmUnit{
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 wordpressCurl,
			machineUbuntuVersion: "18.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectExpose(wordpressCurl.Name)

	content := `
       applications:
           wordpress:
               charm: ch:wordpress
               num_units: 1
               expose: true
   `
	s.runDeploy(c, content)

	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "ubuntu", "18.04")
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-hub with architecture=amd64\n"+
		"- deploy application wordpress from charm-hub\n"+
		"- expose all endpoints of wordpress and allow access from CIDRs 0.0.0.0/0 and ::/0\n"+
		"- add unit wordpress/0 to new machine 0\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleMultipleRelations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl := charm.MustParseURL("ch:wordpress")
	mysqlCurl := charm.MustParseURL("ch:mysql")
	pgresCurl := charm.MustParseURL("ch:postgres")
	varnishCurl := charm.MustParseURL("ch:varnish")
	chUnits := []charmUnit{
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 mysqlCurl,
			machineUbuntuVersion: "18.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 pgresCurl,
			machineUbuntuVersion: "16.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 varnishCurl,
			machineUbuntuVersion: "16.04",
		},
		{
			charmMetaSeries:      []string{"bionic", "xenial"},
			curl:                 wordpressCurl,
			machineUbuntuVersion: "18.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:server"})
	s.expectAddRelation([]string{"varnish:webcache", "wordpress:cache"})
	content := `
       series: bionic
       applications:
           wordpress:
               charm: ch:wordpress
               num_units: 1
           mysql:
               charm: ch:mysql
               num_units: 1
           postgres:
               charm: ch:xenial/postgres
               num_units: 1
           varnish:
               charm: ch:xenial/varnish
               num_units: 1
       relations:
           - ["wordpress:db", "mysql:server"]
           - ["varnish:webcache", "wordpress:cache"]
   `
	s.runDeploy(c, content)

	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "ubuntu", "18.04")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "ubuntu", "18.04")
	s.assertDeployArgs(c, varnishCurl.String(), "varnish", "ubuntu", "18.04")
	s.assertDeployArgs(c, pgresCurl.String(), "postgres", "ubuntu", "18.04")
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-hub\n"+
		"Located charm \"postgres\" in charm-hub\n"+
		"Located charm \"varnish\" in charm-hub\n"+
		"Located charm \"wordpress\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-hub for series bionic with architecture=amd64\n"+
		"- deploy application mysql from charm-hub on bionic\n"+
		"- upload charm postgres from charm-hub for series bionic with architecture=amd64\n"+
		"- deploy application postgres from charm-hub on bionic\n"+
		"- upload charm varnish from charm-hub for series bionic with architecture=amd64\n"+
		"- deploy application varnish from charm-hub on bionic\n"+
		"- upload charm wordpress from charm-hub for series bionic with architecture=amd64\n"+
		"- deploy application wordpress from charm-hub on bionic\n"+
		"- add relation wordpress:db - mysql:server\n"+
		"- add relation varnish:webcache - wordpress:cache\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit postgres/0 to new machine 1\n"+
		"- add unit varnish/0 to new machine 2\n"+
		"- add unit wordpress/0 to new machine 3\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleLocalDeployment(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("local:jammy/mysql-1")
	wordpressCurl := charm.MustParseURL("local:jammy/wordpress-3")
	chUnits := []charmUnit{
		{
			curl:                 mysqlCurl,
			charmMetaSeries:      []string{"focal", "jammy"},
			machineUbuntuVersion: "20.04",
		},
		{
			charmMetaSeries:      []string{"focal", "jammy"},
			curl:                 wordpressCurl,
			machineUbuntuVersion: "20.04",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddOneUnit("mysql", "", "1")
	s.expectAddRelation([]string{"wordpress:db", "mysql:server"})

	content := `
      series: focal
      applications:
          wordpress:
              charm: %s
              num_units: 1
          mysql:
              charm: %s
              num_units: 2
      relations:
          - ["wordpress:db", "mysql:server"]
`
	charmsPath := c.MkDir()
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
	bundle := fmt.Sprintf(content, wordpressPath, mysqlPath)
	bundleData, err := charm.ReadBundleData(strings.NewReader(bundle))
	c.Assert(err, jc.ErrorIsNil)

	err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "ubuntu", "20.04")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "ubuntu", "20.04")
	expectedOutput := "" +
		"Executing changes:\n" +
		"- upload charm %s for series focal with architecture=amd64\n" +
		"- deploy application mysql on focal\n" +
		"- upload charm %s for series focal with architecture=amd64\n" +
		"- deploy application wordpress on focal\n" +
		"- add relation wordpress:db - mysql:server\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit mysql/1 to new machine 1\n" +
		"- add unit wordpress/0 to new machine 2\n" +
		"Deploy of bundle completed.\n"

	c.Check(s.output.String(), gc.Equals, fmt.Sprintf(expectedOutput, mysqlPath, wordpressPath))
}

func (s *BundleDeployRepositorySuite) TestApplicationsForMachineChange(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectResolveCharmWithSeries([]string{"ubuntu@16.04"}, nil)
	s.expectResolveCharmWithSeries([]string{"ubuntu@16.04"}, nil)
	spec := s.bundleDeploySpec()
	bundleData, err := charm.ReadBundleData(strings.NewReader(machineUnitPlacementBundle))
	c.Assert(err, jc.ErrorIsNil)

	h := makeBundleHandler(charm.CharmHub, bundleData, spec)
	err = h.getChanges()
	c.Assert(err, jc.ErrorIsNil)

	var count int
	for _, change := range h.changes {
		switch change := change.(type) {
		case *bundlechanges.AddMachineChange:
			applications := h.applicationsForMachineChange(change)
			switch change.Params.Machine() {
			case "0":
				c.Assert(applications, jc.SameContents, []string{"mysql", "wordpress"})
				count += 1
			case "1":
				c.Assert(applications, jc.SameContents, []string{"wordpress"})
				count += 1
			case "2":
				c.Assert(applications, jc.SameContents, []string{"mysql"})
				count += 1
			case "0/lxd/0":
				c.Assert(applications, jc.SameContents, []string{"mysql"})
				count += 1
			case "1/lxd/0":
				c.Assert(applications, jc.SameContents, []string{"wordpress"})
				count += 1
			default:
				c.Fatalf("%q not expected machine", change.Params.Machine())
			}
		}
	}

	c.Assert(count, gc.Equals, 5, gc.Commentf("All 5 AddMachineChanges not found"))
}

func (s *BundleDeployRepositorySuite) TestDeployBundleWithEndpointBindings(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	grafanaCurl, err := charm.ParseURL("ch:grafana")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{{
		curl:                 grafanaCurl,
		charmMetaSeries:      []string{"bionic", "xenial"},
		machine:              "0",
		machineUbuntuVersion: "18.04",
	}}
	s.setupCharmUnits(chUnits)

	bundleData, err := charm.ReadBundleData(strings.NewReader(grafanaBundleEndpointBindings))
	c.Assert(err, jc.ErrorIsNil)
	bundleDeploymentSpec := s.bundleDeploySpec()
	bundleDeploymentSpec.knownSpaceNames = set.NewStrings("alpha", "beta")

	err = bundleDeploy(charm.CharmHub, bundleData, bundleDeploymentSpec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleWithInvalidEndpointBindings(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	s.expectResolveCharm(nil)
	s.expectAddCharm(false)

	bundleData, err := charm.ReadBundleData(strings.NewReader(grafanaBundleEndpointBindings))
	c.Assert(err, jc.ErrorIsNil)
	bundleDeploymentSpec := s.bundleDeploySpec()
	bundleDeploymentSpec.knownSpaceNames = set.NewStrings("alpha")

	err = bundleDeploy(charm.CharmHub, bundleData, bundleDeploymentSpec)
	c.Assert(err, gc.ErrorMatches, `space "beta" not found`)
}

const grafanaBundleEndpointBindings = `
series: bionic
applications:
  grafana:
    charm: grafana
    num_units: 1
    series: bionic
    to:
    - "0"
    bindings:
      "": alpha
      "db": beta
machines:
  "0":
    series: bionic
`

func (s *BundleDeployRepositorySuite) bundleDeploySpec() bundleDeploySpec {
	return s.bundleDeploySpecWithConstraints(constraints.Value{})
}

func (s *BundleDeployRepositorySuite) bundleDeploySpecWithConstraints(cons constraints.Value) bundleDeploySpec {
	deployResourcesFunc := func(_ string,
		_ resources.CharmID,
		_ map[string]string,
		_ map[string]charmresource.Meta,
		_ base.APICallCloser,
		_ modelcmd.Filesystem,
	) (_ map[string]string, _ error) {
		return nil, nil
	}

	return bundleDeploySpec{
		deployAPI: s.deployerAPI,
		ctx: &cmd.Context{
			Stderr: s.stdErr,
			Stdout: s.stdOut,
		},
		bundleResolver:   s.bundleResolver,
		deployResources:  deployResourcesFunc,
		modelConstraints: cons,
	}
}

func (s *BundleDeployRepositorySuite) assertDeployArgs(c *gc.C, curl, appName, os, channel string) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args %s", appName))
	c.Assert(arg.CharmID.URL.String(), gc.Equals, curl)
	c.Assert(arg.CharmOrigin.Base.OS, gc.Equals, os)
	c.Assert(arg.CharmOrigin.Base.Channel.Track, gc.Equals, channel, gc.Commentf("%s", pretty.Sprint(arg)))
}

func (s *BundleDeployRepositorySuite) assertDeployArgsStorage(c *gc.C, appName string, storage map[string]storage.Constraints) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.Storage, gc.DeepEquals, storage)
}

func (s *BundleDeployRepositorySuite) assertDeployArgsConfig(c *gc.C, appName string, options map[string]interface{}) {
	cfg, err := yaml.Marshal(map[string]map[string]interface{}{appName: options})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("cannot marshal options for application %q", appName))
	configYAML := string(cfg)
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.ConfigYAML, gc.DeepEquals, configYAML)
}

func (s *BundleDeployRepositorySuite) assertDeployArgsDevices(c *gc.C, appName string, devices map[string]devices.Constraints) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.Devices, gc.DeepEquals, devices)
}

type charmUnit struct {
	curl                 *charm.URL
	charmMetaSeries      []string
	force                bool
	machine              string
	machineUbuntuVersion string
}

func (s *BundleDeployRepositorySuite) setupCharmUnits(charmUnits []charmUnit) {
	for _, chUnit := range charmUnits {
		switch chUnit.curl.Schema {
		case "ch":
			resolveBases := []string{"ubuntu@18.04", "ubuntu@20.04", "ubuntu@16.04"}
			s.expectResolveCharmWithSeries(resolveBases, nil)
			s.expectAddCharm(chUnit.force)
		case "local":
			s.expectAddLocalCharm(chUnit.curl, chUnit.force)
		}
		charmInfo := &apicharms.CharmInfo{
			Revision: chUnit.curl.Revision,
			URL:      chUnit.curl.String(),
			Meta: &charm.Meta{
				Series: chUnit.charmMetaSeries,
			},
		}
		s.expectCharmInfo(chUnit.curl.String(), charmInfo)
		s.expectDeploy()
		if chUnit.machineUbuntuVersion != "kubernetes" {
			s.expectAddMachine(chUnit.machine, chUnit.machineUbuntuVersion)
			s.expectAddOneUnit(chUnit.curl.Name, chUnit.machine, "0")
		}
	}
}

func (s *BundleDeployRepositorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	s.deployerAPI.EXPECT().BestFacadeVersion("Resources").Return(666).AnyTimes()
	s.deployerAPI.EXPECT().BestFacadeVersion("Charms").Return(666).AnyTimes()
	s.deployerAPI.EXPECT().HTTPClient().Return(&httprequest.Client{}, nil).AnyTimes()
	s.bundleResolver = mocks.NewMockResolver(ctrl)
	s.allWatcher = mocks.NewMockAllWatch(ctrl)
	s.stdOut = mocks.NewMockWriter(ctrl)
	s.stdErr = mocks.NewMockWriter(ctrl)
	logOutput := func(p []byte) {
		c.Logf("%q", p)
		// s.output is setup in SetUpTest
		s.output.Write(p)
	}
	s.stdOut.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes().Do(logOutput)
	s.stdErr.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes().Do(logOutput)
	return ctrl
}

func (s *BundleDeployRepositorySuite) runDeploy(c *gc.C, bundle string) {
	spec := s.bundleDeploySpec()
	s.runDeployWithSpec(c, bundle, spec)
}

func (s *BundleDeployRepositorySuite) runDeployWithSpec(c *gc.C, bundle string, spec bundleDeploySpec) {
	bundleData, err := charm.ReadBundleData(strings.NewReader(bundle))
	c.Assert(err, jc.ErrorIsNil)

	err = bundleDeploy(charm.CharmHub, bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeployRepositorySuite) expectEmptyModelToStart(c *gc.C) {
	// setup for empty current model
	// bundleHandler.makeModel()
	s.expectDeployerAPIEmptyStatus()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
}

func (s *BundleDeployRepositorySuite) expectEmptyModelRepresentation() {
	// BuildModelRepresentation is tested in bundle pkg.
	// Setup as if an empty model
	s.deployerAPI.EXPECT().GetAnnotations(gomock.Any()).Return(nil, nil)
	s.expectEmptyModelRepresentationNotAnnotations()
}

func (s *BundleDeployRepositorySuite) expectEmptyModelRepresentationNotAnnotations() {
	s.deployerAPI.EXPECT().GetConstraints(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().Sequences().Return(nil, errors.NotSupportedf("sequences for test"))
}

func (s *BundleDeployRepositorySuite) expectWatchAll() {
	s.deployerAPI.EXPECT().WatchAll().Return(s.allWatcher, nil)
	s.allWatcher.EXPECT().Stop().Return(nil)
}

func (s *BundleDeployRepositorySuite) expectDeployerAPIEmptyStatus() {
	status := &params.FullStatus{}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil)
}

func (s *BundleDeployRepositorySuite) expectDeployerAPIStatusWordpressBundle() {
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{},
		Machines: map[string]params.MachineStatus{
			"0": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"mysql": {
				Charm:        "ch:mysql",
				Scale:        1,
				Base:         params.Base{Name: "ubuntu", Channel: "18.04"},
				CharmChannel: "stable",
				Units: map[string]params.UnitStatus{
					"mysql/0": {Machine: "0"},
				},
			},
			"wordpress": {
				Charm:        "ch:wordpress",
				Scale:        1,
				Base:         params.Base{Name: "ubuntu", Channel: "18.04"},
				CharmChannel: "stable",
				Units: map[string]params.UnitStatus{
					"mysql/0": {Machine: "1"},
				},
			},
		},
		RemoteApplications: nil,
		Offers:             nil,
		Relations: []params.RelationStatus{
			{
				Endpoints: []params.EndpointStatus{
					{ApplicationName: "wordpress", Name: "db", Role: "requirer"},
					{ApplicationName: "mysql", Name: "db", Role: "provider"},
				},
			},
		},
		ControllerTimestamp: nil,
		Branches:            nil,
	}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil)
}

func (s *BundleDeployRepositorySuite) expectDeployerAPIStatusDjangoBundle() {
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{},
		Machines: map[string]params.MachineStatus{
			"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"django": {
				Charm: "ch:django",
				Scale: 1,
				Base:  params.Base{Name: "ubuntu", Channel: "18.04"},
				Units: map[string]params.UnitStatus{
					"django/0": {Machine: "1"},
				},
			},
		},
		RemoteApplications:  nil,
		Offers:              nil,
		Relations:           nil,
		ControllerTimestamp: nil,
		Branches:            nil,
	}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil)
}

func (s *BundleDeployRepositorySuite) expectDeployerAPIStatusDjangoMemBundle() {
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{},
		Machines: map[string]params.MachineStatus{
			"0": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"django": {
				Charm:        "ch:django",
				Scale:        1,
				Base:         params.Base{Name: "ubuntu", Channel: "18.04"},
				CharmChannel: "stable",
				Units: map[string]params.UnitStatus{
					"django/0": {Machine: "1"},
				},
			},
			"mem": {
				Charm:        "ch:mem",
				Scale:        1,
				Base:         params.Base{Name: "ubuntu", Channel: "18.04"},
				CharmChannel: "stable",
				Units: map[string]params.UnitStatus{
					"mem/0": {Machine: "0"},
				},
			},
		},
		RemoteApplications:  nil,
		Offers:              nil,
		Relations:           nil,
		ControllerTimestamp: nil,
		Branches:            nil,
	}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil)
}

func (s *BundleDeployRepositorySuite) expectDeployerAPIStatusDjango2Units() {
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{},
		Machines: map[string]params.MachineStatus{
			"0": {Base: params.Base{Name: "ubuntu", Channel: "16.04"}},
			"1": {Base: params.Base{Name: "ubuntu", Channel: "16.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"django": {
				Charm:        "ch:django",
				Scale:        1,
				Base:         params.Base{Name: "ubuntu", Channel: "16.04"},
				CharmChannel: "stable",
				Units: map[string]params.UnitStatus{
					"django/0": {Machine: "0"},
					"django/1": {Machine: "1"},
				},
			},
		},
		RemoteApplications:  nil,
		Offers:              nil,
		Relations:           nil,
		ControllerTimestamp: nil,
		Branches:            nil,
	}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil)
}

func (s *BundleDeployRepositorySuite) expectDeployerAPIModelGet(c *gc.C) {
	cfg, err := config.New(true, minimalModelConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.deployerAPI.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil).AnyTimes()
}

func (s *BundleDeployRepositorySuite) expectResolveCharmWithSeries(bases []string, err error) {
	b := transform.Slice(bases, series.MustParseBaseFromString)
	s.bundleResolver.EXPECT().ResolveCharm(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []series.Base, error) {
			return curl, origin, b, err
		}).AnyTimes()
}

func (s *BundleDeployRepositorySuite) expectResolveCharm(err error) {
	s.expectResolveCharmWithSeries([]string{"ubuntu@18.04", "ubuntu@20.04", "ubuntu@16.04"}, err)
}

func (s *BundleDeployRepositorySuite) expectAddCharm(force bool) {
	s.deployerAPI.EXPECT().AddCharm(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		force,
	).DoAndReturn(
		func(_ *charm.URL, origin commoncharm.Origin, _ bool) (commoncharm.Origin, error) {
			return origin, nil
		})
}

func (s *BundleDeployRepositorySuite) expectAddLocalCharm(curl *charm.URL, force bool) {
	s.deployerAPI.EXPECT().AddLocalCharm(gomock.AssignableToTypeOf(&charm.URL{}), charmInterfaceMatcher{}, force).Return(curl, nil)
}

type charmInterfaceMatcher struct{}

func (m charmInterfaceMatcher) Matches(arg interface{}) bool {
	_, ok := arg.(charm.Charm)
	return ok
}

func (m charmInterfaceMatcher) String() string {
	return "Require charm.Charm as arg"
}

func (s *BundleDeployRepositorySuite) expectCharmInfo(name string, info *apicharms.CharmInfo) {
	s.deployerAPI.EXPECT().CharmInfo(name).Return(info, nil)
}

func (s *BundleDeployRepositorySuite) expectDeploy() {
	s.deployerAPI.EXPECT().Deploy(gomock.AssignableToTypeOf(application.DeployArgs{})).DoAndReturn(
		func(args application.DeployArgs) error {
			// Save the args to do a verification of later.
			// Matching up args with expected is non-trival here,
			// so do it later.
			s.deployArgs[args.ApplicationName] = args
			return nil
		})
}

func (s *BundleDeployRepositorySuite) expectExpose(app string) {
	s.deployerAPI.EXPECT().Expose(app, gomock.Any()).Return(nil)
}

func (s *BundleDeployRepositorySuite) expectAddMachine(machine, channel string) {
	if machine == "" {
		return
	}
	s.expectAddContainer("", machine, channel, "")
}

func (s *BundleDeployRepositorySuite) expectAddContainer(parent, machine, channel, container string) {
	args := []params.AddMachineParams{
		{
			ContainerType: instance.ContainerType(container),
			Jobs:          []model.MachineJob{model.JobHostUnits},
			ParentId:      parent,
		},
	}
	if channel != "" {
		args[0].Base = &params.Base{Name: "ubuntu", Channel: channel + "/stable"}
	}
	results := []params.AddMachinesResult{
		{Machine: machine},
	}
	s.deployerAPI.EXPECT().AddMachines(args).Return(results, nil)
}

func (s *BundleDeployRepositorySuite) expectAddRelation(endpoints []string) {
	s.deployerAPI.EXPECT().AddRelation(endpoints, nil).Return(nil, nil)
}

func (s *BundleDeployRepositorySuite) expectAddOneUnit(name, directive, unit string) {
	var placement []*instance.Placement
	if directive != "" {
		placement = []*instance.Placement{{
			Scope:     "#",
			Directive: directive,
		}}
	}
	args := application.AddUnitsParams{
		ApplicationName: name,
		NumUnits:        1,
		Placement:       placement,
	}
	s.deployerAPI.EXPECT().AddUnits(args).Return([]string{name + "/" + unit}, nil)
}

func (s *BundleDeployRepositorySuite) expectSetConfig(c *gc.C, appName string, options map[string]interface{}) {
	cfg, err := yaml.Marshal(map[string]map[string]interface{}{appName: options})
	c.Assert(err, jc.ErrorIsNil)
	s.deployerAPI.EXPECT().SetConfig(gomock.Any(), appName, string(cfg), gomock.Any())
}

func (s *BundleDeployRepositorySuite) expectSetConstraints(name string, cons string) {
	parsedCons, _ := constraints.Parse(cons)
	s.deployerAPI.EXPECT().SetConstraints(name, parsedCons).Return(nil)
}

func (s *BundleDeployRepositorySuite) expectSetCharm(c *gc.C, name string) {
	s.deployerAPI.EXPECT().SetCharm(model.GenerationMaster, setCharmConfigMatcher{name: name, c: c})
}

type setCharmConfigMatcher struct {
	c    *gc.C
	name string
}

func (m setCharmConfigMatcher) Matches(arg interface{}) bool {
	cfg, ok := arg.(application.SetCharmConfig)
	if !ok {
		return false
	}
	m.c.Assert(ok, jc.IsTrue, gc.Commentf("arg is not a application.SetCharmConfig"))
	m.c.Assert(cfg.ApplicationName, gc.Equals, m.name)
	return true
}

func (m setCharmConfigMatcher) String() string {
	return "Verify SetCharmConfig application name"
}

type BundleHandlerOriginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BundleHandlerOriginSuite{})

func (s *BundleHandlerOriginSuite) TestAddOrigin(c *gc.C) {
	handler := &bundleHandler{
		origins: make(map[charm.URL]map[string]commoncharm.Origin),
	}

	curl := charm.MustParseURL("ch:mysql")
	channel := corecharm.MustParseChannel("stable")
	origin := commoncharm.Origin{
		Risk: "stable",
	}

	handler.addOrigin(*curl, channel, origin)
	res, found := handler.getOrigin(*curl, channel)
	c.Assert(found, jc.IsTrue)
	c.Assert(res, gc.DeepEquals, origin)
}

func (s *BundleHandlerOriginSuite) TestGetOriginNotFound(c *gc.C) {
	handler := &bundleHandler{
		origins: make(map[charm.URL]map[string]commoncharm.Origin),
	}

	curl := charm.MustParseURL("ch:mysql")
	channel := corecharm.MustParseChannel("stable")
	origin := commoncharm.Origin{
		Risk: "stable",
	}

	_, found := handler.getOrigin(*curl, channel)
	c.Assert(found, jc.IsFalse)

	channelB := corecharm.MustParseChannel("edge")
	handler.addOrigin(*curl, channelB, origin)
	_, found = handler.getOrigin(*curl, channel)
	c.Assert(found, jc.IsFalse)
}

func (s *BundleHandlerOriginSuite) TestConstructChannelAndOrigin(c *gc.C) {
	handler := &bundleHandler{}

	arch := "arm64"
	curl := charm.MustParseURL("ch:mysql")
	base := series.MustParseBaseFromString("ubuntu@20.04")
	channel := "stable"
	cons := constraints.Value{
		Arch: &arch,
	}

	resultChannel, resultOrigin, err := handler.constructChannelAndOrigin(curl, base, channel, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultChannel, gc.DeepEquals, corecharm.MustParseChannel("stable"))
	c.Assert(resultOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       "charm-hub",
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
		Risk:         "stable",
		Architecture: "arm64",
	})
}

func (s *BundleHandlerOriginSuite) TestConstructChannelAndOriginUsingArchFallback(c *gc.C) {
	handler := &bundleHandler{}

	curl := charm.MustParseURL("ch:mysql")
	base := series.MustParseBaseFromString("ubuntu@20.04")
	channel := "stable"
	cons := constraints.Value{}

	resultChannel, resultOrigin, err := handler.constructChannelAndOrigin(curl, base, channel, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultChannel, gc.DeepEquals, corecharm.MustParseChannel("stable"))
	c.Assert(resultOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       "charm-hub",
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
		Risk:         "stable",
		Architecture: "amd64",
	})
}

func (s *BundleHandlerOriginSuite) TestConstructChannelAndOriginEmptyChannel(c *gc.C) {
	handler := &bundleHandler{}

	arch := "arm64"
	curl := charm.MustParseURL("ch:mysql")
	base := series.MustParseBaseFromString("ubuntu@20.04")
	channel := ""
	cons := constraints.Value{
		Arch: &arch,
	}

	resultChannel, resultOrigin, err := handler.constructChannelAndOrigin(curl, base, channel, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultChannel, gc.DeepEquals, charm.Channel{})
	c.Assert(resultOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       "charm-hub",
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
		Architecture: "arm64",
	})
}

type BundleHandlerResolverSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BundleHandlerResolverSuite{})

func (s *BundleHandlerResolverSuite) TestResolveCharmChannelAndRevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resolver := mocks.NewMockResolver(ctrl)

	handler := &bundleHandler{
		bundleResolver: resolver,
	}

	charmURL := charm.MustParseURL("ch:ubuntu")
	charmChannel := "stable"
	arch := "amd64"
	rev := 33

	origin := commoncharm.Origin{
		Source:       "charm-hub",
		Architecture: arch,
		Risk:         charmChannel,
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
	}
	resolvedOrigin := origin
	resolvedOrigin.Revision = &rev

	resolver.EXPECT().ResolveCharm(charmURL, origin, false).Return(charmURL, resolvedOrigin, nil, nil)

	base := series.MustParseBaseFromString("ubuntu@20.04")
	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), base, charmChannel, arch, -1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(channel, gc.DeepEquals, "stable")
	c.Assert(rev, gc.Equals, rev)
}

func (s *BundleHandlerResolverSuite) TestResolveCharmChannelWithoutRevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resolver := mocks.NewMockResolver(ctrl)

	handler := &bundleHandler{
		bundleResolver: resolver,
	}

	charmURL := charm.MustParseURL("ch:ubuntu")
	charmChannel := "stable"
	arch := "amd64"

	origin := commoncharm.Origin{
		Source:       "charm-hub",
		Architecture: arch,
		Risk:         charmChannel,
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
	}
	resolvedOrigin := origin

	resolver.EXPECT().ResolveCharm(charmURL, origin, false).Return(charmURL, resolvedOrigin, nil, nil)

	base := series.MustParseBaseFromString("ubuntu@20.04")
	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), base, charmChannel, arch, -1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(channel, gc.DeepEquals, "stable")
	c.Assert(rev, gc.Equals, -1)
}

func (s *BundleHandlerResolverSuite) TestResolveLocalCharm(c *gc.C) {
	handler := &bundleHandler{}

	charmURL := charm.URL{
		Schema: string(charm.Local),
		Name:   "local",
	}
	charmBase := series.MustParseBaseFromString("ubuntu@20.04")
	charmChannel := "stable"
	arch := "amd64"

	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), charmBase, charmChannel, arch, -1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(channel, gc.DeepEquals, "stable")
	c.Assert(rev, gc.Equals, -1)
}

type BundleHandlerMakeModelSuite struct {
	testing.IsolationSuite

	deployerAPI *mocks.MockDeployerAPI
}

var _ = gc.Suite(&BundleHandlerMakeModelSuite{})

func (s *BundleHandlerMakeModelSuite) TestEmptyModel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)

	handler := &bundleHandler{
		deployAPI:          s.deployerAPI,
		defaultCharmSchema: charm.CharmHub,
	}

	err := handler.makeModel(false, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleHandlerMakeModelSuite) TestEmptyModelOldController(c *gc.C) {
	// An old controller is pre juju 2.9
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)

	handler := &bundleHandler{
		deployAPI:          s.deployerAPI,
		defaultCharmSchema: charm.CharmHub,
	}

	err := handler.makeModel(false, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleHandlerMakeModelSuite) TestModelOldController(c *gc.C) {
	// An old controller is pre juju 2.9
	defer s.setupMocks(c).Finish()
	s.expectDeployerAPIStatusWordpressBundle()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)

	handler := &bundleHandler{
		deployAPI:          s.deployerAPI,
		defaultCharmSchema: charm.CharmHub,
		unitStatus:         make(map[string]string),
	}

	err := handler.makeModel(false, nil)
	c.Assert(err, jc.ErrorIsNil)
	app := handler.model.GetApplication("mysql")
	c.Assert(app.Base.Channel.Track, gc.Equals, "18.04")
	app = handler.model.GetApplication("wordpress")
	c.Assert(app.Base.Channel.Track, gc.Equals, "18.04")
}

func (s *BundleHandlerMakeModelSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	return ctrl
}

func (s *BundleHandlerMakeModelSuite) expectEmptyModelToStart(c *gc.C) {
	// setup for empty current model
	// bundleHandler.makeModel()
	s.expectDeployerAPIEmptyStatus()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
}

func (s *BundleHandlerMakeModelSuite) expectEmptyModelRepresentation() {
	// BuildModelRepresentation is tested in bundle pkg.
	// Setup as if an empty model
	s.deployerAPI.EXPECT().GetAnnotations(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().GetConstraints(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().Sequences().Return(nil, errors.NotSupportedf("sequences for test"))
}

func (s *BundleHandlerMakeModelSuite) expectDeployerAPIEmptyStatus() {
	status := &params.FullStatus{}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil).AnyTimes()
}

func (s *BundleHandlerMakeModelSuite) expectDeployerAPIModelGet(c *gc.C) {
	cfg, err := config.New(true, minimalModelConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.deployerAPI.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil).AnyTimes()
}

func (s *BundleHandlerMakeModelSuite) expectDeployerAPIStatusWordpressBundle() {
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{},
		Machines: map[string]params.MachineStatus{
			"0": {Base: params.Base{Name: "ubuntu", Channel: "18.04/stable"}},
			"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04/stable"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"mysql": {
				Charm: "ch:mysql",
				Scale: 1,
				Base:  params.Base{Name: "ubuntu", Channel: "18.04/stable"},
				Units: map[string]params.UnitStatus{
					"mysql/0": {Machine: "0"},
				},
			},
			"wordpress": {
				Charm: "ch:wordpress",
				Scale: 1,
				Base:  params.Base{Name: "ubuntu", Channel: "18.04/stable"},
				Units: map[string]params.UnitStatus{
					"mysql/0": {Machine: "1"},
				},
			},
		},
		RemoteApplications: nil,
		Offers:             nil,
		Relations: []params.RelationStatus{
			{
				Endpoints: []params.EndpointStatus{
					{ApplicationName: "wordpress", Name: "db", Role: "requirer"},
					{ApplicationName: "mysql", Name: "db", Role: "provider"},
				},
			},
		},
		ControllerTimestamp: nil,
		Branches:            nil,
	}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil)
}
