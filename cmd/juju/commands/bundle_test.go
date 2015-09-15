// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

// runDeployCommand executes the deploy command in order to deploy the given
// charm or bundle. The deployment output and error are returned.
func runDeployCommand(c *gc.C, id string) (string, error) {
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), id)
	return strings.Trim(coretesting.Stderr(ctx), "\n"), err
}

func (s *DeploySuite) TestDeployBundleNotFoundLocal(c *gc.C) {
	err := runDeploy(c, "local:bundle/no-such")
	c.Assert(err, gc.ErrorMatches, `entity not found in ".*": local:bundle/no-such-0`)
}

func (s *DeployCharmStoreSuite) TestDeployBundeNotFoundCharmStore(c *gc.C) {
	err := runDeploy(c, "bundle/no-such")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:bundle/no-such": bundle not found`)
}

func (s *DeployCharmStoreSuite) TestDeployBundleSuccess(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	output, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
added charm cs:trusty/mysql-42
service mysql deployed (charm: cs:trusty/mysql-42)
added charm cs:trusty/wordpress-47
service wordpress deployed (charm: cs:trusty/wordpress-47)
related wordpress:db and mysql:server
deployment of bundle "cs:bundle/wordpress-simple-1" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:trusty/mysql-42", "cs:trusty/wordpress-47")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "cs:trusty/mysql-42"},
		"wordpress": {charm: "cs:trusty/wordpress-47"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
}

func (s *DeployCharmStoreSuite) TestDeployBundleTwice(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	output, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
added charm cs:trusty/mysql-42
reusing service mysql (charm: cs:trusty/mysql-42)
added charm cs:trusty/wordpress-47
reusing service wordpress (charm: cs:trusty/wordpress-47)
wordpress:db and mysql:server are already related
deployment of bundle "cs:bundle/wordpress-simple-1" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:trusty/mysql-42", "cs:trusty/wordpress-47")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "cs:trusty/mysql-42"},
		"wordpress": {charm: "cs:trusty/wordpress-47"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
}

func (s *DeployCharmStoreSuite) TestDeployBundleGatedCharm(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/mysql-42", "mysql")
	url, _ := testcharms.UploadCharm(c, s.client, "trusty/wordpress-47", "wordpress")
	s.changeReadPerm(c, url, clientUserName)
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUplodaded(c, "cs:trusty/mysql-42", "cs:trusty/wordpress-47")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "cs:trusty/mysql-42"},
		"wordpress": {charm: "cs:trusty/wordpress-47"},
	})
}

func (s *DeployCharmStoreSuite) TestDeployBundleGatedCharmUnauthorized(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/mysql-42", "mysql")
	url, _ := testcharms.UploadCharm(c, s.client, "trusty/wordpress-47", "wordpress")
	s.changeReadPerm(c, url, "who")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: .*: unauthorized: access denied for user "client-username"`)
}

type deployRepoCharmStoreSuite struct {
	charmStoreSuite
	testing.BaseRepoSuite
}

var _ = gc.Suite(&deployRepoCharmStoreSuite{})

func (s *deployRepoCharmStoreSuite) SetUpSuite(c *gc.C) {
	s.charmStoreSuite.SetUpSuite(c)
	s.BaseRepoSuite.SetUpSuite(c)
}

func (s *deployRepoCharmStoreSuite) TearDownSuite(c *gc.C) {
	s.BaseRepoSuite.TearDownSuite(c)
	s.charmStoreSuite.TearDownSuite(c)
}

func (s *deployRepoCharmStoreSuite) SetUpTest(c *gc.C) {
	s.charmStoreSuite.SetUpTest(c)
	s.BaseRepoSuite.SetUpTest(c)
}

func (s *deployRepoCharmStoreSuite) TearDownTest(c *gc.C) {
	s.BaseRepoSuite.TearDownTest(c)
	s.charmStoreSuite.TearDownTest(c)
}

// deployBundleYAML uses the given bundle content to create a bundle in the
// local repository and then deploy it. It returns the bundle deployment output
// and error.
func (s *deployRepoCharmStoreSuite) deployBundleYAML(c *gc.C, content string) (string, error) {
	bundlePath := filepath.Join(s.BundlesPath, "example")
	c.Assert(os.Mkdir(bundlePath, 0777), jc.ErrorIsNil)
	defer os.RemoveAll(bundlePath)
	err := ioutil.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return runDeployCommand(c, "local:bundle/example")
}

var deployBundleErrorsTests = []struct {
	about   string
	content string
	err     string
}{{
	about: "local charm not found",
	content: `
        services:
            mysql:
                charm: local:mysql
                num_units: 1
    `,
	err: `cannot deploy bundle: cannot resolve URL "local:mysql": entity not found .*`,
}, {
	about: "charm store charm not found",
	content: `
        services:
            rails:
                charm: trusty/rails-42
                num_units: 1
    `,
	err: `cannot deploy bundle: cannot add charm "trusty/rails-42": cannot retrieve "cs:trusty/rails-42": charm not found`,
}, {
	about:   "invalid bundle content",
	content: "!",
	err:     `cannot unmarshal bundle data: YAML error: .*`,
}, {
	about: "invalid bundle data",
	content: `
        services:
            mysql:
                charm: mysql
                num_units: -1
    `,
	err: `cannot deploy bundle: negative number of units specified on service "mysql"`,
}, {
	about: "invalid constraints",
	content: `
        services:
            mysql:
                charm: mysql
                num_units: 1
                constraints: bad-wolf
    `,
	err: `cannot deploy bundle: invalid constraints "bad-wolf" in service "mysql": malformed constraint "bad-wolf"`,
}, {
	about: "bundle inception",
	content: `
        services:
            example:
                charm: local:bundle/example
                num_units: 1
    `,
	err: `cannot deploy bundle: expected charm URL, got bundle URL "local:bundle/example"`,
}}

func (s *deployRepoCharmStoreSuite) TestDeployBundleErrors(c *gc.C) {
	for i, test := range deployBundleErrorsTests {
		c.Logf("test %d: %s", i, test.about)
		_, err := s.deployBundleYAML(c, test.content)
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleInvalidOptions(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-42", "wordpress")
	_, err := s.deployBundleYAML(c, `
        services:
            wp:
                charm: trusty/wordpress-42
                num_units: 1
                options:
                    blog-title: 42
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot set options for service "wp": option "blog-title" expected string, got 42`)
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleLocalDeployment(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "mysql")
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "wordpress")
	output, err := s.deployBundleYAML(c, `
        services:
            wordpress:
                charm: local:wordpress
                num_units: 1
            mysql:
                charm: local:mysql
                num_units: 1
        relations:
            - ["wordpress:db", "mysql:server"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
added charm local:trusty/mysql-1
service mysql deployed (charm: local:trusty/mysql-1)
added charm local:trusty/wordpress-3
service wordpress deployed (charm: local:trusty/wordpress-3)
related wordpress:db and mysql:server
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "local:trusty/mysql-1", "local:trusty/wordpress-3")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "local:trusty/mysql-1"},
		"wordpress": {charm: "local:trusty/wordpress-3"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleLocalAndCharmStoreCharms(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-42", "wordpress")
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "mysql")
	output, err := s.deployBundleYAML(c, `
        services:
            wordpress:
                charm: trusty/wordpress-42
                num_units: 1
            mysql:
                charm: local:mysql
                num_units: 1
        relations:
            - ["wordpress:db", "mysql:server"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
added charm local:trusty/mysql-1
service mysql deployed (charm: local:trusty/mysql-1)
added charm cs:trusty/wordpress-42
service wordpress deployed (charm: cs:trusty/wordpress-42)
related wordpress:db and mysql:server
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "local:trusty/mysql-1", "cs:trusty/wordpress-42")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "local:trusty/mysql-1"},
		"wordpress": {charm: "cs:trusty/wordpress-42"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleServiceOptions(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-42", "wordpress")
	testcharms.UploadCharm(c, s.client, "precise/dummy-0", "dummy")
	output, err := s.deployBundleYAML(c, `
        services:
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
	expectedOutput := `
added charm cs:precise/dummy-0
service customized deployed (charm: cs:precise/dummy-0)
service customized configured
added charm cs:trusty/wordpress-42
service wordpress deployed (charm: cs:trusty/wordpress-42)
service wordpress configured
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:precise/dummy-0", "cs:trusty/wordpress-42")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"customized": {
			charm:  "cs:precise/dummy-0",
			config: charm.Settings{"username": "who", "skill-level": int64(47)},
		},
		"wordpress": {
			charm:  "cs:trusty/wordpress-42",
			config: charm.Settings{"blog-title": "these are the voyages"},
		},
	})
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleServiceUpgrade(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-42", "wordpress")
	testcharms.UploadCharm(c, s.client, "vivid/upgrade-1", "upgrade1")
	testcharms.UploadCharm(c, s.client, "vivid/upgrade-2", "upgrade2")

	// First deploy the bundle.
	output, err := s.deployBundleYAML(c, `
        services:
            wordpress:
                charm: wordpress-42
                num_units: 1
                options:
                    blog-title: these are the voyages
            up:
                charm: vivid/upgrade-1
                num_units: 1
    `)
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
added charm cs:vivid/upgrade-1
service up deployed (charm: cs:vivid/upgrade-1)
added charm cs:trusty/wordpress-42
service wordpress deployed (charm: cs:trusty/wordpress-42)
service wordpress configured
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:vivid/upgrade-1", "cs:trusty/wordpress-42")

	// Then deploy a new bundle with modified charm revision and options.
	output, err = s.deployBundleYAML(c, `
        services:
            wordpress:
                charm: wordpress-42
                num_units: 1
                options:
                    blog-title: new title
            up:
                charm: vivid/upgrade-2
                num_units: 1
    `)
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput = `
added charm cs:vivid/upgrade-2
upgraded charm for existing service up (from cs:vivid/upgrade-1 to cs:vivid/upgrade-2)
added charm cs:trusty/wordpress-42
reusing service wordpress (charm: cs:trusty/wordpress-42)
service wordpress configured
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:vivid/upgrade-1", "cs:vivid/upgrade-2", "cs:trusty/wordpress-42")
	s.assertServicesDeployed(c, map[string]serviceInfo{
		"up": {charm: "cs:vivid/upgrade-2"},
		"wordpress": {
			charm:  "cs:trusty/wordpress-42",
			config: charm.Settings{"blog-title": "new title"},
		},
	})
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleServiceUpgradeFailure(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Try upgrading to a different charm name.
	testcharms.UploadCharm(c, s.client, "trusty/incompatible-42", "wordpress")
	_, err := s.deployBundleYAML(c, `
        services:
            wordpress:
                charm: trusty/incompatible-42
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade service "wordpress": bundle charm "cs:trusty/incompatible-42" is incompatible with existing charm "local:quantal/wordpress-3"`)

	// Try upgrading to a different user.
	testcharms.UploadCharm(c, s.client, "~who/trusty/wordpress-42", "wordpress")
	_, err = s.deployBundleYAML(c, `
        services:
            wordpress:
                charm: cs:~who/trusty/wordpress-42
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade service "wordpress": bundle charm "cs:~who/trusty/wordpress-42" is incompatible with existing charm "local:quantal/wordpress-3"`)

	// Try upgrading to a different series.
	testcharms.UploadCharm(c, s.client, "vivid/wordpress-42", "wordpress")
	_, err = s.deployBundleYAML(c, `
        services:
            wordpress:
                charm: vivid/wordpress
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade service "wordpress": bundle charm "cs:vivid/wordpress-42" is incompatible with existing charm "local:quantal/wordpress-3"`)
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleMultipleRelations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-0", "wordpress")
	testcharms.UploadCharm(c, s.client, "trusty/mysql-1", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/postgres-2", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/varnish-3", "varnish")
	output, err := s.deployBundleYAML(c, `
        services:
            wp:
                charm: wordpress
                num_units: 1
            mysql:
                charm: mysql-1
                num_units: 1
            pgres:
                charm: trusty/postgres-2
                num_units: 1
            varnish:
                charm: trusty/varnish
                num_units: 1
        relations:
            - ["wp:db", "mysql:server"]
            - ["wp:db", "pgres:server"]
            - ["varnish:webcache", "wp:cache"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
added charm cs:trusty/mysql-1
service mysql deployed (charm: cs:trusty/mysql-1)
added charm cs:trusty/postgres-2
service pgres deployed (charm: cs:trusty/postgres-2)
added charm cs:trusty/varnish-3
service varnish deployed (charm: cs:trusty/varnish-3)
added charm cs:trusty/wordpress-0
service wp deployed (charm: cs:trusty/wordpress-0)
related wp:db and mysql:server
related wp:db and pgres:server
related varnish:webcache and wp:cache
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:db pgres:server", "wp:cache varnish:webcache")
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleNewRelations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-0", "wordpress")
	testcharms.UploadCharm(c, s.client, "trusty/mysql-1", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/postgres-2", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/varnish-3", "varnish")
	_, err := s.deployBundleYAML(c, `
        services:
            wp:
                charm: wordpress
                num_units: 1
            mysql:
                charm: mysql-1
                num_units: 1
            varnish:
                charm: trusty/varnish
                num_units: 1
        relations:
            - ["wp:db", "mysql:server"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	output, err := s.deployBundleYAML(c, `
        services:
            wp:
                charm: wordpress
                num_units: 1
            mysql:
                charm: mysql-1
                num_units: 1
            varnish:
                charm: trusty/varnish
                num_units: 1
        relations:
            - ["wp:db", "mysql:server"]
            - ["varnish:webcache", "wp:cache"]
    `)
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := `
added charm cs:trusty/mysql-1
reusing service mysql (charm: cs:trusty/mysql-1)
added charm cs:trusty/varnish-3
reusing service varnish (charm: cs:trusty/varnish-3)
added charm cs:trusty/wordpress-0
reusing service wp (charm: cs:trusty/wordpress-0)
wp:db and mysql:server are already related
related varnish:webcache and wp:cache
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:cache varnish:webcache")
}
