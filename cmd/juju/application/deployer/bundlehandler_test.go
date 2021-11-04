// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/api/resources/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/modelcmd"
	bundlechanges "github.com/juju/juju/core/bundle/changes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type BundleDeployRepositorySuite struct {
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
}

func (s *BundleDeployRepositorySuite) TearDownTest(_ *gc.C) {
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
	curl := charm.MustParseURL("cs:bundle/no-such")
	s.expectResolveCharm(errors.NotFoundf("bundle"))
	bundleData := &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"no-such": {
				Charm: curl.String(),
			},
		},
	}

	_, err := bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, `cannot resolve charm or bundle "no-such": bundle not found`)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("cs:mysql-42")
	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
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

	s.runDeploy(c, wordpressBundle)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "xenial")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "xenial")
	s.assertDeployArgsConfig(c, "mysql", map[string]interface{}{"foo": "bar"})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store for series xenial with architecture=amd64\n"+
		"- add new machine 0\n"+
		"- upload charm mysql from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on xenial\n"+
		"- add new machine 1\n"+
		"- deploy application mysql from charm-store on xenial\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"Deploy of bundle completed.\n")
}

const wordpressBundle = `
series: bionic
applications:
  mysql:
    charm: cs:mysql-42
    series: xenial
    num_units: 1
    options:
      foo: bar
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
	s.expectResolveCharm(nil)

	s.expectAddMachine("0", "bionic")
	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
	charmInfo2 := &apicharms.CharmInfo{
		Revision: wordpressCurl.Revision,
		URL:      wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"xenial", "bionic"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), charmInfo2)
	s.expectDeploy()

	s.expectAddMachine("0", "focal")
	s.expectAddCharm(false)
	mysqlCurl := charm.MustParseURL("cs:mysql-42")
	charmInfo := &apicharms.CharmInfo{
		Revision: mysqlCurl.Revision,
		URL:      mysqlCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"zesty", "xenial", "trusty"},
		},
	}
	s.expectCharmInfo(mysqlCurl.String(), charmInfo)

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundleInvalidSeries))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(charm.CharmHub, bundleData, s.bundleDeploySpec())

	c.Assert(err, gc.ErrorMatches, "series \"focal\" not supported by charm, supported series are: zesty,xenial,trusty")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleWithInvalidSeriesWithForce(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("cs:mysql-42")
	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
	chUnits := []charmUnit{
		{
			charmMetaSeries: []string{"xenial", "bionic"},
			curl:            mysqlCurl,
			force:           true,
			machine:         "0",
			machineSeries:   "focal",
		},
		{
			charmMetaSeries: []string{"zesty", "xenial", "trusty"},
			curl:            wordpressCurl,
			force:           true,
			machine:         "1",
			machineSeries:   "bionic",
		},
	}
	s.setupCharmUnits(chUnits)

	s.expectAddRelation([]string{"wordpress:db", "mysql:db"})

	spec := s.bundleDeploySpec()
	spec.force = true
	s.runDeployWithSpec(c, wordpressBundleInvalidSeries, spec)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "focal")
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- add new machine 0\n"+
		"- upload charm mysql from charm-store for series focal with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on bionic\n"+
		"- add new machine 1\n"+
		"- deploy application mysql from charm-store on focal\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"Deploy of bundle completed.\n")
}

const wordpressBundleInvalidSeries = `
series: bionic
applications:
  mysql:
    charm: cs:mysql-42
    series: focal
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
    series: focal
  "1": {}
relations:
- - wordpress:db
  - mysql:db
`

func (s *BundleDeployRepositorySuite) TestDeployKubernetesBundleSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mariadbCurl := charm.MustParseURL("cs:~juju/mariadb-k8s")
	gitlabCurl := charm.MustParseURL("cs:~juju/gitlab-k8s")
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

	s.runDeploy(c, kubernetesGitlabBundle)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, gitlabCurl.String(), "gitlab", "kubernetes")
	s.assertDeployArgs(c, mariadbCurl.String(), "mariadb", "kubernetes")
	s.assertDeployArgsStorage(c, "mariadb", map[string]storage.Constraints{"database": {Pool: "mariadb-pv", Size: 0x14, Count: 0x1}})
	s.assertDeployArgsConfig(c, "mariadb", map[string]interface{}{"dataset-size": "70%"})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"gitlab-k8s\" in charm-store\n"+
		"Located charm \"mariadb-k8s\" in charm-store\n"+
		"Executing changes:\n"+
		"- upload charm mariadb-k8s from charm-store with architecture=amd64\n"+
		"- upload charm gitlab-k8s from charm-store with architecture=amd64\n"+
		"- deploy application mariadb from charm-store with 2 units using mariadb-k8s\n"+
		"- deploy application gitlab from charm-store with 1 unit using gitlab-k8s\n"+
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

	mariadbCurl := charm.MustParseURL("mariadb-k8s")
	gitlabCurl := charm.MustParseURL("gitlab-k8s")
	chUnits := []charmUnit{
		{
			curl:          mariadbCurl,
			machineSeries: "kubernetes",
		},
		{
			curl:          gitlabCurl,
			machineSeries: "kubernetes",
		},
	}
	s.setupMetadataV2CharmUnits(chUnits)
	s.expectAddRelation([]string{"gitlab:mysql", "mariadb:server"})

	s.runDeploy(c, kubernetesCharmhubGitlabBundle)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, gitlabCurl.String(), "gitlab", "focal")
	s.assertDeployArgs(c, mariadbCurl.String(), "mariadb", "focal")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"gitlab-k8s\" in charm-hub, channel new/edge\n"+
		"Located charm \"mariadb-k8s\" in charm-hub, channel old/stable\n"+
		"Executing changes:\n"+
		"- upload charm mariadb-k8s from charm-hub from channel old/stable with architecture=amd64\n"+
		"- upload charm gitlab-k8s from charm-hub from channel new/edge with architecture=amd64\n"+
		"- deploy application mariadb from charm-hub with 2 units with old/stable using mariadb-k8s\n"+
		"- deploy application gitlab from charm-hub with 1 unit with new/edge using gitlab-k8s\n"+
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

func (s *BundleDeployRepositorySuite) TestDeployKubernetesBundleSuccessWithRevisionCharmhub(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mariadbCurl := charm.MustParseURL("mariadb-k8s")
	gitlabCurl := charm.MustParseURL("gitlab-k8s")
	chUnits := []charmUnit{
		{
			curl:          mariadbCurl,
			machineSeries: "kubernetes",
		},
		{
			curl:          gitlabCurl,
			machineSeries: "kubernetes",
		},
	}
	s.setupMetadataV2CharmUnits(chUnits)
	s.expectAddRelation([]string{"gitlab:mysql", "mariadb:server"})

	s.runDeploy(c, kubernetesCharmhubGitlabRevisionBundle)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, gitlabCurl.String(), "gitlab", "focal")
	s.assertDeployArgs(c, mariadbCurl.String(), "mariadb", "focal")

	str := s.output.String()
	c.Check(strings.Contains(str, "Located charm \"gitlab-k8s\" in charm-hub, channel new/edge\n"), jc.IsTrue)
	c.Check(strings.Contains(str, "Located charm \"mariadb-k8s\" in charm-hub, channel old/stable\n"), jc.IsTrue)
	c.Check(strings.Contains(str, "- upload charm mariadb-k8s from charm-hub with revision 8 with architecture=amd64\n"), jc.IsTrue)
}

const kubernetesCharmhubGitlabRevisionBundle = `
bundle: kubernetes
applications:
  mariadb:
    charm: mariadb-k8s
    scale: 2
    revision: 8
    channel: old/stable
  gitlab:
    charm: gitlab-k8s
    scale: 1
    channel: new/edge
relations:
  - - gitlab:mysql
    - mariadb:server
`

func (s *BundleDeployRepositorySuite) TestDeployBundleStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("cs:mysql-42")
	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
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

	s.runDeploy(c, wordpressBundleWithStorage)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "bionic")
	s.assertDeployArgsStorage(c, "mysql", map[string]storage.Constraints{"database": {Pool: "mysql-pv", Size: 0x14, Count: 0x1}})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- add new machine 0\n"+
		"- upload charm mysql from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on bionic\n"+
		"- add new machine 1\n"+
		"- deploy application mysql from charm-store on bionic\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add relation wordpress:db - mysql:db\n"+
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

	bitcoinCurl := charm.MustParseURL("cs:bitcoin-miner")
	dashboardCurl := charm.MustParseURL("cs:dashboard4miner")
	chUnits := []charmUnit{
		{
			curl:            bitcoinCurl,
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
		{
			charmMetaSeries: []string{"kubernetes"},
			curl:            dashboardCurl,
			machineSeries:   "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})

	spec := s.bundleDeploySpec()
	spec.bundleDevices = map[string]map[string]devices.Constraints{
		"bitcoin-miner": {
			"bitcoinminer": {
				Count: 10, Type: "nvidia.com/gpu",
			},
		},
	}
	s.runDeployWithSpec(c, kubernetesBitcoinBundle, spec)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "kubernetes")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "kubernetes")
	s.assertDeployArgsDevices(c, bitcoinCurl.Name,
		map[string]devices.Constraints{
			"bitcoinminer": {Type: "nvidia.com/gpu", Count: 10}},
	)

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-store\n"+
		"Located charm \"dashboard4miner\" in charm-store\n"+
		"Executing changes:\n"+
		"- upload charm dashboard4miner from charm-store with architecture=amd64\n"+
		"- upload charm bitcoin-miner from charm-store with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-store with 1 unit\n"+
		"- deploy application bitcoin-miner from charm-store with 1 unit\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"Deploy of bundle completed.\n")
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

	bitcoinCurl := charm.MustParseURL("bitcoin-miner")
	dashboardCurl := charm.MustParseURL("dashboard4miner")
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

	s.runDeploy(c, kubernetesBitcoinBundleWithoutDevices)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "focal")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "focal")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-hub\n"+
		"Located charm \"dashboard4miner\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm dashboard4miner from charm-hub for series focal with architecture=amd64\n"+
		"- upload charm bitcoin-miner from charm-hub for series focal with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-hub with 1 unit on focal\n"+
		"- deploy application bitcoin-miner from charm-hub with 1 unit on focal\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployKubernetesV1BundleWithResolveCharmFocal(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	bitcoinCurl := charm.MustParseURL("bitcoin-miner")
	dashboardCurl := charm.MustParseURL("dashboard4miner")
	chUnits := []charmUnit{
		{
			curl:            bitcoinCurl,
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
		{
			curl:            dashboardCurl,
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})

	s.runDeploy(c, kubernetesBitcoinBundleWithoutDevices)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "kubernetes")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "kubernetes")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-hub\n"+
		"Located charm \"dashboard4miner\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm dashboard4miner from charm-hub for series focal with architecture=amd64\n"+
		"- upload charm bitcoin-miner from charm-hub for series focal with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-hub with 1 unit on focal\n"+
		"- deploy application bitcoin-miner from charm-hub with 1 unit on focal\n"+
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

func (s *BundleDeployRepositorySuite) TestDeployKubernetesV1Bundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	bitcoinCurl := charm.MustParseURL("bitcoin-miner")
	dashboardCurl := charm.MustParseURL("dashboard4miner")
	chUnits := []charmUnit{
		{
			curl:            bitcoinCurl,
			resolveSeries:   []string{"focal"},
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
		{
			curl:            dashboardCurl,
			resolveSeries:   []string{"focal"},
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})

	s.runDeploy(c, kubernetesBitcoinBundleWithoutSeriesAndDevices)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "kubernetes")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "kubernetes")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-hub\n"+
		"Located charm \"dashboard4miner\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm dashboard4miner from charm-hub with architecture=amd64\n"+
		"- upload charm bitcoin-miner from charm-hub with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-hub with 1 unit\n"+
		"- deploy application bitcoin-miner from charm-hub with 1 unit\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployKubernetesV1BundleWithKubernetesResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	bitcoinCurl := charm.MustParseURL("bitcoin-miner")
	dashboardCurl := charm.MustParseURL("dashboard4miner")
	chUnits := []charmUnit{
		{
			curl:            bitcoinCurl,
			resolveSeries:   []string{"kubernetes"},
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
		{
			curl:            dashboardCurl,
			resolveSeries:   []string{"kubernetes"},
			charmMetaSeries: []string{"kubernetes"},
			machineSeries:   "kubernetes",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"dashboard4miner:miner", "bitcoin-miner:miner"})

	s.runDeploy(c, kubernetesBitcoinBundleWithoutSeriesAndDevices)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, dashboardCurl.String(), dashboardCurl.Name, "kubernetes")
	s.assertDeployArgs(c, bitcoinCurl.String(), bitcoinCurl.Name, "kubernetes")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"bitcoin-miner\" in charm-hub\n"+
		"Located charm \"dashboard4miner\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm dashboard4miner from charm-hub with architecture=amd64\n"+
		"- upload charm bitcoin-miner from charm-hub with architecture=amd64\n"+
		"- deploy application dashboard4miner from charm-hub with 1 unit\n"+
		"- deploy application bitcoin-miner from charm-hub with 1 unit\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"Deploy of bundle completed.\n")
}

const kubernetesBitcoinBundleWithoutSeriesAndDevices = `
bundle: kubernetes
applications:
    dashboard4miner:
        charm: dashboard4miner
        num_units: 1
    bitcoin-miner:
        charm: bitcoin-miner
        num_units: 1
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

	mysqlCurl := charm.MustParseURL("cs:mysql-42")
	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
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
	s.expectResolveCharm(nil)

	spec := s.bundleDeploySpec()
	s.runDeployWithSpec(c, wordpressBundleWithStorage, spec)

	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "bionic")

	expectedOutput := "" +
		"Located charm \"mysql\" in charm-store, revision 42\n" +
		"Located charm \"wordpress\" in charm-store, revision 47\n" +
		"Executing changes:\n" +
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n" +
		"- add new machine 0\n" +
		"- upload charm mysql from charm-store for series bionic with architecture=amd64\n" +
		"- deploy application wordpress from charm-store on bionic\n" +
		"- add new machine 1\n" +
		"- deploy application mysql from charm-store on bionic\n" +
		"- add unit wordpress/0 to new machine 1\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add relation wordpress:db - mysql:db\n" +
		"Deploy of bundle completed.\n"
	c.Check(s.output.String(), gc.Equals, expectedOutput)

	// Setup to run with --dry-run, no changes
	s.expectDeployerAPIStatusWordpressBundle()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
	s.expectWatchAll()

	expectedOutput += "No changes to apply.\n"
	spec.dryRun = dryRun
	spec.useExistingMachines = true
	spec.bundleMachines = map[string]string{}
	s.runDeployWithSpec(c, wordpressBundleWithStorage, spec)
	c.Check(s.output.String(), gc.Equals, expectedOutput)
}

const charmWithResourcesBundle = `
       applications:
           django:
               charm: cs:django
               series: xenial
   `

func (s *BundleDeployRepositorySuite) TestDeployBundleResources(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	djangoCurl := charm.MustParseURL("cs:django")
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
		_ client.CharmID,
		_ *macaroon.Macaroon,
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
	spec.getResourceLister = func(DeployerAPI) (utils.ResourceLister, error) {
		// the resourceLister is passed to the deployResources func we've mocked above.
		return nil, nil
	}

	s.runDeployWithSpec(c, charmWithResourcesBundle, spec)
	c.Assert(strings.Contains(s.output.String(), "added resource one"), jc.IsTrue)
	c.Assert(strings.Contains(s.output.String(), "added resource two"), jc.IsTrue)
}

const specifyResourcesBundle = `
       applications:
           django:
               charm: cs:django
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
	djangoCurl := charm.MustParseURL("cs:django")
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
		_ client.CharmID,
		_ *macaroon.Macaroon,
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
	spec.getResourceLister = func(DeployerAPI) (utils.ResourceLister, error) {
		// the resourceLister is passed to the deployResources func we've mocked above.
		return nil, nil
	}

	s.runDeployWithSpec(c, specifyResourcesBundle, spec)
	c.Assert(strings.Contains(s.output.String(), "added resource one"), jc.IsTrue)
	c.Assert(strings.Contains(s.output.String(), "added resource two"), jc.IsTrue)
}

const wordpressBundleWithStorageUpgradeConstraints = `
series: bionic
applications:
  wordpress:
    charm: cs:wordpress-52
    num_units: 1
    options:
      blog-title: new title
    constraints: spaces=new cores=8
    to:
    - "1"
  mysql:
    charm: cs:mysql-42
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
	wordpressCurl := charm.MustParseURL("cs:wordpress-52")
	s.expectAddCharm(false)
	s.expectSetCharm(c, "wordpress")
	charmInfo := &apicharms.CharmInfo{
		Revision: wordpressCurl.Revision,
		URL:      wordpressCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(wordpressCurl.String(), charmInfo)

	s.expectSetConfig(c, "wordpress", map[string]interface{}{"blog-title": "new title"})
	s.expectSetConstraints("wordpress", "spaces=new cores=8")

	s.runDeploy(c, wordpressBundleWithStorageUpgradeConstraints)

	c.Assert(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-store, revision 52\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- set constraints for wordpress to \"spaces=new cores=8\"\n"+
		"- set application options for wordpress\n"+
		"- upgrade wordpress from charm-store using charm wordpress for series bionic\n"+
		"Deploy of bundle completed.\n",
	)
}

const wordpressBundleWithStorageUpgradeRelations = `
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
  varnish:
    charm: cs:varnish
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
	varnishCurl := charm.MustParseURL("cs:varnish")
	chUnits := []charmUnit{
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            varnishCurl,
			machine:         "2",
			machineSeries:   "bionic",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddRelation([]string{"varnish:webcache", "wordpress:cache"})

	s.runDeploy(c, wordpressBundleWithStorageUpgradeRelations)

	c.Assert(s.output.String(), gc.Equals, ""+
		"Located charm \"varnish\" in charm-store\n"+
		"Executing changes:\n"+
		"- upload charm varnish from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application varnish from charm-store on bionic\n"+
		"- add new machine 2\n"+
		"- add unit varnish/0 to new machine 2\n"+
		"- add relation varnish:webcache - wordpress:cache\n"+
		"Deploy of bundle completed.\n",
	)
}

const machineUnitPlacementBundle = `
      applications:
          wordpress:
              charm: cs:xenial/wordpress
              num_units: 2
              to:
                  - 1
                  - lxd:2
              options:
                  blog-title: these are the voyages
          mysql:
              charm: cs:xenial/mysql
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

	s.expectAddMachine("0", "xenial")
	s.expectAddMachine("1", "xenial")
	s.expectAddMachine("2", "xenial")
	s.expectAddContainer("0", "0/lxd/0", "xenial", "lxd")
	s.expectAddContainer("1", "1/lxd/0", "xenial", "lxd")

	wordpressCurl := charm.MustParseURL("cs:wordpress")
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

	mysqlCurl := charm.MustParseURL("cs:mysql")
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
               charm: cs:django
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
	djangoCurl := charm.MustParseURL("cs:django")
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
			Series:        "xenial",
		},
	}
	results := []params.AddMachinesResult{
		{Machine: "0"},
	}
	s.deployerAPI.EXPECT().AddMachines(args).Return(results, nil)
	s.expectAddMachine("1", "xenial")
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

	djangoCurl := charm.MustParseURL("cs:django-42")
	s.expectResolveCharmWithSeries([]string{"bionic", "xenial"}, nil)
	s.expectAddCharm(false)
	charmInfo := &apicharms.CharmInfo{
		Revision: djangoCurl.Revision,
		URL:      djangoCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(djangoCurl.String(), charmInfo)
	s.expectDeploy()
	s.expectAddOneUnit("django", "", "0")
	s.expectAddOneUnit("django", "", "1")

	s.runDeploy(c, `
       applications:
           django:
               charm: cs:django-42
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
               charm: cs:django-42
               series: xenial
               num_units: 5
   `)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleUnitPlacedInApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()
	s.expectResolveCharm(nil)

	wordpressCurl := charm.MustParseURL("cs:wordpress")
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

	djangoCurl := charm.MustParseURL("cs:django-42")
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
               charm: cs:wordpress
               num_units: 3
           django:
               charm: cs:django-42
               num_units: 2
               to: [wordpress]
   `)
}

const peerContainerBundle = `
       applications:
           wordpress:
               charm: cs:wordpress
               num_units: 2
               to: ["lxd:new"]
           django:
               charm: cs:django-42
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

	wordpressCurl := charm.MustParseURL("cs:wordpress")
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
	s.expectAddOneUnit("wordpress", "0/lxd/0", "0")
	s.expectAddOneUnit("wordpress", "1/lxd/0", "1")

	djangoCurl := charm.MustParseURL("cs:django-42")
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
	s.expectAddOneUnit("django", "0/lxd/1", "0")
	s.expectAddOneUnit("django", "1/lxd/1", "1")

	s.runDeploy(c, peerContainerBundle)

	c.Assert(strings.Contains(s.output.String(), "add unit django/0 to 0/lxd/1 to satisfy [lxd:wordpress]"), jc.IsTrue)
	c.Assert(strings.Contains(s.output.String(), "add unit django/1 to 1/lxd/1 to satisfy [lxd:wordpress]"), jc.IsTrue)
}

const unitColocationWithUnitBundle = `
       applications:
           mem:
               charm: cs:mem-47
               series: xenial
               num_units: 3
               to: [1, new]
           django:
               charm: cs:django-42
               series: xenial
               num_units: 5
               to:
                   - mem/0
                   - lxd:mem/1
                   - lxd:mem/2
                   - kvm:ror
           ror:
               charm: cs:rails
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
	s.expectAddMachine("0", "xenial")
	s.expectAddMachine("1", "xenial")
	s.expectAddMachine("2", "xenial")
	s.expectAddMachine("3", "xenial")
	s.expectAddContainer("1", "1/lxd/0", "xenial", "lxd")
	s.expectAddContainer("2", "2/lxd/0", "xenial", "lxd")
	s.expectAddContainer("3", "3/kvm/0", "xenial", "kvm")
	s.expectAddContainer("0", "0/kvm/0", "xenial", "kvm")

	// Setup for mem charm
	memCurl := charm.MustParseURL("cs:mem-47")
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
	djangoCurl := charm.MustParseURL("cs:django-42")
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
	railsCurl := charm.MustParseURL("cs:rails")
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
               charm: cs:django
               series: bionic
               num_units: 1
           rails:
               charm: cs:rails-47
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

	railsCurl := charm.MustParseURL("cs:rails-47")
	s.expectResolveCharm(nil)
	s.expectAddCharm(false)
	charmInfo := &apicharms.CharmInfo{
		Revision: railsCurl.Revision,
		URL:      railsCurl.String(),
		Meta: &charm.Meta{
			Series: []string{"bionic", "xenial"},
		},
	}
	s.expectCharmInfo(railsCurl.String(), charmInfo)
	s.expectDeploy()
	s.expectAddOneUnit(railsCurl.Name, "", "0")

	// Redeploy a very similar bundle with another application unit. The new unit
	// is placed on the first unit of memcached. Due to ordering of the applications
	// there is no deterministic way to determine "least crowded" in a meaningful way.
	s.runDeploy(c, switchBundle)
}

const annotationsBundle = `
        applications:
            django:
                charm: cs:django
                num_units: 1
                annotations:
                    key1: value1
                    key2: value2
                to: [1]
            mem:
                charm: cs:mem-47
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

	djangoCurl := charm.MustParseURL("cs:django")
	memCurl := charm.MustParseURL("cs:mem-47")
	chUnits := []charmUnit{
		{
			curl:            memCurl,
			charmMetaSeries: []string{"bionic", "focal"},
			machine:         "0",
			machineSeries:   "bionic",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            djangoCurl,
			machine:         "1",
			machineSeries:   "bionic",
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
                charm: cs:django
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
	s.expectResolveCharmWithSeries([]string{"bionic", "xenial"}, nil)
	s.expectAddCharm(false)

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

	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
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

	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
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

	s.runDeploy(c, quickBundle)

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- add new machine 0 (bundle machine 4)\n"+
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- add new machine 1 (bundle machine 8)\n"+
		"- deploy application wp from charm-store on bionic using wordpress\n"+
		"- add new machine 2\n"+
		"- add unit wp/0 to new machine 2\n"+
		"- add unit wp/1 to new machine 0\n"+
		"- add kvm container 1/kvm/0 on new machine 1\n"+
		"- add unit wp/2 to 1/kvm/0\n"+
		"- add lxd container 0/lxd/0 on new machine 0\n"+
		"- add unit wp/3 to 0/lxd/0\n"+
		"- add lxd container 3/lxd/0 on new machine 3\n"+
		"- add unit wp/4 to 3/lxd/0\n"+
		"- add lxd container 4/lxd/0 on new machine 4\n"+
		"- add unit wp/5 to 4/lxd/0\n"+
		"- add lxd container 5/lxd/0 on new machine 5\n"+
		"- add unit wp/6 to 5/lxd/0\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleExpose(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
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
	s.runDeploy(c, content)

	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store with architecture=amd64\n"+
		"- deploy application wordpress from charm-store\n"+
		"- add unit wordpress/0 to new machine 0\n"+
		"- expose all endpoints of wordpress and allow access from CIDRs 0.0.0.0/0 and ::/0\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleMultipleRelations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl := charm.MustParseURL("cs:wordpress-47")
	mysqlCurl := charm.MustParseURL("cs:mysql-32")
	pgresCurl := charm.MustParseURL("cs:xenial/postgres-2")
	varnishCurl := charm.MustParseURL("cs:xenial/varnish")
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
	s.runDeploy(c, content)

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
		"- upload charm wordpress from charm-store for series bionic with architecture=amd64\n"+
		"- upload charm varnish from charm-store for series xenial with architecture=amd64\n"+
		"- upload charm postgres from charm-store for series xenial with architecture=amd64\n"+
		"- upload charm mysql from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on bionic\n"+
		"- deploy application varnish from charm-store on xenial\n"+
		"- deploy application postgres from charm-store on xenial\n"+
		"- deploy application mysql from charm-store on bionic\n"+
		"- add unit wordpress/0 to new machine 3\n"+
		"- add unit varnish/0 to new machine 2\n"+
		"- add unit postgres/0 to new machine 1\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add relation varnish:webcache - wordpress:cache\n"+
		"- add relation wordpress:db - mysql:server\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployRepositorySuite) TestDeployBundleLocalDeployment(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl := charm.MustParseURL("local:xenial/mysql-1")
	wordpressCurl := charm.MustParseURL("local:xenial/wordpress-3")
	chUnits := []charmUnit{
		{
			curl:            mysqlCurl,
			charmMetaSeries: []string{"bionic", "xenial"},
			machineSeries:   "xenial",
		},
		{
			charmMetaSeries: []string{"bionic", "xenial"},
			curl:            wordpressCurl,
			machineSeries:   "xenial",
		},
	}
	s.setupCharmUnits(chUnits)
	s.expectAddOneUnit("mysql", "", "1")
	s.expectAddRelation([]string{"wordpress:db", "mysql:server"})

	content := `
      series: xenial
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

	s.runDeploy(c, bundle)

	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "xenial")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "xenial")
	expectedOutput := "" +
		"Executing changes:\n" +
		"- upload charm %s for series xenial with architecture=amd64\n" +
		"- upload charm %s for series xenial with architecture=amd64\n" +
		"- deploy application mysql on xenial\n" +
		"- deploy application wordpress on xenial\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit wordpress/0 to new machine 2\n" +
		"- add unit mysql/1 to new machine 1\n" +
		"- add relation wordpress:db - mysql:server\n" +
		"Deploy of bundle completed.\n"

	c.Check(s.output.String(), gc.Equals, fmt.Sprintf(expectedOutput, mysqlPath, wordpressPath))
}

func (s *BundleDeployRepositorySuite) TestApplicationsForMachineChange(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectResolveCharmWithSeries([]string{"xenial"}, nil)
	s.expectResolveCharmWithSeries([]string{"xenial"}, nil)
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

func (s *BundleDeployRepositorySuite) bundleDeploySpec() bundleDeploySpec {
	deployResourcesFunc := func(_ string,
		_ client.CharmID,
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
		bundleResolver:  s.bundleResolver,
		deployResources: deployResourcesFunc,
		getResourceLister: func(DeployerAPI) (utils.ResourceLister, error) {
			return nil, nil
		},
	}
}

func (s *BundleDeployRepositorySuite) assertDeployArgs(c *gc.C, curl, appName, series string) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.CharmID.URL.String(), gc.Equals, curl)
	c.Assert(arg.Series, gc.Equals, series)
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

func (s *BundleDeployRepositorySuite) setupMetadataV2CharmUnits(charmUnits []charmUnit) {
	for _, chUnit := range charmUnits {
		s.expectResolveCharm(nil)
		s.expectAddCharm(chUnit.force)
		charmInfo := &apicharms.CharmInfo{
			Revision: chUnit.curl.Revision,
			URL:      chUnit.curl.String(),
			Meta: &charm.Meta{
				Containers: map[string]charm.Container{
					"test": {
						Resource: "test-oci",
					},
				},
			},
			Manifest: &charm.Manifest{Bases: []charm.Base{{
				Name:    "ubuntu",
				Channel: charm.Channel{Track: "20.04"},
			}}},
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
	return ctrl
}

func (s *BundleDeployRepositorySuite) runDeploy(c *gc.C, bundle string) {
	spec := s.bundleDeploySpec()
	s.runDeployWithSpec(c, bundle, spec)
}

func (s *BundleDeployRepositorySuite) runDeployWithSpec(c *gc.C, bundle string, spec bundleDeploySpec) {
	bundleData, err := charm.ReadBundleData(strings.NewReader(bundle))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(charm.CharmHub, bundleData, spec)
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
			"0": {Series: "bionic"},
			"1": {Series: "bionic"},
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

func (s *BundleDeployRepositorySuite) expectDeployerAPIStatusDjangoBundle() {
	status := &params.FullStatus{
		Model: params.ModelStatusInfo{},
		Machines: map[string]params.MachineStatus{
			"1": {Series: "bionic"},
		},
		Applications: map[string]params.ApplicationStatus{
			"django": {
				Charm:  "cs:django",
				Scale:  1,
				Series: "bionic",
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
			"0": {Series: "bionic"},
			"1": {Series: "bionic"},
		},
		Applications: map[string]params.ApplicationStatus{
			"django": {
				Charm:  "cs:django",
				Scale:  1,
				Series: "bionic",
				Units: map[string]params.UnitStatus{
					"django/0": {Machine: "1"},
				},
			},
			"mem": {
				Charm:  "cs:mem-47",
				Scale:  1,
				Series: "bionic",
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
			"0": {Series: "xenial"},
			"1": {Series: "xenial"},
		},
		Applications: map[string]params.ApplicationStatus{
			"django": {
				Charm:  "cs:django-42",
				Scale:  1,
				Series: "xenial",
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
	s.deployerAPI.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil)
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
		OS:           "ubuntu",
		Series:       "focal",
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
		OS:           "ubuntu",
		Series:       "focal",
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
		OS:           "ubuntu",
		Series:       "focal",
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
		OS:           "ubuntu",
		Series:       charmSeries,
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
		OS:           "ubuntu",
		Series:       charmSeries,
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
