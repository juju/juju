// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
)

type BundleDeploySuite struct {
	FakeStoreStateSuite
}

var _ = gc.Suite(&BundleDeployInvalidSeries{})

func (s *BundleDeploySuite) SetUpSuite(c *gc.C) {
	s.DeploySuiteBase.SetUpSuite(c)
	s.PatchValue(&watcher.Period, 10*time.Millisecond)
}

func (s *BundleDeploySuite) SetUpTest(c *gc.C) {
	// Set metering URL config so the config is set during bootstrap
	if s.ControllerConfigAttrs == nil {
		s.ControllerConfigAttrs = make(map[string]interface{})
	}

	s.FakeStoreStateSuite.SetUpTest(c)
	logger.SetLogLevel(loggo.TRACE)
}

// DeployBundleYAML uses the given bundle content to create a bundle in the
// local repository and then deploy it. It returns the bundle deployment output
// and error.
func (s *BundleDeploySuite) DeployBundleYAML(c *gc.C, content string, extraArgs ...string) error {
	_, _, err := s.DeployBundleYAMLWithOutput(c, content, extraArgs...)
	return err
}

func (s *BundleDeploySuite) DeployBundleYAMLWithOutput(c *gc.C, content string, extraArgs ...string) (string, string, error) {
	bundlePath := s.makeBundleDir(c, content)
	args := append([]string{bundlePath}, extraArgs...)
	return s.runDeployWithOutput(c, args...)
}

func (s *BundleDeploySuite) makeBundleDir(c *gc.C, content string) string {
	bundlePath := filepath.Join(c.MkDir(), "example")
	c.Assert(os.Mkdir(bundlePath, 0777), jc.ErrorIsNil)
	err := os.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	return bundlePath
}

type BundleDeployInvalidSeries struct {
	BundleDeploySuite
}

var _ = gc.Suite(&BundleDeployInvalidSeries{})

func (s *BundleDeployInvalidSeries) TestDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C) {
	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, true)
}

func (s *BundleDeployInvalidSeries) TestDeployBundleLocalPathInvalidSeriesWithoutForce(c *gc.C) {
	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, false)
}

func (s *BundleDeployInvalidSeries) assertDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C, force bool) {
	dir := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")

	dummyURL := charm.MustParseURL("local:quantal/dummy-1")
	withAllWatcher(s.fakeAPI)
	withLocalCharmDeployable(s.fakeAPI, dummyURL, charmDir, force)
	withLocalBundleCharmDeployable(
		s.fakeAPI, dummyURL, base.MustParseBaseFromString("ubuntu@12.10"),
		charmDir.Meta(), charmDir.Manifest(), force,
	)
	s.fakeAPI.Call("CharmInfo", "local:quantal/dummy-1").Returns(
		&apicommoncharms.CharmInfo{
			URL:  "local:dummy",
			Meta: &charm.Meta{Name: "dummy", Series: []string{"jammy"}},
		},
		error(nil),
	)

	path := filepath.Join(dir, "mybundle")
	data := `
        series: quantal
        applications:
            dummy:
                charm: ./dummy
                num_units: 1
    `
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	args := []string{path}
	if force {
		args = append(args, "--force")
	}
	err = s.runDeploy(c, args...)
	if force {
		c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: base: ubuntu@12.10/stable")
	} else {
		c.Assert(err, gc.ErrorMatches, `cannot deploy bundle:.*base "ubuntu@12.10" not supported by charm.*`)
	}
}

// NOTE:
// Do not add new tests to this file.  The tests here are slowly migrating
// to deployer/bundlerhandler_test.go in mock format.

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

// NOTE(jack-w-shaw) These tests were originally part of the above suite. However,
// the below were separated out so they could be skipped. They're restored properly
// in 4.0

type BundleDeployCharmStoreSuite struct {
	BundleDeploySuite
}

var _ = gc.Suite(&BundleDeployCharmStoreSuite{})

func (s *BundleDeployCharmStoreSuite) SetUpSuite(c *gc.C) {
	c.Skip("this is a badly written e2e test that is invoking external APIs which we cannot mock")

	s.BundleDeploySuite.SetUpSuite(c)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidFlags(c *gc.C) {
	s.setupCharm(c, "ch:xenial/mysql-42", "mysql", "bionic")
	s.setupCharm(c, "ch:xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "ch:bundle/wordpress-simple-1", "wordpress-simple", "bionic", "xenial")

	err := s.runDeploy(c, "ch:bundle/wordpress-simple", "--config", "config.yaml")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --config")
	err = s.runDeploy(c, "ch:bundle/wordpress-simple", "-n", "2")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: -n")
	err = s.runDeploy(c, "ch:bundle/wordpress-simple", "--series", "xenial")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --series")
}

func (s *BundleDeployCharmStoreSuite) TestDryRunTwice(c *gc.C) {
	s.setupCharmMaybeAdd(c, "ch:xenial/mysql-42", "mysql", "bionic", false)
	s.setupCharmMaybeAdd(c, "ch:xenial/wordpress-47", "wordpress", "bionic", false)
	s.setupBundle(c, "ch:bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	stdOut, _, err := s.runDeployWithOutput(c, "ch:bundle/wordpress-simple", "--dry-run")
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
	stdOut, _, err = s.runDeployWithOutput(c, "ch:bundle/wordpress-simple", "--dry-run")
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
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runDeploy(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:xenial/dummy-1")
	ch, err := s.State.Charm("local:xenial/dummy-1")
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"dummy": {
			charm:  "local:xenial/dummy-1",
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
		os.WriteFile(filepath.Join(dir, "dummy-resource.zip"), []byte("zip file"), 0644),
		jc.ErrorIsNil)
	err := s.runDeploy(c, dir)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:bionic/dummy-resource-0")
	ch, err := s.State.Charm("local:bionic/dummy-resource-0")
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
                charm: ch:xenial/rails-42
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
                charm: ch:mysql
                num_units: -1
    `,
	err: `the provided bundle has the following errors:
negative number of units specified on application "mysql"`,
}, {
	about: "invalid constraints",
	content: `
        applications:
            mysql:
                charm: ch:mysql
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
                charm: ch:mysql
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

	s.setupCharm(c, "ch:xenial/django-0", "django", "bionic")
	s.setupCharm(c, "ch:xenial/wordpress-0", "wordpress", "bionic")
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
	lxdProfile, err := s.State.Charm("local:bionic/lxd-profile-0")
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
		os.WriteFile(
			configFile, []byte(`
                applications:
                    wordpress:
                        options:
                            blog-title: include-file://title
            `), 0644),
		jc.ErrorIsNil)
	c.Assert(
		os.WriteFile(
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
		os.WriteFile(bundleFile, []byte(bundleContent), 0644),
		jc.ErrorIsNil)

	err := s.runDeploy(c, bundleFile)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.Application("dummy")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalAndCharmStoreCharms(c *gc.C) {
	charmsPath := c.MkDir()
	wpch := s.setupCharm(c, "ch:xenial/wordpress-42", "wordpress", "bionic")
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       series: xenial
       applications:
           wordpress:
               charm: ch:xenial/wordpress-42
               series: xenial
               num_units: 1
           mysql:
               charm: %s
               num_units: 1
       relations:
           - ["wordpress:db", "mysql:server"]
   `, mysqlPath))
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:xenial/mysql-1", "ch:xenial/wordpress-42")
	mysqlch, err := s.State.Charm("local:xenial/mysql-1")
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"mysql": {
			charm:  "local:xenial/mysql-1",
			config: mysqlch.Config().DefaultSettings(),
		},
		"wordpress": {
			charm:  "ch:xenial/wordpress-42",
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
	wpch := s.setupCharm(c, "ch:xenial/wordpress-42", "wordpress", "bionic")
	dch := s.setupCharm(c, "ch:bionic/dummy-0", "dummy", "bionic")

	err := s.DeployBundleYAML(c, `
       applications:
           wordpress:
               charm: ch:wordpress
               constraints: mem=4G cores=2
           customized:
               charm: ch:bionic/dummy-0
               num_units: 1
               constraints: arch=amd64
   `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "ch:bionic/dummy-0", "ch:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"customized": {
			charm:       "ch:bionic/dummy-0",
			constraints: constraints.MustParse("arch=amd64"),
			config:      dch.Config().DefaultSettings(),
		},
		"wordpress": {
			charm:       "ch:xenial/wordpress-42",
			constraints: constraints.MustParse("mem=4G cores=2"),
			config:      wpch.Config().DefaultSettings(),
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"customized/0": "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationConstraints(c *gc.C) {
	wpch := s.setupCharm(c, "ch:xenial/wordpress-42", "wordpress", "bionic")
	dch := s.setupCharmWithArch(c, "ch:bionic/dummy-0", "dummy", "bionic", "s390x")

	err := s.DeployBundleYAML(c, `
       applications:
           wordpress:
               charm: ch:wordpress
               series: bionic
               constraints: mem=4G cores=2
           customized:
               charm: ch:dummy
               revision: 0
               channel: stable
               series: xenial
               num_units: 1
               constraints: arch=s390x
   `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "ch:bionic/dummy-0", "ch:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"customized": {
			charm:       "ch:bionic/dummy-0",
			constraints: constraints.MustParse("arch=s390x"),
			config:      dch.Config().DefaultSettings(),
		},
		"wordpress": {
			charm:       "ch:xenial/wordpress-42",
			constraints: constraints.MustParse("mem=4G cores=2"),
			config:      wpch.Config().DefaultSettings(),
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"customized/0": "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleSetAnnotations(c *gc.C) {
	s.setupCharm(c, "ch:xenial/wordpress", "wordpress", "bionic")
	s.setupCharm(c, "ch:xenial/mysql", "mysql", "bionic")
	s.setupBundle(c, "ch:bundle/wordpress-simple-1", "wordpress-simple", "bionic")

	deploy := s.deployCommandForState()
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(deploy), "ch:bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	ann, err := s.Model.Annotations(application)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{"bundleURL": "ch:bundle/wordpress-simple-1"})
	application2, err := s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	ann2, err := s.Model.Annotations(application2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann2, jc.DeepEquals, map[string]string{"bundleURL": "ch:bundle/wordpress-simple-1"})
}

func (s *BundleDeployCharmStoreSuite) TestLXCTreatedAsLXD(c *gc.C) {
	s.setupCharm(c, "ch:xenial/wordpress-0", "wordpress", "bionic")

	// Note that we use lxc here, to represent a 1.x bundle that specifies lxc.
	content := `
        applications:
            wp:
                charm: ch:wordpress
                num_units: 1
                to:
                    - lxc:0
                options:
                    blog-title: these are the voyages
            wp2:
                charm: ch:wordpress
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
	s.setupCharm(c, "ch:bionic/django-42", "dummy", "bionic")
	s.setupCharm(c, "ch:bionic/mem-47", "dummy", "bionic")
	s.setupCharm(c, "ch:bionic/rails-0", "dummy", "bionic")

	err := s.DeployBundleYAML(c, `
       applications:
           memcached:
               charm: ch:mem
               revision: 47
               channel: stable
               series: bionic
               num_units: 3
               to: [1, 2, 3]
           django:
               charm: ch:django
               revision: 42
               channel: stable
               series: bionic
               num_units: 4
               to:
                   - 1
                   - lxd:memcached
           ror:
               charm: ch:rails
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
               charm: ch:mem
               revision: 47
               channel: stable
               series: bionic
               num_units: 3
               to: [1, 2, 3]
           django:
               charm: ch:django
               revision: 42
               channel: stable
               series: bionic
               num_units: 4
               to:
                   - 1
                   - lxd:memcached
           node:
               charm: ch:django
               revision: 42
               channel: stable
               series: bionic
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
		"- deploy application node from charm-hub on bionic using django\n"+
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
	s.setupCharm(c, "ch:bionic/django-42", "dummy", "bionic")
	s.setupCharm(c, "ch:bionic/mem-47", "dummy", "bionic")
	stdOut, stdErr, err := s.DeployBundleYAMLWithOutput(c, `
       applications:
           django:
               charm: ch:django
               num_units: 1
               annotations:
                   key1: value1
                   key2: value2
               to: [1]
           memcached:
               charm: ch:mem
               revision: 47
               channel: stable
               series: bionic
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
