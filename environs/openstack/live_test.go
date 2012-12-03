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
	// environment variables to make the Amazon testing work:
	//  access-key: $OS_USERNAME
	//  secret-key: $OS_PASSWORD
	attrs := map[string]interface{}{
		"name":         "sample-" + uniqueName,
		"type":         "openstack",
		"admin-secret": "for real",
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
	novaClient  *nova.Client
	testServers []nova.Entity
}

func (t *LiveTests) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	_, err := environs.NewFromAttrs(t.Config)
	c.Assert(err, IsNil)
	// Get a nova client and start some test service instances.
	cred, err := identity.CompleteCredentialsFromEnv()
	c.Assert(err, IsNil)
	client := client.NewClient(cred, identity.AuthUserPass)
	t.novaClient = nova.New(client)
	t.testServers, err = t.createInstances(2)
	c.Assert(err, IsNil)
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
	// Delete any test servers started during suite setup.
	for _, inst := range t.testServers {
		err := t.novaClient.DeleteServer(inst.Id)
		c.Check(err, IsNil)
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

// The OpenStack provider is being developed a few methods at a time. The juju tests exercise the whole stack and so
// currently fail because not everything is implemented yet. So below we add some tests for those methods which have
// so far been completed.

// createInstances runs some test servers using a known pre-existing image.
func (t *LiveTests) createInstances(numInstances int) (instances []nova.Entity, err error) {
	for n := 1; n <= numInstances; n++ {
		opts := nova.RunServerOpts{
			Name:     fmt.Sprintf("test_server%d", n),
			FlavorId: "1", // m1.tiny
			ImageId:  "0f602ea9-c09e-440c-9e29-cfae5635afa3",
			UserData: nil,
		}
		entity, err := t.novaClient.RunServer(opts)
		if err != nil {
			return nil, err
		}
		instances = append(t.testServers, *entity)
	}
	return instances, nil
}

func (t *LiveTests) TestAllInstances(c *C) {
	observedInst, err := t.Env.AllInstances()
	c.Assert(err, IsNil)
	idSet := make(map[string]bool)
	for _, inst := range observedInst {
		idSet[inst.Id()] = true
	}
	for _, inst := range t.testServers {
		_, ok := idSet[inst.Id]
		if !ok {
			c.Logf("Server id '%s' was not listed in AllInstances %v", inst.Id, observedInst)
			c.Fail()
		}
	}
}

func (t *LiveTests) TestInstances(c *C) {
	observedInst, err := t.Env.Instances([]string{t.testServers[0].Id})
	c.Assert(err, IsNil)
	c.Assert(len(observedInst), Equals, 1)
	c.Assert(observedInst[0].Id(), Equals, t.testServers[0].Id)
}
