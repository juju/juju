// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/go-goose/goose/v5/client"
	"github.com/go-goose/goose/v5/identity"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/openstack"
	coretesting "github.com/juju/juju/testing"
)

// generate a different bucket name for each config instance, so that
// we are not polluted by previous test state.
func randomName() string {
	buf := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

func makeTestConfig(cred *identity.Credentials) map[string]interface{} {
	// The following attributes hold the environment configuration
	// for running the OpenStack integration tests.
	//
	// This is missing keys for security reasons; set the following
	// environment variables to make the OpenStack testing work:
	//  access-key: $OS_USERNAME
	//  secret-key: $OS_PASSWORD
	//
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":        "sample-" + randomName(),
		"type":        "openstack",
		"auth-mode":   "userpass",
		"username":    cred.User,
		"password":    cred.Secrets,
		"region":      cred.Region,
		"auth-url":    cred.URL,
		"tenant-name": cred.TenantName,
	})
	return attrs
}

// Register tests to run against a real Openstack instance.
func registerLiveTests(cred *identity.Credentials) {
	config := makeTestConfig(cred)
	gc.Suite(&LiveTests{
		cred: cred,
		LiveTests: jujutest.LiveTests{
			TestConfig:     config,
			Attempt:        *openstack.ShortAttempt,
			CanOpenState:   true,
			HasProvisioner: true,
		},
	})
}

// LiveTests contains tests that can be run against OpenStack deployments.
// The deployment can be a real live instance or service doubles.
// Each test runs using the same connection.
type LiveTests struct {
	coretesting.BaseSuite
	jujutest.LiveTests
	cred            *identity.Credentials
	metadataStorage storage.Storage
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	t.BaseSuite.SetUpSuite(c)
	// Update some Config items now that we have services running.
	// This is setting the simplestreams urls and auth-url because that
	// information is set during startup of the localLiveSuite
	cl := client.NewClient(t.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, jc.ErrorIsNil)
	containerURL, err := cl.MakeServiceURL("object-store", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	t.TestConfig = t.TestConfig.Merge(coretesting.Attrs{
		"agent-metadata-url": containerURL + "/juju-dist-test/tools",
		"image-metadata-url": containerURL + "/juju-dist-test",
		"auth-url":           t.cred.URL,
	})
	t.LiveTests.SetUpSuite(c)
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	t.PrepareOnce(c)
	t.metadataStorage = openstack.MetadataStorage(t.Env)
	// Put some fake tools metadata in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running can find the metadata.
	envtesting.UploadFakeTools(c, t.metadataStorage, t.Env.Config().AgentStream(), t.Env.Config().AgentStream())
}

func (t *LiveTests) TearDownSuite(c *gc.C) {
	if t.Env == nil {
		// This can happen if SetUpSuite fails.
		return
	}
	if t.metadataStorage != nil {
		envtesting.RemoveFakeToolsMetadata(c, t.metadataStorage)
	}
	t.LiveTests.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *LiveTests) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.LiveTests.TearDownTest(c)
	t.BaseSuite.TearDownTest(c)
}

func (s *LiveTests) assertStartInstanceDefaultSecurityGroup(c *gc.C, useDefault bool) {
	s.LiveTests.PatchValue(&s.TestConfig, s.TestConfig.Merge(coretesting.Attrs{
		"use-default-secgroup": useDefault,
	}))
	s.Destroy(c)
	s.BootstrapOnce(c)

	inst, _ := jujutesting.AssertStartInstance(c, s.Env, context.NewEmptyCloudCallContext(), s.ControllerUUID, "100")
	// Check whether the instance has the default security group assigned.
	novaClient := openstack.GetNovaClient(s.Env)
	groups, err := novaClient.GetServerSecurityGroups(string(inst.Id()))
	c.Assert(err, jc.ErrorIsNil)
	defaultGroupFound := false
	for _, group := range groups {
		if group.Name == "default" {
			defaultGroupFound = true
			break
		}
	}
	c.Assert(defaultGroupFound, gc.Equals, useDefault)
}

func (s *LiveTests) TestStartInstanceWithDefaultSecurityGroup(c *gc.C) {
	s.assertStartInstanceDefaultSecurityGroup(c, true)
}

func (s *LiveTests) TestStartInstanceWithoutDefaultSecurityGroup(c *gc.C) {
	s.assertStartInstanceDefaultSecurityGroup(c, false)
}
