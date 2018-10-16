// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"

	"github.com/juju/juju/api"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNotFoundCharmStore(c *gc.C) {
	err := runDeploy(c, "bundle/no-such")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:bundle/no-such": bundle not found`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidFlags(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	err := runDeploy(c, "bundle/wordpress-simple", "--config", "config.yaml")
	c.Assert(err, gc.ErrorMatches, "flags provided but not supported when deploying a bundle: --config")
	err = runDeploy(c, "bundle/wordpress-simple", "-n", "2")
	c.Assert(err, gc.ErrorMatches, "flags provided but not supported when deploying a bundle: -n")
	err = runDeploy(c, "bundle/wordpress-simple", "--series", "xenial")
	c.Assert(err, gc.ErrorMatches, "flags provided but not supported when deploying a bundle: --series")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleSuccess(c *gc.C) {
	_, mysqlch := testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	err := runDeploy(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":     {charm: "cs:xenial/mysql-42", config: mysqlch.Config().DefaultSettings()},
		"wordpress": {charm: "cs:xenial/wordpress-47", config: wpch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestAddMetricCredentials(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-with-plans-1", "wordpress-with-plans")

	deploy := NewDeployCommandForTest(
		nil,
		[]DeployStep{&RegisterMeteredCharm{PlanURL: s.server.URL, RegisterPath: "", QueryPath: ""}},
	)
	_, err := cmdtesting.RunCommand(c, deploy, "bundle/wordpress-with-plans")
	c.Assert(err, jc.ErrorIsNil)

	// The order of calls here does not matter and is, in fact, not guaranteed.
	// All we care about here is that the calls exist.
	s.stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "DefaultPlan",
		Args:     []interface{}{"cs:wordpress"},
	}, {
		FuncName: "Authorize",
		Args: []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:wordpress",
			ApplicationName: "wordpress",
			PlanURL:         "thisplan",
			IncreaseBudget:  0,
		}},
	}, {
		FuncName: "Authorize",
		Args: []interface{}{metricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:mysql",
			ApplicationName: "mysql",
			PlanURL:         "test/plan",
			IncreaseBudget:  0,
		}},
	}})

	mysqlApp, err := s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysqlApp.MetricCredentials(), jc.DeepEquals, append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA))

	wordpressApp, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpressApp.MetricCredentials(), jc.DeepEquals, append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA))
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithTermsSuccess(c *gc.C) {
	_, ch1 := testcharms.UploadCharm(c, s.client, "xenial/terms1-17", "terms1")
	_, ch2 := testcharms.UploadCharm(c, s.client, "xenial/terms2-42", "terms2")
	testcharms.UploadBundle(c, s.client, "bundle/terms-simple-1", "terms-simple")
	err := runDeploy(c, "bundle/terms-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/terms1-17", "cs:xenial/terms2-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"terms1": {charm: "cs:xenial/terms1-17", config: ch1.Config().DefaultSettings()},
		"terms2": {charm: "cs:xenial/terms2-42", config: ch2.Config().DefaultSettings()},
	})
	s.assertUnitsCreated(c, map[string]string{
		"terms1/0": "0",
		"terms2/0": "1",
	})
	c.Assert(s.termsString, gc.Not(gc.Equals), "")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleStorage(c *gc.C) {
	_, mysqlch := testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql-storage")
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-with-mysql-storage-1", "wordpress-with-mysql-storage")
	err := runDeploy(
		c, "bundle/wordpress-with-mysql-storage",
		"--storage", "mysql:logs=tmpfs,10G", // override logs
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql": {
			charm:  "cs:xenial/mysql-42",
			config: mysqlch.Config().DefaultSettings(),
			storage: map[string]state.StorageConstraints{
				"data": {Pool: "rootfs", Size: 50 * 1024, Count: 1},
				"logs": {Pool: "tmpfs", Size: 10 * 1024, Count: 1},
			},
		},
		"wordpress": {charm: "cs:xenial/wordpress-47", config: wpch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

type CAASModelDeployCharmStoreSuite struct {
	CAASDeploySuiteBase
}

var _ = gc.Suite(&CAASModelDeployCharmStoreSuite{})

func (s *CAASModelDeployCharmStoreSuite) TestDeployBundleDevices(c *gc.C) {
	c.Skip("Test disabled until flakiness is fixed - see bug lp:1781250")

	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, minerCharm := testcharms.UploadCharmWithSeries(c, s.client, "kubernetes/bitcoin-miner-1", "bitcoin-miner", "kubernetes")
	_, dashboardCharm := testcharms.UploadCharmWithSeries(c, s.client, "kubernetes/dashboard4miner-3", "dashboard4miner", "kubernetes")

	testcharms.UploadBundle(c, s.client, "bundle/bitcoinminer-with-dashboard-1", "bitcoinminer-with-dashboard")
	err = runDeploy(
		c, "bundle/bitcoinminer-with-dashboard",
		"-m", m.Name(),
		"--device", "miner:bitcoinminer=10,nvidia.com/gpu", // override bitcoinminer
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:kubernetes/dashboard4miner-3", "cs:kubernetes/bitcoin-miner-1")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"miner": {
			charm:  "cs:kubernetes/bitcoin-miner-1",
			config: minerCharm.Config().DefaultSettings(),
			devices: map[string]state.DeviceConstraints{
				"bitcoinminer": {Type: "nvidia.com/gpu", Count: 10, Attributes: map[string]string{}},
			},
		},
		"dashboard": {charm: "cs:kubernetes/dashboard4miner-3", config: dashboardCharm.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "dashboard:miner miner:miner")
	s.assertUnitsCreated(c, map[string]string{
		"miner/0":     "",
		"dashboard/0": "",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleEndpointBindingsSpaceMissing(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings")
	stdOut, stdErr, err := runDeployWithOutput(c, "bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, gc.ErrorMatches, ""+
		"cannot deploy bundle: cannot deploy application \"mysql\": "+
		"cannot add application \"mysql\": unknown space \"db\" not valid")
	c.Assert(stdErr, gc.Equals, ""+
		`Located bundle "cs:bundle/wordpress-with-endpoint-bindings-1"`+"\n"+
		"Resolving charm: mysql\n"+
		"Resolving charm: wordpress-extra-bindings")
	c.Assert(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:xenial/mysql-42 for series xenial\n"+
		"- deploy application mysql on xenial using cs:xenial/mysql-42")
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{})
	s.assertUnitsCreated(c, map[string]string{})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleEndpointBindingsSuccess(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	_, mysqlch := testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings")
	err = runDeploy(c, "bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-extra-bindings-47")

	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":                    {charm: "cs:xenial/mysql-42", config: mysqlch.Config().DefaultSettings()},
		"wordpress-extra-bindings": {charm: "cs:xenial/wordpress-extra-bindings-47", config: wpch.Config().DefaultSettings()},
	})
	s.assertDeployedApplicationBindings(c, map[string]applicationInfo{
		"mysql": {
			endpointBindings: map[string]string{"server": "db", "server-admin": ""},
		},
		"wordpress-extra-bindings": {
			endpointBindings: map[string]string{
				"cache":           "",
				"url":             "public",
				"logging-dir":     "",
				"monitoring-port": "",
				"db":              "db",
				"cluster":         "",
				"db-client":       "db",
				"admin-api":       "public",
				"foo-bar":         "",
			},
		},
	})
	s.assertRelationsEstablished(c, "wordpress-extra-bindings:cluster", "wordpress-extra-bindings:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":                    "0",
		"wordpress-extra-bindings/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleTwice(c *gc.C) {
	_, mysqlch := testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	stdOut, stdErr, err := runDeployWithOutput(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:xenial/mysql-42 for series xenial\n"+
		"- deploy application mysql on xenial using cs:xenial/mysql-42\n"+
		"- set annotations for mysql\n"+
		"- upload charm cs:xenial/wordpress-47 for series xenial\n"+
		"- deploy application wordpress on xenial using cs:xenial/wordpress-47\n"+
		"- set annotations for wordpress\n"+
		"- add relation wordpress:db - mysql:server\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1",
	)
	stdOut, stdErr, err = runDeployWithOutput(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	// Nothing to do...
	c.Check(stdOut, gc.Equals, "")
	c.Check(stdErr, gc.Equals, ""+
		"Located bundle \"cs:bundle/wordpress-simple-1\"\n"+
		"Resolving charm: mysql\n"+
		"Resolving charm: wordpress\n"+
		"No changes to apply.",
	)

	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":     {charm: "cs:xenial/mysql-42", config: mysqlch.Config().DefaultSettings()},
		"wordpress": {charm: "cs:xenial/wordpress-47", config: wpch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDryRunTwice(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	stdOut, _, err := runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	expected := "" +
		"Changes to deploy bundle:\n" +
		"- upload charm cs:xenial/mysql-42 for series xenial\n" +
		"- deploy application mysql on xenial using cs:xenial/mysql-42\n" +
		"- set annotations for mysql\n" +
		"- upload charm cs:xenial/wordpress-47 for series xenial\n" +
		"- deploy application wordpress on xenial using cs:xenial/wordpress-47\n" +
		"- set annotations for wordpress\n" +
		"- add relation wordpress:db - mysql:server\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit wordpress/0 to new machine 1"

	c.Check(stdOut, gc.Equals, expected)
	stdOut, _, err = runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, expected)

	s.assertCharmsUploaded(c /* none */)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{})
	s.assertRelationsEstablished(c /* none */)
	s.assertUnitsCreated(c, map[string]string{})
}

func (s *BundleDeployCharmStoreSuite) TestDryRunExistingModel(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadCharm(c, s.client, "trusty/multi-series-subordinate-13", "multi-series-subordinate")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	// Start with a mysql that already has the right charm.
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql", Series: "xenial", Revision: "42"})
	mysql := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "mysql", Charm: ch})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})
	// Also add a subordinate that isn't attached to anything.
	sub := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "multi-series-subordinate", Series: "trusty", Revision: "13"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "sub", Charm: sub})

	stdOut, _, err := runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	expected := "" +
		"Changes to deploy bundle:\n" +
		"- set annotations for mysql\n" +
		"- upload charm cs:xenial/wordpress-47 for series xenial\n" +
		"- deploy application wordpress on xenial using cs:xenial/wordpress-47\n" +
		"- set annotations for wordpress\n" +
		"- add relation wordpress:db - mysql:server\n" +
		"- add unit wordpress/0 to new machine 1"

	c.Check(stdOut, gc.Equals, expected)
	stdOut, _, err = runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, expected)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleGatedCharm(c *gc.C) {
	_, mysqlch := testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	url, _ := testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	s.changeReadPerm(c, url, clientUserName)
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	err := runDeploy(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	ch, err := s.State.Charm(charm.MustParseURL("cs:xenial/wordpress-47"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":     {charm: "cs:xenial/mysql-42", config: mysqlch.Config().DefaultSettings()},
		"wordpress": {charm: "cs:xenial/wordpress-47", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalPath(c *gc.C) {
	dir := c.MkDir()
	testcharms.Repo.ClonedDir(dir, "dummy")
	path := filepath.Join(dir, "mybundle")
	data := `
        series: xenial
        applications:
            dummy:
                charm: ./dummy
                series: xenial
                num_units: 1
    `
	err := ioutil.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:xenial/dummy-1")
	ch, err := s.State.Charm(charm.MustParseURL("local:xenial/dummy-1"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {charm: "local:xenial/dummy-1", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalResources(c *gc.C) {
	data := `
        series: quantal
        applications:
            "dummy-resource":
                charm: ./dummy-resource
                series: quantal
                num_units: 1
                resources:
                  dummy: ./dummy-resource.zip
    `
	dir := s.makeBundleDir(c, data)
	testcharms.Repo.ClonedDir(dir, "dummy-resource")
	c.Assert(
		ioutil.WriteFile(filepath.Join(dir, "dummy-resource.zip"), []byte("zip file"), 0644),
		jc.ErrorIsNil)
	err := runDeploy(c, dir)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:quantal/dummy-resource-0")
	ch, err := s.State.Charm(charm.MustParseURL("local:quantal/dummy-resource-0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy-resource": {charm: "local:quantal/dummy-resource-0", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNoSeriesInCharmURL(c *gc.C) {
	testcharms.UploadCharmMultiSeries(c, s.client, "~who/multi-series", "multi-series")
	dir := c.MkDir()
	testcharms.Repo.ClonedDir(dir, "dummy")
	path := filepath.Join(dir, "mybundle")
	data := `
        series: trusty
        applications:
            dummy:
                charm: cs:~who/multi-series
    `
	err := ioutil.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:~who/multi-series-0")
	ch, err := s.State.Charm(charm.MustParseURL("~who/multi-series-0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {charm: "cs:~who/multi-series-0", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleGatedCharmUnauthorized(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	url, _ := testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	s.changeReadPerm(c, url, "who")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	err := runDeploy(c, "bundle/wordpress-simple")
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: .*: access denied for user "client-username"`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleResources(c *gc.C) {
	testcharms.UploadCharm(c, s.Client(), "trusty/starsay-42", "starsay")
	bundleMeta := `
        applications:
            starsay:
                charm: cs:starsay
                num_units: 1
                resources:
                    store-resource: 0
                    install-resource: 0
                    upload-resource: 0
    `
	stdOut, stdErr, err := s.DeployBundleYAMLWithOutput(c, bundleMeta)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:trusty/starsay-42 for series trusty\n"+
		"- deploy application starsay on trusty using cs:trusty/starsay-42\n"+
		"- add unit starsay/0 to new machine 0",
	)
	// Info messages go to stdErr.
	c.Check(stdErr, gc.Equals, ""+
		"Resolving charm: cs:starsay\n"+
		"  added resource install-resource\n"+
		"  added resource store-resource\n"+
		"  added resource upload-resource\n"+
		"Deploy of bundle completed.",
	)

	resourceHash := func(content string) charmresource.Fingerprint {
		fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
		c.Assert(err, jc.ErrorIsNil)
		return fp
	}

	s.checkResources(c, "starsay", []resource.Resource{{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "install-resource",
				Type:        charmresource.TypeFile,
				Path:        "gotta-have-it.txt",
				Description: "get things started",
			},
			Origin:      charmresource.OriginStore,
			Revision:    0,
			Fingerprint: resourceHash("install-resource content"),
			Size:        int64(len("install-resource content")),
		},
		ID:            "starsay/install-resource",
		ApplicationID: "starsay",
	}, {
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "store-resource",
				Type:        charmresource.TypeFile,
				Path:        "filename.tgz",
				Description: "One line that is useful when operators need to push it.",
			},
			Origin:      charmresource.OriginStore,
			Fingerprint: resourceHash("store-resource content"),
			Size:        int64(len("store-resource content")),
			Revision:    0,
		},
		ID:            "starsay/store-resource",
		ApplicationID: "starsay",
	}, {
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "upload-resource",
				Type:        charmresource.TypeFile,
				Path:        "somename.xml",
				Description: "Who uses xml anymore?",
			},
			Origin:      charmresource.OriginStore,
			Fingerprint: resourceHash("upload-resource content"),
			Size:        int64(len("upload-resource content")),
			Revision:    0,
		},
		ID:            "starsay/upload-resource",
		ApplicationID: "starsay",
	}})
}

func (s *BundleDeployCharmStoreSuite) checkResources(c *gc.C, serviceapplication string, expected []resource.Resource) {
	_, err := s.State.Application("starsay")
	c.Check(err, jc.ErrorIsNil)
	st, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	svcResources, err := st.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)
	resources := svcResources.Resources
	resource.Sort(resources)
	c.Assert(resources, jc.DeepEquals, expected)
}

type BundleDeployCharmStoreSuite struct {
	charmStoreSuite

	stub   *testing.Stub
	server *httptest.Server
}

var _ = gc.Suite(&BundleDeployCharmStoreSuite{})

func (s *BundleDeployCharmStoreSuite) SetUpSuite(c *gc.C) {
	s.charmStoreSuite.SetUpSuite(c)
	s.PatchValue(&watcher.Period, 10*time.Millisecond)
}

func (s *BundleDeployCharmStoreSuite) SetUpTest(c *gc.C) {
	s.stub = &testing.Stub{}
	handler := &testMetricsRegistrationHandler{Stub: s.stub}
	s.server = httptest.NewServer(handler)
	// Set metering URL config so the config is set during bootstrap
	if s.ControllerConfigAttrs == nil {
		s.ControllerConfigAttrs = make(map[string]interface{})
	}
	s.ControllerConfigAttrs[controller.MeteringURL] = s.server.URL

	s.charmStoreSuite.SetUpTest(c)
	logger.SetLogLevel(loggo.TRACE)

	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.LXDProfile)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *BundleDeployCharmStoreSuite) TearDownTest(c *gc.C) {
	if s.server != nil {
		s.server.Close()
	}
	s.charmStoreSuite.TearDownTest(c)
}

func (s *BundleDeployCharmStoreSuite) Client() *csclient.Client {
	return s.client
}

// DeployBundleYAML uses the given bundle content to create a bundle in the
// local repository and then deploy it. It returns the bundle deployment output
// and error.
func (s *BundleDeployCharmStoreSuite) DeployBundleYAML(c *gc.C, content string, extraArgs ...string) error {
	_, _, err := s.DeployBundleYAMLWithOutput(c, content, extraArgs...)
	return err
}

func (s *BundleDeployCharmStoreSuite) DeployBundleYAMLWithOutput(c *gc.C, content string, extraArgs ...string) (string, string, error) {
	bundlePath := s.makeBundleDir(c, content)
	args := append([]string{bundlePath}, extraArgs...)
	return runDeployWithOutput(c, args...)
}

func (s *BundleDeployCharmStoreSuite) makeBundleDir(c *gc.C, content string) string {
	bundlePath := filepath.Join(c.MkDir(), "example")
	c.Assert(os.Mkdir(bundlePath, 0777), jc.ErrorIsNil)
	err := ioutil.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return bundlePath
}

var deployBundleErrorsTests = []struct {
	about   string
	content string
	err     string
}{{
	about: "local charm not found",
	content: `
        applications:
            mysql:
                charm: ./mysql
                num_units: 1
    `,
	err: `the provided bundle has the following errors:
charm path in application "mysql" does not exist: .*mysql`,
}, {
	about: "charm store charm not found",
	content: `
        applications:
            rails:
                charm: xenial/rails-42
                num_units: 1
    `,
	err: `cannot resolve URL "xenial/rails-42": cannot resolve URL "cs:xenial/rails-42": charm not found`,
}, {
	about:   "invalid bundle content",
	content: "!",
	err:     `(?s)cannot unmarshal bundle data: yaml: .*`,
}, {
	about: "invalid bundle data",
	content: `
        applications:
            mysql:
                charm: mysql
                num_units: -1
    `,
	err: `the provided bundle has the following errors:
negative number of units specified on application "mysql"`,
}, {
	about: "invalid constraints",
	content: `
        applications:
            mysql:
                charm: mysql
                num_units: 1
                constraints: bad-wolf
    `,
	err: `the provided bundle has the following errors:
invalid constraints "bad-wolf" in application "mysql": malformed constraint "bad-wolf"`,
}, {
	about: "multiple bundle verification errors",
	content: `
        applications:
            mysql:
                charm: mysql
                num_units: -1
                constraints: bad-wolf
    `,
	err: `the provided bundle has the following errors:
invalid constraints "bad-wolf" in application "mysql": malformed constraint "bad-wolf"
negative number of units specified on application "mysql"`,
}, {
	about: "bundle inception",
	content: `
        applications:
            example:
                charm: local:wordpress
                num_units: 1
    `,
	err: `cannot resolve URL "local:wordpress": unknown schema for charm URL "local:wordpress"`,
}}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleErrors(c *gc.C) {
	for i, test := range deployBundleErrorsTests {
		c.Logf("test %d: %s", i, test.about)
		err := s.DeployBundleYAML(c, test.content)
		pass := c.Check(err, gc.ErrorMatches, "cannot deploy bundle: "+test.err)
		if !pass {
			c.Logf("error: \n%s\n", errors.ErrorStack(err))
		}
	}
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidOptions(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: xenial/wordpress-42
                num_units: 1
                options:
                    blog-title: 42
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": option "blog-title" expected string, got 42`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidMachineContainerType(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: xenial/wordpress
                num_units: 1
                to: ["bad:1"]
        machines:
            1:
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot create machine for holding wp unit: invalid container type "bad"`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidSeries(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "vivid/django-0", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: vivid/django
                num_units: 1
                to:
                    - 1
        machines:
            1:
                series: xenial
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot add unit for application "django": adding new machine to host unit "django/0": cannot assign unit "django/0" to machine 0: series does not match`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidBinding(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: xenial/wordpress-42
                num_units: 1
                bindings:
                  noturl: public
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": invalid binding\(s\) supplied "noturl", valid binding names are "admin-api",.* "url"`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidSpace(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: xenial/wordpress-42
                num_units: 1
                bindings:
                  url: public
    `)
	// TODO(jam): 2017-02-05 double repeating "cannot deploy application" and "cannot add application" is a bit ugly
	// https://pad.lv/1661937
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": cannot add application "wp": unknown space "public" not valid`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWatcherTimeout(c *gc.C) {
	// Inject an "AllWatcher" that never delivers a result.
	ch := make(chan struct{})
	defer close(ch)
	watcher := mockAllWatcher{
		next: func() []multiwatcher.Delta {
			<-ch
			return nil
		},
	}
	s.PatchValue(&watchAll, func(*api.Client) (allWatcher, error) {
		return watcher, nil
	})

	testcharms.UploadCharm(c, s.client, "xenial/django-0", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")
	s.PatchValue(&updateUnitStatusPeriod, 0*time.Second)
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: django
                num_units: 1
            wordpress:
                charm: wordpress
                num_units: 1
                to: [django]
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot retrieve placement for "wordpress" unit: cannot resolve machine: timeout while trying to get new changes from the watcher`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeployment(c *gc.C) {
	charmsPath := c.MkDir()
	mysqlPath := testcharms.Repo.ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.Repo.ClonedDirPath(charmsPath, "wordpress")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
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
    `, wordpressPath, mysqlPath))
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:xenial/mysql-1", "local:xenial/wordpress-3")
	mysqlch, err := s.State.Charm(charm.MustParseURL("local:xenial/mysql-1"))
	c.Assert(err, jc.ErrorIsNil)
	wpch, err := s.State.Charm(charm.MustParseURL("local:xenial/wordpress-3"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":     {charm: "local:xenial/mysql-1", config: mysqlch.Config().DefaultSettings()},
		"wordpress": {charm: "local:xenial/wordpress-3", config: wpch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"mysql/1":     "1",
		"wordpress/0": "2",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentBadConfig(c *gc.C) {
	charmsPath := c.MkDir()
	mysqlPath := testcharms.Repo.ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.Repo.ClonedDirPath(charmsPath, "wordpress")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
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
    `, wordpressPath, mysqlPath),
		"--overlay", "missing-file")
	c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: unable to read bundle overlay file .*")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentLXDProfile(c *gc.C) {
	charmsPath := c.MkDir()
	lxdProfilePath := testcharms.Repo.ClonedDirPath(charmsPath, "lxd-profile")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
        series: bionic
        services:
            lxd-profile:
                charm: %s
                num_units: 1
    `, lxdProfilePath))
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:bionic/lxd-profile-0")
	lxdProfile, err := s.State.Charm(charm.MustParseURL("local:bionic/lxd-profile-0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"lxd-profile": {charm: "local:bionic/lxd-profile-0", config: lxdProfile.Config().DefaultSettings()},
	})
	s.assertUnitsCreated(c, map[string]string{
		"lxd-profile/0": "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentBadLXDProfile(c *gc.C) {
	charmsPath := c.MkDir()
	lxdProfilePath := testcharms.Repo.ClonedDirPath(charmsPath, "lxd-profile-fail")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
        series: bionic
        services:
            lxd-profile-fail:
                charm: %s
                num_units: 1
    `, lxdProfilePath))
	c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: cannot deploy local charm at .*: invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentWithBundleOverlay(c *gc.C) {
	configDir := c.MkDir()
	configFile := filepath.Join(configDir, "config.yaml")
	c.Assert(
		ioutil.WriteFile(
			configFile, []byte(`
                applications:
                    wordpress:
                        options:
                            blog-title: include-file://title
            `), 0644),
		jc.ErrorIsNil)
	c.Assert(
		ioutil.WriteFile(
			filepath.Join(configDir, "title"), []byte("magic bundle config"), 0644),
		jc.ErrorIsNil)

	charmsPath := c.MkDir()
	mysqlPath := testcharms.Repo.ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.Repo.ClonedDirPath(charmsPath, "wordpress")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
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
    `, wordpressPath, mysqlPath),
		"--overlay", configFile)

	c.Assert(err, jc.ErrorIsNil)
	// Now check the blog-title of the wordpress.	le")
	wordpress, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := wordpress.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings["blog-title"], gc.Equals, "magic bundle config")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalAndCharmStoreCharms(c *gc.C) {
	charmsPath := c.MkDir()
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	mysqlPath := testcharms.Repo.ClonedDirPath(charmsPath, "mysql")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
        series: xenial
        applications:
            wordpress:
                charm: xenial/wordpress-42
                series: xenial
                num_units: 1
            mysql:
                charm: %s
                num_units: 1
        relations:
            - ["wordpress:db", "mysql:server"]
    `, mysqlPath))
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:xenial/mysql-1", "cs:xenial/wordpress-42")
	mysqlch, err := s.State.Charm(charm.MustParseURL("local:xenial/mysql-1"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":     {charm: "local:xenial/mysql-1", config: mysqlch.Config().DefaultSettings()},
		"wordpress": {charm: "cs:xenial/wordpress-42", config: wpch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationOptions(c *gc.C) {
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	_, dch := testcharms.UploadCharm(c, s.client, "precise/dummy-0", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                options:
                    blog-title: these are the voyages
            customized:
                charm: precise/dummy-0
                num_units: 1
                options:
                    username: who
                    skill-level: 47
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:precise/dummy-0", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"customized": {
			charm:  "cs:precise/dummy-0",
			config: s.combinedSettings(dch, charm.Settings{"username": "who", "skill-level": int64(47)}),
		},
		"wordpress": {
			charm:  "cs:xenial/wordpress-42",
			config: s.combinedSettings(wpch, charm.Settings{"blog-title": "these are the voyages"}),
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"wordpress/0":  "1",
		"customized/0": "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationConstraints(c *gc.C) {
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	_, dch := testcharms.UploadCharm(c, s.client, "precise/dummy-0", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: wordpress
                constraints: mem=4G cores=2
            customized:
                charm: precise/dummy-0
                num_units: 1
                constraints: arch=i386
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:precise/dummy-0", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"customized": {
			charm:       "cs:precise/dummy-0",
			constraints: constraints.MustParse("arch=i386"),
			config:      dch.Config().DefaultSettings(),
		},
		"wordpress": {
			charm:       "cs:xenial/wordpress-42",
			constraints: constraints.MustParse("mem=4G cores=2"),
			config:      wpch.Config().DefaultSettings(),
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"customized/0": "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleSetAnnotations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	err := runDeploy(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	ann, err := s.Model.Annotations(application)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{"bundleURL": "cs:bundle/wordpress-simple-1"})
	application2, err := s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	ann2, err := s.Model.Annotations(application2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann2, jc.DeepEquals, map[string]string{"bundleURL": "cs:bundle/wordpress-simple-1"})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationUpgrade(c *gc.C) {
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	testcharms.UploadCharm(c, s.client, "vivid/upgrade-1", "upgrade1")
	_, ch := testcharms.UploadCharm(c, s.client, "vivid/upgrade-2", "upgrade2")

	// First deploy the bundle.
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                options:
                    blog-title: these are the voyages
                constraints: spaces=final,frontiers mem=8000M
            up:
                charm: vivid/upgrade-1
                num_units: 1
                constraints: mem=8G
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:vivid/upgrade-1", "cs:xenial/wordpress-42")

	// Then deploy a new bundle with modified charm revision and options.
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                options:
                    blog-title: new title
                constraints: spaces=new cores=8
            up:
                charm: vivid/upgrade-2
                num_units: 1
                constraints: mem=8G
    `)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:vivid/upgrade-2 for series vivid\n"+
		"- upgrade up to use charm cs:vivid/upgrade-2 for series vivid\n"+
		"- set application options for wordpress\n"+
		`- set constraints for wordpress to "spaces=new cores=8"`,
	)

	s.assertCharmsUploaded(c, "cs:vivid/upgrade-1", "cs:vivid/upgrade-2", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"up": {
			charm:       "cs:vivid/upgrade-2",
			config:      ch.Config().DefaultSettings(),
			constraints: constraints.MustParse("mem=8G"),
		},
		"wordpress": {
			charm:       "cs:xenial/wordpress-42",
			config:      s.combinedSettings(wpch, charm.Settings{"blog-title": "new title"}),
			constraints: constraints.MustParse("spaces=new cores=8"),
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"up/0":        "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleExpose(c *gc.C) {
	_, ch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	content := `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                expose: true
    `
	expectedApplications := map[string]applicationInfo{
		"wordpress": {
			charm:   "cs:xenial/wordpress-42",
			config:  ch.Config().DefaultSettings(),
			exposed: true,
		},
	}

	// First deploy the bundle.
	err := s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, expectedApplications)

	// Then deploy the same bundle again: no error is produced when the application
	// is exposed again.
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, expectedApplications)
	c.Check(stdOut, gc.Equals, "") // Nothing to do.

	// Then deploy a bundle with the application unexposed, and check that the
	// application is not unexposed.
	stdOut, _, err = s.DeployBundleYAMLWithOutput(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                expose: false
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, expectedApplications)
	c.Check(stdOut, gc.Equals, "") // Nothing to do.
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationUpgradeFailure(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Try upgrading to a different series.
	// Note that this test comes before the next one because
	// otherwise we can't resolve the charm URL because the charm's
	// "base entity" is not marked as promulgated so the query by
	// promulgated will find it.
	testcharms.UploadCharm(c, s.client, "vivid/wordpress-42", "wordpress")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: vivid/wordpress
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade application "wordpress" to charm "cs:vivid/wordpress-42": cannot change an application's series`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMultipleRelations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")
	testcharms.UploadCharm(c, s.client, "xenial/mysql-1", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/postgres-2", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/varnish-3", "varnish")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: wordpress
                num_units: 1
            mysql:
                charm: mysql
                num_units: 1
            pgres:
                charm: xenial/postgres-2
                num_units: 1
            varnish:
                charm: xenial/varnish
                num_units: 1
        relations:
            - ["wp:db", "mysql:server"]
            - ["wp:db", "pgres:server"]
            - ["varnish:webcache", "wp:cache"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:db pgres:server", "wp:cache varnish:webcache")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":   "0",
		"pgres/0":   "1",
		"varnish/0": "2",
		"wp/0":      "3",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNewRelations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")
	testcharms.UploadCharm(c, s.client, "xenial/mysql-1", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/postgres-2", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/varnish-3", "varnish")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: wordpress
                num_units: 1
            mysql:
                charm: mysql
                num_units: 1
            varnish:
                charm: xenial/varnish
                num_units: 1
        relations:
            - ["wp:db", "mysql:server"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            wp:
                charm: wordpress
                num_units: 1
            mysql:
                charm: mysql
                num_units: 1
            varnish:
                charm: xenial/varnish
                num_units: 1
        relations:
            - ["wp:db", "mysql:server"]
            - ["varnish:webcache", "wp:cache"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- add relation varnish:webcache - wp:cache",
	)
	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:cache varnish:webcache")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":   "0",
		"varnish/0": "1",
		"wp/0":      "2",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMachinesUnitsPlacement(c *gc.C) {
	_, wpch := testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")
	_, mysqlch := testcharms.UploadCharm(c, s.client, "xenial/mysql-2", "mysql")

	content := `
        applications:
            wp:
                charm: cs:xenial/wordpress-0
                num_units: 2
                to:
                    - 1
                    - lxd:2
                options:
                    blog-title: these are the voyages
            sql:
                charm: cs:xenial/mysql
                num_units: 2
                to:
                    - lxd:wp/0
                    - new
        machines:
            1:
                series: xenial
            2:
    `
	err := s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"sql": {charm: "cs:xenial/mysql-2", config: mysqlch.Config().DefaultSettings()},
		"wp": {
			charm:  "cs:xenial/wordpress-0",
			config: s.combinedSettings(wpch, charm.Settings{"blog-title": "these are the voyages"}),
		},
	})
	s.assertRelationsEstablished(c)

	// We explicitly pull out the map creation in the call to
	// s.assertUnitsCreated() and create the map as a new variable
	// because this /appears/ to tickle a bug on ppc64le using
	// gccgo-4.9; the bug is that the map on the receiving side
	// does not have the same contents as it does here - which is
	// weird because that pattern is used elsewhere in this
	// function. And just pulling the map instantiation out of the
	// call is not enough; we need to do something benign with the
	// variable to keep a reference beyond the call to the
	// s.assertUnitsCreated(). I have to chosen to delete a
	// non-existent key. This problem does not occur on amd64
	// using gc or gccgo-4.9. Nor does it happen using go1.6 on
	// ppc64. Once we switch to go1.6 across the board this change
	// should be reverted. See http://pad.lv/1556116.
	expectedUnits := map[string]string{
		"sql/0": "0/lxd/0",
		"sql/1": "2",
		"wp/0":  "0",
		"wp/1":  "1/lxd/0",
	}
	s.assertUnitsCreated(c, expectedUnits)
	delete(expectedUnits, "non-existent")

	// Redeploy the same bundle again.
	err = s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"sql/0": "0/lxd/0",
		"sql/1": "2",
		"wp/0":  "0",
		"wp/1":  "1/lxd/0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestLXCTreatedAsLXD(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")

	// Note that we use lxc here, to represent a 1.x bundle that specifies lxc.
	content := `
        applications:
            wp:
                charm: cs:xenial/wordpress-0
                num_units: 1
                to:
                    - lxc:0
                options:
                    blog-title: these are the voyages
            wp2:
                charm: cs:xenial/wordpress-0
                num_units: 1
                to:
                    - lxc:0
                options:
                    blog-title: these are the voyages
        machines:
            0:
                series: xenial
    `
	_, output, err := s.DeployBundleYAMLWithOutput(c, content)
	c.Assert(err, jc.ErrorIsNil)
	expectedUnits := map[string]string{
		"wp/0":  "0/lxd/0",
		"wp2/0": "0/lxd/1",
	}
	idx := strings.Index(output, "Bundle has one or more containers specified as lxc. lxc containers are deprecated in Juju 2.0. lxd containers will be deployed instead.")
	lastIdx := strings.LastIndex(output, "Bundle has one or more containers specified as lxc. lxc containers are deprecated in Juju 2.0. lxd containers will be deployed instead.")
	// The message exists.
	c.Assert(idx, jc.GreaterThan, -1)
	// No more than one instance of the message was printed.
	c.Assert(idx, gc.Equals, lastIdx)
	s.assertUnitsCreated(c, expectedUnits)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMachineAttributes(c *gc.C) {
	_, ch := testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:xenial/django-42
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
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"django": {charm: "cs:xenial/django-42", config: ch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c)
	s.assertUnitsCreated(c, map[string]string{
		"django/0": "0",
		"django/1": "1",
	})
	m, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Series(), gc.Equals, "xenial")
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	expectedCons, err := constraints.Parse("cores=4 mem=4G")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, expectedCons)
	ann, err := s.Model.Annotations(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleTwiceScaleUp(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:xenial/django-42
                num_units: 2
    `)
	c.Assert(err, jc.ErrorIsNil)
	err = s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:xenial/django-42
                num_units: 5
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0": "0",
		"django/1": "1",
		"django/2": "2",
		"django/3": "3",
		"django/4": "4",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitPlacedInApplication(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 3
            django:
                charm: cs:xenial/django-42
                num_units: 2
                to: [wordpress]
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0":    "0",
		"django/1":    "1",
		"wordpress/0": "0",
		"wordpress/1": "1",
		"wordpress/2": "2",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundlePeerContainer(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")

	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 2
                to: ["lxd:new"]
            django:
                charm: cs:xenial/django-42
                num_units: 2
                to: ["lxd:wordpress"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:xenial/django-42 for series xenial\n"+
		"- deploy application django on xenial using cs:xenial/django-42\n"+
		"- upload charm cs:xenial/wordpress-0 for series xenial\n"+
		"- deploy application wordpress on xenial using cs:xenial/wordpress-0\n"+
		"- add lxd container 0/lxd/0 on new machine 0\n"+
		"- add lxd container 1/lxd/0 on new machine 1\n"+
		"- add unit wordpress/0 to 0/lxd/0\n"+
		"- add unit wordpress/1 to 1/lxd/0\n"+
		"- add lxd container 0/lxd/1 on new machine 0\n"+
		"- add lxd container 1/lxd/1 on new machine 1\n"+
		"- add unit django/0 to 0/lxd/1 to satisfy [lxd:wordpress]\n"+
		"- add unit django/1 to 1/lxd/1 to satisfy [lxd:wordpress]",
	)

	s.assertUnitsCreated(c, map[string]string{
		"django/0":    "0/lxd/1",
		"django/1":    "1/lxd/1",
		"wordpress/0": "0/lxd/0",
		"wordpress/1": "1/lxd/0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitColocationWithUnit(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/mem-47", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/rails-0", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 3
                to: [1, new]
            django:
                charm: cs:xenial/django-42
                num_units: 5
                to:
                    - memcached/0
                    - lxd:memcached/1
                    - lxd:memcached/2
                    - kvm:ror
            ror:
                charm: rails
                num_units: 2
                to:
                    - new
                    - 1
        machines:
            1:
                series: xenial
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0":    "0",
		"django/1":    "1/lxd/0",
		"django/2":    "2/lxd/0",
		"django/3":    "3/kvm/0",
		"django/4":    "0/kvm/0",
		"memcached/0": "0",
		"memcached/1": "1",
		"memcached/2": "2",
		"ror/0":       "3",
		"ror/1":       "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitPlacedToMachines(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "bionic/django-42", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:django
                num_units: 7
                to:
                    - new
                    - 4
                    - kvm:8
                    - lxd:4
                    - lxd:4
                    - lxd:new
        machines:
            4:
            8:
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0": "2",       // Machine "new" in the bundle.
		"django/1": "0",       // Machine "4" in the bundle.
		"django/2": "1/kvm/0", // The KVM container in bundle machine "8".
		"django/3": "0/lxd/0", // First lxd container in bundle machine "4".
		"django/4": "0/lxd/1", // Second lxd container in bundle machine "4".
		"django/5": "3/lxd/0", // First lxd in new machine.
		"django/6": "4/lxd/0", // Second lxd in new machine.
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMassiveUnitColocation(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "bionic/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "bionic/mem-47", "dummy")
	testcharms.UploadCharm(c, s.client, "bionic/rails-0", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            memcached:
                charm: cs:bionic/mem-47
                num_units: 3
                to: [1, 2, 3]
            django:
                charm: cs:bionic/django-42
                num_units: 4
                to:
                    - 1
                    - lxd:memcached
            ror:
                charm: rails
                num_units: 3
                to:
                    - 1
                    - kvm:3
        machines:
            1:
            2:
            3:
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0":    "0",
		"django/1":    "0/lxd/0",
		"django/2":    "1/lxd/0",
		"django/3":    "2/lxd/0",
		"memcached/0": "0",
		"memcached/1": "1",
		"memcached/2": "2",
		"ror/0":       "0",
		"ror/1":       "2/kvm/0",
		"ror/2":       "3",
	})

	// Redeploy a very similar bundle with another application unit. The new unit
	// is placed on the first unit of memcached. Due to ordering of the applications
	// there is no deterministic way to determine "least crowded" in a meaningful way.
	content := `
        applications:
            memcached:
                charm: cs:bionic/mem-47
                num_units: 3
                to: [1, 2, 3]
            django:
                charm: cs:bionic/django-42
                num_units: 4
                to:
                    - 1
                    - lxd:memcached
            node:
                charm: cs:bionic/django-42
                num_units: 1
                to:
                    - lxd:memcached
        machines:
            1:
            2:
            3:
    `
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, content)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- deploy application node on bionic using cs:bionic/django-42\n"+
		"- add unit node/0 to 0/lxd/0 to satisfy [lxd:memcached]",
	)

	// Redeploy the same bundle again and check that nothing happens.
	stdOut, _, err = s.DeployBundleYAMLWithOutput(c, content)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdOut, gc.Equals, "")
	s.assertUnitsCreated(c, map[string]string{
		"django/0":    "0",
		"django/1":    "0/lxd/0",
		"django/2":    "1/lxd/0",
		"django/3":    "2/lxd/0",
		"memcached/0": "0",
		"memcached/1": "1",
		"memcached/2": "2",
		"node/0":      "0/lxd/1",
		"ror/0":       "0",
		"ror/1":       "2/kvm/0",
		"ror/2":       "3",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithAnnotations_OutputIsCorrect(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "bionic/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "bionic/mem-47", "dummy")
	stdOut, stdErr, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            django:
                charm: cs:django
                num_units: 1
                annotations:
                    key1: value1
                    key2: value2
                to: [1]
            memcached:
                charm: bionic/mem-47
                num_units: 1
        machines:
            1:
                annotations: {foo: bar}
    `)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:bionic/django-42 for series bionic\n"+
		"- deploy application django on bionic using cs:bionic/django-42\n"+
		"- set annotations for django\n"+
		"- upload charm cs:bionic/mem-47 for series bionic\n"+
		"- deploy application memcached on bionic using cs:bionic/mem-47\n"+
		"- add new machine 0 (bundle machine 1)\n"+
		"- set annotations for new machine 0\n"+
		"- add unit django/0 to new machine 0\n"+
		"- add unit memcached/0 to new machine 1",
	)
	c.Check(stdErr, gc.Equals, ""+
		"Resolving charm: cs:django\n"+
		"Resolving charm: bionic/mem-47\n"+
		"Deploy of bundle completed.",
	)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleAnnotations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "bionic/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "bionic/mem-47", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:django
                num_units: 1
                annotations:
                    key1: value1
                    key2: value2
                to: [1]
            memcached:
                charm: bionic/mem-47
                num_units: 1
        machines:
            1:
                annotations: {foo: bar}
    `)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := s.State.Application("django")
	c.Assert(err, jc.ErrorIsNil)
	ann, err := s.Model.Annotations(svc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{
		"key1": "value1",
		"key2": "value2",
	})
	m, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	ann, err = s.Model.Annotations(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{"foo": "bar"})

	// Update the annotations and deploy the bundle again.
	err = s.DeployBundleYAML(c, `
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
    `)
	c.Assert(err, jc.ErrorIsNil)
	ann, err = s.Model.Annotations(svc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{
		"key1": "new value!",
		"key2": "value2",
	})
	ann, err = s.Model.Annotations(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{
		"foo":    "bar",
		"answer": "42",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleExistingMachines(c *gc.C) {
	xenialMachine := &factory.MachineParams{Series: "xenial"}
	s.Factory.MakeMachine(c, xenialMachine) // machine-0
	s.Factory.MakeMachine(c, xenialMachine) // machine-1
	s.Factory.MakeMachine(c, xenialMachine) // machine-2
	s.Factory.MakeMachine(c, xenialMachine) // machine-3
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:django
                num_units: 3
                to: [0,1,2]
        machines:
            0:
            1:
            2:
    `, "--map-machines", "existing,2=3")
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0": "0",
		"django/1": "1",
		"django/2": "3",
	})
}

type mockAllWatcher struct {
	next func() []multiwatcher.Delta
}

func (w mockAllWatcher) Next() ([]multiwatcher.Delta, error) {
	return w.next(), nil
}

func (mockAllWatcher) Stop() error {
	return nil
}

type ProcessIncludesSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ProcessIncludesSuite{})

func (*ProcessIncludesSuite) TestNonString(c *gc.C) {
	value := 1234
	result, changed, err := processValue("", value)

	c.Check(err, jc.ErrorIsNil)
	c.Check(changed, jc.IsFalse)
	c.Check(result, gc.Equals, value)
}

func (*ProcessIncludesSuite) TestSimpleString(c *gc.C) {
	value := "simple"
	result, changed, err := processValue("", value)

	c.Check(err, jc.ErrorIsNil)
	c.Check(changed, jc.IsFalse)
	c.Check(result, gc.Equals, value)
}

func (*ProcessIncludesSuite) TestMissingFile(c *gc.C) {
	value := "include-file://simple"
	result, changed, err := processValue("", value)

	c.Check(err, gc.ErrorMatches, "unable to read file: "+missingFileRegex("simple"))
	c.Check(changed, jc.IsFalse)
	c.Check(result, gc.IsNil)
}

func (*ProcessIncludesSuite) TestFileNameIsInDir(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "content")
	err := ioutil.WriteFile(filename, []byte("testing"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	value := "include-file://content"
	result, changed, err := processValue(dir, value)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(changed, jc.IsTrue)
	c.Check(result, gc.Equals, "testing")
}

func (*ProcessIncludesSuite) TestRelativePath(c *gc.C) {
	dir := c.MkDir()
	c.Assert(os.Mkdir(filepath.Join(dir, "nested"), 0755), jc.ErrorIsNil)

	filename := filepath.Join(dir, "nested", "content")
	err := ioutil.WriteFile(filename, []byte("testing"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	value := "include-file://./nested/content"
	result, changed, err := processValue(dir, value)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(changed, jc.IsTrue)
	c.Check(result, gc.Equals, "testing")
}

func (*ProcessIncludesSuite) TestAbsolutePath(c *gc.C) {
	dir := c.MkDir()
	c.Assert(os.Mkdir(filepath.Join(dir, "nested"), 0755), jc.ErrorIsNil)

	filename := filepath.Join(dir, "nested", "content")
	c.Check(filepath.IsAbs(filename), jc.IsTrue)
	err := ioutil.WriteFile(filename, []byte("testing"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	value := "include-file://" + filename
	result, changed, err := processValue(dir, value)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(changed, jc.IsTrue)
	c.Check(result, gc.Equals, "testing")
}

func (*ProcessIncludesSuite) TestBase64Encode(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "content")
	err := ioutil.WriteFile(filename, []byte("testing"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	value := "include-base64://content"
	result, changed, err := processValue(dir, value)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(changed, jc.IsTrue)
	encoded := base64.StdEncoding.EncodeToString([]byte("testing"))
	c.Check(result, gc.Equals, encoded)
}

func (*ProcessIncludesSuite) TestBundleReplacements(c *gc.C) {
	bundleYAML := `
        applications:
            django:
                charm: cs:django
                num_units: 1
                options:
                    private: include-base64://sekrit.binary
                annotations:
                    key1: value1
                    key2: value2
                    key3: include-file://annotation
                to: [1]
            memcached:
                charm: xenial/mem-47
                num_units: 1
        machines:
            1:
                annotations: {foo: bar, baz: "include-file://machine" }
            2:
    `

	baseDir := c.MkDir()
	bundleFile := filepath.Join(baseDir, "bundle.yaml")
	err := ioutil.WriteFile(bundleFile, []byte(bundleYAML), 0644)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(
		ioutil.WriteFile(
			filepath.Join(baseDir, "sekrit.binary"),
			[]byte{42, 12, 0, 23, 8}, 0644),
		jc.ErrorIsNil)
	c.Assert(
		ioutil.WriteFile(
			filepath.Join(baseDir, "annotation"),
			[]byte("value3"), 0644),
		jc.ErrorIsNil)
	c.Assert(
		ioutil.WriteFile(
			filepath.Join(baseDir, "machine"),
			[]byte("wibble"), 0644),
		jc.ErrorIsNil)

	bundleData, err := charmrepo.ReadBundleFile(bundleFile)
	c.Assert(err, jc.ErrorIsNil)

	err = processBundleIncludes(baseDir, bundleData)
	c.Assert(err, jc.ErrorIsNil)

	django := bundleData.Applications["django"]
	c.Check(django.Annotations["key1"], gc.Equals, "value1")
	c.Check(django.Annotations["key2"], gc.Equals, "value2")
	c.Check(django.Annotations["key3"], gc.Equals, "value3")
	c.Check(django.Options["private"], gc.Equals, "KgwAFwg=")
	annotations := bundleData.Machines["1"].Annotations
	c.Check(annotations["foo"], gc.Equals, "bar")
	c.Check(annotations["baz"], gc.Equals, "wibble")
}

type ProcessBundleOverlaySuite struct {
	coretesting.BaseSuite

	bundleData *charm.BundleData
}

var _ = gc.Suite(&ProcessBundleOverlaySuite{})

func (s *ProcessBundleOverlaySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	baseBundle := `
        applications:
            django:
                expose: true
                charm: cs:django
                num_units: 1
                options:
                    general: good
                annotations:
                    key1: value1
                    key2: value2
                to: [1]
            memcached:
                charm: xenial/mem-47
                num_units: 1
                options:
                    key: value
        relations:
            - - "django"
              - "memcached"
        machines:
            1:
                annotations: {foo: bar}`

	baseDir := c.MkDir()
	bundleFile := filepath.Join(baseDir, "bundle.yaml")
	err := ioutil.WriteFile(bundleFile, []byte(baseBundle), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.bundleData, err = charmrepo.ReadBundleFile(bundleFile)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ProcessBundleOverlaySuite) writeFile(c *gc.C, content string) string {
	// Write the content to a file in a new directoryt and return the file.
	baseDir := c.MkDir()
	filename := filepath.Join(baseDir, "config.yaml")
	err := ioutil.WriteFile(filename, []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return filename
}

func (s *ProcessBundleOverlaySuite) TestNoFile(c *gc.C) {
	err := processBundleOverlay(s.bundleData)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ProcessBundleOverlaySuite) TestBadFile(c *gc.C) {
	err := processBundleOverlay(s.bundleData, "bad")
	c.Assert(err, gc.ErrorMatches, `unable to read bundle overlay file ".*": bundle not found: .*bad`)
}

func (s *ProcessBundleOverlaySuite) TestGoodYAML(c *gc.C) {
	filename := s.writeFile(c, "bad:\n\tindent")
	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, gc.ErrorMatches, `unable to read bundle overlay file ".*": cannot unmarshal bundle data: yaml: line 2: found character that cannot start any token`)
}

func (s *ProcessBundleOverlaySuite) TestReplaceZeroValues(c *gc.C) {
	config := `
        applications:
            django:
                expose: false
                num_units: 0
    `
	filename := s.writeFile(c, config)
	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, jc.ErrorIsNil)
	django := s.bundleData.Applications["django"]

	c.Check(django.Expose, jc.IsFalse)
	c.Check(django.NumUnits, gc.Equals, 0)
}

func (s *ProcessBundleOverlaySuite) TestMachineReplacement(c *gc.C) {
	config := `
        machines:
            1:
            2:
    `
	filename := s.writeFile(c, config)
	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, jc.ErrorIsNil)

	var machines []string
	for key := range s.bundleData.Machines {
		machines = append(machines, key)
	}
	sort.Strings(machines)
	c.Assert(machines, jc.DeepEquals, []string{"1", "2"})
}

func (s *ProcessBundleOverlaySuite) assertApplications(c *gc.C, appNames ...string) {
	var applications []string
	for key := range s.bundleData.Applications {
		applications = append(applications, key)
	}
	sort.Strings(applications)
	sort.Strings(appNames)
	c.Assert(applications, jc.DeepEquals, appNames)
}

func (s *ProcessBundleOverlaySuite) TestNewApplication(c *gc.C) {
	config := `
        applications:
            postgresql:
                charm: cs:postgresql
                num_units: 1
        relations:
            - - "postgresql:pgsql"
              - "django:pgsql"
    `
	filename := s.writeFile(c, config)
	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplications(c, "django", "memcached", "postgresql")
	c.Assert(s.bundleData.Relations, jc.DeepEquals, [][]string{
		{"django", "memcached"},
		{"postgresql:pgsql", "django:pgsql"},
	})
}

func (s *ProcessBundleOverlaySuite) TestRemoveApplication(c *gc.C) {
	config := `
        applications:
            memcached:
    `
	filename := s.writeFile(c, config)
	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplications(c, "django")
	c.Assert(s.bundleData.Relations, gc.HasLen, 0)
}

func (s *ProcessBundleOverlaySuite) TestRemoveUnknownApplication(c *gc.C) {
	config := `
        applications:
            unknown:
    `
	filename := s.writeFile(c, config)
	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplications(c, "django", "memcached")
	c.Assert(s.bundleData.Relations, jc.DeepEquals, [][]string{
		{"django", "memcached"},
	})
}

func (s *ProcessBundleOverlaySuite) TestIncludes(c *gc.C) {
	config := `
        applications:
            django:
                options:
                    private: include-base64://sekrit.binary
                annotations:
                    key3: include-file://annotation
    `
	filename := s.writeFile(c, config)
	baseDir := filepath.Dir(filename)

	c.Assert(
		ioutil.WriteFile(
			filepath.Join(baseDir, "sekrit.binary"),
			[]byte{42, 12, 0, 23, 8}, 0644),
		jc.ErrorIsNil)
	c.Assert(
		ioutil.WriteFile(
			filepath.Join(baseDir, "annotation"),
			[]byte("value3"), 0644),
		jc.ErrorIsNil)

	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, jc.ErrorIsNil)
	django := s.bundleData.Applications["django"]
	c.Check(django.Annotations, jc.DeepEquals, map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3"})
	c.Check(django.Options, jc.DeepEquals, map[string]interface{}{
		"general": "good",
		"private": "KgwAFwg="})
}

func (s *ProcessBundleOverlaySuite) TestRemainingFields(c *gc.C) {
	// Note that we don't care about the actual values here
	// as bundle validation is done after replacement.
	config := `
        applications:
            django:
                charm: cs:django-23
                series: wisty
                resources:
                    something: or other
                to:
                - 3
                constraints: big machine
                storage:
                    disk: big
                devices:
                    gpu: 1,nvidia.com/gpu
                bindings:
                    where: dmz
    `
	filename := s.writeFile(c, config)
	err := processBundleOverlay(s.bundleData, filename)
	c.Assert(err, jc.ErrorIsNil)
	django := s.bundleData.Applications["django"]

	c.Check(django.Charm, gc.Equals, "cs:django-23")
	c.Check(django.Series, gc.Equals, "wisty")
	c.Check(django.Resources, jc.DeepEquals, map[string]interface{}{
		"something": "or other"})
	c.Check(django.To, jc.DeepEquals, []string{"3"})
	c.Check(django.Constraints, gc.Equals, "big machine")
	c.Check(django.Storage, jc.DeepEquals, map[string]string{
		"disk": "big"})
	c.Check(django.Devices, jc.DeepEquals, map[string]string{
		"gpu": "1,nvidia.com/gpu"})
	c.Check(django.EndpointBindings, jc.DeepEquals, map[string]string{
		"where": "dmz"})
}

func (s *ProcessBundleOverlaySuite) TestYAMLInterpolation(c *gc.C) {

	removeDjango := s.writeFile(c, `
applications:
    django:
    `)

	addWiki := s.writeFile(c, `
defaultwiki: &DEFAULTWIKI
    charm: "cs:trusty/mediawiki-5"
    num_units: 1
    options: &WIKIOPTS
        debug: false
        name: Please set name of wiki
        skin: vector

applications:
    wiki:
        <<: *DEFAULTWIKI
        options:
            <<: *WIKIOPTS
            name: The name override
relations:
    - - "wiki"
      - "memcached"
`)

	err := processBundleOverlay(s.bundleData, removeDjango, addWiki)
	c.Assert(err, jc.ErrorIsNil)

	s.assertApplications(c, "memcached", "wiki")
	c.Assert(s.bundleData.Relations, jc.DeepEquals, [][]string{
		{"wiki", "memcached"},
	})
	wiki := s.bundleData.Applications["wiki"]
	c.Assert(wiki.Charm, gc.Equals, "cs:trusty/mediawiki-5")
	c.Assert(wiki.Options, gc.DeepEquals,
		map[string]interface{}{
			"debug": false,
			"name":  "The name override",
			"skin":  "vector",
		})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundlePassesSequences(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")

	// Deploy another django app with two units, this will bump the sequences
	// for machines and the django application. Then remove them both.
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "django"})
	u1 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	u2 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	var machines []*state.Machine
	var ids []string
	destroyUnitsMachine := func(u *state.Unit) {
		id, err := u.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		ids = append(ids, id)
		m, err := s.State.Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		machines = append(machines, m)
		c.Assert(m.ForceDestroy(), jc.ErrorIsNil)
	}
	// Tear them down. This is somewhat convoluted, but it is what we need
	// to do to properly cleanly tear down machines.
	c.Assert(app.Destroy(), jc.ErrorIsNil)
	destroyUnitsMachine(u1)
	destroyUnitsMachine(u2)
	c.Assert(s.State.Cleanup(), jc.ErrorIsNil)
	for _, m := range machines {
		c.Assert(m.EnsureDead(), jc.ErrorIsNil)
		c.Assert(m.MarkForRemoval(), jc.ErrorIsNil)
	}
	c.Assert(s.State.CompleteMachineRemovals(ids...), jc.ErrorIsNil)

	apps, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apps, gc.HasLen, 0)
	machines, err = s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 0)

	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            django:
                charm: cs:xenial/django-42
                num_units: 2
    `)
	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:xenial/django-42 for series xenial\n"+
		"- deploy application django on xenial using cs:xenial/django-42\n"+
		"- add unit django/2 to new machine 2\n"+
		"- add unit django/3 to new machine 3",
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/2": "2",
		"django/3": "3",
	})
}

type removeRelationsSuite struct{}

var (
	_ = gc.Suite(&removeRelationsSuite{})

	sampleRelations = [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-master:etcd", "etcd:db"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
		{"flannel", "etcd"}, // removed :endpoint
		{"flannel:cni", "kubernetes-master:cni"},
		{"flannel:cni", "kubernetes-worker:cni"},
	}
)

func (*removeRelationsSuite) TestNil(c *gc.C) {
	result := removeRelations(nil, "foo")
	c.Assert(result, gc.HasLen, 0)
}

func (*removeRelationsSuite) TestEmpty(c *gc.C) {
	result := removeRelations([][]string{}, "foo")
	c.Assert(result, gc.HasLen, 0)
}

func (*removeRelationsSuite) TestAppNotThere(c *gc.C) {
	result := removeRelations(sampleRelations, "foo")
	c.Assert(result, jc.DeepEquals, sampleRelations)
}

func (*removeRelationsSuite) TestAppBadRelationsKept(c *gc.C) {
	badRelations := [][]string{{"single value"}, {"three", "string", "values"}}
	result := removeRelations(badRelations, "foo")
	c.Assert(result, jc.DeepEquals, badRelations)
}

func (*removeRelationsSuite) TestRemoveFromRight(c *gc.C) {
	result := removeRelations(sampleRelations, "etcd")
	c.Assert(result, jc.DeepEquals, [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
		{"flannel:cni", "kubernetes-master:cni"},
		{"flannel:cni", "kubernetes-worker:cni"},
	})
}

func (*removeRelationsSuite) TestRemoveFromLeft(c *gc.C) {
	result := removeRelations(sampleRelations, "flannel")
	c.Assert(result, jc.DeepEquals, [][]string{
		{"kubernetes-master:kube-control", "kubernetes-worker:kube-control"},
		{"kubernetes-master:etcd", "etcd:db"},
		{"kubernetes-worker:kube-api-endpoint", "kubeapi-load-balancer:website"},
	})
}

func missingFileRegex(filename string) string {
	text := "no such file or directory"
	if runtime.GOOS == "windows" {
		text = "The system cannot find the file specified."
	}
	return fmt.Sprintf("open .*%s: %s", filename, text)
}
