// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/api/resources/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
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
	s.expectResolveCharm(errors.NotFoundf("bundle"), 1)
	bundleData := &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"no-such": {
				Charm: curl.String(),
			},
		},
	}

	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
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

	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "xenial")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "xenial")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series xenial\n"+
		"- deploy application mysql from charm-store on xenial\n"+
		"- upload charm wordpress from charm-store for series xenial\n"+
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
	s.expectResolveCharm(nil, 2)
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
	s.expectResolveCharm(nil, 1)

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundleInvalidSeries))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
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
	_, err = bundleDeploy(bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "precise")
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series precise\n"+
		"- deploy application mysql from charm-store on precise\n"+
		"- upload charm wordpress from charm-store for series bionic\n"+
		"- deploy application wordpress from charm-store on bionic\n"+
		"- add new machine 0\n"+
		"- add new machine 1\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1\n"+
		"Deploy of bundle completed.\n")
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

	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, gitlabCurl.String(), "gitlab", "kubernetes")
	s.assertDeployArgs(c, mariadbCurl.String(), "mariadb", "kubernetes")
	s.assertDeployArgsStorage(c, "mariadb", map[string]storage.Constraints{"database": {Pool: "mariadb-pv", Size: 0x14, Count: 0x1}})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"gitlab-k8s\" in charm-store\n"+
		"Located charm \"mariadb-k8s\" in charm-store\n"+
		"Executing changes:\n"+
		"- upload charm gitlab-k8s from charm-store for series kubernetes\n"+
		"- deploy application gitlab from charm-store with 1 unit on kubernetes using gitlab-k8s\n"+
		"- upload charm mariadb-k8s from charm-store for series kubernetes\n"+
		"- deploy application mariadb from charm-store with 2 units on kubernetes using mariadb-k8s\n"+
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

	mariadbCurl, err := charm.ParseURL("mariadb-k8s")
	c.Assert(err, jc.ErrorIsNil)
	gitlabCurl, err := charm.ParseURL("gitlab-k8s")
	c.Assert(err, jc.ErrorIsNil)
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

	bundleData, err := charm.ReadBundleData(strings.NewReader(kubernetesCharmhubGitlabBundle))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, gitlabCurl.String(), "gitlab", "focal")
	s.assertDeployArgs(c, mariadbCurl.String(), "mariadb", "focal")

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"gitlab-k8s\" in charm-hub\n"+
		"Located charm \"mariadb-k8s\" in charm-hub\n"+
		"Executing changes:\n"+
		"- upload charm gitlab-k8s from charm-hub for series kubernetes\n"+
		"- deploy application gitlab from charm-hub with 1 unit on kubernetes using gitlab-k8s\n"+
		"- upload charm mariadb-k8s from charm-hub for series kubernetes\n"+
		"- deploy application mariadb from charm-hub with 2 units on kubernetes using mariadb-k8s\n"+
		"- add relation gitlab:mysql - mariadb:server\n"+
		"Deploy of bundle completed.\n")
}

const kubernetesCharmhubGitlabBundle = `
bundle: kubernetes
applications:
  mariadb:
    charm: mariadb-k8s
    scale: 2
  gitlab:
    charm: gitlab-k8s
    scale: 1
relations:
  - - gitlab:mysql
    - mariadb:server
`

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

	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "bionic")
	s.assertDeployArgsStorage(c, "mysql", map[string]storage.Constraints{"database": {Pool: "mysql-pv", Size: 0x14, Count: 0x1}})

	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"mysql\" in charm-store, revision 42\n"+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series bionic\n"+
		"- deploy application mysql from charm-store on bionic\n"+
		"- upload charm wordpress from charm-store for series bionic\n"+
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

	bitcoinCurl, err := charm.ParseURL("cs:bitcoin-miner")
	c.Assert(err, jc.ErrorIsNil)
	dashboardCurl, err := charm.ParseURL("cs:dashboard4miner")
	c.Assert(err, jc.ErrorIsNil)
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

	bundleData, err := charm.ReadBundleData(strings.NewReader(kubernetesBitcoinBundle))
	c.Assert(err, jc.ErrorIsNil)

	spec := s.bundleDeploySpec()
	spec.bundleDevices = map[string]map[string]devices.Constraints{
		"bitcoin-miner": {
			"bitcoinminer": {
				Count: 10, Type: "nvidia.com/gpu",
			},
		},
	}
	_, err = bundleDeploy(bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
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
		"- upload charm bitcoin-miner from charm-store for series kubernetes\n"+
		"- deploy application bitcoin-miner from charm-store with 1 unit on kubernetes\n"+
		"- upload charm dashboard4miner from charm-store for series kubernetes\n"+
		"- deploy application dashboard4miner from charm-store with 1 unit on kubernetes\n"+
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
	_, err = bundleDeploy(bundleData, spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "bionic")

	expectedOutput := "" +
		"Located charm \"mysql\" in charm-store, revision 42\n" +
		"Located charm \"wordpress\" in charm-store, revision 47\n" +
		"Executing changes:\n" +
		"- upload charm mysql from charm-store for series bionic\n" +
		"- deploy application mysql from charm-store on bionic\n" +
		"- upload charm wordpress from charm-store for series bionic\n" +
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
	_, err = bundleDeploy(bundleData, spec)
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
	s.expectResolveCharm(nil, 2)
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
	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, `cannot create machine for holding wp unit: invalid container type "bad"`)
}

func (s *BundleDeployRepositorySuite) TestDeployBundleUnitPlacedToMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	s.expectAddCharm(false)
	s.expectResolveCharm(nil, 2)
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
	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store for series bionic\n"+
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
	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.output.String(), gc.Equals, ""+
		"Located charm \"wordpress\" in charm-store, revision 47\n"+
		"Executing changes:\n"+
		"- upload charm wordpress from charm-store\n"+
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
	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
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
		"- upload charm mysql from charm-store for series bionic\n"+
		"- deploy application mysql from charm-store on bionic\n"+
		"- upload charm postgres from charm-store for series xenial\n"+
		"- deploy application postgres from charm-store on xenial\n"+
		"- upload charm varnish from charm-store for series xenial\n"+
		"- deploy application varnish from charm-store on xenial\n"+
		"- upload charm wordpress from charm-store for series bionic\n"+
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

	mysqlCurl, err := charm.ParseURL("local:xenial/mysql-1")
	c.Assert(err, jc.ErrorIsNil)
	wordpressCurl, err := charm.ParseURL("local:xenial/wordpress-3")
	c.Assert(err, jc.ErrorIsNil)
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
	bundleData, err := charm.ReadBundleData(strings.NewReader(bundle))
	c.Assert(err, jc.ErrorIsNil)

	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, jc.ErrorIsNil)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "xenial")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "xenial")
	expectedOutput := "" +
		"Executing changes:\n" +
		"- upload charm %s for series xenial\n" +
		"- deploy application mysql on xenial\n" +
		"- upload charm %s for series xenial\n" +
		"- deploy application wordpress on xenial\n" +
		"- add relation wordpress:db - mysql:server\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit mysql/1 to new machine 1\n" +
		"- add unit wordpress/0 to new machine 2\n" +
		"Deploy of bundle completed.\n"

	c.Check(s.output.String(), gc.Equals, fmt.Sprintf(expectedOutput, mysqlPath, wordpressPath))
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

func (s *BundleDeployRepositorySuite) assertDeployArgsDevices(c *gc.C, appName string, devices map[string]devices.Constraints) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.Devices, gc.DeepEquals, devices)
}

type charmUnit struct {
	curl            *charm.URL
	charmMetaSeries []string
	force           bool
	machine         string
	machineSeries   string
}

func (s *BundleDeployRepositorySuite) setupCharmUnits(charmUnits []charmUnit) {
	for _, chUnit := range charmUnits {
		switch chUnit.curl.Schema {
		case "cs":
			s.expectResolveCharm(nil, 2)
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
		s.expectResolveCharm(nil, 2)
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
				Name: "ubuntu",
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

func (s *BundleDeployRepositorySuite) expectResolveCharm(err error, times int) {
	s.bundleResolver.EXPECT().ResolveCharm(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, origin commoncharm.Origin) (*charm.URL, commoncharm.Origin, []string, error) {
			return curl, origin, []string{"bionic", "focal", "xenial"}, err
		}).Times(times)
}

func (s *BundleDeployRepositorySuite) expectBestFacadeVersion() {
	s.deployerAPI.EXPECT().BestFacadeVersion("Application").Return(6)
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
	return fmt.Sprintf("Require charm.Charm as arg")
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

	resolver.EXPECT().ResolveCharm(charmURL, origin).Return(charmURL, resolvedOrigin, nil, nil)

	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), charmSeries, charmChannel, arch)
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

	resolver.EXPECT().ResolveCharm(charmURL, origin).Return(charmURL, resolvedOrigin, nil, nil)

	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), charmSeries, charmChannel, arch)
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

	channel, rev, err := handler.resolveCharmChannelAndRevision(charmURL.String(), charmSeries, charmChannel, arch)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(channel, gc.DeepEquals, "stable")
	c.Assert(rev, gc.Equals, -1)
}
