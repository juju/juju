// Copyright 2015 Canonical Ltd.
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
	"github.com/juju/juju/caas"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
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

//func (s *BundleDeployCharmStoreSuite) TestDeployBundleResources(c *gc.C) {
//	s.setupCharm(c, "trusty/starsay-42", "starsay", "bionic")
//	bundleMeta := `
//        applications:
//            starsay:
//                charm: cs:starsay
//                num_units: 1
//                resources:
//                    store-resource: 0
//                    install-resource: 0
//                    upload-resource: 0
//    `
//	stdOut, stdErr, err := s.DeployBundleYAMLWithOutput(c, bundleMeta)
//	c.Assert(err, jc.ErrorIsNil)
//
//	c.Check(stdOut, gc.Equals, ""+
//		"Executing changes:\n"+
//		"- upload charm cs:trusty/starsay-42 for series trusty\n"+
//		"- deploy application starsay on trusty using cs:trusty/starsay-42\n"+
//		"- add unit starsay/0 to new machine 0",
//	)
//	// Info messages go to stdErr.
//	c.Check(stdErr, gc.Equals, ""+
//		"Resolving charm: cs:starsay\n"+
//		"  added resource install-resource\n"+
//		"  added resource store-resource\n"+
//		"  added resource upload-resource\n"+
//		"Deploy of bundle completed.",
//	)
//
//	resourceHash := func(content string) charmresource.Fingerprint {
//		fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
//		c.Assert(err, jc.ErrorIsNil)
//		return fp
//	}
//
//	s.checkResources(c, "starsay", []resource.Resource{{
//		Resource: charmresource.Resource{
//			Meta: charmresource.Meta{
//				Name:        "install-resource",
//				Type:        charmresource.TypeFile,
//				Path:        "gotta-have-it.txt",
//				Description: "get things started",
//			},
//			Origin:      charmresource.OriginStore,
//			Revision:    0,
//			Fingerprint: resourceHash("install-resource content"),
//			Size:        int64(len("install-resource content")),
//		},
//		ID:            "starsay/install-resource",
//		ApplicationID: "starsay",
//	}, {
//		Resource: charmresource.Resource{
//			Meta: charmresource.Meta{
//				Name:        "store-resource",
//				Type:        charmresource.TypeFile,
//				Path:        "filename.tgz",
//				Description: "One line that is useful when operators need to push it.",
//			},
//			Origin:      charmresource.OriginStore,
//			Fingerprint: resourceHash("store-resource content"),
//			Size:        int64(len("store-resource content")),
//			Revision:    0,
//		},
//		ID:            "starsay/store-resource",
//		ApplicationID: "starsay",
//	}, {
//		Resource: charmresource.Resource{
//			Meta: charmresource.Meta{
//				Name:        "upload-resource",
//				Type:        charmresource.TypeFile,
//				Path:        "somename.xml",
//				Description: "Who uses xml anymore?",
//			},
//			Origin:      charmresource.OriginUpload,
//			Fingerprint: resourceHash("some-data"),
//			Size:        int64(len("some-data")),
//			Revision:    0,
//		},
//		ID:            "starsay/upload-resource",
//		ApplicationID: "starsay",
//		Username:      "admin",
//	}})
//}

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

//func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidFlags(c *gc.C) {
//	s.setupCharm(c, "xenial/mysql-42", "mysql", "bionic")
//	s.setupCharm(c, "xenial/wordpress-47", "wordpress", "bionic")
//	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")
//
//	err := s.runDeploy(c, "bundle/wordpress-simple", "--config", "config.yaml")
//	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --config")
//	err = s.runDeploy(c, "bundle/wordpress-simple", "-n", "2")
//	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: -n")
//	err = s.runDeploy(c, "bundle/wordpress-simple", "--series", "xenial")
//	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --series")
//}

//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalPath(c *gc.C) {
//	dir := c.MkDir()
//	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")
//	path := filepath.Join(dir, "mybundle")
//	data := `
//        series: xenial
//        applications:
//            dummy:
//                charm: ./dummy
//                series: xenial
//                num_units: 1
//    `
//	err := ioutil.WriteFile(path, []byte(data), 0644)
//	c.Assert(err, jc.ErrorIsNil)
//	err = s.runDeploy(c, path)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "local:xenial/dummy-1")
//	ch, err := s.State.Charm(charm.MustParseURL("local:xenial/dummy-1"))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"dummy": {charm: "local:xenial/dummy-1", config: ch.Config().DefaultSettings()},
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C) {
//	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, true)
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalPathInvalidSeriesWithoutForce(c *gc.C) {
//	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, false)
//}
//
//func (s *BundleDeployCharmStoreSuite) assertDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C, force bool) {
//	dir := c.MkDir()
//	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")
//	path := filepath.Join(dir, "mybundle")
//	data := `
//        series: focal
//        applications:
//            dummy:
//                charm: ./dummy
//                num_units: 1
//    `
//	err := ioutil.WriteFile(path, []byte(data), 0644)
//	c.Assert(err, jc.ErrorIsNil)
//	args := []string{path}
//	if force {
//		args = append(args, "--force")
//	}
//	err = s.runDeploy(c, args...)
//	if !force {
//		c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: dummy is not available on the following series: focal not supported")
//		return
//	}
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "local:focal/dummy-1")
//	ch, err := s.State.Charm(charm.MustParseURL("local:focal/dummy-1"))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"dummy": {charm: "local:focal/dummy-1", config: ch.Config().DefaultSettings()},
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalResources(c *gc.C) {
//	data := `
//        series: bionic
//        applications:
//            "dummy-resource":
//                charm: ./dummy-resource
//                series: bionic
//                num_units: 1
//                resources:
//                  dummy: ./dummy-resource.zip
//    `
//	dir := s.makeBundleDir(c, data)
//	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy-resource")
//	c.Assert(
//		ioutil.WriteFile(filepath.Join(dir, "dummy-resource.zip"), []byte("zip file"), 0644),
//		jc.ErrorIsNil)
//	err := s.runDeploy(c, dir)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "local:bionic/dummy-resource-0")
//	ch, err := s.State.Charm(charm.MustParseURL("local:bionic/dummy-resource-0"))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"dummy-resource": {charm: "local:bionic/dummy-resource-0", config: ch.Config().DefaultSettings()},
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleNoSeriesInCharmURL(c *gc.C) {
//	s.setupCharm(c, "~who/multi-series-0", "multi-series", "bionic")
//	dir := c.MkDir()
//	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")
//	path := filepath.Join(dir, "mybundle")
//	data := `
//        series: trusty
//        applications:
//            dummy:
//                charm: cs:~who/multi-series
//    `
//	err := ioutil.WriteFile(path, []byte(data), 0644)
//	c.Assert(err, jc.ErrorIsNil)
//	err = s.runDeploy(c, path)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "cs:~who/multi-series-0")
//	ch, err := s.State.Charm(charm.MustParseURL("~who/multi-series-0"))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"dummy": {charm: "cs:~who/multi-series-0", config: ch.Config().DefaultSettings()},
//	})
//}
//

//
//func (s *BundleDeployCharmStoreSuite) checkResources(c *gc.C, app string, expected []resource.Resource) {
//	_, err := s.State.Application(app)
//	c.Check(err, jc.ErrorIsNil)
//	st, err := s.State.Resources()
//	c.Assert(err, jc.ErrorIsNil)
//	svcResources, err := st.ListResources(app)
//	c.Assert(err, jc.ErrorIsNil)
//	resources := svcResources.Resources
//	resource.Sort(resources)
//	c.Assert(resources, gc.HasLen, 3)
//	c.Check(resources[2].Timestamp, gc.Not(gc.Equals), time.Time{})
//	resources[2].Timestamp = time.Time{}
//	c.Assert(resources, jc.DeepEquals, expected)
//}
//

//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleWatcherTimeout(c *gc.C) {
//	// Inject an "AllWatcher" that never delivers a result.
//	ch := make(chan struct{})
//	defer close(ch)
//	watcher := mockAllWatcher{
//		next: func() []params.Delta {
//			<-ch
//			return nil
//		},
//	}
//
//	s.setupCharm(c, "xenial/django-0", "django", "bionic")
//	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
//	s.PatchValue(&updateUnitStatusPeriod, 0*time.Second)
//	err := s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: django
//                num_units: 1
//            wordpress:
//                charm: wordpress
//                num_units: 1
//                to: [django]
//    `)
//	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot retrieve placement for "wordpress" unit: cannot resolve machine: timeout while trying to get new changes from the watcher`)
//}

//

//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentBadConfig(c *gc.C) {
//	charmsPath := c.MkDir()
//	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
//	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
//	err := s.DeployBundleYAML(c, fmt.Sprintf(`
//        series: xenial
//        applications:
//            wordpress:
//                charm: %s
//                num_units: 1
//            mysql:
//                charm: %s
//                num_units: 2
//        relations:
//            - ["wordpress:db", "mysql:server"]
//    `, wordpressPath, mysqlPath),
//		"--overlay", "missing-file")
//	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: unable to process overlays: "missing-file" not found`)
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentLXDProfile(c *gc.C) {
//	charmsPath := c.MkDir()
//	lxdProfilePath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "lxd-profile")
//	err := s.DeployBundleYAML(c, fmt.Sprintf(`
//        series: bionic
//        services:
//            lxd-profile:
//                charm: %s
//                num_units: 1
//    `, lxdProfilePath))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "local:bionic/lxd-profile-0")
//	lxdProfile, err := s.State.Charm(charm.MustParseURL("local:bionic/lxd-profile-0"))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"lxd-profile": {charm: "local:bionic/lxd-profile-0", config: lxdProfile.Config().DefaultSettings()},
//	})
//	s.assertUnitsCreated(c, map[string]string{
//		"lxd-profile/0": "0",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentBadLXDProfile(c *gc.C) {
//	charmsPath := c.MkDir()
//	lxdProfilePath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "lxd-profile-fail")
//	err := s.DeployBundleYAML(c, fmt.Sprintf(`
//        series: bionic
//        services:
//            lxd-profile-fail:
//                charm: %s
//                num_units: 1
//    `, lxdProfilePath))
//	c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: cannot deploy local charm at .*: invalid lxd-profile.yaml: contains device type \"unix-disk\"")
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentBadLXDProfileWithForce(c *gc.C) {
//	charmsPath := c.MkDir()
//	lxdProfilePath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "lxd-profile-fail")
//	err := s.DeployBundleYAML(c, fmt.Sprintf(`
//        series: bionic
//        services:
//            lxd-profile-fail:
//                charm: %s
//                num_units: 1
//    `, lxdProfilePath), "--force")
//	c.Assert(err, jc.ErrorIsNil)
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentWithBundleOverlay(c *gc.C) {
//	configDir := c.MkDir()
//	configFile := filepath.Join(configDir, "config.yaml")
//	c.Assert(
//		ioutil.WriteFile(
//			configFile, []byte(`
//                applications:
//                    wordpress:
//                        options:
//                            blog-title: include-file://title
//            `), 0644),
//		jc.ErrorIsNil)
//	c.Assert(
//		ioutil.WriteFile(
//			filepath.Join(configDir, "title"), []byte("magic bundle config"), 0644),
//		jc.ErrorIsNil)
//
//	charmsPath := c.MkDir()
//	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
//	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
//	err := s.DeployBundleYAML(c, fmt.Sprintf(`
//        series: xenial
//        applications:
//            wordpress:
//                charm: %s
//                num_units: 1
//            mysql:
//                charm: %s
//                num_units: 2
//        relations:
//            - ["wordpress:db", "mysql:server"]
//    `, wordpressPath, mysqlPath),
//		"--overlay", configFile)
//
//	c.Assert(err, jc.ErrorIsNil)
//	// Now check the blog-title of the wordpress.	le")
//	wordpress, err := s.State.Application("wordpress")
//	c.Assert(err, jc.ErrorIsNil)
//	settings, err := wordpress.CharmConfig(model.GenerationMaster)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(settings["blog-title"], gc.Equals, "magic bundle config")
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployLocalBundleWithRelativeCharmPaths(c *gc.C) {
//	bundleDir := c.MkDir()
//	_ = testcharms.RepoWithSeries("bionic").ClonedDirPath(bundleDir, "dummy")
//
//	bundleFile := filepath.Join(bundleDir, "bundle.yaml")
//	bundleContent := `
//series: bionic
//applications:
//  dummy:
//    charm: ./dummy
//`
//	c.Assert(
//		ioutil.WriteFile(bundleFile, []byte(bundleContent), 0644),
//		jc.ErrorIsNil)
//
//	err := s.runDeploy(c, bundleFile)
//	c.Assert(err, jc.ErrorIsNil)
//
//	_, err = s.State.Application("dummy")
//	c.Assert(err, jc.ErrorIsNil)
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalAndCharmStoreCharms(c *gc.C) {
//	charmsPath := c.MkDir()
//	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
//	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
//	err := s.DeployBundleYAML(c, fmt.Sprintf(`
//        series: xenial
//        applications:
//            wordpress:
//                charm: xenial/wordpress-42
//                series: xenial
//                num_units: 1
//            mysql:
//                charm: %s
//                num_units: 1
//        relations:
//            - ["wordpress:db", "mysql:server"]
//    `, mysqlPath))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "local:xenial/mysql-1", "cs:xenial/wordpress-42")
//	mysqlch, err := s.State.Charm(charm.MustParseURL("local:xenial/mysql-1"))
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"mysql":     {charm: "local:xenial/mysql-1", config: mysqlch.Config().DefaultSettings()},
//		"wordpress": {charm: "cs:xenial/wordpress-42", config: wpch.Config().DefaultSettings()},
//	})
//	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
//	s.assertUnitsCreated(c, map[string]string{
//		"mysql/0":     "0",
//		"wordpress/0": "1",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationOptions(c *gc.C) {
//	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
//	dch := s.setupCharm(c, "bionic/dummy-0", "dummy", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            wordpress:
//                charm: wordpress
//                num_units: 1
//                options:
//                    blog-title: these are the voyages
//            customized:
//                charm: bionic/dummy-0
//                num_units: 1
//                options:
//                    username: who
//                    skill-level: 47
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "cs:bionic/dummy-0", "cs:xenial/wordpress-42")
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"customized": {
//			charm:  "cs:bionic/dummy-0",
//			config: s.combinedSettings(dch, charm.Settings{"username": "who", "skill-level": int64(47)}),
//		},
//		"wordpress": {
//			charm:  "cs:xenial/wordpress-42",
//			config: s.combinedSettings(wpch, charm.Settings{"blog-title": "these are the voyages"}),
//		},
//	})
//	s.assertUnitsCreated(c, map[string]string{
//		"wordpress/0":  "1",
//		"customized/0": "0",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationConstraints(c *gc.C) {
//	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
//	dch := s.setupCharm(c, "bionic/dummy-0", "dummy", "bionic")
//
//	err := s.DeployBundleYAML(c, `
//        applications:
//            wordpress:
//                charm: wordpress
//                constraints: mem=4G cores=2
//            customized:
//                charm: bionic/dummy-0
//                num_units: 1
//                constraints: arch=i386
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "cs:bionic/dummy-0", "cs:xenial/wordpress-42")
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"customized": {
//			charm:       "cs:bionic/dummy-0",
//			constraints: constraints.MustParse("arch=i386"),
//			config:      dch.Config().DefaultSettings(),
//		},
//		"wordpress": {
//			charm:       "cs:xenial/wordpress-42",
//			constraints: constraints.MustParse("mem=4G cores=2"),
//			config:      wpch.Config().DefaultSettings(),
//		},
//	})
//	s.assertUnitsCreated(c, map[string]string{
//		"customized/0": "0",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleSetAnnotations(c *gc.C) {
//	s.setupCharm(c, "xenial/wordpress", "wordpress", "bionic")
//	s.setupCharm(c, "xenial/mysql", "mysql", "bionic")
//	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")
//
//	deploy := s.deployCommandForState()
//	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "bundle/wordpress-simple")
//	c.Assert(err, jc.ErrorIsNil)
//	application, err := s.State.Application("wordpress")
//	c.Assert(err, jc.ErrorIsNil)
//	ann, err := s.Model.Annotations(application)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(ann, jc.DeepEquals, map[string]string{"bundleURL": "cs:bundle/wordpress-simple-1"})
//	application2, err := s.State.Application("mysql")
//	c.Assert(err, jc.ErrorIsNil)
//	ann2, err := s.Model.Annotations(application2)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(ann2, jc.DeepEquals, map[string]string{"bundleURL": "cs:bundle/wordpress-simple-1"})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationUpgrade(c *gc.C) {
//	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
//	s.setupCharm(c, "trusty/upgrade-1", "upgrade1", "bionic")
//
//	// First deploy the bundle.
//	err := s.DeployBundleYAML(c, `
//        applications:
//            wordpress:
//                charm: wordpress
//                num_units: 1
//                options:
//                    blog-title: these are the voyages
//                constraints: spaces=final,frontiers mem=8000M
//            up:
//                charm: trusty/upgrade-1
//                num_units: 1
//                constraints: mem=8G
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertCharmsUploaded(c, "cs:trusty/upgrade-1", "cs:xenial/wordpress-42")
//
//	ch := s.setupCharm(c, "trusty/upgrade-2", "upgrade2", "bionic")
//	// Then deploy a new bundle with modified charm revision and options.
//	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
//        applications:
//            wordpress:
//                charm: wordpress
//                num_units: 1
//                options:
//                    blog-title: new title
//                constraints: spaces=new cores=8
//            up:
//                charm: trusty/upgrade-2
//                num_units: 1
//                constraints: mem=8G
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(stdOut, gc.Equals, ""+
//		"Executing changes:\n"+
//		"- upload charm cs:trusty/upgrade-2 for series trusty\n"+
//		"- upgrade up to use charm cs:trusty/upgrade-2 for series trusty\n"+
//		"- set application options for wordpress\n"+
//		`- set constraints for wordpress to "spaces=new cores=8"`,
//	)
//
//	s.assertCharmsUploaded(c, "cs:trusty/upgrade-1", "cs:trusty/upgrade-2", "cs:xenial/wordpress-42")
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"up": {
//			charm:       "cs:trusty/upgrade-2",
//			config:      ch.Config().DefaultSettings(),
//			constraints: constraints.MustParse("mem=8G"),
//		},
//		"wordpress": {
//			charm:       "cs:xenial/wordpress-42",
//			config:      s.combinedSettings(wpch, charm.Settings{"blog-title": "new title"}),
//			constraints: constraints.MustParse("spaces=new cores=8"),
//		},
//	})
//	s.assertUnitsCreated(c, map[string]string{
//		"up/0":        "0",
//		"wordpress/0": "1",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleExpose(c *gc.C) {
//	ch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
//	content := `
//        applications:
//            wordpress:
//                charm: wordpress
//                num_units: 1
//                expose: true
//    `
//	expectedApplications := map[string]applicationInfo{
//		"wordpress": {
//			charm:   "cs:xenial/wordpress-42",
//			config:  ch.Config().DefaultSettings(),
//			exposed: true,
//		},
//	}
//
//	// First deploy the bundle.
//	err := s.DeployBundleYAML(c, content)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, expectedApplications)
//
//	// Then deploy the same bundle again: no error is produced when the application
//	// is exposed again.
//	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, content)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, expectedApplications)
//	c.Check(stdOut, gc.Equals, "") // Nothing to do.
//
//	// Then deploy a bundle with the application unexposed, and check that the
//	// application is not unexposed.
//	stdOut, _, err = s.DeployBundleYAMLWithOutput(c, `
//        applications:
//            wordpress:
//                charm: wordpress
//                num_units: 1
//                expose: false
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, expectedApplications)
//	c.Check(stdOut, gc.Equals, "") // Nothing to do.
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationUpgradeFailure(c *gc.C) {
//	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
//
//	// Try upgrading to a different series.
//	// Note that this test comes before the next one because
//	// otherwise we can't resolve the charm URL because the charm's
//	// "base entity" is not marked as promulgated so the query by
//	// promulgated will find it.
//	s.setupCharm(c, "vivid/wordpress-42", "wordpress", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            wordpress:
//                charm: vivid/wordpress
//                num_units: 1
//    `)
//	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade application "wordpress" to charm "cs:vivid/wordpress-42": cannot change an application's series`)
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleMultipleRelations(c *gc.C) {
//	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
//	s.setupCharm(c, "xenial/mysql-1", "mysql", "bionic")
//	s.setupCharm(c, "xenial/postgres-2", "mysql", "bionic")
//	s.setupCharm(c, "xenial/varnish-3", "varnish", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            wp:
//                charm: wordpress
//                num_units: 1
//            mysql:
//                charm: mysql
//                num_units: 1
//            pgres:
//                charm: xenial/postgres-2
//                num_units: 1
//            varnish:
//                charm: xenial/varnish
//                num_units: 1
//        relations:
//            - ["wp:db", "mysql:server"]
//            - ["varnish:webcache", "wp:cache"]
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:cache varnish:webcache")
//	s.assertUnitsCreated(c, map[string]string{
//		"mysql/0":   "0",
//		"pgres/0":   "1",
//		"varnish/0": "2",
//		"wp/0":      "3",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleNewRelations(c *gc.C) {
//	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
//	s.setupCharm(c, "xenial/mysql-1", "mysql", "bionic")
//	s.setupCharm(c, "xenial/postgres-2", "mysql", "bionic")
//	s.setupCharm(c, "xenial/varnish-3", "varnish", "bionic")
//
//	err := s.DeployBundleYAML(c, `
//        applications:
//            wp:
//                charm: wordpress
//                num_units: 1
//            mysql:
//                charm: mysql
//                num_units: 1
//            varnish:
//                charm: xenial/varnish
//                num_units: 1
//        relations:
//            - ["wp:db", "mysql:server"]
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
//        applications:
//            wp:
//                charm: wordpress
//                num_units: 1
//            mysql:
//                charm: mysql
//                num_units: 1
//            varnish:
//                charm: xenial/varnish
//                num_units: 1
//        relations:
//            - ["wp:db", "mysql:server"]
//            - ["varnish:webcache", "wp:cache"]
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(stdOut, gc.Equals, ""+
//		"Executing changes:\n"+
//		"- add relation varnish:webcache - wp:cache",
//	)
//	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:cache varnish:webcache")
//	s.assertUnitsCreated(c, map[string]string{
//		"mysql/0":   "0",
//		"varnish/0": "1",
//		"wp/0":      "2",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleMachinesUnitsPlacement(c *gc.C) {
//	mysqlch := s.setupCharm(c, "xenial/mysql-2", "mysql", "bionic")
//	wpch := s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
//
//	content := `
//        applications:
//            wp:
//                charm: cs:xenial/wordpress-0
//                num_units: 2
//                to:
//                    - 1
//                    - lxd:2
//                options:
//                    blog-title: these are the voyages
//            sql:
//                charm: cs:xenial/mysql
//                num_units: 2
//                to:
//                    - lxd:wp/0
//                    - new
//        machines:
//            1:
//                series: xenial
//            2:
//    `
//	err := s.DeployBundleYAML(c, content)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"sql": {charm: "cs:xenial/mysql-2", config: mysqlch.Config().DefaultSettings()},
//		"wp": {
//			charm:  "cs:xenial/wordpress-0",
//			config: s.combinedSettings(wpch, charm.Settings{"blog-title": "these are the voyages"}),
//		},
//	})
//	s.assertRelationsEstablished(c)
//
//	// We explicitly pull out the map creation in the call to
//	// s.assertUnitsCreated() and create the map as a new variable
//	// because this /appears/ to tickle a bug on ppc64le using
//	// gccgo-4.9; the bug is that the map on the receiving side
//	// does not have the same contents as it does here - which is
//	// weird because that pattern is used elsewhere in this
//	// function. And just pulling the map instantiation out of the
//	// call is not enough; we need to do something benign with the
//	// variable to keep a reference beyond the call to the
//	// s.assertUnitsCreated(). I have to chosen to delete a
//	// non-existent key. This problem does not occur on amd64
//	// using gc or gccgo-4.9. Nor does it happen using go1.6 on
//	// ppc64. Once we switch to go1.6 across the board this change
//	// should be reverted. See http://pad.lv/1556116.
//	expectedUnits := map[string]string{
//		"sql/0": "0/lxd/0",
//		"sql/1": "2",
//		"wp/0":  "0",
//		"wp/1":  "1/lxd/0",
//	}
//	s.assertUnitsCreated(c, expectedUnits)
//	delete(expectedUnits, "non-existent")
//
//	// Redeploy the same bundle again.
//	err = s.DeployBundleYAML(c, content)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"sql/0": "0/lxd/0",
//		"sql/1": "2",
//		"wp/0":  "0",
//		"wp/1":  "1/lxd/0",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestLXCTreatedAsLXD(c *gc.C) {
//	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
//
//	// Note that we use lxc here, to represent a 1.x bundle that specifies lxc.
//	content := `
//        applications:
//            wp:
//                charm: cs:xenial/wordpress-0
//                num_units: 1
//                to:
//                    - lxc:0
//                options:
//                    blog-title: these are the voyages
//            wp2:
//                charm: cs:xenial/wordpress-0
//                num_units: 1
//                to:
//                    - lxc:0
//                options:
//                    blog-title: these are the voyages
//        machines:
//            0:
//                series: xenial
//    `
//	_, output, err := s.DeployBundleYAMLWithOutput(c, content)
//	c.Assert(err, jc.ErrorIsNil)
//	expectedUnits := map[string]string{
//		"wp/0":  "0/lxd/0",
//		"wp2/0": "0/lxd/1",
//	}
//	idx := strings.Index(output, "Bundle has one or more containers specified as lxc. lxc containers are deprecated in Juju 2.0. lxd containers will be deployed instead.")
//	lastIdx := strings.LastIndex(output, "Bundle has one or more containers specified as lxc. lxc containers are deprecated in Juju 2.0. lxd containers will be deployed instead.")
//	// The message exists.
//	c.Assert(idx, jc.GreaterThan, -1)
//	// No more than one instance of the message was printed.
//	c.Assert(idx, gc.Equals, lastIdx)
//	s.assertUnitsCreated(c, expectedUnits)
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleMachineAttributes(c *gc.C) {
//	ch := s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: cs:xenial/django-42
//                num_units: 2
//                to:
//                    - 1
//                    - new
//        machines:
//            1:
//                series: xenial
//                constraints: "cores=4 mem=4G"
//                annotations:
//                    foo: bar
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertApplicationsDeployed(c, map[string]applicationInfo{
//		"django": {charm: "cs:xenial/django-42", config: ch.Config().DefaultSettings()},
//	})
//	s.assertRelationsEstablished(c)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0": "0",
//		"django/1": "1",
//	})
//	m, err := s.State.Machine("0")
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(m.Series(), gc.Equals, "xenial")
//	cons, err := m.Constraints()
//	c.Assert(err, jc.ErrorIsNil)
//	expectedCons, err := constraints.Parse("cores=4 mem=4G")
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(cons, jc.DeepEquals, expectedCons)
//	ann, err := s.Model.Annotations(m)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(ann, jc.DeepEquals, map[string]string{"foo": "bar"})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleTwiceScaleUp(c *gc.C) {
//	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: cs:xenial/django-42
//                num_units: 2
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	err = s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: cs:xenial/django-42
//                num_units: 5
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0": "0",
//		"django/1": "1",
//		"django/2": "2",
//		"django/3": "3",
//		"django/4": "4",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitPlacedInApplication(c *gc.C) {
//	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
//	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            wordpress:
//                charm: wordpress
//                num_units: 3
//            django:
//                charm: cs:xenial/django-42
//                num_units: 2
//                to: [wordpress]
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0":    "0",
//		"django/1":    "1",
//		"wordpress/0": "0",
//		"wordpress/1": "1",
//		"wordpress/2": "2",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundlePeerContainer(c *gc.C) {
//	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
//	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
//
//	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
//        applications:
//            wordpress:
//                charm: wordpress
//                num_units: 2
//                to: ["lxd:new"]
//            django:
//                charm: cs:xenial/django-42
//                num_units: 2
//                to: ["lxd:wordpress"]
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Check(stdOut, gc.Equals, ""+
//		"Executing changes:\n"+
//		"- upload charm cs:xenial/django-42 for series xenial\n"+
//		"- deploy application django on xenial using cs:xenial/django-42\n"+
//		"- upload charm cs:xenial/wordpress-0 for series xenial\n"+
//		"- deploy application wordpress on xenial using cs:xenial/wordpress-0\n"+
//		"- add lxd container 0/lxd/0 on new machine 0\n"+
//		"- add lxd container 1/lxd/0 on new machine 1\n"+
//		"- add unit wordpress/0 to 0/lxd/0\n"+
//		"- add unit wordpress/1 to 1/lxd/0\n"+
//		"- add lxd container 0/lxd/1 on new machine 0\n"+
//		"- add lxd container 1/lxd/1 on new machine 1\n"+
//		"- add unit django/0 to 0/lxd/1 to satisfy [lxd:wordpress]\n"+
//		"- add unit django/1 to 1/lxd/1 to satisfy [lxd:wordpress]",
//	)
//
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0":    "0/lxd/1",
//		"django/1":    "1/lxd/1",
//		"wordpress/0": "0/lxd/0",
//		"wordpress/1": "1/lxd/0",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitColocationWithUnit(c *gc.C) {
//	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
//	s.setupCharm(c, "xenial/mem-47", "dummy", "bionic")
//	s.setupCharm(c, "xenial/rails-0", "dummy", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            memcached:
//                charm: cs:xenial/mem-47
//                num_units: 3
//                to: [1, new]
//            django:
//                charm: cs:xenial/django-42
//                num_units: 5
//                to:
//                    - memcached/0
//                    - lxd:memcached/1
//                    - lxd:memcached/2
//                    - kvm:ror
//            ror:
//                charm: rails
//                num_units: 2
//                to:
//                    - new
//                    - 1
//        machines:
//            1:
//                series: xenial
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0":    "0",
//		"django/1":    "1/lxd/0",
//		"django/2":    "2/lxd/0",
//		"django/3":    "3/kvm/0",
//		"django/4":    "0/kvm/0",
//		"memcached/0": "0",
//		"memcached/1": "1",
//		"memcached/2": "2",
//		"ror/0":       "3",
//		"ror/1":       "0",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitPlacedToMachines(c *gc.C) {
//	s.setupCharm(c, "bionic/django-42", "dummy", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: cs:django
//                num_units: 7
//                to:
//                    - new
//                    - 4
//                    - kvm:8
//                    - lxd:4
//                    - lxd:4
//                    - lxd:new
//        machines:
//            4:
//            8:
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0": "2",       // Machine "new" in the bundle.
//		"django/1": "0",       // Machine "4" in the bundle.
//		"django/2": "1/kvm/0", // The KVM container in bundle machine "8".
//		"django/3": "0/lxd/0", // First lxd container in bundle machine "4".
//		"django/4": "0/lxd/1", // Second lxd container in bundle machine "4".
//		"django/5": "3/lxd/0", // First lxd in new machine.
//		"django/6": "4/lxd/0", // Second lxd in new machine.
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleMassiveUnitColocation(c *gc.C) {
//	s.setupCharm(c, "bionic/django-42", "dummy", "bionic")
//	s.setupCharm(c, "bionic/mem-47", "dummy", "bionic")
//	s.setupCharm(c, "bionic/rails-0", "dummy", "bionic")
//
//	err := s.DeployBundleYAML(c, `
//        applications:
//            memcached:
//                charm: cs:bionic/mem-47
//                num_units: 3
//                to: [1, 2, 3]
//            django:
//                charm: cs:bionic/django-42
//                num_units: 4
//                to:
//                    - 1
//                    - lxd:memcached
//            ror:
//                charm: rails
//                num_units: 3
//                to:
//                    - 1
//                    - kvm:3
//        machines:
//            1:
//            2:
//            3:
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0":    "0",
//		"django/1":    "0/lxd/0",
//		"django/2":    "1/lxd/0",
//		"django/3":    "2/lxd/0",
//		"memcached/0": "0",
//		"memcached/1": "1",
//		"memcached/2": "2",
//		"ror/0":       "0",
//		"ror/1":       "2/kvm/0",
//		"ror/2":       "3",
//	})
//
//	// Redeploy a very similar bundle with another application unit. The new unit
//	// is placed on the first unit of memcached. Due to ordering of the applications
//	// there is no deterministic way to determine "least crowded" in a meaningful way.
//	content := `
//        applications:
//            memcached:
//                charm: cs:bionic/mem-47
//                num_units: 3
//                to: [1, 2, 3]
//            django:
//                charm: cs:bionic/django-42
//                num_units: 4
//                to:
//                    - 1
//                    - lxd:memcached
//            node:
//                charm: cs:bionic/django-42
//                num_units: 1
//                to:
//                    - lxd:memcached
//        machines:
//            1:
//            2:
//            3:
//    `
//	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, content)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Check(stdOut, gc.Equals, ""+
//		"Executing changes:\n"+
//		"- deploy application node on bionic using cs:bionic/django-42\n"+
//		"- add unit node/0 to 0/lxd/0 to satisfy [lxd:memcached]",
//	)
//
//	// Redeploy the same bundle again and check that nothing happens.
//	stdOut, _, err = s.DeployBundleYAMLWithOutput(c, content)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(stdOut, gc.Equals, "")
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0":    "0",
//		"django/1":    "0/lxd/0",
//		"django/2":    "1/lxd/0",
//		"django/3":    "2/lxd/0",
//		"memcached/0": "0",
//		"memcached/1": "1",
//		"memcached/2": "2",
//		"node/0":      "0/lxd/1",
//		"ror/0":       "0",
//		"ror/1":       "2/kvm/0",
//		"ror/2":       "3",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithAnnotations_OutputIsCorrect(c *gc.C) {
//	s.setupCharm(c, "bionic/django-42", "dummy", "bionic")
//	s.setupCharm(c, "bionic/mem-47", "dummy", "bionic")
//	stdOut, stdErr, err := s.DeployBundleYAMLWithOutput(c, `
//        applications:
//            django:
//                charm: cs:django
//                num_units: 1
//                annotations:
//                    key1: value1
//                    key2: value2
//                to: [1]
//            memcached:
//                charm: bionic/mem-47
//                num_units: 1
//        machines:
//            1:
//                annotations: {foo: bar}
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//
//	c.Check(stdOut, gc.Equals, ""+
//		"Executing changes:\n"+
//		"- upload charm cs:bionic/django-42 for series bionic\n"+
//		"- deploy application django on bionic using cs:bionic/django-42\n"+
//		"- set annotations for django\n"+
//		"- upload charm cs:bionic/mem-47 for series bionic\n"+
//		"- deploy application memcached on bionic using cs:bionic/mem-47\n"+
//		"- add new machine 0 (bundle machine 1)\n"+
//		"- set annotations for new machine 0\n"+
//		"- add unit django/0 to new machine 0\n"+
//		"- add unit memcached/0 to new machine 1",
//	)
//	c.Check(stdErr, gc.Equals, ""+
//		"Resolving charm: cs:django\n"+
//		"Resolving charm: bionic/mem-47\n"+
//		"Deploy of bundle completed.",
//	)
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleAnnotations(c *gc.C) {
//	s.setupCharm(c, "bionic/django", "django", "bionic")
//	s.setupCharm(c, "bionic/mem-47", "mem", "bionic")
//
//	err := s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: cs:django
//                num_units: 1
//                annotations:
//                    key1: value1
//                    key2: value2
//                to: [1]
//            memcached:
//                charm: bionic/mem-47
//                num_units: 1
//        machines:
//            1:
//                annotations: {foo: bar}
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	svc, err := s.State.Application("django")
//	c.Assert(err, jc.ErrorIsNil)
//	ann, err := s.Model.Annotations(svc)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(ann, jc.DeepEquals, map[string]string{
//		"key1": "value1",
//		"key2": "value2",
//	})
//	m, err := s.State.Machine("0")
//	c.Assert(err, jc.ErrorIsNil)
//	ann, err = s.Model.Annotations(m)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(ann, jc.DeepEquals, map[string]string{"foo": "bar"})
//
//	// Update the annotations and deploy the bundle again.
//	err = s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: cs:django
//                num_units: 1
//                annotations:
//                    key1: new value!
//                    key2: value2
//                to: [1]
//        machines:
//            1:
//                annotations: {answer: 42}
//    `)
//	c.Assert(err, jc.ErrorIsNil)
//	ann, err = s.Model.Annotations(svc)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(ann, jc.DeepEquals, map[string]string{
//		"key1": "new value!",
//		"key2": "value2",
//	})
//	ann, err = s.Model.Annotations(m)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(ann, jc.DeepEquals, map[string]string{
//		"foo":    "bar",
//		"answer": "42",
//	})
//}
//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundleExistingMachines(c *gc.C) {
//	xenialMachine := &factory.MachineParams{Series: "xenial"}
//	s.Factory.MakeMachine(c, xenialMachine) // machine-0
//	s.Factory.MakeMachine(c, xenialMachine) // machine-1
//	s.Factory.MakeMachine(c, xenialMachine) // machine-2
//	s.Factory.MakeMachine(c, xenialMachine) // machine-3
//	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
//	err := s.DeployBundleYAML(c, `
//        applications:
//            django:
//                charm: cs:django
//                num_units: 3
//                to: [0,1,2]
//        machines:
//            0:
//            1:
//            2:
//    `, "--map-machines", "existing,2=3")
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/0": "0",
//		"django/1": "1",
//		"django/2": "3",
//	})
//}
//
//type mockAllWatcher struct {
//	api.AllWatcher
//	next func() []params.Delta
//}
//
//func (w mockAllWatcher) Next() ([]params.Delta, error) {
//	return w.next(), nil
//}
//
//func (mockAllWatcher) Stop() error {
//	return nil
//}

//
//func (s *BundleDeployCharmStoreSuite) TestDeployBundlePassesSequences(c *gc.C) {
//	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
//
//	// Deploy another django app with two units, this will bump the sequences
//	// for machines and the django application. Then remove them both.
//	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
//		Name: "django"})
//	u1 := s.Factory.MakeUnit(c, &factory.UnitParams{
//		Application: app,
//	})
//	u2 := s.Factory.MakeUnit(c, &factory.UnitParams{
//		Application: app,
//	})
//	var machines []*state.Machine
//	var ids []string
//	destroyUnitsMachine := func(u *state.Unit) {
//		id, err := u.AssignedMachineId()
//		c.Assert(err, jc.ErrorIsNil)
//		ids = append(ids, id)
//		m, err := s.State.Machine(id)
//		c.Assert(err, jc.ErrorIsNil)
//		machines = append(machines, m)
//		c.Assert(m.ForceDestroy(time.Duration(0)), jc.ErrorIsNil)
//	}
//	// Tear them down. This is somewhat convoluted, but it is what we need
//	// to do to properly cleanly tear down machines.
//	c.Assert(app.Destroy(), jc.ErrorIsNil)
//	destroyUnitsMachine(u1)
//	destroyUnitsMachine(u2)
//	c.Assert(s.State.Cleanup(), jc.ErrorIsNil)
//	for _, m := range machines {
//		c.Assert(m.EnsureDead(), jc.ErrorIsNil)
//		c.Assert(m.MarkForRemoval(), jc.ErrorIsNil)
//	}
//	c.Assert(s.State.CompleteMachineRemovals(ids...), jc.ErrorIsNil)
//
//	// Now that the machines are removed, the units should be dead,
//	// we need 1 more Cleanup step to remove the applications.
//	c.Assert(s.State.Cleanup(), jc.ErrorIsNil)
//	apps, err := s.State.AllApplications()
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(apps, gc.HasLen, 0)
//	machines, err = s.State.AllMachines()
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(machines, gc.HasLen, 0)
//
//	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
//        applications:
//            django:
//                charm: cs:xenial/django-42
//                num_units: 2
//    `)
//	c.Check(stdOut, gc.Equals, ""+
//		"Executing changes:\n"+
//		"- upload charm cs:xenial/django-42 for series xenial\n"+
//		"- deploy application django on xenial using cs:xenial/django-42\n"+
//		"- add unit django/2 to new machine 2\n"+
//		"- add unit django/3 to new machine 3",
//	)
//	c.Assert(err, jc.ErrorIsNil)
//	s.assertUnitsCreated(c, map[string]string{
//		"django/2": "2",
//		"django/3": "3",
//	})
//}

type fakeProvider struct {
	caas.ContainerEnvironProvider
}

func (*fakeProvider) Open(_ environs.OpenParams) (caas.Broker, error) {
	return &fakeBroker{}, nil
}

func (*fakeProvider) Validate(cfg, old *config.Config) (valid *config.Config, _ error) {
	return cfg, nil
}

type fakeBroker struct {
	caas.Broker
}

type mockProvider struct {
	storage.Provider
}

func (m *mockProvider) Supports(kind storage.StorageKind) bool {
	return kind == storage.StorageKindFilesystem
}

func (*fakeBroker) StorageProvider(p storage.ProviderType) (storage.Provider, error) {
	if p == k8sprovider.K8s_ProviderType {
		return &mockProvider{}, nil
	}
	return nil, errors.NotFoundf("provider type %q", p)
}

func (*fakeBroker) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

func (*fakeBroker) PrecheckInstance(context.ProviderCallContext, environs.PrecheckInstanceParams) error {
	return nil
}

func (*fakeBroker) Version() (*version.Number, error) {
	ver := version.MustParse("1.15.1")
	return &ver, nil
}

func (*fakeBroker) ValidateStorageClass(_ map[string]interface{}) error {
	return nil
}
