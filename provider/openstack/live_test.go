// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"crypto/rand"
	"fmt"
	"io"
	gc "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/identity"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/provider/openstack"
	coretesting "launchpad.net/juju-core/testing"
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
	attrs := envtesting.FakeConfig.Merge(coretesting.Attrs{
		"name":            "sample-" + randomName(),
		"type":            "openstack",
		"auth-mode":       "userpass",
		"control-bucket":  "juju-test-" + randomName(),
		"username":        cred.User,
		"password":        cred.Secrets,
		"region":          cred.Region,
		"auth-url":        cred.URL,
		"tenant-name":     cred.TenantName,
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
	coretesting.LoggingSuite
	jujutest.LiveTests
	cred                   *identity.Credentials
	writeablePublicStorage environs.Storage
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	t.LoggingSuite.SetUpSuite(c)
	// Update some Config items now that we have services running.
	// This is setting the public-bucket-url and auth-url because that
	// information is set during startup of the localLiveSuite
	cl := client.NewClient(t.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, gc.IsNil)
	publicBucketURL, err := cl.MakeServiceURL("object-store", nil)
	c.Assert(err, gc.IsNil)
	t.TestConfig = t.TestConfig.Merge(coretesting.Attrs{
		"public-bucket-url": publicBucketURL,
		"auth-url":          t.cred.URL,
	})
	t.LiveTests.SetUpSuite(c)
	openstack.SetFakeToolsStorage(true)
	// Environ.PublicStorage() is read only.
	// For testing, we create a specific storage instance which is authorised to write to
	// the public storage bucket so that we can upload files for testing.
	t.writeablePublicStorage = openstack.WritablePublicStorage(t.Env)
	// Put some fake tools in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running will find them in the public bucket.
	envtesting.UploadFakeTools(c, t.writeablePublicStorage)
}

func (t *LiveTests) TearDownSuite(c *gc.C) {
	if t.Env == nil {
		// This can happen if SetUpSuite fails.
		return
	}
	if t.writeablePublicStorage != nil {
		envtesting.RemoveFakeTools(c, t.writeablePublicStorage)
	}
	openstack.SetFakeToolsStorage(false)
	t.LiveTests.TearDownSuite(c)
	t.LoggingSuite.TearDownSuite(c)
}

func (t *LiveTests) SetUpTest(c *gc.C) {
	t.LoggingSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.LiveTests.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}
