package openstack_test

import (
	"flag"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/openstack"
	"reflect"
	"testing"
)

// Out-of-the-box, we support live testing using Canonistack or HP Cloud.
var testConstraints = map[string]openstack.ImageDetails{
	"canonistack": {
		Flavor: "m1.tiny", ImageId: "c876e5fe-abb0-41f0-8f29-f0b47481f523"},
	"hpcloud": {
		Flavor: "standard.xsmall", ImageId: "75845"},
}

var live = flag.Bool("live", false, "Include live OpenStack tests")
var vendor = flag.String("vendor", "", "The Openstack vendor to test against")
var imageId = flag.String("image", "", "The image id for which a test service is to be started")
var flavor = flag.String("flavor", "", "The flavor of the test service")

func Test(t *testing.T) {
	if *live {
		// We can either specify a vendor, or imageId and flavor separately.
		var testImageDetails openstack.ImageDetails
		if *vendor != "" {
			var ok bool
			if testImageDetails, ok = testConstraints[*vendor]; !ok {
				keys := reflect.ValueOf(testConstraints).MapKeys()
				t.Fatalf("Unknown vendor %s. Must be one of %s", *vendor, keys)
			}
		} else {
			if *imageId == "" {
				t.Fatalf("Must specify image id to use for test instance, "+
					"eg %s for Canonistack", "-image c876e5fe-abb0-41f0-8f29-f0b47481f523")
			}
			if *flavor == "" {
				t.Fatalf("Must specify flavor to use for test instance, "+
					"eg %s for Canonistack", "-flavor m1.tiny")
			}
			testImageDetails = openstack.ImageDetails{*flavor, *imageId}
		}
		cred, err := identity.CompleteCredentialsFromEnv()
		if err != nil {
			t.Fatalf("Error setting up test suite: %s", err.Error())
		}
		registerLiveTests(cred, testImageDetails)
	}
	registerLocalTests()
	TestingT(t)
}

// localTests contains tests which do not require a live service or test double to run.
type localTests struct{}

var _ = Suite(&localTests{})

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

func (t *localTests) TestGetServerAddresses(c *C) {
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
