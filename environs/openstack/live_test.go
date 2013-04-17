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
	envtesting "launchpad.net/juju-core/environs/testing"
	coretesting "launchpad.net/juju-core/testing"
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

func makeTestConfig(cred *identity.Credentials) map[string]interface{} {
	// The following attributes hold the environment configuration
	// for running the OpenStack integration tests.
	//
	// This is missing keys for security reasons; set the following
	// environment variables to make the OpenStack testing work:
	//  access-key: $OS_USERNAME
	//  secret-key: $OS_PASSWORD
	//
	attrs := map[string]interface{}{
		"name":            "sample-" + randomName(),
		"type":            "openstack",
		"auth-mode":       "userpass",
		"control-bucket":  "juju-test-" + randomName(),
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
		"authorized-keys": "fakekey",
		"admin-secret":    "secret",
		"username":        cred.User,
		"password":        cred.Secrets,
		"region":          cred.Region,
		"auth-url":        cred.URL,
		"tenant-name":     cred.TenantName,
	}
	return attrs
}

// Register tests to run against a real Openstack instance.
func registerLiveTests(cred *identity.Credentials, testImageDetails openstack.ImageDetails) {
	config := makeTestConfig(cred)
	config["default-image-id"] = testImageDetails.ImageId
	config["default-instance-type"] = testImageDetails.Flavor
	Suite(&LiveTests{
		cred: cred,
		LiveTests: jujutest.LiveTests{
			TestConfig: jujutest.TestConfig{config},
			Attempt:    *openstack.ShortAttempt,
			// TODO: Bug #1133263, once the infrastructure is set up,
			//       enable The state tests on openstack
			CanOpenState: false,
			// TODO: Bug #1133272, enabling this requires mapping from
			//       'series' to an image id, when we have support, set
			//       this flag to True.
			HasProvisioner: false,
		},
		testImageId: testImageDetails.ImageId,
		testFlavor:  testImageDetails.Flavor,
	})
}

// LiveTests contains tests that can be run against OpenStack deployments.
// The deployment can be a real live instance or service doubles.
// Each test runs using the same connection.
type LiveTests struct {
	coretesting.LoggingSuite
	jujutest.LiveTests
	cred                   *identity.Credentials
	testImageId            string
	testFlavor             string
	writeablePublicStorage environs.Storage
}

var privateBucketImagesData = map[string]string {
	"precise": imagesFields(
		"inst1 amd64 region-1 id-a paravirtual",
		"inst2 amd64 region-2 id-b paravirtual",
	),
	"quantal": imagesFields(
		"inst3 amd64 some-region id-1 paravirtual",
		"inst4 amd64 another-region id-2 paravirtual",
	),
}

var publicBucketImagesData = map[string]string {
	"raring": imagesFields(
		"inst5 amd64 some-region id-y paravirtual",
		"inst6 amd64 another-region id-z paravirtual",
	),
}

func imagesFields(srcs ...string) string {
	strs := make([]string, len(srcs))
	for i, src := range srcs {
		parts := strings.Split(src, " ")
		if len(parts) != 5 {
			panic("bad clouddata field input")
		}
		args := make([]interface{}, len(parts))
		for i, part := range parts {
			args[i] = part
		}
		// Ignored fields are left empty for clarity's sake, and two additional
		// tabs are tacked on to the end to verify extra columns are ignored.
		strs[i] = fmt.Sprintf("\t\t\t\t%s\t%s\t%s\t%s\t\t\t%s\t\t\n", args...)
	}
	return strings.Join(strs, "")
}

func (t *LiveTests) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	// Update some Config items now that we have services running.
	// This is setting the public-bucket-url and auth-url because that
	// information is set during startup of the localLiveSuite
	cl := client.NewClient(t.cred, identity.AuthUserPass, nil)
	err := cl.Authenticate()
	c.Assert(err, IsNil)
	publicBucketURL, err := cl.MakeServiceURL("object-store", nil)
	c.Assert(err, IsNil)
	t.TestConfig.UpdateConfig(map[string]interface{}{
		"public-bucket-url": publicBucketURL,
		"auth-url":          t.cred.URL,
	})
	t.LiveTests.SetUpSuite(c)
	// Environ.PublicStorage() is read only.
	// For testing, we create a specific storage instance which is authorised to write to
	// the public storage bucket so that we can upload files for testing.
	t.writeablePublicStorage = openstack.WritablePublicStorage(t.Env)
	// Put some fake tools in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running will find them in the public bucket.
	envtesting.UploadFakeTools(c, t.writeablePublicStorage)
}

func (t *LiveTests) TearDownSuite(c *C) {
	if t.Env == nil {
		// This can happen if SetUpSuite fails.
		return
	}
	t.LiveTests.TearDownSuite(c)
	t.LoggingSuite.TearDownSuite(c)
}

func metadataFilePath(series string) string {
	return fmt.Sprintf("series-image-metadata/%s/server/released.current.txt", series)
}

func (t *LiveTests) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
	openstack.SetDefaultImageId(t.Env, t.testImageId)
	openstack.SetDefaultInstanceType(t.Env, t.testFlavor)
	// Put some image metadata files into the public and private storage.
	for series, imagesData := range privateBucketImagesData {
		t.Env.Storage().Put(metadataFilePath(series), strings.NewReader(imagesData), int64(len(imagesData)))
	}
	for series, imagesData := range publicBucketImagesData {
		t.writeablePublicStorage.Put(metadataFilePath(series), strings.NewReader(imagesData), int64(len(imagesData)))
	}
}

func (t *LiveTests) TearDownTest(c *C) {
	for series := range privateBucketImagesData {
		t.Env.Storage().Remove(metadataFilePath(series))
	}
	for series := range publicBucketImagesData {
		t.writeablePublicStorage.Remove(metadataFilePath(series))
	}
	t.LiveTests.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}

func (t *LiveTests) TestFindImageSpecPrivateStorage(c *C) {
	openstack.SetDefaultInstanceType(t.Env, "")
	openstack.SetDefaultImageId(t.Env, "")
	spec, err := openstack.FindInstanceSpec(t.Env, "quantal", "amd64", "mem=512M")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, "id-1")
	c.Assert(spec.InstanceTypeName, Equals, "m1.tiny")
}

func (t *LiveTests) TestFindImageSpecPublicStorage(c *C) {
	openstack.SetDefaultInstanceType(t.Env, "")
	openstack.SetDefaultImageId(t.Env, "")
	spec, err := openstack.FindInstanceSpec(t.Env, "raring", "amd64", "mem=512M")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, "id-y")
	c.Assert(spec.InstanceTypeName, Equals, "m1.tiny")
}

// If no suitable image is found, use the default if specified.
func (t *LiveTests) TestFindImageSpecDefaultImage(c *C) {
	spec, err := openstack.FindInstanceSpec(t.Env, "precise", "amd64", "")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, t.testImageId)
	c.Assert(spec.InstanceTypeName, Not(Equals), "")
}

// If no matching instance type is found, use the default flavor if specified.
func (t *LiveTests) TestFindImageSpecDefaultFlavor(c *C) {
	openstack.SetDefaultInstanceType(t.Env, "m1.small")
	spec, err := openstack.FindInstanceSpec(t.Env, "precise", "amd64", "mem=8G")
	c.Assert(err, IsNil)
	c.Assert(spec.Image.Id, Equals, t.testImageId)
	c.Assert(spec.InstanceTypeName, Equals, "m1.small")
}

// An error occurs if no matching instance type is found and the default flavor is invalid.
func (t *LiveTests) TestFindImageBadDefaultFlavor(c *C) {
	openstack.SetDefaultInstanceType(t.Env, "bad.flavor")
	_, err := openstack.FindInstanceSpec(t.Env, "precise", "amd64", "mem=8G")
	c.Assert(err, ErrorMatches, `no instance types in some-region matching constraints "cpu-power=100 mem=8192M"`)
}

// An error occurs if no suitable image is found and the default not specified.
func (t *LiveTests) TestFindImageBadDefaultImage(c *C) {
	openstack.SetDefaultImageId(t.Env, "")
	_, err := openstack.FindInstanceSpec(t.Env, "precise", "amd64", "mem=8G")
	c.Assert(err, ErrorMatches, `unable to find image for series/arch/region precise/amd64/some-region and no default specified.`)
}
