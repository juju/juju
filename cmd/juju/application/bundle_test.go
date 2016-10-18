// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"

	"github.com/juju/juju/api"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

// LTS-dependent requires new entry upon new LTS release. There are numerous
// places "xenial" exists in strings throughout this file. If we update the
// target in testing/base.go:SetupSuite we'll need to also update the entries
// herein.

// runDeployCommand executes the deploy command in order to deploy the given
// charm or bundle. The deployment output and error are returned.
func runDeployCommand(c *gc.C, id string, args ...string) (string, error) {
	args = append([]string{id}, args...)
	ctx, err := coretesting.RunCommand(c, NewDefaultDeployCommand(), args...)
	return strings.Trim(coretesting.Stderr(ctx), "\n"), err
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleNotFoundCharmStore(c *gc.C) {
	err := runDeploy(c, "bundle/no-such")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:bundle/no-such": bundle not found`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidFlags(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple", "--config", "config.yaml")
	c.Assert(err, gc.ErrorMatches, "Flags provided but not supported when deploying a bundle: --config.")
	_, err = runDeployCommand(c, "bundle/wordpress-simple", "-n", "2")
	c.Assert(err, gc.ErrorMatches, "Flags provided but not supported when deploying a bundle: -n.")
	_, err = runDeployCommand(c, "bundle/wordpress-simple", "--series", "xenial", "--force")
	c.Assert(err, gc.ErrorMatches, "Flags provided but not supported when deploying a bundle: --force, --series.")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleSuccess(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "cs:xenial/mysql-42"},
		"wordpress": {charm: "cs:xenial/wordpress-47"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithTermsSuccess(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/terms1-17", "terms1")
	testcharms.UploadCharm(c, s.client, "xenial/terms2-42", "terms2")
	testcharms.UploadBundle(c, s.client, "bundle/terms-simple-1", "terms-simple")
	_, err := runDeployCommand(c, "bundle/terms-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/terms1-17", "cs:xenial/terms2-42")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"terms1": {charm: "cs:xenial/terms1-17"},
		"terms2": {charm: "cs:xenial/terms2-42"},
	})
	s.assertUnitsCreated(c, map[string]string{
		"terms1/0": "0",
		"terms2/0": "1",
	})
	c.Assert(s.termsString, gc.Not(gc.Equals), "")
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleStorage(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql-storage")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-with-mysql-storage-1", "wordpress-with-mysql-storage")
	_, err := runDeployCommand(
		c, "bundle/wordpress-with-mysql-storage",
		"--storage", "mysql:logs=tmpfs,10G", // override logs
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"mysql": {
			charm: "cs:xenial/mysql-42",
			storage: map[string]state.StorageConstraints{
				"data": state.StorageConstraints{Pool: "rootfs", Size: 50 * 1024, Count: 1},
				"logs": state.StorageConstraints{Pool: "tmpfs", Size: 10 * 1024, Count: 1},
			},
		},
		"wordpress": {charm: "cs:xenial/wordpress-47"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleEndpointBindingsSpaceMissing(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings")
	output, err := runDeployCommand(c, "bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, gc.ErrorMatches, ""+
		"cannot deploy bundle: cannot deploy application \"mysql\": "+
		"cannot add application \"mysql\": unknown space \"db\" not valid")
	c.Assert(output, gc.Equals, ""+
		`Located bundle "cs:bundle/wordpress-with-endpoint-bindings-1"`+"\n"+
		`Deploying charm "cs:xenial/mysql-42"`,
	)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{})
	s.assertUnitsCreated(c, map[string]string{})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleEndpointBindingsSuccess(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-extra-bindings-47", "wordpress-extra-bindings")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-with-endpoint-bindings-1", "wordpress-with-endpoint-bindings")
	_, err = runDeployCommand(c, "bundle/wordpress-with-endpoint-bindings")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-extra-bindings-47")

	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"mysql":                    {charm: "cs:xenial/mysql-42"},
		"wordpress-extra-bindings": {charm: "cs:xenial/wordpress-extra-bindings-47"},
	})
	s.assertDeployedServiceBindings(c, map[string]serviceInfo{
		"mysql": {
			endpointBindings: map[string]string{"server": "db"},
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
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	_, err = runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "cs:xenial/mysql-42"},
		"wordpress": {charm: "cs:xenial/wordpress-47"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleGatedCharm(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	url, _ := testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	s.changeReadPerm(c, url, clientUserName)
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:xenial/mysql-42", "cs:xenial/wordpress-47")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "cs:xenial/mysql-42"},
		"wordpress": {charm: "cs:xenial/wordpress-47"},
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
	_, err = runDeployCommand(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "local:xenial/dummy-1")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"dummy": {charm: "local:xenial/dummy-1"},
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
	_, err = runDeployCommand(c, path)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:~who/multi-series-0")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"dummy": {charm: "cs:~who/multi-series-0"},
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleGatedCharmUnauthorized(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/mysql-42", "mysql")
	url, _ := testcharms.UploadCharm(c, s.client, "xenial/wordpress-47", "wordpress")
	s.changeReadPerm(c, url, "who")
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: .*: unauthorized: access denied for user "client-username"`)
}

type BundleDeployCharmStoreSuite struct {
	charmStoreSuite
}

var _ = gc.Suite(&BundleDeployCharmStoreSuite{})

func (s *BundleDeployCharmStoreSuite) SetUpSuite(c *gc.C) {
	s.charmStoreSuite.SetUpSuite(c)
	s.PatchValue(&watcher.Period, 10*time.Millisecond)
}

func (s *BundleDeployCharmStoreSuite) Client() *csclient.Client {
	return s.client
}

// DeployBundleYAML uses the given bundle content to create a bundle in the
// local repository and then deploy it. It returns the bundle deployment output
// and error.
func (s *BundleDeployCharmStoreSuite) DeployBundleYAML(c *gc.C, content string) (string, error) {
	bundlePath := filepath.Join(c.MkDir(), "example")
	c.Assert(os.Mkdir(bundlePath, 0777), jc.ErrorIsNil)
	defer os.RemoveAll(bundlePath)
	err := ioutil.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return runDeployCommand(c, bundlePath)
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
charm path in application "mysql" does not exist: mysql`,
}, {
	about: "charm store charm not found",
	content: `
        applications:
            rails:
                charm: xenial/rails-42
                num_units: 1
    `,
	err: `cannot deploy bundle: cannot resolve URL "xenial/rails-42": cannot resolve URL "cs:xenial/rails-42": charm not found`,
}, {
	about:   "invalid bundle content",
	content: "!",
	err:     `cannot unmarshal bundle data: yaml: .*`,
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
	err: `cannot deploy bundle: cannot resolve URL "local:wordpress": unknown schema for charm URL "local:wordpress"`,
}}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleErrors(c *gc.C) {
	for i, test := range deployBundleErrorsTests {
		c.Logf("test %d: %s", i, test.about)
		_, err := s.DeployBundleYAML(c, test.content)
		c.Check(err, gc.ErrorMatches, test.err)
	}
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleInvalidOptions(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	_, err := s.DeployBundleYAML(c, `
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
	_, err := s.DeployBundleYAML(c, `
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
	_, err := s.DeployBundleYAML(c, `
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
	_, err := s.DeployBundleYAML(c, `
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
	_, err := s.DeployBundleYAML(c, fmt.Sprintf(`
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
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "local:xenial/mysql-1"},
		"wordpress": {charm: "local:xenial/wordpress-3"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"mysql/1":     "1",
		"wordpress/0": "2",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalAndCharmStoreCharms(c *gc.C) {
	charmsPath := c.MkDir()
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	mysqlPath := testcharms.Repo.ClonedDirPath(charmsPath, "mysql")
	_, err := s.DeployBundleYAML(c, fmt.Sprintf(`
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
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"mysql":     {charm: "local:xenial/mysql-1"},
		"wordpress": {charm: "cs:xenial/wordpress-42"},
	})
	s.assertRelationsEstablished(c, "wordpress:db mysql:server")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":     "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationOptions(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	testcharms.UploadCharm(c, s.client, "precise/dummy-0", "dummy")
	_, err := s.DeployBundleYAML(c, `
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
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"customized": {
			charm:  "cs:precise/dummy-0",
			config: charm.Settings{"username": "who", "skill-level": int64(47)},
		},
		"wordpress": {
			charm:  "cs:xenial/wordpress-42",
			config: charm.Settings{"blog-title": "these are the voyages"},
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"wordpress/0":  "1",
		"customized/0": "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationConstrants(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	testcharms.UploadCharm(c, s.client, "precise/dummy-0", "dummy")
	_, err := s.DeployBundleYAML(c, `
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
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"customized": {
			charm:       "cs:precise/dummy-0",
			constraints: constraints.MustParse("arch=i386"),
		},
		"wordpress": {
			charm:       "cs:xenial/wordpress-42",
			constraints: constraints.MustParse("mem=4G cores=2"),
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"customized/0": "0",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationUpgrade(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	testcharms.UploadCharm(c, s.client, "vivid/upgrade-1", "upgrade1")
	testcharms.UploadCharm(c, s.client, "vivid/upgrade-2", "upgrade2")

	// First deploy the bundle.
	_, err := s.DeployBundleYAML(c, `
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
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:vivid/upgrade-1", "cs:xenial/wordpress-42")

	// Then deploy a new bundle with modified charm revision and options.
	_, err = s.DeployBundleYAML(c, `
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
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUploaded(c, "cs:vivid/upgrade-1", "cs:vivid/upgrade-2", "cs:xenial/wordpress-42")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"up": {charm: "cs:vivid/upgrade-2"},
		"wordpress": {
			charm:       "cs:xenial/wordpress-42",
			config:      charm.Settings{"blog-title": "new title"},
			constraints: constraints.MustParse("spaces=new cores=8"),
		},
	})
	s.assertUnitsCreated(c, map[string]string{
		"up/0":        "0",
		"wordpress/0": "1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleExpose(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-42", "wordpress")
	content := `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                expose: true
    `
	expectedApplications := map[string]serviceInfo{
		"wordpress": {
			charm:   "cs:xenial/wordpress-42",
			exposed: true,
		},
	}

	// First deploy the bundle.
	_, err := s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, expectedApplications)

	// Then deploy the same bundle again: no error is produced when the application
	// is exposed again.
	_, err = s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, expectedApplications)

	// Then deploy a bundle with the application unexposed, and check that the
	// application is not unexposed.
	_, err = s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: wordpress
                num_units: 1
                expose: false
    `)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, expectedApplications)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleApplicationUpgradeFailure(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Try upgrading to a different charm name.
	testcharms.UploadCharm(c, s.client, "xenial/incompatible-42", "wordpress")
	_, err := s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: xenial/incompatible-42
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade application "wordpress": bundle charm "cs:xenial/incompatible-42" is incompatible with existing charm "local:quantal/wordpress-3"`)

	// Try upgrading to a different series.
	// Note that this test comes before the next one because
	// otherwise we can't resolve the charm URL because the charm's
	// "base entity" is not marked as promulgated so the query by
	// promulgated will find it.
	testcharms.UploadCharm(c, s.client, "vivid/wordpress-42", "wordpress")
	_, err = s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: vivid/wordpress
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade application "wordpress": bundle charm "cs:vivid/wordpress-42" is incompatible with existing charm "local:quantal/wordpress-3"`)

	// Try upgrading to a different user.
	testcharms.UploadCharm(c, s.client, "~who/xenial/wordpress-42", "wordpress")
	_, err = s.DeployBundleYAML(c, `
        applications:
            wordpress:
                charm: cs:~who/xenial/wordpress-42
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot upgrade application "wordpress": bundle charm "cs:~who/xenial/wordpress-42" is incompatible with existing charm "local:quantal/wordpress-3"`)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMultipleRelations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")
	testcharms.UploadCharm(c, s.client, "xenial/mysql-1", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/postgres-2", "mysql")
	testcharms.UploadCharm(c, s.client, "xenial/varnish-3", "varnish")
	_, err := s.DeployBundleYAML(c, `
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
	_, err := s.DeployBundleYAML(c, `
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
	_, err = s.DeployBundleYAML(c, `
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
	s.assertRelationsEstablished(c, "wp:db mysql:server", "wp:cache varnish:webcache")
	s.assertUnitsCreated(c, map[string]string{
		"mysql/0":   "0",
		"varnish/0": "1",
		"wp/0":      "2",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMachinesUnitsPlacement(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/wordpress-0", "wordpress")
	testcharms.UploadCharm(c, s.client, "xenial/mysql-2", "mysql")

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
	_, err := s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"sql": {charm: "cs:xenial/mysql-2"},
		"wp": {
			charm:  "cs:xenial/wordpress-0",
			config: charm.Settings{"blog-title": "these are the voyages"},
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
	_, err = s.DeployBundleYAML(c, content)
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
	output, err := s.DeployBundleYAML(c, content)
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
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	_, err := s.DeployBundleYAML(c, `
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
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"django": {charm: "cs:xenial/django-42"},
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
	ann, err := s.State.Annotations(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleTwiceScaleUp(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	_, err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:xenial/django-42
                num_units: 2
    `)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.DeployBundleYAML(c, `
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
	_, err := s.DeployBundleYAML(c, `
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

func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitColocationWithUnit(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/mem-47", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/rails-0", "dummy")
	_, err := s.DeployBundleYAML(c, `
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
		"django/1":    "0/kvm/0",
		"django/2":    "1/lxd/0",
		"django/3":    "2/lxd/0",
		"django/4":    "3/kvm/0",
		"memcached/0": "0",
		"memcached/1": "1",
		"memcached/2": "2",
		"ror/0":       "0",
		"ror/1":       "3",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleUnitPlacedToMachines(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	_, err := s.DeployBundleYAML(c, `
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
		"django/0": "0",       // Machine "4" in the bundle.
		"django/1": "2",       // Machine "new" in the bundle.
		"django/2": "1/kvm/0", // The KVM container in bundle machine "8".
		"django/3": "0/lxd/0", // First lxd container in bundle machine "4".
		"django/4": "0/lxd/1", // Second lxd container in bundle machine "4".
		"django/5": "3/lxd/0", // First lxd in new machine.
		"django/6": "4/lxd/0", // Second lxd in new machine.
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleMassiveUnitColocation(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/mem-47", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/rails-0", "dummy")
	_, err := s.DeployBundleYAML(c, `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 3
                to: [1, 2, 3]
            django:
                charm: cs:xenial/django-42
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
		"ror/2":       "2/kvm/1",
	})

	// Redeploy a very similar bundle with another application unit. The new unit
	// is placed on machine 1 because that's the least crowded machine.
	content := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 3
                to: [1, 2, 3]
            django:
                charm: cs:xenial/django-42
                num_units: 4
                to:
                    - 1
                    - lxd:memcached
            node:
                charm: cs:xenial/django-42
                num_units: 1
                to:
                    - lxd:memcached
        machines:
            1:
            2:
            3:
    `
	_, err = s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)

	// Redeploy the same bundle again and check that nothing happens.
	_, err = s.DeployBundleYAML(c, content)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitsCreated(c, map[string]string{
		"django/0":    "0",
		"django/1":    "0/lxd/0",
		"django/2":    "1/lxd/0",
		"django/3":    "2/lxd/0",
		"memcached/0": "0",
		"memcached/1": "1",
		"memcached/2": "2",
		"node/0":      "1/lxd/1",
		"ror/0":       "0",
		"ror/1":       "2/kvm/0",
		"ror/2":       "2/kvm/1",
	})
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleWithAnnotations_OutputIsCorrect(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/mem-47", "dummy")
	output, err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:django
                num_units: 1
                annotations:
                    key1: value1
                    key2: value2
                to: [1]
            memcached:
                charm: xenial/mem-47
                num_units: 1
        machines:
            1:
                annotations: {foo: bar}
    `)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(output, gc.Equals, ""+
		`Deploying charm "cs:xenial/django-42"`+"\n"+
		`Deploying charm "cs:xenial/mem-47"`+"\n"+
		`Deploy of bundle completed.`,
	)
}

func (s *BundleDeployCharmStoreSuite) TestDeployBundleAnnotations(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "xenial/django-42", "dummy")
	testcharms.UploadCharm(c, s.client, "xenial/mem-47", "dummy")
	_, err := s.DeployBundleYAML(c, `
        applications:
            django:
                charm: cs:django
                num_units: 1
                annotations:
                    key1: value1
                    key2: value2
                to: [1]
            memcached:
                charm: xenial/mem-47
                num_units: 1
        machines:
            1:
                annotations: {foo: bar}
    `)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := s.State.Application("django")
	c.Assert(err, jc.ErrorIsNil)
	ann, err := s.State.Annotations(svc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{
		"key1": "value1",
		"key2": "value2",
	})
	m, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	ann, err = s.State.Annotations(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{"foo": "bar"})

	// Update the annotations and deploy the bundle again.
	_, err = s.DeployBundleYAML(c, `
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
	ann, err = s.State.Annotations(svc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{
		"key1": "new value!",
		"key2": "value2",
	})
	ann, err = s.State.Annotations(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ann, jc.DeepEquals, map[string]string{
		"foo":    "bar",
		"answer": "42",
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
