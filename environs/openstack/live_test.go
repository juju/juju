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
	"launchpad.net/juju-core/environs/openstack"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"strings"
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
		"name":           "sample-" + randomName(),
		"type":           "openstack",
		"auth-mode":      "userpass",
		"control-bucket": "juju-test-" + randomName(),
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
	return attrs
}

// Register tests to run against a real Openstack instance.
func registerLiveTests(cred *identity.Credentials) {
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
	cl := client.NewClient(t.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, IsNil)
	publicBucketURL, err := cl.MakeServiceURL("object-store", nil)
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
	t.LiveTests.SetUpSuite(c)
	// Environ.PublicStorage() is read only.
	// For testing, we create a specific storage instance which is authorised to write to
	// the public storage bucket so that we can upload files for testing.
	t.writeablePublicStorage = openstack.WritablePublicStorage(t.Env)
	// Put some fake tools in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running will find them in the public bucket.
	putFakeTools(c, t.writeablePublicStorage)
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

// ported from lp:juju/juju/providers/openstack/tests/test_machine.py
var addressTests = []struct {
	summary  string
	private  []nova.IPAddress
	public   []nova.IPAddress
	networks []string
	expected string
	failure  error
}{
	{
		summary:  "missing",
		expected: "",
		failure:  environs.ErrNoDNSName,
	},
	{
		summary:  "empty",
		private:  []nova.IPAddress{},
		networks: []string{"private"},
		expected: "",
		failure:  environs.ErrNoDNSName,
	},
	{
		summary:  "private only",
		private:  []nova.IPAddress{{4, "127.0.0.4"}},
		networks: []string{"private"},
		expected: "127.0.0.4",
		failure:  nil,
	},
	{
		summary:  "private plus (HP cloud)",
		private:  []nova.IPAddress{{4, "127.0.0.4"}, {4, "8.8.4.4"}},
		networks: []string{"private"},
		expected: "8.8.4.4",
		failure:  nil,
	},
	{
		summary:  "public only",
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"", "public"},
		expected: "8.8.8.8",
		failure:  nil,
	},
	{
		summary:  "public and private",
		private:  []nova.IPAddress{{4, "127.0.0.4"}},
		public:   []nova.IPAddress{{4, "8.8.4.4"}},
		networks: []string{"private", "public"},
		expected: "8.8.4.4",
		failure:  nil,
	},
	{
		summary:  "public private plus",
		private:  []nova.IPAddress{{4, "127.0.0.4"}, {4, "8.8.4.4"}},
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"private", "public"},
		expected: "8.8.8.8",
		failure:  nil,
	},
	{
		summary:  "custom only",
		private:  []nova.IPAddress{{4, "127.0.0.2"}},
		networks: []string{"special"},
		expected: "127.0.0.2",
		failure:  nil,
	},
	{
		summary:  "custom and public",
		private:  []nova.IPAddress{{4, "127.0.0.2"}},
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"special", "public"},
		expected: "8.8.8.8",
		failure:  nil,
	},
	{
		summary:  "non-IPv4",
		private:  []nova.IPAddress{{6, "::dead:beef:f00d"}},
		networks: []string{"private"},
		expected: "",
		failure:  environs.ErrNoDNSName,
	},
}

func (s *LiveTests) TestGetServerAddresses(c *C) {
	for i, t := range addressTests {
		c.Logf("#%d. %s -> %s (%v)", i, t.summary, t.expected, t.failure)
		addresses := make(map[string][]nova.IPAddress)
		if t.private != nil {
			if len(t.networks) < 1 {
				addresses["private"] = t.private
			} else {
				addresses[t.networks[0]] = t.private
			}
		}
		if t.public != nil {
			if len(t.networks) < 2 {
				addresses["public"] = t.public
			} else {
				addresses[t.networks[1]] = t.public
			}
		}
		addr, err := openstack.InstanceAddress(addresses)
		c.Assert(err, Equals, t.failure)
		c.Assert(addr, Equals, t.expected)
	}
}
