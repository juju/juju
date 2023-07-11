// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
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

func (s *BundleDeployRepositorySuite) SetUpTest(c *gc.C) {
	s.deployArgs = make(map[string]application.DeployArgs)
	s.output = bytes.NewBuffer([]byte{})
	//logger.SetLogLevel(loggo.TRACE)
}

func (s *BundleDeployRepositorySuite) TearDownTest(c *gc.C) {
	s.output.Reset()
}

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

func (s *BundleDeployRepositorySuite) TestDeployBundleNotFoundCharmStore(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)

	// bundleHandler.addCharm():
	curl, err := charm.ParseURL("cs:bundle/no-such")
	c.Assert(err, jc.ErrorIsNil)
	s.expectResolveCharm(errors.NotFoundf("bundle"))
	bundleData := &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"no-such": {
				Charm: curl.String(),
			},
		},
	}

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, `cannot resolve charm or bundle "no-such": bundle not found`)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl, err := charm.ParseURL("cs:mysql-42")
	c.Assert(err, jc.ErrorIsNil)
	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:            mysqlCurl,
			charmMetaSeries: []string{"bionic", "xenial"},
			machine:         "0",
			machineSeries:   "xenial",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			machine:         "1",
			machineSeries:   "xenial",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundle))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "xenial")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "xenial")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application mysql from charm-store on xenial\n"+
		"- upload charm wordpress from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on xenial\n"+
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

	mysqlCurl, err := charm.ParseURL("cs:mysql-42")
	c.Assert(err, jc.ErrorIsNil)
	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:            mysqlCurl,
			charmMetaSeries: []string{"bionic", "xenial"},
			machine:         "0",
			machineSeries:   "xenial",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			machine:         "1",
			machineSeries:   "xenial",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundle))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpecWithConstraints(constraints.MustParse("arch=arm64")))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "xenial")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "xenial")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series xenial with architecture=arm64\n"+
		"- deploy application mysql from charm-store on xenial\n"+
		"- upload charm wordpress from charm-store for series xenial with architecture=arm64\n"+
		"- deploy application wordpress from charm-store on xenial\n"+
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
    charm: cs:mysql-42
    series: xenial
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: cs:wordpress-47
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

func (s *BundleDeployRepositorySuite) TestDeployBundleWithInvalidSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl, err := charm.ParseURL("cs:mysql-42")
	c.Assert(err, jc.ErrorIsNil)
	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	charmInfo := &apicharms.CharmInfo{
		Revision: mysqlCurl.Revision,
		URL:      mysqlCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial", "precise"},
		},
	}
	s.expectCharmInfo(mysqlCurl.String(), charmInfo)

	// For wordpress
	s.expectResolveCharm(nil)

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundleInvalidSeries))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, "mysql is not available on the following series: precise not supported")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleWithInvalidSeriesWithForce(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl, err := charm.ParseURL("cs:mysql-42")
	c.Assert(err, jc.ErrorIsNil)

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)

	chUnits := []charmUnit{
		{
			charmMetaSeries: []string{"bionic", "xenial", "precise"},
			curl:            mysqlCurl,
			force:           true,
			machine:         "0",
			machineSeries:   "precise",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			force:           true,
			machine:         "1",
			machineSeries:   "bionic",
		},
	}
	s.setupCharmUnits(chUnits)

	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundleInvalidSeries))
	c.Assert(err, jc.ErrorIsNil)
	spec := s.bundleDeploySpec()
	spec.force = true
	_, err = bundleDeploy(charm.CharmHub, bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "precise")
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series precise with architecture=amd64\n"+
		"- deploy application mysql from charm-store on precise\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on bionic\n"+
		"- add new machine 0\n"+
		"- add new machine 1\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"Deploy of bundle completed.\n")
}

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

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 3)
	s.assertDeployArgs(c, fullGatewayURL.String(), "istio-ingressgateway", "focal")
	s.assertDeployArgs(c, fullTrainingURL.String(), "training-operator", "focal")
	s.assertDeployArgs(c, fullPilotURL.String(), "istio-pilot", "focal")
}

const wordpressBundleInvalidSeries = `
series: bionic
applications:
  mysql:
    charm: cs:mysql-42
    series: precise
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: cs:wordpress-47
    num_units: 1
    to:
    - "1"
machines:
  "0":
    series: precise
  "1": {}
relations:
- - wordpress:db
  - mysql:db
`

func (s *BundleDeployRepositorySuite) TestDeployKubernetesBundleSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mariadbCurl, err := charm.ParseURL("cs:~juju/mariadb-k8s")
	c.Assert(err, jc.ErrorIsNil)
	gitlabCurl, err := charm.ParseURL("cs:~juju/gitlab-k8s")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:            mariadbCurl,
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
		{
			charmMetaSeries: []string{"kubernetes"},
			curl:            gitlabCurl,
			machineSeries:   "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"gitlab:mysql", "mariadb:server"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(kubernetesGitlabBundle))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, gitlabCurl.String(), "gitlab", "kubernetes")
	s.assertDeployArgs(c, mariadbCurl.String(), "mariadb", "kubernetes")
	s.assertDeployArgsStorage(c, "mariadb", map[string]storage.Constraints{"database": {Pool: "mariadb-pv", Size: 0x14, Count: 0x1}})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"gitlab-k8s\" in charm-store\n"+
		"Located charm \"mariadb-k8s\" in charm-store\n"+
		"Executing changes:\n"+
		"- upload charm gitlab-k8s from charm-store with architecture=amd64\n"+
		"- deploy application gitlab from charm-store with 1 unit using gitlab-k8s\n"+
		"- upload charm mariadb-k8s from charm-store with architecture=amd64\n"+
		"- deploy application mariadb from charm-store with 2 units using mariadb-k8s\n"+
		"- add relation gitlab:mysql - mariadb:server\n"+
		"Deploy of bundle completed.\n")
}

const kubernetesGitlabBundle = `
bundle: kubernetes
applications:
  mariadb:
    charm: cs:~juju/mariadb-k8s
    scale: 2
    constraints: mem=1G
    options:
        dataset-size: 70%
    storage:
      database: mariadb-pv,20M
  gitlab:
    charm: cs:~juju/gitlab-k8s
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

	bundleData, err := charm.ReadBundleData(strings.NewReader(kubernetesCharmhubGitlabBundle))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, fullGitlabCurl.String(), "gitlab", "focal")
	s.assertDeployArgs(c, fullMariadbCurl.String(), "mariadb", "focal")

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
			origin.Series = "focal"
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

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, fullGitlabCurl.String(), "gitlab", "focal")
	s.assertDeployArgs(c, fullMariadbCurl.String(), "mariadb", "focal")

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
			origin.Series = "focal"
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

	mysqlCurl, err := charm.ParseURL("cs:mysql-42")
	c.Assert(err, jc.ErrorIsNil)
	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:            mysqlCurl,
			charmMetaSeries: []string{"bionic", "xenial"},
			machine:         "0",
			machineSeries:   "bionic",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			machine:         "1",
			machineSeries:   "bionic",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundleWithStorage))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "bionic")
	s.assertDeployArgsStorage(c, "mysql", map[string]storage.Constraints{"database": {Pool: "mysql-pv", Size: 0x14, Count: 0x1}})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application mysql from charm-store on bionic\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on bionic\n"+
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
    charm: cs:mysql-42
    num_units: 1
    storage:
      database: mysql-pv,20M
    to:
    - "0"
  wordpress:
    charm: cs:wordpress-47
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

	bitcoinCurl := s.expectCharmstoreK8sCharm(charm.MustParseURL("cs:bitcoin-miner"), 3)
	dashboardCurl := s.expectCharmstoreK8sCharm(charm.MustParseURL("cs:dashboard4miner"), 43)
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(kubernetesBitcoinBundle))
	c.Assert(err, jc.ErrorIsNil)

	spec := s.bundleDeploySpec()
	devConstraints := map[string]devices.Constraints{
		"bitcoinminer": {
			Count: 10, Type: "nvidia.com/gpu",
		},
	}
	spec.bundleDevices = map[string]map[string]devices.Constraints{
		"bitcoin-miner": devConstraints,
	}
	_, err = bundleDeploy(charm.CharmHub, bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "kubernetes")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "kubernetes")
	s.assertDeployArgsDevices(c, bitcoinCurl.Name, devConstraints)

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-store\n"+
		"Located charm \"dashboard4miner\" in charm-store\n"+
		"Executing changes:\n"+
		"- upload charm bitcoin-miner from charm-store with architecture=amd64\n"+
		"- deploy application bitcoin-miner from charm-store with 1 unit\n"+
		"- upload charm dashboard4miner from charm-store with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-store with 1 unit\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) expectCharmstoreK8sCharm(curl *charm.URL, rev int) *charm.URL {
	fullCurl := curl.WithRevision(rev)
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
		}).Times(1)

	s.bundleResolver.EXPECT().ResolveCharm(
		fullCurl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []string, error) {
			origin.Type = "charm"
			return curl, origin, []string{"kubernetes"}, nil
		}).Times(1)

	s.deployerAPI.EXPECT().AddCharm(
		fullCurl,
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		func(_ *charm.URL, origin commoncharm.Origin, _ bool) (commoncharm.Origin, error) {
			origin.Series = "kubernetes"
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
bundle: kubernetes
applications:
    dashboard4miner:
        charm: cs:dashboard4miner
        num_units: 1
    bitcoin-miner:
        charm: cs:bitcoin-miner
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

	bitcoinCurl, err := charm.ParseURL("bitcoin-miner")
	c.Assert(err, jc.ErrorIsNil)
	dashboardCurl, err := charm.ParseURL("dashboard4miner")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:          bitcoinCurl,
			machineSeries: "kubernetes",
		},
		{
			curl:          dashboardCurl,
			machineSeries: "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(kubernetesBitcoinBundleWithoutDevices))
	c.Assert(err, jc.ErrorIsNil)

	spec := s.bundleDeploySpec()
	_, err = bundleDeploy(charm.CharmHub, bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "focal")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "focal")

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

func (s *BundleDeployRepositorySuite) TestDryRunExistingModel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl, err := charm.ParseURL("cs:mysql-42")
	c.Assert(err, jc.ErrorIsNil)
	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:            mysqlCurl,
			charmMetaSeries: []string{"bionic", "xenial"},
			machine:         "0",
			machineSeries:   "bionic",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			machine:         "1",
			machineSeries:   "bionic",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundleWithStorage))
	c.Assert(err, jc.ErrorIsNil)

	spec := s.bundleDeploySpec()
	_, err = bundleDeploy(charm.CharmHub, bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "bionic")

	expectedOutput := "" +
		"Located charm \"mysql\" in charm-store, revision 42\n" +
		"Located charm \"wordpress\" in charm-store, revision 47\n" +
		"Executing changes:\n" +
		"- upload charm mysql from charm-store for series bionic with architecture=amd64\n" +
		"- deploy application mysql from charm-store on bionic\n" +
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n" +
		"- deploy application wordpress from charm-store on bionic\n" +
		"- add new machine 0\n" +
		"- add new machine 1\n" +
		"- add relation wordpress:db - mysql:db\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit wordpress/0 to new machine 1\n" +
		"Deploy of bundle completed.\n"
	c.Check(s.output.String(), gc.Equals, expectedOutput)

	// Setup to run with --dry-run, no changes
	s.expectDeployerAPIStatusWordpressBundle()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()

	expectedOutput += "No changes to apply.\n"
	spec.dryRun = true
	spec.useExistingMachines = true
	spec.bundleMachines = map[string]string{}
	_, err = bundleDeploy(charm.CharmHub, bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.output.String(), gc.Equals, expectedOutput)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleInvalidMachineContainerType(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
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
	s.expectAddMachine("1", "bionic")

	quickBundle := `
       series: bionic
       applications:
           wp:
               charm: cs:wordpress-47
               num_units: 1
               to: ["bad:1"]
       machines:
           1:
   `

	bundleData, err := charm.ReadBundleData(strings.NewReader(quickBundle))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, `cannot create machine for holding wp unit: invalid container type "bad"`)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleUnitPlacedToMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
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
	s.expectAddMachine("0", "bionic")
	s.expectAddContainer("0", "0/lxd/0", "bionic", "lxd")
	s.expectAddMachine("1", "bionic")
	s.expectAddContainer("1", "1/kvm/0", "bionic", "kvm")
	s.expectAddMachine("2", "bionic")
	s.expectAddContainer("", "3/lxd/0", "bionic", "lxd")
	s.expectAddContainer("", "4/lxd/0", "bionic", "lxd")
	s.expectAddContainer("", "5/lxd/0", "bionic", "lxd")
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
               charm: cs:wordpress-47
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

	bundleData, err := charm.ReadBundleData(strings.NewReader(quickBundle))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application wp from charm-store on bionic using wordpress\n"+
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

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			machineSeries:   "bionic",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectExpose(wordpressCurl.Name)

	content := `
       applications:
           wordpress:
               charm: cs:wordpress-47
               num_units: 1
               expose: true
   `
	bundleData, err := charm.ReadBundleData(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store with architecture=amd64\n"+
		"- deploy application wordpress from charm-store\n"+
		"- expose all endpoints of wordpress and allow access from CIDRs 0.0.0.0/0 and ::/0\n"+
		"- add unit wordpress/0 to new machine 0\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleMultipleRelations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	mysqlCurl, err := charm.ParseURL("cs:mysql-32")
	c.Assert(err, jc.ErrorIsNil)
	pgresCurl, err := charm.ParseURL("cs:xenial/postgres-2")
	c.Assert(err, jc.ErrorIsNil)
	varnishCurl, err := charm.ParseURL("cs:xenial/varnish")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            mysqlCurl,
			machineSeries:   "bionic",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            pgresCurl,
			machineSeries:   "xenial",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            varnishCurl,
			machineSeries:   "xenial",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			machineSeries:   "bionic",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"wordpress:db", "mysql:server"})
	s.expectAddRelation([]string{"varnish:webcache", "wordpress:cache"})
	content := `
       series: bionic
       applications:
           wordpress:
               charm: cs:wordpress-47
               num_units: 1
           mysql:
               charm: cs:mysql-32
               num_units: 1
           postgres:
               charm: cs:xenial/postgres-2
               num_units: 1
           varnish:
               charm: cs:xenial/varnish
               num_units: 1
       relations:
           - ["wordpress:db", "mysql:server"]
           - ["varnish:webcache", "wordpress:cache"]
   `
	bundleData, err := charm.ReadBundleData(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "bionic")
	s.assertDeployArgs(c, varnishCurl.String(), "varnish", "xenial")
	s.assertDeployArgs(c, pgresCurl.String(), "postgres", "xenial")
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 32\n"+
		"Located charm \"postgres\" in charm-store, revision 2\n"+
		"Located charm \"varnish\" in charm-store\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application mysql from charm-store on bionic\n"+
		"- upload charm postgres from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application postgres from charm-store on xenial\n"+
		"- upload charm varnish from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application varnish from charm-store on xenial\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on bionic\n"+
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

	mysqlCurl, err := charm.ParseURL("local:jammy/mysql-1")
	c.Assert(err, jc.ErrorIsNil)
	wordpressCurl, err := charm.ParseURL("local:jammy/wordpress-3")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{
		{
			curl:            mysqlCurl,
			charmMetaSeries: []string{"focal", "jammy"},
			machineSeries:   "focal",
		},
		{
			charmMetaSeries: []string{"focal", "jammy"},
			curl:            wordpressCurl,
			machineSeries:   "focal",
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

	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "focal")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "focal")
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

func (s *BundleDeployRepositorySuite) TestDeployBundleWithEndpointBindings(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	grafanaCurl, err := charm.ParseURL("ch:grafana")
	c.Assert(err, jc.ErrorIsNil)
	chUnits := []charmUnit{{
		curl:            grafanaCurl,
		charmMetaSeries: []string{"bionic", "xenial"},
		machine:         "0",
		machineSeries:   "bionic",
	}}
	s.setupCharmUnits(chUnits)

	bundleData, err := charm.ReadBundleData(strings.NewReader(grafanaBundleEndpointBindings))
	c.Assert(err, jc.ErrorIsNil)
	bundleDeploymentSpec := s.bundleDeploySpec()
	bundleDeploymentSpec.knownSpaceNames = set.NewStrings("alpha", "beta")

	_, err = bundleDeploy(charm.CharmHub, bundleData, bundleDeploymentSpec)
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

	_, err = bundleDeploy(charm.CharmHub, bundleData, bundleDeploymentSpec)
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
		_ *macaroon.Macaroon,
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

func (s *BundleDeployRepositorySuite) assertDeployArgs(c *gc.C, curl, appName, series string) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args %s", appName))
	c.Assert(arg.CharmID.URL.String(), gc.Equals, curl)
	c.Assert(arg.Series, gc.Equals, series, gc.Commentf("%s", pretty.Sprint(arg)))
}

func (s *BundleDeployRepositorySuite) assertDeployArgsStorage(c *gc.C, appName string, storage map[string]storage.Constraints) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.Storage, gc.DeepEquals, storage)
}

func (s *BundleDeployRepositorySuite) assertDeployArgsDevices(c *gc.C, appName string, devices map[string]devices.Constraints) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.Devices, gc.DeepEquals, devices)
}

type charmUnit struct {
	curl            *charm.URL
	resolveSeries   []string
	charmMetaSeries []string
	force           bool
	machine         string
	machineSeries   string
}

func (s *BundleDeployRepositorySuite) setupCharmUnits(charmUnits []charmUnit) {
	for _, chUnit := range charmUnits {
		switch chUnit.curl.Schema {
		case "cs", "ch":
			resolveSeries := chUnit.resolveSeries
			if len(resolveSeries) == 0 {
				resolveSeries = []string{"bionic", "focal", "xenial"}
			}
			s.expectResolveCharmWithSeries(resolveSeries, nil)
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
		if chUnit.machineSeries != "kubernetes" {
			s.expectAddMachine(chUnit.machine, chUnit.machineSeries)
			s.expectAddOneUnit(chUnit.curl.Name, chUnit.machine, "0")
		}
	}
}

func (s *BundleDeployRepositorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
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
	s.deployerAPI.EXPECT().BestAPIVersion().Return(7).AnyTimes()
	return ctrl
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
			"0": {Series: "bionic", Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"1": {Series: "bionic", Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"mysql": {
				Charm:        "cs:mysql-42",
				Scale:        1,
				Series:       "bionic",
				CharmChannel: "stable",
				Units: map[string]params.UnitStatus{
					"mysql/0": {Machine: "0"},
				},
			},
			"wordpress": {
				Charm:        "cs:wordpress-47",
				Scale:        1,
				Series:       "bionic",
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

func (s *BundleDeployRepositorySuite) expectDeployerAPIModelGet(c *gc.C) {
	cfg, err := config.New(true, minimalModelConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.deployerAPI.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil).AnyTimes()
}

func (s *BundleDeployRepositorySuite) expectResolveCharmWithSeries(series []string, err error) {
	s.bundleResolver.EXPECT().ResolveCharm(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []string, error) {
			return curl, origin, series, err
		}).AnyTimes()
}

func (s *BundleDeployRepositorySuite) expectResolveCharm(err error) {
	s.expectResolveCharmWithSeries([]string{"bionic", "focal", "xenial"}, err)
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

func (s *BundleDeployRepositorySuite) expectAddMachine(machine, series string) {
	if machine == "" {
		return
	}
	s.expectAddContainer("", machine, series, "")
}

func (s *BundleDeployRepositorySuite) expectAddContainer(parent, machine, series, container string) {
	args := []params.AddMachineParams{
		{
			ContainerType: instance.ContainerType(container),
			Jobs:          []model.MachineJob{model.JobHostUnits},
			Series:        series,
			ParentId:      parent,
		},
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
	series := "focal"
	channel := "stable"
	cons := constraints.Value{
		Arch: &arch,
	}

	resultChannel, resultOrigin, err := handler.constructChannelAndOrigin(curl, series, channel, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultChannel, gc.DeepEquals, corecharm.MustParseChannel("stable"))
	c.Assert(resultOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       "charm-hub",
		Series:       "focal",
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
		Risk:         "stable",
		Architecture: "arm64",
	})
}

func (s *BundleHandlerOriginSuite) TestConstructChannelAndOriginUsingArchFallback(c *gc.C) {
	handler := &bundleHandler{}

	curl := charm.MustParseURL("ch:mysql")
	series := "focal"
	channel := "stable"
	cons := constraints.Value{}

	resultChannel, resultOrigin, err := handler.constructChannelAndOrigin(curl, series, channel, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultChannel, gc.DeepEquals, corecharm.MustParseChannel("stable"))
	c.Assert(resultOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       "charm-hub",
		Series:       "focal",
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
		Risk:         "stable",
		Architecture: "amd64",
	})
}

func (s *BundleHandlerOriginSuite) TestConstructChannelAndOriginEmptyChannel(c *gc.C) {
	handler := &bundleHandler{}

	arch := "arm64"
	curl := charm.MustParseURL("ch:mysql")
	series := "focal"
	channel := ""
	cons := constraints.Value{
		Arch: &arch,
	}

	resultChannel, resultOrigin, err := handler.constructChannelAndOrigin(curl, series, channel, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultChannel, gc.DeepEquals, charm.Channel{})
	c.Assert(resultOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       "charm-hub",
		Series:       "focal",
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
	charmSeries := "focal"
	charmChannel := "stable"
	arch := "amd64"
	rev := 33

	origin := commoncharm.Origin{
		Source:       "charm-hub",
		Architecture: arch,
		Risk:         charmChannel,
		Series:       charmSeries,
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
	}
	resolvedOrigin := origin
	resolvedOrigin.Revision = &rev

	resolver.EXPECT().ResolveCharm(charmURL, origin, false).Return(charmURL, resolvedOrigin, nil, nil)

	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), charmSeries, charmChannel, arch, -1)
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
	charmSeries := "focal"
	charmChannel := "stable"
	arch := "amd64"

	origin := commoncharm.Origin{
		Source:       "charm-hub",
		Architecture: arch,
		Risk:         charmChannel,
		Series:       charmSeries,
		Base:         coreseries.MakeDefaultBase("ubuntu", "20.04"),
	}
	resolvedOrigin := origin

	resolver.EXPECT().ResolveCharm(charmURL, origin, false).Return(charmURL, resolvedOrigin, nil, nil)

	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), charmSeries, charmChannel, arch, -1)
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
	charmSeries := "focal"
	charmChannel := "stable"
	arch := "amd64"

	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), charmSeries, charmChannel, arch, -1)
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
		defaultCharmSchema: charm.CharmStore,
	}

	err := handler.makeModel(false, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleHandlerMakeModelSuite) TestModelOldController(c *gc.C) {
	// An old controller is pre juju 2.9
	defer s.setupMocks(c).Finish()
	s.expectDeployerAPIStatusWordpressBundle()
	s.expectApplicationInfo(c)
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)

	handler := &bundleHandler{
		deployAPI:          s.deployerAPI,
		defaultCharmSchema: charm.CharmStore,
		unitStatus:         make(map[string]string),
	}

	err := handler.makeModel(false, nil)
	c.Assert(err, jc.ErrorIsNil)
	app := handler.model.GetApplication("mysql")
	c.Assert(app.Channel, gc.Equals, "stable")
	app = handler.model.GetApplication("wordpress")
	c.Assert(app.Channel, gc.Equals, "candidate")
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
			"0": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
		},
		Applications: map[string]params.ApplicationStatus{
			"mysql": {
				Charm:  "cs:mysql-42",
				Scale:  1,
				Series: "bionic",
				Units: map[string]params.UnitStatus{
					"mysql/0": {Machine: "0"},
				},
			},
			"wordpress": {
				Charm:  "cs:wordpress-47",
				Scale:  1,
				Series: "bionic",
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

func (s *BundleHandlerMakeModelSuite) expectApplicationInfo(c *gc.C) {
	s.deployerAPI.EXPECT().ApplicationsInfo(applicationInfoMatcher{c: c}).DoAndReturn(
		func(args []names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
			// args content ensured by applicationInfoMatcher
			info := make([]params.ApplicationInfoResult, 2)
			for i, arg := range args {
				if arg == names.NewApplicationTag("mysql") {
					info[i] = params.ApplicationInfoResult{Result: &params.ApplicationResult{Channel: "stable"}}
				}
				if arg == names.NewApplicationTag("wordpress") {
					info[i] = params.ApplicationInfoResult{Result: &params.ApplicationResult{Channel: "candidate"}}
				}
			}
			return info, nil
		})
}

type applicationInfoMatcher struct {
	c *gc.C
}

func (m applicationInfoMatcher) Matches(x interface{}) bool {
	obtained, ok := x.([]names.ApplicationTag)
	m.c.Assert(ok, jc.IsTrue)
	m.c.Assert(obtained, jc.SameContents, []names.ApplicationTag{
		names.NewApplicationTag("mysql"),
		names.NewApplicationTag("wordpress"),
	})
	return true
}

func (m applicationInfoMatcher) String() string {
	return "Match ApplicationInfo args"
}
