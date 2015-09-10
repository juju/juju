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
adding charm cs:trusty/mysql-42
adding charm cs:trusty/wordpress-47
deployment of bundle "cs:bundle/wordpress-simple-1" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:trusty/mysql-42", "cs:trusty/wordpress-47")
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
adding charm cs:trusty/mysql-42
adding charm cs:trusty/wordpress-47
deployment of bundle "cs:bundle/wordpress-simple-1" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "cs:trusty/mysql-42", "cs:trusty/wordpress-47")
}

func (s *DeployCharmStoreSuite) TestDeployBundleGatedCharm(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/mysql-42", "mysql")
	url, _ := testcharms.UploadCharm(c, s.client, "trusty/wordpress-47", "wordpress")
	s.changeReadPerm(c, url, clientUserName)
	testcharms.UploadBundle(c, s.client, "bundle/wordpress-simple-1", "wordpress-simple")
	_, err := runDeployCommand(c, "bundle/wordpress-simple")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharmsUplodaded(c, "cs:trusty/mysql-42", "cs:trusty/wordpress-47")
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
adding charm local:trusty/mysql-1
adding charm local:trusty/wordpress-3
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "local:trusty/mysql-1", "local:trusty/wordpress-3")
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleCharmNotFound(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "wordpress")
	_, err := s.deployBundleYAML(c, `
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
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: cannot resolve URL "local:mysql": entity not found .*`)
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
adding charm local:trusty/mysql-1
adding charm cs:trusty/wordpress-42
deployment of bundle "local:bundle/example-0" completed`
	c.Assert(output, gc.Equals, strings.TrimSpace(expectedOutput))
	s.assertCharmsUplodaded(c, "local:trusty/mysql-1", "cs:trusty/wordpress-42")
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleInvalidContent(c *gc.C) {
	_, err := s.deployBundleYAML(c, "!")
	c.Assert(err, gc.ErrorMatches, "cannot unmarshal bundle data: YAML error: .*")
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleInvalidData(c *gc.C) {
	_, err := s.deployBundleYAML(c, `
        services:
            mysql:
                charm: mysql
                num_units: -1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: negative number of units specified on service "mysql"`)
}

func (s *deployRepoCharmStoreSuite) TestDeployBundleInception(c *gc.C) {
	_, err := s.deployBundleYAML(c, `
        services:
            example:
                charm: local:bundle/example
                num_units: 1
    `)
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: expected charm URL, got bundle URL "local:bundle/example"`)
}
