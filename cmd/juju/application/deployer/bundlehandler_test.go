// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type BundleDeployCharmStoreSuite struct {
	allWatcher     *mocks.MockAllWatch
	bundleResolver *mocks.MockBundleResolver
	deployerAPI    *mocks.MockDeployerAPI
	stdOut         *mocks.MockWriter
	stdErr         *mocks.MockWriter

	deployArgs map[string]application.DeployArgs
	output     *bytes.Buffer
}

var _ = gc.Suite(&BundleDeployCharmStoreSuite{})

func (s *BundleDeployCharmStoreSuite) SetUpTest(c *gc.C) {
	s.deployArgs = make(map[string]application.DeployArgs)
	s.output = bytes.NewBuffer([]byte{})
	//logger.SetLogLevel(loggo.TRACE)
}

func (s *BundleDeployCharmStoreSuite) TearDownTest(c *gc.C) {
	s.output.Reset()
}

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNotFoundCharmStore(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)

	// bundleHandler.addCharm():
	curl, err := charm.ParseURL("cs:bundle/no-such")
	c.Assert(err, jc.ErrorIsNil)
	s.expectResolveWithPreferredChannel(errors.NotFoundf("bundle"), 1)
	bundleData := &charm.BundleData{
		Applications: map[string]*charm.ApplicationSpec{
			"no-such": &charm.ApplicationSpec{
				Charm: curl.String(),
			},
		},
	}

	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:bundle/no-such": bundle not found`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleSuccess(c *gc.C) {
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
		"Resolving charm: cs:mysql-42\n"+
		"Resolving charm: cs:wordpress-47\n"+
		"Executing changes:\n"+
		"- upload charm cs:mysql-42 for series xenial\n"+
		"- deploy application mysql on xenial using cs:mysql-42\n"+
		"- upload charm cs:wordpress-47 for series xenial\n"+
		"- deploy application wordpress on xenial using cs:wordpress-47\n"+
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithInvalidSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	mysqlCurl, err := charm.ParseURL("cs:mysql-42")
	c.Assert(err, jc.ErrorIsNil)
	s.expectResolveWithPreferredChannel(nil, 2)
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
	s.expectResolveWithPreferredChannel(nil, 1)

	bundleData, err := charm.ReadBundleData(strings.NewReader(wordpressBundleInvalidSeries))
	c.Assert(err, jc.ErrorIsNil)
	_, err = bundleDeploy(bundleData, s.bundleDeploySpec())
	c.Assert(err, gc.ErrorMatches, "mysql is not available on the following series: precise not supported")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithInvalidSeriesWithForce(c *gc.C) {
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
	c.Assert(s.deployArgs, gc.HasLen, 2)
	s.assertDeployArgs(c, wordpressCurl.String(), "wordpress", "bionic")
	s.assertDeployArgs(c, mysqlCurl.String(), "mysql", "precise")
	c.Check(s.output.String(), gc.Equals, ""+
		"Resolving charm: cs:mysql-42\n"+
		"Resolving charm: cs:wordpress-47\n"+
		"Executing changes:\n"+
		"- upload charm cs:mysql-42 for series precise\n"+
		"- deploy application mysql on precise using cs:mysql-42\n"+
		"- upload charm cs:wordpress-47 for series bionic\n"+
		"- deploy application wordpress on bionic using cs:wordpress-47\n"+
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

func (s *BundleDeployCharmStoreSuite) TestDeployKubernetesBundleSuccess(c *gc.C) {
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
		"Resolving charm: cs:~juju/gitlab-k8s\n"+
		"Resolving charm: cs:~juju/mariadb-k8s\n"+
		"Executing changes:\n"+
		"- upload charm cs:~juju/gitlab-k8s for series kubernetes\n"+
		"- deploy application gitlab with 1 unit on kubernetes using cs:~juju/gitlab-k8s\n"+
		"- upload charm cs:~juju/mariadb-k8s for series kubernetes\n"+
		"- deploy application mariadb with 2 units on kubernetes using cs:~juju/mariadb-k8s\n"+
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleStorage(c *gc.C) {
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
		"Resolving charm: cs:mysql-42\n"+
		"Resolving charm: cs:wordpress-47\n"+
		"Executing changes:\n"+
		"- upload charm cs:mysql-42 for series bionic\n"+
		"- deploy application mysql on bionic using cs:mysql-42\n"+
		"- upload charm cs:wordpress-47 for series bionic\n"+
		"- deploy application wordpress on bionic using cs:wordpress-47\n"+
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleDevices(c *gc.C) {
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
		"bitcoin-miner": {"bitcoinminer": {Count: 10, Type: "nvidia.com/gpu"}},
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
		"Resolving charm: bitcoin-miner\n"+
		"Resolving charm: dashboard4miner\n"+
		"Executing changes:\n"+
		"- upload charm cs:bitcoin-miner for series kubernetes\n"+
		"- deploy application bitcoin-miner with 1 unit on kubernetes using cs:bitcoin-miner\n"+
		"- upload charm cs:dashboard4miner for series kubernetes\n"+
		"- deploy application dashboard4miner with 1 unit on kubernetes using cs:dashboard4miner\n"+
		"- add relation dashboard4miner:miner - bitcoin-miner:miner\n"+
		"Deploy of bundle completed.\n")
}

const kubernetesBitcoinBundle = `
bundle: kubernetes
applications:
    dashboard4miner:
        charm: dashboard4miner
        num_units: 1
    bitcoin-miner:
        charm: bitcoin-miner
        num_units: 1
        devices:
            bitcoinminer: 1,nvidia.com/gpu
relations:
    - ["dashboard4miner:miner", "bitcoin-miner:miner"]
`

func (s *BundleDeployCharmStoreSuite) TestDryRunExistingModel(c *gc.C) {
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
		"Resolving charm: cs:mysql-42\n" +
		"Resolving charm: cs:wordpress-47\n" +
		"Executing changes:\n" +
		"- upload charm cs:mysql-42 for series bionic\n" +
		"- deploy application mysql on bionic using cs:mysql-42\n" +
		"- upload charm cs:wordpress-47 for series bionic\n" +
		"- deploy application wordpress on bionic using cs:wordpress-47\n" +
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidMachineContainerType(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	s.expectAddCharm(false)
	s.expectResolveWithPreferredChannel(nil, 2)
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitPlacedToMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEmptyModelToStart(c)
	s.expectWatchAll()

	wordpressCurl, err := charm.ParseURL("cs:wordpress-47")
	c.Assert(err, jc.ErrorIsNil)
	s.expectAddCharm(false)
	s.expectResolveWithPreferredChannel(nil, 2)
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
		"Resolving charm: cs:wordpress-47\n"+
		"Executing changes:\n"+
		"- upload charm cs:wordpress-47 for series bionic\n"+
		"- deploy application wp on bionic using cs:wordpress-47\n"+
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleExpose(c *gc.C) {
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
		"Resolving charm: cs:wordpress-47\n"+
		"Executing changes:\n"+
		"- upload charm cs:wordpress-47\n"+
		"- deploy application wordpress using cs:wordpress-47\n"+
		"- expose wordpress\n"+
		"- add unit wordpress/0 to new machine 0\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMultipleRelations(c *gc.C) {
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
               charm: wordpress-47
               num_units: 1
           mysql:
               charm: mysql-32
               num_units: 1
           postgres:
               charm: xenial/postgres-2
               num_units: 1
           varnish:
               charm: xenial/varnish
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
		"Resolving charm: mysql-32\n"+
		"Resolving charm: xenial/postgres-2\n"+
		"Resolving charm: xenial/varnish\n"+
		"Resolving charm: wordpress-47\n"+
		"Executing changes:\n"+
		"- upload charm cs:mysql-32 for series bionic\n"+
		"- deploy application mysql on bionic using cs:mysql-32\n"+
		"- upload charm cs:xenial/postgres-2 for series xenial\n"+
		"- deploy application postgres on xenial using cs:xenial/postgres-2\n"+
		"- upload charm cs:xenial/varnish for series xenial\n"+
		"- deploy application varnish on xenial using cs:xenial/varnish\n"+
		"- upload charm cs:wordpress-47 for series bionic\n"+
		"- deploy application wordpress on bionic using cs:wordpress-47\n"+
		"- add relation wordpress:db - mysql:server\n"+
		"- add relation varnish:webcache - wordpress:cache\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit postgres/0 to new machine 1\n"+
		"- add unit varnish/0 to new machine 2\n"+
		"- add unit wordpress/0 to new machine 3\n"+
		"Deploy of bundle completed.\n")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeployment(c *gc.C) {
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
		"- deploy application mysql on xenial using %s\n" +
		"- upload charm %s for series xenial\n" +
		"- deploy application wordpress on xenial using %s\n" +
		"- add relation wordpress:db - mysql:server\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit mysql/1 to new machine 1\n" +
		"- add unit wordpress/0 to new machine 2\n" +
		"Deploy of bundle completed.\n"

	c.Check(s.output.String(), gc.Equals, fmt.Sprintf(expectedOutput, mysqlPath, mysqlPath, wordpressPath, wordpressPath))
}

func (s *BundleDeployCharmStoreSuite) bundleDeploySpec() bundleDeploySpec {
	deployResourcesFunc := func(_ string,
		_ charmstore.CharmID,
		_ *macaroon.Macaroon,
		_ map[string]string,
		_ map[string]charmresource.Meta,
		_ base.APICallCloser,
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

func (s *BundleDeployCharmStoreSuite) assertDeployArgs(c *gc.C, curl, appName, series string) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.CharmID.URL.String(), gc.Equals, curl)
	c.Assert(arg.Series, gc.Equals, series)
}

func (s *BundleDeployCharmStoreSuite) assertDeployArgsStorage(c *gc.C, appName string, storage map[string]storage.Constraints) {
	arg, found := s.deployArgs[appName]
	c.Assert(found, jc.IsTrue, gc.Commentf("Application %q not found in deploy args", appName))
	c.Assert(arg.Storage, gc.DeepEquals, storage)
}

func (s *BundleDeployCharmStoreSuite) assertDeployArgsDevices(c *gc.C, appName string, devices map[string]devices.Constraints) {
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

func (s *BundleDeployCharmStoreSuite) setupCharmUnits(charmUnits []charmUnit) {
	for _, chUnit := range charmUnits {
		switch chUnit.curl.Schema {
		case "cs":
			s.expectResolveWithPreferredChannel(nil, 2)
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

func (s *BundleDeployCharmStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	s.bundleResolver = mocks.NewMockBundleResolver(ctrl)
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

func (s *BundleDeployCharmStoreSuite) expectEmptyModelToStart(c *gc.C) {
	// setup for empty current model
	// bundleHandler.makeModel()
	s.expectDeployerAPIEmptyStatus()
	s.expectEmptyModelRepresentation()
	s.expectDeployerAPIModelGet(c)
}

func (s *BundleDeployCharmStoreSuite) expectEmptyModelRepresentation() {
	// BuildModelRepresentation is tested in bundle pkg.
	// Setup as if an empty model
	s.deployerAPI.EXPECT().GetAnnotations(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().GetConstraints(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().Sequences().Return(nil, errors.NotSupportedf("sequences for test"))
}

func (s *BundleDeployCharmStoreSuite) expectWatchAll() {
	s.deployerAPI.EXPECT().WatchAll().Return(s.allWatcher, nil)
	s.allWatcher.EXPECT().Stop().Return(nil)
}

func (s *BundleDeployCharmStoreSuite) expectDeployerAPIEmptyStatus() {
	status := &params.FullStatus{}
	s.deployerAPI.EXPECT().Status(gomock.Any()).Return(status, nil)
}

func (s *BundleDeployCharmStoreSuite) expectDeployerAPIStatusWordpressBundle() {
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

func (s *BundleDeployCharmStoreSuite) expectDeployerAPIModelGet(c *gc.C) {
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

func (s *BundleDeployCharmStoreSuite) expectResolveWithPreferredChannel(err error, times int) {
	s.bundleResolver.EXPECT().ResolveWithPreferredChannel(
		gomock.AssignableToTypeOf(&charm.URL{}),
		csparams.NoChannel,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, channel csparams.Channel) (*charm.URL, csparams.Channel, []string, error) {
			return curl, csparams.Channel(csparams.NoChannel), []string{"bionic", "focal", "xenial"}, err
		}).Times(times)
}

func (s *BundleDeployCharmStoreSuite) expectBestFacadeVersion() {
	s.deployerAPI.EXPECT().BestFacadeVersion("Application").Return(6)
}

func (s *BundleDeployCharmStoreSuite) expectAddCharm(force bool) {
	s.deployerAPI.EXPECT().AddCharm(gomock.AssignableToTypeOf(&charm.URL{}), csparams.Channel(csparams.NoChannel), force).Return(nil)
}

func (s *BundleDeployCharmStoreSuite) expectAddLocalCharm(curl *charm.URL, force bool) {
	s.deployerAPI.EXPECT().AddLocalCharm(gomock.AssignableToTypeOf(&charm.URL{}), charmInterfaceMatcher{}, force).Return(curl, nil)
}

type charmInterfaceMatcher struct {
}

func (m charmInterfaceMatcher) Matches(arg interface{}) bool {
	_, ok := arg.(charm.Charm)
	return ok
}

func (m charmInterfaceMatcher) String() string {
	return fmt.Sprintf("Require charm.Charm as arg")
}

func (s *BundleDeployCharmStoreSuite) expectCharmInfo(name string, info *apicharms.CharmInfo) {
	s.deployerAPI.EXPECT().CharmInfo(name).Return(info, nil)
}

func (s *BundleDeployCharmStoreSuite) expectDeploy() {
	s.deployerAPI.EXPECT().Deploy(gomock.AssignableToTypeOf(application.DeployArgs{})).DoAndReturn(
		func(args application.DeployArgs) error {
			// Save the args to do a verification of later.
			// Matching up args with expected is non-trival here,
			// so do it later.
			s.deployArgs[args.ApplicationName] = args
			return nil
		})
}

func (s *BundleDeployCharmStoreSuite) expectExpose(app string) {
	s.deployerAPI.EXPECT().Expose(app).Return(nil)
}

func (s *BundleDeployCharmStoreSuite) expectAddMachine(machine, series string) {
	if machine == "" {
		return
	}
	s.expectAddContainer("", machine, series, "")
}

func (s *BundleDeployCharmStoreSuite) expectAddContainer(parent, machine, series, container string) {
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

func (s *BundleDeployCharmStoreSuite) expectAddRelation(endpoints []string) {
	s.deployerAPI.EXPECT().AddRelation(endpoints, nil).Return(nil, nil)
}

func (s *BundleDeployCharmStoreSuite) expectAddOneUnit(name, directive, unit string) {
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
