// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
)

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNotFoundCharmStore(c *gc.C) {
	err := s.runDeploy(c, "bundle/no-such")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:bundle/no-such": bundle not found`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidFlags(c *gc.C) {
	s.setupCharm(c, "xenial/mysql-42", "mysql", "bionic")
	s.setupCharm(c, "xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	err := s.runDeploy(c, "bundle/wordpress-simple", "--config", "config.yaml")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --config")
	err = s.runDeploy(c, "bundle/wordpress-simple", "-n", "2")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: -n")
	err = s.runDeploy(c, "bundle/wordpress-simple", "--series", "xenial")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --series")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleSuccess(c *gc.C) {
	mysqlch := s.setupCharm(c, "xenial/mysql-42", "mysql", "bionic")
	wpch := s.setupCharm(c, "xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	err := s.runDeploy(c, "bundle/wordpress-simple")
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithInvalidSeries(c *gc.C) {
	s.setupCharm(c, "precise/mysql-42", "mysql", "bionic")
	s.setupCharm(c, "xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	err := s.runDeploy(c, "bundle/wordpress-simple")
	c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: mysql is not available on the following series: precise not supported")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithInvalidSeriesWithForce(c *gc.C) {
	mysqlch := s.setupCharmMaybeAddForce(c, "precise/mysql-42", "mysql", "bionic", true, true)
	wpch := s.setupCharmMaybeAddForce(c, "xenial/wordpress-47", "wordpress", "bionic", true, true)
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	err := s.runDeploy(c, "bundle/wordpress-simple", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:precise/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":     {charm: "cs:precise/mysql-42", config: mysqlch.Config().DefaultSettings()},
		"wordpress": {charm: "cs:xenial/wordpress-47", config: wpch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployKubernetesBundleSuccess(c *gc.C) {
	unregister := caas.RegisterContainerProvider("kubernetes-test", &fakeProvider{})
	s.AddCleanup(func(_ *gc.C) { unregister() })

	// Set up a CAAS model to replace the IAAS one.
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "caascloud",
		Type:      "kubernetes-test",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}, s.Model.Owner().Id())
	c.Assert(err, jc.ErrorIsNil)

	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		Name:      "test",
		CloudName: "caascloud",
	})
	s.CleanupSuite.AddCleanup(func(*gc.C) { _ = st.Close() })

	s.State = st
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = m.UpdateModelConfig(map[string]interface{}{
		"operator-storage": "k8s-storage",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	mysqlch := s.setupCharm(c, "kubernetes/mariadb-42", "mariadb", "kubernetes")
	wpch := s.setupCharm(c, "kubernetes/gitlab-47", "gitlab", "kubernetes")
	s.setupBundle(c, "bundle/kubernetes-simple-1", "kubernetes-simple", "kubernetes")

	settings := state.NewStateSettings(s.State)
	registry := storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"kubernetes": &dummystorage.StorageProvider{},
		},
	}
	pm := poolmanager.New(settings, registry)
	_, err = pm.Create("operator-storage", provider.K8s_ProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.runDeploy(c, "-m", "admin/test", "bundle/kubernetes-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:kubernetes/gitlab-47", "cs:kubernetes/mariadb-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mariadb": {charm: "cs:kubernetes/mariadb-42", config: mysqlch.Config().DefaultSettings()},
		"gitlab":  {charm: "cs:kubernetes/gitlab-47", config: wpch.Config().DefaultSettings(), scale: 1},
	})
	s.assertRelationsEstablished(c, "gitlab:ring", "gitlab:db mariadb:server")
}

func (s *BundleDeployCharmStoreSuite) TestAddMetricCredentials(c *gc.C) {
	s.fakeAPI.planURL = s.server.URL
	s.setupCharm(c, "xenial/wordpress", "wordpress", "bionic")
	s.setupCharm(c, "xenial/mysql", "mysql", "bionic")
	s.setupBundle(c, "bundle/wordpress-with-plans-1", "wordpress-with-plans", "xenial")

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	s.fakeAPI.Call("SetMetricCredentials", "wordpress", creds).Returns(error(nil))

	deploy := s.deployCommandForState()
	deploy.Steps = []DeployStep{&RegisterMeteredCharm{PlanURL: s.server.URL, RegisterPath: "", QueryPath: ""}}
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "bundle/wordpress-with-plans")
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleStorage(c *gc.C) {
	mysqlch := s.setupCharm(c, "xenial/mysql-42", "mysql-storage", "bionic")
	wpch := s.setupCharm(c, "xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "bundle/wordpress-with-mysql-storage-1", "wordpress-with-mysql-storage", "bionic")

	err := s.runDeploy(
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleDevices(c *gc.C) {
	c.Skip("Test disabled until flakiness is fixed - see bug lp:1781250")

	minerCharm := s.setupCharm(c, "kubernetes/bitcoin-miner-1", "bitcoin-miner", "kubernetes")
	dashboardCharm := s.setupCharm(c, "kubernetes/dashboard4miner-3", "dashboard4miner", "kubernetes")
	s.setupBundle(c, "bundle/bitcoinminer-with-dashboard-1", "bitcoinminer-with-dashboard", "kubernetes")
	err := s.runDeploy(c, "bundle/bitcoinminer-with-dashboard",
		"-m", "foo",
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
	s.setupCharm(c, "xenial/mysql-42", "mysql", "bionic")
	s.setupCharmMaybeAdd(c, "xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings", "bionic", false)
	s.setupBundle(c, "bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings", "bionic")

	stdOut, stdErr, err := s.runDeployWithOutput(c, "bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, gc.ErrorMatches, ""+
		"cannot deploy bundle: cannot deploy application \"mysql\": "+
		"space not found")
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
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	publicSpace, err := s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	mysqlch := s.setupCharm(c, "xenial/mysql-42", "mysql", "bionic")
	wpch := s.setupCharm(c, "xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings", "bionic")
	s.setupBundle(c, "bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings", "bionic")

	err = s.runDeploy(c, "bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-extra-bindings-47")

	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":                    {charm: "cs:xenial/mysql-42", config: mysqlch.Config().DefaultSettings()},
		"wordpress-extra-bindings": {charm: "cs:xenial/wordpress-extra-bindings-47", config: wpch.Config().DefaultSettings()},
	})
	s.assertDeployedApplicationBindings(c, map[string]applicationInfo{
		"mysql": {
			endpointBindings: map[string]string{
				"":               network.AlphaSpaceId,
				"server":         dbSpace.Id(),
				"server-admin":   network.AlphaSpaceId,
				"metrics-client": network.AlphaSpaceId},
		},
		"wordpress-extra-bindings": {
			endpointBindings: map[string]string{
				"":                network.AlphaSpaceId,
				"cache":           network.AlphaSpaceId,
				"url":             publicSpace.Id(),
				"logging-dir":     network.AlphaSpaceId,
				"monitoring-port": network.AlphaSpaceId,
				"db":              dbSpace.Id(),
				"cluster":         network.AlphaSpaceId,
				"db-client":       dbSpace.Id(),
				"admin-api":       publicSpace.Id(),
				"foo-bar":         network.AlphaSpaceId,
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
	mysqlch := s.setupCharm(c, "xenial/mysql-42", "mysql", "bionic")
	wpch := s.setupCharm(c, "xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	stdOut, stdErr, err := s.runDeployWithOutput(c, "bundle/wordpress-simple")
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
	c.Check(stdErr, gc.Equals, ""+
		"Located bundle \"cs:bundle/wordpress-simple-1\"\n"+
		"Resolving charm: mysql\n"+
		"Resolving charm: wordpress\n"+
		"Deploy of bundle completed.",
	)
	stdOut, stdErr, err = s.runDeployWithOutput(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	// Nothing to do...
	c.Check(stdOut, gc.Equals, "")
	c.Check(stdErr, gc.Equals, ""+
		"Located bundle \"cs:bundle/wordpress-simple-1\"\n"+
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
	s.setupCharmMaybeAdd(c, "xenial/mysql-42", "mysql", "bionic", false)
	s.setupCharmMaybeAdd(c, "xenial/wordpress-47", "wordpress", "bionic", false)
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	stdOut, _, err := s.runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
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
	stdOut, _, err = s.runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, expected)

	s.assertCharmsUploaded(c /* none */)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{})
	s.assertRelationsEstablished(c /* none */)
	s.assertUnitsCreated(c, map[string]string{})
}

func (s *BundleDeployCharmStoreSuite) TestDryRunExistingModel(c *gc.C) {
	s.setupCharmMaybeAdd(c, "xenial/mysql-42", "mysql", "bionic", false)
	s.setupCharmMaybeAdd(c, "xenial/wordpress-47", "wordpress", "bionic", false)
	s.setupCharmMaybeAdd(c, "trusty/multi-series-subordinate-13", "multi-series-subordinate", "bionic", false)
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

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

	stdOut, _, err := s.runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
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
	stdOut, _, err = s.runDeployWithOutput(c, "bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, expected)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalPath(c *gc.C) {
	dir := c.MkDir()
	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")
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
	err = s.runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:xenial/dummy-1")
	ch, err := s.State.Charm(charm.MustParseURL("local:xenial/dummy-1"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {charm: "local:xenial/dummy-1", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C) {
	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, true)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalPathInvalidSeriesWithoutForce(c *gc.C) {
	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, false)
}

func (s *BundleDeployCharmStoreSuite) assertDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C, force bool) {
	dir := c.MkDir()
	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")
	path := filepath.Join(dir, "mybundle")
	data := `
        series: focal
        applications:
            dummy:
                charm: ./dummy
                num_units: 1
    `
	err := ioutil.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	args := []string{path}
	if force {
		args = append(args, "--force")
	}
	err = s.runDeploy(c, args...)
	if !force {
		c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: dummy is not available on the following series: focal not supported")
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:focal/dummy-1")
	ch, err := s.State.Charm(charm.MustParseURL("local:focal/dummy-1"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {charm: "local:focal/dummy-1", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalResources(c *gc.C) {
	data := `
        series: bionic
        applications:
            "dummy-resource":
                charm: ./dummy-resource
                series: bionic
                num_units: 1
                resources:
                  dummy: ./dummy-resource.zip
    `
	dir := s.makeBundleDir(c, data)
	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy-resource")
	c.Assert(
		ioutil.WriteFile(filepath.Join(dir, "dummy-resource.zip"), []byte("zip file"), 0644),
		jc.ErrorIsNil)
	err := s.runDeploy(c, dir)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:bionic/dummy-resource-0")
	ch, err := s.State.Charm(charm.MustParseURL("local:bionic/dummy-resource-0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy-resource": {charm: "local:bionic/dummy-resource-0", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNoSeriesInCharmURL(c *gc.C) {
	s.setupCharm(c, "~who/multi-series-0", "multi-series", "bionic")
	dir := c.MkDir()
	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")
	path := filepath.Join(dir, "mybundle")
	data := `
        series: trusty
        applications:
            dummy:
                charm: cs:~who/multi-series
    `
	err := ioutil.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:~who/multi-series-0")
	ch, err := s.State.Charm(charm.MustParseURL("~who/multi-series-0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {charm: "cs:~who/multi-series-0", config: ch.Config().DefaultSettings()},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleResources(c *gc.C) {
	s.setupCharm(c, "trusty/starsay-42", "starsay", "bionic")
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
			Origin:      charmresource.OriginUpload,
			Fingerprint: resourceHash("some-data"),
			Size:        int64(len("some-data")),
			Revision:    0,
		},
		ID:            "starsay/upload-resource",
		ApplicationID: "starsay",
		Username:      "admin",
	}})
}

func (s *BundleDeployCharmStoreSuite) checkResources(c *gc.C, app string, expected []resource.Resource) {
	_, err := s.State.Application(app)
	c.Check(err, jc.ErrorIsNil)
	st, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	svcResources, err := st.ListResources(app)
	c.Assert(err, jc.ErrorIsNil)
	resources := svcResources.Resources
	resource.Sort(resources)
	c.Assert(resources, gc.HasLen, 3)
	c.Check(resources[2].Timestamp, gc.Not(gc.Equals), time.Time{})
	resources[2].Timestamp = time.Time{}
	c.Assert(resources, jc.DeepEquals, expected)
}

type BundleDeployCharmStoreSuite struct {
	FakeStoreStateSuite

	stub   *testing.Stub
	server *httptest.Server
}

var _ = gc.Suite(&BundleDeployCharmStoreSuite{})

func (s *BundleDeployCharmStoreSuite) SetUpSuite(c *gc.C) {
	s.DeploySuiteBase.SetUpSuite(c)
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

	s.FakeStoreStateSuite.SetUpTest(c)
	logger.SetLogLevel(loggo.TRACE)
}

func (s *BundleDeployCharmStoreSuite) TearDownTest(c *gc.C) {
	if s.server != nil {
		s.server.Close()
	}
	s.FakeStoreStateSuite.TearDownTest(c)
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
	return s.runDeployWithOutput(c, args...)
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
	err: `cannot resolve URL "xenial/rails-42": .* charm or bundle not found`,
}, {
	about:   "invalid bundle content",
	content: "!",
	err:     `(?s)cannot unmarshal bundle contents:.* yaml: unmarshal errors:.*`,
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
	s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
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
	s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
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
	s.setupCharm(c, "trusty/django-0", "django", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: trusty/django
                num_units: 1
                to:
                    - 1
        machines:
            1:
                series: xenial
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot add unit for application "django": acquiring machine to host unit "django/0": cannot assign unit "django/0" to machine 0: series does not match`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidBinding(c *gc.C) {
	_, err := s.State.AddSpace("public", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
	err = s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: xenial/wordpress-42
                num_units: 1
                bindings:
                  noturl: public
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": cannot add application "wp": unknown endpoint "noturl" not valid`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidSpace(c *gc.C) {
	s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
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
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": space not found`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWatcherTimeout(c *gc.C) {
	// Inject an "AllWatcher" that never delivers a result.
	ch := make(chan struct{})
	defer close(ch)
	watcher := mockAllWatcher{
		next: func() []params.Delta {
			<-ch
			return nil
		},
	}
	s.PatchValue(&watchAll, func(*api.Client) (allWatcher, error) {
		return watcher, nil
	})

	s.setupCharm(c, "xenial/django-0", "django", "bionic")
	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
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
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
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
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
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
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: unable to process overlays: "missing-file" not found`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentLXDProfile(c *gc.C) {
	charmsPath := c.MkDir()
	lxdProfilePath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "lxd-profile")
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
	lxdProfilePath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "lxd-profile-fail")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
        series: bionic
        services:
            lxd-profile-fail:
                charm: %s
                num_units: 1
    `, lxdProfilePath))
	c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: cannot deploy local charm at .*: invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentBadLXDProfileWithForce(c *gc.C) {
	charmsPath := c.MkDir()
	lxdProfilePath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "lxd-profile-fail")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
        series: bionic
        services:
            lxd-profile-fail:
                charm: %s
                num_units: 1
    `, lxdProfilePath), "--force")
	c.Assert(err, jc.ErrorIsNil)
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
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
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
	settings, err := wordpress.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings["blog-title"], gc.Equals, "magic bundle config")
}

func (s *BundleDeployCharmStoreSuite) TestDeployLocalBundleWithRelativeCharmPaths(c *gc.C) {
	bundleDir := c.MkDir()
	_ = testcharms.RepoWithSeries("bionic").ClonedDirPath(bundleDir, "dummy")

	bundleFile := filepath.Join(bundleDir, "bundle.yaml")
	bundleContent := `
series: bionic
applications:
  dummy:
    charm: ./dummy
`
	c.Assert(
		ioutil.WriteFile(bundleFile, []byte(bundleContent), 0644),
		jc.ErrorIsNil)

	err := s.runDeploy(c, bundleFile)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.Application("dummy")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalAndCharmStoreCharms(c *gc.C) {
	charmsPath := c.MkDir()
	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
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
	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
	dch := s.setupCharm(c, "bionic/dummy-0", "dummy", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                options:
                    blog-title: these are the voyages
            customized:
                charm: bionic/dummy-0
                num_units: 1
                options:
                    username: who
                    skill-level: 47
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:bionic/dummy-0", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"customized": {
			charm:  "cs:bionic/dummy-0",
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
	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
	dch := s.setupCharm(c, "bionic/dummy-0", "dummy", "bionic")

	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: wordpress
                constraints: mem=4G cores=2
            customized:
                charm: bionic/dummy-0
                num_units: 1
                constraints: arch=i386
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:bionic/dummy-0", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"customized": {
			charm:       "cs:bionic/dummy-0",
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
	s.setupCharm(c, "xenial/wordpress", "wordpress", "bionic")
	s.setupCharm(c, "xenial/mysql", "mysql", "bionic")
	s.setupBundle(c, "bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	deploy := s.deployCommandForState()
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "bundle/wordpress-simple")
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
	wpch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
	s.setupCharm(c, "trusty/upgrade-1", "upgrade1", "bionic")

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
                charm: trusty/upgrade-1
                num_units: 1
                constraints: mem=8G
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:trusty/upgrade-1", "cs:xenial/wordpress-42")

	ch := s.setupCharm(c, "trusty/upgrade-2", "upgrade2", "bionic")
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
                charm: trusty/upgrade-2
                num_units: 1
                constraints: mem=8G
    `)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm cs:trusty/upgrade-2 for series trusty\n"+
		"- upgrade up to use charm cs:trusty/upgrade-2 for series trusty\n"+
		"- set application options for wordpress\n"+
		`- set constraints for wordpress to "spaces=new cores=8"`,
	)

	s.assertCharmsUploaded(c, "cs:trusty/upgrade-1", "cs:trusty/upgrade-2", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"up": {
			charm:       "cs:trusty/upgrade-2",
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
	ch := s.setupCharm(c, "xenial/wordpress-42", "wordpress", "bionic")
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
	s.setupCharm(c, "vivid/wordpress-42", "wordpress", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: vivid/wordpress
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade application "wordpress" to charm "cs:vivid/wordpress-42": cannot change an application's series`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMultipleRelations(c *gc.C) {
	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
	s.setupCharm(c, "xenial/mysql-1", "mysql", "bionic")
	s.setupCharm(c, "xenial/postgres-2", "mysql", "bionic")
	s.setupCharm(c, "xenial/varnish-3", "varnish", "bionic")
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
            - ["varnish:webcache", "wp:cache"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:cache varnish:webcache")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":   "0",
		"pgres/0":   "1",
		"varnish/0": "2",
		"wp/0":      "3",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNewRelations(c *gc.C) {
	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
	s.setupCharm(c, "xenial/mysql-1", "mysql", "bionic")
	s.setupCharm(c, "xenial/postgres-2", "mysql", "bionic")
	s.setupCharm(c, "xenial/varnish-3", "varnish", "bionic")

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
	mysqlch := s.setupCharm(c, "xenial/mysql-2", "mysql", "bionic")
	wpch := s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")

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
	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")

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
	ch := s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
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
	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
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
	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")
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
	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
	s.setupCharm(c, "xenial/wordpress-0", "wordpress", "bionic")

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
	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
	s.setupCharm(c, "xenial/mem-47", "dummy", "bionic")
	s.setupCharm(c, "xenial/rails-0", "dummy", "bionic")
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
	s.setupCharm(c, "bionic/django-42", "dummy", "bionic")
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
	s.setupCharm(c, "bionic/django-42", "dummy", "bionic")
	s.setupCharm(c, "bionic/mem-47", "dummy", "bionic")
	s.setupCharm(c, "bionic/rails-0", "dummy", "bionic")

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
	s.setupCharm(c, "bionic/django-42", "dummy", "bionic")
	s.setupCharm(c, "bionic/mem-47", "dummy", "bionic")
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
	s.setupCharm(c, "bionic/django", "django", "bionic")
	s.setupCharm(c, "bionic/mem-47", "mem", "bionic")

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
	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")
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
	next func() []params.Delta
}

func (w mockAllWatcher) Next() ([]params.Delta, error) {
	return w.next(), nil
}

func (mockAllWatcher) Stop() error {
	return nil
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundlePassesSequences(c *gc.C) {
	s.setupCharm(c, "xenial/django-42", "dummy", "bionic")

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
		c.Assert(m.ForceDestroy(time.Duration(0)), jc.ErrorIsNil)
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

	// Now that the machines are removed, the units should be dead,
	// we need 1 more Cleanup step to remove the applications.
	c.Assert(s.State.Cleanup(), jc.ErrorIsNil)
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
