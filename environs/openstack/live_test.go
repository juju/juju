package openstack_test

import (
	"crypto/rand"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/client"
	"launchpad.net/goose/identity"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/openstack"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"strings"
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

func makeTestConfig() map[string]interface{} {
	// The following attributes hold the environment configuration
	// for running the OpenStack integration tests.
	//
	// This is missing keys for security reasons; set the following
	// environment variables to make the OpenStack testing work:
	//  access-key: $OS_USERNAME
	//  secret-key: $OS_PASSWORD
	//
	attrs := map[string]interface{}{
		"name":           "sample-" + uniqueName,
		"type":           "openstack",
		"auth-mode":      "userpass",
		"control-bucket": "juju-test-" + uniqueName,
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
	return attrs
}

// Register tests to run against a real Openstack instance.
func registerOpenStackTests(cred *identity.Credentials) {
	Suite(&LiveTests{
		cred: cred,
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

const (
	// TODO (wallyworld) - ideally, something like http://cloud-images.ubuntu.com would have images we could use
	// but until it does, we allow a default image id to be specified.
	// This is an existing image on Canonistack - smoser-cloud-images/ubuntu-quantal-12.10-i386-server-20121017
	testImageId = "0f602ea9-c09e-440c-9e29-cfae5635afa3"
)

func (t *LiveTests) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	// Get an authenticated Goose client to extract some configuration parameters for the test environment.
	client := client.NewClient(t.cred, identity.AuthUserPass, nil)
	err := client.Authenticate()
	c.Assert(err, IsNil)
	publicBucketURL, err := client.MakeServiceURL("object-store", nil)
	c.Assert(err, IsNil)
	attrs := makeTestConfig()
	attrs["admin-secret"] = "secret"
	attrs["username"] = t.cred.User
	attrs["password"] = t.cred.Secrets
	attrs["region"] = t.cred.Region
	attrs["auth-url"] = t.cred.URL
	attrs["tenant-name"] = t.cred.TenantName
	attrs["public-bucket-url"] = publicBucketURL
	attrs["default-image-id"] = testImageId
	t.Config = attrs
	t.LiveTests = jujutest.LiveTests{
		Config:         attrs,
		Attempt:        *openstack.ShortAttempt,
		CanOpenState:   false, // no state; local tests (unless -live is passed)
		HasProvisioner: false, // don't deploy anything
	}
	e, err := environs.NewFromAttrs(t.Config)
	c.Assert(err, IsNil)

	// Environ.PublicStorage() is read only.
	// For testing, we create a specific storage instance which is authorised to write to
	// the public storage bucket so that we can upload files for testing.
	t.writeablePublicStorage = openstack.WritablePublicStorage(e)
	// Put some fake tools in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running will find them in the public bucket.
	putFakeTools(c, t.writeablePublicStorage)
	t.LiveTests.SetUpSuite(c)
}

func (t *LiveTests) TearDownSuite(c *C) {
	if t.Env == nil {
		// This can happen if SetUpSuite fails.
		return
	}
	if t.writeablePublicStorage != nil {
		err := openstack.DeleteStorageContent(t.writeablePublicStorage)
		c.Check(err, IsNil)
	}
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

// putFakeTools sets up a bucket containing something
// that looks like a tools archive so test methods
// that start an instance can succeed even though they
// do not upload tools.
func putFakeTools(c *C, s environs.StorageWriter) {
	path := environs.ToolsStoragePath(version.Current)
	c.Logf("putting fake tools at %v", path)
	toolsContents := "tools archive, honest guv"
	err := s.Put(path, strings.NewReader(toolsContents), int64(len(toolsContents)))
	c.Assert(err, IsNil)
}

func (t *LiveTests) TestFindImageSpec(c *C) {
	imageId, flavorId, err := openstack.FindInstanceSpec(t.Env, "precise", "amd64", "m1.small")
	c.Assert(err, IsNil)
	// For now, the imageId always comes from the environment config.
	c.Assert(imageId, Equals, testImageId)
	c.Assert(flavorId, Not(Equals), "")
}

func (t *LiveTests) TestFindImageBadFlavor(c *C) {
	imageId, flavorId, err := openstack.FindInstanceSpec(t.Env, "precise", "amd64", "bad.flavor")
	_, ok := err.(environs.NotFoundError)
	c.Assert(ok, Equals, true)
	c.Assert(imageId, Equals, "")
	c.Assert(flavorId, Equals, "")
}

// The following tests need to be enabled once the coding is complete.

func (s *LiveTests) TestGlobalPorts(c *C) {
	c.Skip("Work in progress")
}

func (s *LiveTests) TestPorts(c *C) {
	c.Skip("Work in progress")
}

func (s *LiveTests) TestStartStop(c *C) {
	c.Skip("Work in progress")
}
