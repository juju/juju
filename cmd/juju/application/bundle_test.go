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

	"github.com/juju/charm/v9"
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
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
)

// NOTE:
// Do not add new tests to this file.  The tests here are slowly migrating
// to deployer/bundlerhandler_test.go in mock format.

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

type BundleDeployCharmStoreSuite struct {
	FakeStoreStateSuite

	stub   *testing.Stub
	server *httptest.Server
}

var _ = gc.Suite(&BundleDeployCharmStoreSuite{})

func (s *BundleDeployCharmStoreSuite) SetUpSuite(c *gc.C) {
	c.Skip("this is a badly written e2e test that is invoking external APIs which we cannot mock0")

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

func (s *BundleDeployCharmStoreSuite) TestDryRunTwice(c *gc.C) {
	s.setupCharmMaybeAdd(c, "cs:xenial/mysql-42", "mysql", "bionic", false)
	s.setupCharmMaybeAdd(c, "cs:xenial/wordpress-47", "wordpress", "bionic", false)
	s.setupBundle(c, "cs:bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	stdOut, _, err := s.runDeployWithOutput(c, "cs:bundle/wordpress-simple", "--dry-run")
	c.Assert(err, jc.ErrorIsNil)
	expected := "" +
		"Changes to deploy bundle:\n" +
		"- upload charm wordpress from charm-store for series xenial with architecture=amd64\n" +
		"- upload charm mysql from charm-store for series xenial with architecture=amd64\n" +
		"- deploy application wordpress from charm-store on xenial\n" +
		"- deploy application mysql from charm-store on xenial\n" +
		"- add unit wordpress/0 to new machine 1\n" +
		"- add unit mysql/0 to new machine 0\n" +
		"- add relation wordpress:db - mysql:db\n" +
		"- set annotations for wordpress\n" +
		"- set annotations for mysql"

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
		"mysql/0":     "1",
		"wordpress/0": "0",
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
		"- upload charm mem from charm-store for series bionic with architecture=amd64\n"+
		"- upload charm django from charm-store for series bionic with architecture=amd64\n"+
		"- deploy application memcached from charm-store on bionic using mem\n"+
		"- deploy application django from charm-store on bionic\n"+
		"- add new machine 0 (bundle machine 1)\n"+
		"- add unit memcached/0 to new machine 1\n"+
		"- add unit django/0 to new machine 0\n"+
		"- set annotations for new machine 0\n"+
		"- set annotations for django",
	)
	c.Check(stdErr, gc.Equals, ""+
		"Located charm \"django\" in charm-store\n"+
		"Located charm \"mem\" in charm-store, revision 47\n"+
		"Deploy of bundle completed.",
	)
}
