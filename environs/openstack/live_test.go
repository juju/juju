package openstack_test

import (
	"crypto/rand"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	coretesting "launchpad.net/juju-core/testing"
	"os"
)

// uniqueName is generated afresh for every test run, so that
// we are not polluted by previous test state.
var uniqueName = randomName()

func randomName() string {
	buf := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

func registerOpenStackTests() {
	// The following attributes hold the environment configuration
	// for running the OpenStack integration tests.
	//
	// This is missing keys for security reasons; set the following
	// environment variables to make the OpenStack testing work:
	//  access-key: $OS_USERNAME
	//  secret-key: $OS_PASSWORD
	//
	// There is no standard public bucket for storing the tools as yet.
	// Individuals can create their own public bucket for testing, which is
	// specified using the following environment variable: $OS_PUBLIC_BUCKET_URL
	var publicBucketURL = os.Getenv("OS_PUBLIC_BUCKET_URL")
	attrs := map[string]interface{}{
		"name":              "sample-" + uniqueName,
		"type":              "openstack",
		"auth-method":       "userpass",
		"control-bucket":    "juju-test-" + uniqueName,
		"public-bucket-url": publicBucketURL,
	}
	Suite(&LiveTests{
		LiveTests: jujutest.LiveTests{
			Config: attrs,
		},
	})
}

// LiveTests contains tests that can be run against OpenStack deployments.
// Each test runs using the same connection.
type LiveTests struct {
	coretesting.LoggingSuite
	jujutest.LiveTests
	novaClient *nova.Client
}

func (t *LiveTests) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	_, err := environs.NewFromAttrs(t.Config)
	c.Assert(err, IsNil)

	// Get a nova client and start some test service instances.
	cred, err := identity.CompleteCredentialsFromEnv()
	c.Assert(err, IsNil)
	client := client.NewClient(cred, identity.AuthUserPass, nil)
	t.novaClient = nova.New(client)

	// TODO: Put some fake tools in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running will find them in the public bucket.
	//	putFakeTools(c, e.PublicStorage().(environs.Storage))
	t.LiveTests.SetUpSuite(c)
}

func (t *LiveTests) TearDownSuite(c *C) {
	if t.Env == nil {
		// This can happen if SetUpSuite fails.
		return
	}
	// TODO: delete any content put into swift
	t.LiveTests.TearDownSuite(c)
	t.LoggingSuite.TearDownSuite(c)
}

func (t *LiveTests) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
}

func (t *LiveTests) TearDownTest(c *C) {
	t.LiveTests.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}
