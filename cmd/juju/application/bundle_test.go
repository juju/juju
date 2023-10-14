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

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
)

// NOTE:
// Do not add new tests to this file.  The tests here are slowly migrating
// to bundle/bundlerhandler_test.go in mock format.

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidFlags(c *gc.C) {
	s.setupCharm(c, "cs:xenial/mysql-42", "mysql", "bionic")
	s.setupCharm(c, "cs:xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "cs:bundle/wordpress-simple-1", "wordpress-simple", "bionic", "xenial")

	err := s.runDeploy(c, "cs:bundle/wordpress-simple", "--config", "config.yaml")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --config")
	err = s.runDeploy(c, "cs:bundle/wordpress-simple", "-n", "2")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: -n")
	err = s.runDeploy(c, "cs:bundle/wordpress-simple", "--series", "xenial")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --series")
}

func (s *BundleDeployCharmStoreSuite) TestAddMetricCredentials(c *gc.C) {
	s.fakeAPI.planURL = s.server.URL
	s.setupCharm(c, "cs:xenial/wordpress", "wordpress", "bionic")
	s.setupCharm(c, "cs:xenial/mysql", "mysql", "bionic")
	s.setupBundle(c, "cs:bundle/wordpress-with-plans-1", "wordpress-with-plans", "xenial")

	// `"hello registration"\n` (quotes and newline from json
	// encoding) is returned by the fake http server. This is binary64
	// encoded before the call into SetMetricCredentials.
	creds := append([]byte(`"aGVsbG8gcmVnaXN0cmF0aW9u"`), 0xA)
	s.fakeAPI.Call("SetMetricCredentials", "wordpress", creds).Returns(error(nil))

	deploy := s.deployCommandForState()
	deploy.Steps = []deployer.DeployStep{&deployer.RegisterMeteredCharm{PlanURL: s.server.URL, RegisterPath: "", QueryPath: ""}}
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bundle/wordpress-with-plans")
	c.Assert(err, jc.ErrorIsNil)

	// The order of calls here does not matter and is, in fact, not guaranteed.
	// All we care about here is that the calls exist.
	s.stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "DefaultPlan",
		Args:     []interface{}{"cs:wordpress"},
	}, {
		FuncName: "Authorize",
		Args: []interface{}{deployer.MetricRegistrationPost{
			ModelUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			CharmURL:        "cs:wordpress",
			ApplicationName: "wordpress",
			PlanURL:         "thisplan",
			IncreaseBudget:  0,
		}},
	}, {
		FuncName: "Authorize",
		Args: []interface{}{deployer.MetricRegistrationPost{
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleEndpointBindingsSpaceMissing(c *gc.C) {
	s.setupCharm(c, "cs:xenial/mysql-42", "mysql", "bionic")
	s.setupCharmMaybeAdd(c, "cs:xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings", "bionic", false)
	s.setupBundle(c, "cs:bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings", "bionic")

	stdOut, stdErr, err := s.runDeployWithOutput(c, "cs:bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, gc.ErrorMatches, ""+
		"cannot deploy bundle: cannot deploy application \"mysql\": "+
		"space not found")
	c.Assert(stdErr, gc.Equals, ""+
		`Located bundle "wordpress-with-endpoint-bindings" in charm-store, revision 1`+"\n"+
		"Located charm \"mysql\" in charm-store\n"+
		"Located charm \"wordpress-extra-bindings\" in charm-store")
	c.Assert(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application mysql from charm-store on xenial")
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{})
	s.assertUnitsCreated(c, map[string]string{})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleEndpointBindingsSuccess(c *gc.C) {
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	publicSpace, err := s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	mysqlch := s.setupCharm(c, "cs:xenial/mysql-42", "mysql", "bionic")
	wpch := s.setupCharm(c, "cs:xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings", "bionic")
	s.setupBundle(c, "cs:bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings", "bionic")

	err = s.runDeploy(c, "cs:bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-extra-bindings-47")

	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql": {
			charm:  "cs:xenial/mysql-42",
			config: mysqlch.Config().DefaultSettings(),
		},
		"wordpress-extra-bindings": {
			charm:  "cs:xenial/wordpress-extra-bindings-47",
			config: wpch.Config().DefaultSettings(),
		},
	})
	s.assertDeployedApplicationBindings(c, map[string]applicationInfo{
		"mysql": {
			endpointBindings: map[string]string{
				"":               network.AlphaSpaceId,
				"db":             network.AlphaSpaceId,
				"server":         dbSpace.Id(),
				"server-admin":   network.AlphaSpaceId,
				"metrics-client": network.AlphaSpaceId,
			},
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
	mysqlch := s.setupCharm(c, "cs:xenial/mysql-42", "mysql", "bionic")
	wpch := s.setupCharm(c, "cs:xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "cs:bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	stdOut, stdErr, err := s.runDeployWithOutput(c, "cs:bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm mysql from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application mysql from charm-store on xenial\n"+
		"- set annotations for mysql\n"+
		"- upload charm wordpress from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on xenial\n"+
		"- set annotations for wordpress\n"+
		"- add relation wordpress:db - mysql:db\n"+
		"- add unit mysql/0 to new machine 0\n"+
		"- add unit wordpress/0 to new machine 1",
	)
	c.Check(stdErr, gc.Equals, ""+
		"Located bundle \"wordpress-simple\" in charm-store, revision 1\n"+
		"Located charm \"mysql\" in charm-store\n"+
		"Located charm \"wordpress\" in charm-store\n"+
		"Deploy of bundle completed.",
	)
	stdOut, stdErr, err = s.runDeployWithOutput(c, "cs:bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	// Nothing to do... not quite...
	c.Check(stdOut, gc.Equals, "")
	c.Check(stdErr, gc.Equals, ""+
		"Located bundle \"wordpress-simple\" in charm-store, revision 1\n"+
		"No changes to apply.",
	)

	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql":     {charm: "cs:xenial/mysql-42", config: mysqlch.Config().DefaultSettings()},
		"wordpress": {charm: "cs:xenial/wordpress-47", config: wpch.Config().DefaultSettings()},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:db")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDryRunTwice(c *gc.C) {
	s.setupCharmMaybeAdd(c, "cs:xenial/mysql-42", "mysql", "bionic", false)
	s.setupCharmMaybeAdd(c, "cs:xenial/wordpress-47", "wordpress", "bionic", false)
	s.setupBundle(c, "cs:bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	stdOut, _, err := s.runDeployWithOutput(c, "cs:bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	expected := "" +
		"Changes to deploy bundle:\n" +
		"- upload charm mysql from charm-store for series xenial with architecture=amd64\n" +
		"- deploy application mysql from charm-store on xenial\n" +
		"- set annotations for mysql\n" +
		"- upload charm wordpress from charm-store for series xenial with architecture=amd64\n" +
		"- deploy application wordpress from charm-store on xenial\n" +
		"- set annotations for wordpress\n" +
		"- add relation wordpress:db - mysql:db\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add unit wordpress/0 to new machine 1"

	c.Check(stdOut, gc.Equals, expected)
	stdOut, _, err = s.runDeployWithOutput(c, "cs:bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, expected)

	s.assertCharmsUploaded(c /* none */)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{})
	s.assertRelationsEstablished(c /* none */)
	s.assertUnitsCreated(c, map[string]string{})
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
		"dummy": {
			charm:  "local:xenial/dummy-1",
			config: ch.Config().DefaultSettings(),
		},
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
        series: quantal
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
		c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: dummy is not available on the following series: quantal not supported")
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:quantal/dummy-1")
	ch, err := s.State.Charm(charm.MustParseURL("local:quantal/dummy-1"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {
			charm:  "local:quantal/dummy-1",
			config: ch.Config().DefaultSettings(),
		},
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
		"dummy-resource": {
			charm:  "local:bionic/dummy-resource-0",
			config: ch.Config().DefaultSettings(),
		},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNoSeriesInCharmURL(c *gc.C) {
	s.setupCharm(c, "cs:~who/multi-series-0", "multi-series", "bionic")
	dir := c.MkDir()
	testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")
	path := filepath.Join(dir, "mybundle")
	data := `
        series: bionic
        applications:
            dummy:
                charm: cs:~who/multi-series
    `
	err := ioutil.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:~who/multi-series-0")
	ch, err := s.State.Charm(charm.MustParseURL("cs:~who/multi-series-0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {
			charm:  "cs:~who/multi-series-0",
			config: ch.Config().DefaultSettings(),
		},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleResources(c *gc.C) {
	c.Skip("Test is trying to get resources from real api, not fake")
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
		"- upload charm cs:trusty/starsay-42 for series trusty with architecture=amd64\n"+
		"- deploy application starsay on trusty using cs:trusty/starsay-42\n"+
		"- add unit starsay/0 to new machine 0",
	)
	// Info messages go to stdErr.
	c.Check(stdErr, gc.Equals, ""+
		"Located charm \"startsay\" in charm-store\n"+
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

	s.checkResources(c, "starsay", []resources.Resource{{
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

func (s *BundleDeployCharmStoreSuite) checkResources(c *gc.C, app string, expected []resources.Resource) {
	_, err := s.State.Application(app)
	c.Check(err, jc.ErrorIsNil)
	st := s.State.Resources()
	svcResources, err := st.ListResources(app)
	c.Assert(err, jc.ErrorIsNil)
	res := svcResources.Resources
	resources.Sort(res)
	c.Assert(res, gc.HasLen, 3)
	c.Check(res[2].Timestamp, gc.Not(gc.Equals), time.Time{})
	res[2].Timestamp = time.Time{}
	c.Assert(res, jc.DeepEquals, expected)
}

type BundleDeployCharmStoreSuite struct {
	FakeStoreStateSuite

	stub   *testing.Stub
	server *httptest.Server
}

// Disabled: see "this is a badly written e2e test that is invoking external APIs which we cannot mock0"
//var _ = gc.Suite(&BundleDeployCharmStoreSuite{})

func (s *BundleDeployCharmStoreSuite) SetUpSuite(c *gc.C) {
	s.DeploySuiteBase.SetUpSuite(c)
	s.PatchValue(&watcher.Period, 10*time.Millisecond)
}

func (s *BundleDeployCharmStoreSuite) SetUpTest(c *gc.C) {
	c.Skip("this is a badly written e2e test that is invoking external APIs which we cannot mock0")

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
                charm: cs:xenial/rails-42
                num_units: 1
    `,
	err: `cannot resolve charm or bundle "rails": .* charm or bundle not found`,
}, {
	about:   "invalid bundle content",
	content: "!",
	err:     `(?s)cannot unmarshal bundle contents:.* yaml: unmarshal errors:.*`,
}, {
	about: "invalid bundle data",
	content: `
        applications:
            mysql:
                charm: cs:mysql
                num_units: -1
    `,
	err: `the provided bundle has the following errors:
negative number of units specified on application "mysql"`,
}, {
	about: "invalid constraints",
	content: `
        applications:
            mysql:
                charm: cs:mysql
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
                charm: cs:mysql
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
	err: `cannot deploy local charm at ".*wordpress": file does not exist`,
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
	s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: cs:xenial/wordpress-42
                num_units: 1
                options:
                    blog-title: 42
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": option "blog-title" expected string, got 42`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidSeries(c *gc.C) {
	s.setupCharm(c, "cs:trusty/django-0", "django", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:trusty/django
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
	s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	err = s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: cs:xenial/wordpress-42
                num_units: 1
                bindings:
                  noturl: public
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": cannot add application "wp": unknown endpoint "noturl" not valid`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidSpace(c *gc.C) {
	s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: cs:xenial/wordpress-42
                num_units: 1
                bindings:
                  url: public
    `)
	// TODO(jam): 2017-02-05 double repeating "cannot deploy application" and "cannot add application" is a bit ugly
	// https://pad.lv/1661937
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot deploy application "wp": space not found`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWatcherTimeout(c *gc.C) {
	c.Skip("Move me to bundle/bundlerhander_test.go and use mocks")
	// Inject an "AllWatcher" that never delivers a result.
	ch := make(chan struct{})
	defer close(ch)
	//watcher := mockAllWatcher{
	//	next: func() []params.Delta {
	//		<-ch
	//		return nil
	//	},
	//}
	//s.PatchValue(&watchAll, func(*api.Client) (api.AllWatch, error) {
	//	return watcher, nil
	//})

	s.setupCharm(c, "cs:xenial/django-0", "django", "bionic")
	s.setupCharm(c, "cs:xenial/wordpress-0", "wordpress", "bionic")
	//s.PatchValue(&updateUnitStatusPeriod, 0*time.Second)
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
        applications:
            lxd-profile:
                charm: %s
                num_units: 1
    `, lxdProfilePath))
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:bionic/lxd-profile-0")
	lxdProfile, err := s.State.Charm(charm.MustParseURL("local:bionic/lxd-profile-0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"lxd-profile": {
			charm:  "local:bionic/lxd-profile-0",
			config: lxdProfile.Config().DefaultSettings(),
		},
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
        applications:
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
        applications:
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
	wpch := s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
        series: xenial
        applications:
            wordpress:
                charm: cs:xenial/wordpress-42
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
		"mysql": {
			charm:  "local:xenial/mysql-1",
			config: mysqlch.Config().DefaultSettings(),
		},
		"wordpress": {
			charm:  "cs:xenial/wordpress-42",
			config: wpch.Config().DefaultSettings(),
		},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationOptions(c *gc.C) {
	wpch := s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	dch := s.setupCharm(c, "cs:bionic/dummy-0", "dummy", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: cs:wordpress
                num_units: 1
                options:
                    blog-title: these are the voyages
            customized:
                charm: cs:bionic/dummy-0
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationDefaultArchConstraints(c *gc.C) {
	wpch := s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	dch := s.setupCharm(c, "cs:bionic/dummy-0", "dummy", "bionic")

	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: cs:wordpress
                constraints: mem=4G cores=2
            customized:
                charm: cs:bionic/dummy-0
                num_units: 1
                constraints: arch=amd64
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:bionic/dummy-0", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"customized": {
			charm:       "cs:bionic/dummy-0",
			constraints: constraints.MustParse("arch=amd64"),
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationConstraints(c *gc.C) {
	wpch := s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	dch := s.setupCharmWithArch(c, "cs:bionic/dummy-0", "dummy", "bionic", "i386")

	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: cs:wordpress
                constraints: mem=4G cores=2
            customized:
                charm: cs:bionic/dummy-0
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
	s.setupCharm(c, "cs:xenial/wordpress", "wordpress", "bionic")
	s.setupCharm(c, "cs:xenial/mysql", "mysql", "bionic")
	s.setupBundle(c, "cs:bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	deploy := s.deployCommandForState()
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "cs:bundle/wordpress-simple")
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
	wpch := s.setupCharm(c, "cs:xenial/wordpress-42", "wordpress", "bionic")
	s.setupCharm(c, "cs:trusty/upgrade-1", "upgrade1", "bionic")

	// First deploy the bundle.
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: cs:wordpress
                num_units: 1
                options:
                    blog-title: these are the voyages
                constraints: spaces=final,frontiers mem=8000M
            up:
                charm: cs:trusty/upgrade-1
                num_units: 1
                constraints: mem=8G
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:trusty/upgrade-1", "cs:xenial/wordpress-42")

	ch := s.setupCharm(c, "cs:trusty/upgrade-2", "upgrade2", "bionic")
	// Then deploy a new bundle with modified charm revision and options.
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            wordpress:
                charm: cs:wordpress
                num_units: 1
                options:
                    blog-title: new title
                constraints: spaces=new cores=8
            up:
                charm: cs:trusty/upgrade-2
                num_units: 1
                constraints: mem=8G
    `)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm upgrade from charm-store for series trusty with architecture=amd64\n"+
		"- upgrade up from charm-store using charm upgrade for series trusty\n"+
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationUpgradeFailure(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Try upgrading to a different series.
	// Note that this test comes before the next one because
	// otherwise we can't resolve the charm URL because the charm's
	// "base entity" is not marked as promulgated so the query by
	// promulgated will find it.
	s.setupCharm(c, "cs:vivid/wordpress-42", "wordpress", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: cs:vivid/wordpress
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade application "wordpress" to charm "cs:vivid/wordpress-42": cannot change an application's series`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNewRelations(c *gc.C) {
	s.setupCharm(c, "cs:xenial/wordpress-0", "wordpress", "bionic")
	s.setupCharm(c, "cs:xenial/mysql-1", "mysql", "bionic")
	s.setupCharm(c, "cs:xenial/postgres-2", "mysql", "bionic")
	s.setupCharm(c, "cs:xenial/varnish-3", "varnish", "bionic")

	err := s.DeployBundleYAML(c, `
        applications:
            wp:
                charm: cs:wordpress
                num_units: 1
            mysql:
                charm: cs:mysql
                num_units: 1
            varnish:
                charm: cs:xenial/varnish
                num_units: 1
        relations:
            - ["wp:db", "mysql:server"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            wp:
                charm: cs:wordpress
                num_units: 1
            mysql:
                charm: cs:mysql
                num_units: 1
            varnish:
                charm: cs:xenial/varnish
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
	mysqlch := s.setupCharm(c, "cs:xenial/mysql-2", "mysql", "bionic")
	wpch := s.setupCharm(c, "cs:xenial/wordpress-0", "wordpress", "bionic")

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
		"sql": {
			charm:  "cs:xenial/mysql-2",
			config: mysqlch.Config().DefaultSettings(),
		},
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
	s.setupCharm(c, "cs:xenial/wordpress-0", "wordpress", "bionic")

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
	ch := s.setupCharm(c, "cs:xenial/django-42", "dummy", "bionic")
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
		"django": {
			charm:  "cs:xenial/django-42",
			config: ch.Config().DefaultSettings(),
		},
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
	s.setupCharm(c, "cs:xenial/django-42", "dummy", "bionic")
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
	s.setupCharm(c, "cs:xenial/django-42", "dummy", "bionic")
	s.setupCharm(c, "cs:xenial/wordpress-0", "wordpress", "bionic")
	err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: cs:wordpress
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
	s.setupCharm(c, "cs:xenial/django-42", "dummy", "bionic")
	s.setupCharm(c, "cs:xenial/wordpress-0", "wordpress", "bionic")

	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, `
        applications:
            wordpress:
                charm: cs:wordpress
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
		"- upload charm django from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application django from charm-store on xenial\n"+
		"- upload charm wordpress from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application wordpress from charm-store on xenial\n"+
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
	s.setupCharm(c, "cs:xenial/django-42", "dummy", "bionic")
	s.setupCharm(c, "cs:xenial/mem-47", "dummy", "bionic")
	s.setupCharm(c, "cs:xenial/rails-0", "dummy", "bionic")
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
                charm: cs:rails
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleSwitch(c *gc.C) {
	s.setupCharm(c, "cs:bionic/django-42", "dummy", "bionic")
	s.setupCharm(c, "cs:bionic/rails-0", "dummy", "bionic")

	err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:bionic/django-42
                num_units: 1
            ror:
                charm: cs:rails
                num_units: 1
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0": "0",
		"ror/0":    "1",
	})

	// Redeploy a very similar bundle with another application unit. The new unit
	// is placed on the first unit of memcached. Due to ordering of the applications
	// there is no deterministic way to determine "least crowded" in a meaningful way.
	content := `
        applications:
            django:
                charm: cs:bionic/django-42
                num_units: 1
            node:
                charm: cs:bionic/django-42
                num_units: 1
    `
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, content)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- deploy application node from charm-store on bionic using django\n"+
		"- add unit node/0 to new machine 2",
	)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMassiveUnitColocation(c *gc.C) {
	s.setupCharm(c, "cs:bionic/django-42", "dummy", "bionic")
	s.setupCharm(c, "cs:bionic/mem-47", "dummy", "bionic")
	s.setupCharm(c, "cs:bionic/rails-0", "dummy", "bionic")

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
                charm: cs:rails
                num_units: 3
                to:
                    - 1
                    - kvm:3
        machines:
            1:
                series: bionic
            2:
                series: bionic
            3:
                series: bionic
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
                series: bionic
            2:
                series: bionic
            3:
                series: bionic
    `
	stdOut, _, err := s.DeployBundleYAMLWithOutput(c, content)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- deploy application node from charm-store on bionic using django\n"+
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
	s.setupCharm(c, "cs:bionic/django-42", "dummy", "bionic")
	s.setupCharm(c, "cs:bionic/mem-47", "dummy", "bionic")
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
                charm: cs:bionic/mem-47
                num_units: 1
        machines:
            1:
                annotations: {foo: bar}
                series: bionic
    `)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(stdOut, gc.Equals, ""+
		"Executing changes:\n"+
		"- upload charm django from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application django from charm-store on bionic\n"+
		"- set annotations for django\n"+
		"- upload charm mem from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application memcached from charm-store on bionic using mem\n"+
		"- add new machine 0 (bundle machine 1)\n"+
		"- set annotations for new machine 0\n"+
		"- add unit django/0 to new machine 0\n"+
		"- add unit memcached/0 to new machine 1",
	)
	c.Check(stdErr, gc.Equals, ""+
		"Located charm \"django\" in charm-store\n"+
		"Located charm \"mem\" in charm-store, revision 47\n"+
		"Deploy of bundle completed.",
	)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleAnnotations(c *gc.C) {
	s.setupCharm(c, "cs:bionic/django", "django", "bionic")
	s.setupCharm(c, "cs:bionic/mem-47", "mem", "bionic")

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
                charm: cs:bionic/mem-47
                num_units: 1
        machines:
            1:
                annotations: {foo: bar}
                series: bionic
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
                series: bionic
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
	s.setupCharm(c, "cs:xenial/django-42", "dummy", "bionic")
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundlePassesSequences(c *gc.C) {
	s.setupCharm(c, "cs:xenial/django-42", "dummy", "bionic")

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
		"- upload charm django from charm-store for series xenial with architecture=amd64\n"+
		"- deploy application django from charm-store on xenial\n"+
		"- add unit django/2 to new machine 2\n"+
		"- add unit django/3 to new machine 3",
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/2": "2",
		"django/3": "3",
	})
}
