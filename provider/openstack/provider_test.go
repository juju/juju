// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/goose.v2/neutron"
	"gopkg.in/goose.v2/nova"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
)

// localTests contains tests which do not require a live service or test double to run.
type localTests struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&localTests{})

// ported from lp:juju/juju/providers/openstack/tests/test_machine.py
var addressTests = []struct {
	summary    string
	floatingIP string
	private    []nova.IPAddress
	public     []nova.IPAddress
	networks   []string
	expected   string
	failure    error
}{{
	summary:  "missing",
	expected: "",
}, {
	summary:  "empty",
	private:  []nova.IPAddress{},
	networks: []string{"private"},
	expected: "",
}, {
	summary:  "private IPv4 only",
	private:  []nova.IPAddress{{4, "192.168.0.1", "fixed"}},
	networks: []string{"private"},
	expected: "192.168.0.1",
}, {
	summary:  "private IPv6 only",
	private:  []nova.IPAddress{{6, "fc00::1", "fixed"}},
	networks: []string{"private"},
	expected: "fc00::1",
}, {
	summary:  "private only, both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "192.168.0.1", "fixed"}, {6, "fc00::1", "fixed"}},
	networks: []string{"private"},
	expected: "192.168.0.1",
}, {
	summary:  "private IPv4 plus (what HP cloud used to do)",
	private:  []nova.IPAddress{{4, "10.0.0.1", "fixed"}, {4, "8.8.4.4", "fixed"}},
	networks: []string{"private"},
	expected: "8.8.4.4",
}, {
	summary:  "public IPv4 only",
	public:   []nova.IPAddress{{4, "8.8.8.8", "floating"}},
	networks: []string{"", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "public IPv6 only",
	public:   []nova.IPAddress{{6, "2001:db8::1", "floating"}},
	networks: []string{"", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "public only, both IPv4 and IPv6",
	public:   []nova.IPAddress{{4, "8.8.8.8", "floating"}, {6, "2001:db8::1", "floating"}},
	networks: []string{"", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "public and private both IPv4",
	private:  []nova.IPAddress{{4, "10.0.0.4", "fixed"}},
	public:   []nova.IPAddress{{4, "8.8.4.4", "floating"}},
	networks: []string{"private", "public"},
	expected: "8.8.4.4",
}, {
	summary:  "public and private both IPv6",
	private:  []nova.IPAddress{{6, "fc00::1", "fixed"}},
	public:   []nova.IPAddress{{6, "2001:db8::1", "floating"}},
	networks: []string{"private", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "public, private, and localhost IPv4",
	private:  []nova.IPAddress{{4, "127.0.0.4", "fixed"}, {4, "192.168.0.1", "fixed"}},
	public:   []nova.IPAddress{{4, "8.8.8.8", "floating"}},
	networks: []string{"private", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "public, private, and localhost IPv6",
	private:  []nova.IPAddress{{6, "::1", "fixed"}, {6, "fc00::1", "fixed"}},
	public:   []nova.IPAddress{{6, "2001:db8::1", "floating"}},
	networks: []string{"private", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "public, private, and localhost - both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "127.0.0.4", "fixed"}, {4, "192.168.0.1", "fixed"}, {6, "::1", "fixed"}, {6, "fc00::1", "fixed"}},
	public:   []nova.IPAddress{{4, "8.8.8.8", "floating"}, {6, "2001:db8::1", "floating"}},
	networks: []string{"private", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "custom only IPv4",
	private:  []nova.IPAddress{{4, "192.168.0.1", "fixed"}},
	networks: []string{"special"},
	expected: "192.168.0.1",
}, {
	summary:  "custom only IPv6",
	private:  []nova.IPAddress{{6, "fc00::1", "fixed"}},
	networks: []string{"special"},
	expected: "fc00::1",
}, {
	summary:  "custom only - both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "192.168.0.1", "fixed"}, {6, "fc00::1", "fixed"}},
	networks: []string{"special"},
	expected: "192.168.0.1",
}, {
	summary:  "custom and public IPv4",
	private:  []nova.IPAddress{{4, "172.16.0.1", "fixed"}},
	public:   []nova.IPAddress{{4, "8.8.8.8", "floating"}},
	networks: []string{"special", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "custom and public IPv6",
	private:  []nova.IPAddress{{6, "fc00::1", "fixed"}},
	public:   []nova.IPAddress{{6, "2001:db8::1", "floating"}},
	networks: []string{"special", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "custom and public - both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "172.16.0.1", "fixed"}, {6, "fc00::1", "fixed"}},
	public:   []nova.IPAddress{{4, "8.8.8.8", "floating"}, {6, "2001:db8::1", "floating"}},
	networks: []string{"special", "public"},
	expected: "8.8.8.8",
}, {
	summary:    "floating and public, same address",
	floatingIP: "8.8.8.8",
	public:     []nova.IPAddress{{4, "8.8.8.8", "floating"}},
	networks:   []string{"", "public"},
	expected:   "8.8.8.8",
}, {
	summary:    "floating and public, different address",
	floatingIP: "8.8.4.4",
	public:     []nova.IPAddress{{4, "8.8.8.8", "floating"}},
	networks:   []string{"", "public"},
	expected:   "8.8.4.4",
}, {
	summary:    "floating and private",
	floatingIP: "8.8.4.4",
	private:    []nova.IPAddress{{4, "10.0.0.1", "fixed"}},
	networks:   []string{"private"},
	expected:   "8.8.4.4",
}, {
	summary:    "floating, custom and public",
	floatingIP: "8.8.4.4",
	private:    []nova.IPAddress{{4, "172.16.0.1", "fixed"}},
	public:     []nova.IPAddress{{4, "8.8.8.8", "floating"}},
	networks:   []string{"special", "public"},
	expected:   "8.8.4.4",
}}

func (t *localTests) TestGetServerAddresses(c *gc.C) {
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
		addr := InstanceAddress(t.floatingIP, addresses)
		c.Check(addr, gc.Equals, t.expected)
	}
}

func (*localTests) TestPortsToRuleInfo(c *gc.C) {
	groupId := "groupid"
	testCases := []struct {
		about    string
		rules    []network.IngressRule
		expected []neutron.RuleInfoV2
	}{{
		about: "single port",
		rules: []network.IngressRule{network.MustNewIngressRule("tcp", 80, 80)},
		expected: []neutron.RuleInfoV2{{
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   80,
			RemoteIPPrefix: "0.0.0.0/0",
			ParentGroupId:  groupId,
		}},
	}, {
		about: "multiple ports",
		rules: []network.IngressRule{network.MustNewIngressRule("tcp", 80, 82)},
		expected: []neutron.RuleInfoV2{{
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   82,
			RemoteIPPrefix: "0.0.0.0/0",
			ParentGroupId:  groupId,
		}},
	}, {
		about: "multiple port ranges",
		rules: []network.IngressRule{
			network.MustNewIngressRule("tcp", 80, 82),
			network.MustNewIngressRule("tcp", 100, 120),
		},
		expected: []neutron.RuleInfoV2{{
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   82,
			RemoteIPPrefix: "0.0.0.0/0",
			ParentGroupId:  groupId,
		}, {
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   100,
			PortRangeMax:   120,
			RemoteIPPrefix: "0.0.0.0/0",
			ParentGroupId:  groupId,
		}},
	}, {
		about: "source range",
		rules: []network.IngressRule{network.MustNewIngressRule(
			"tcp", 80, 100, "192.168.1.0/24", "0.0.0.0/0")},
		expected: []neutron.RuleInfoV2{{
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   100,
			RemoteIPPrefix: "192.168.1.0/24",
			ParentGroupId:  groupId,
		}, {
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   100,
			RemoteIPPrefix: "0.0.0.0/0",
			ParentGroupId:  groupId,
		}},
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		rules := PortsToRuleInfo(groupId, t.rules)
		c.Check(len(rules), gc.Equals, len(t.expected))
		c.Check(rules, gc.DeepEquals, t.expected)
	}
}

func (*localTests) TestSecGroupMatchesIngressRule(c *gc.C) {
	proto_tcp := "tcp"
	proto_udp := "udp"
	port_80 := 80
	port_85 := 85

	testCases := []struct {
		about        string
		rule         network.IngressRule
		secGroupRule neutron.SecurityGroupRuleV2
		expected     bool
	}{{
		about: "single port",
		rule:  network.MustNewIngressRule(proto_tcp, 80, 80),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   &proto_tcp,
			PortRangeMin: &port_80,
			PortRangeMax: &port_80,
		},
		expected: true,
	}, {
		about: "multiple port",
		rule:  network.MustNewIngressRule(proto_tcp, 80, 85),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   &proto_tcp,
			PortRangeMin: &port_80,
			PortRangeMax: &port_85,
		},
		expected: true,
	}, {
		about: "nil rule components",
		rule:  network.MustNewIngressRule(proto_tcp, 80, 85),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   nil,
			PortRangeMin: nil,
			PortRangeMax: nil,
		},
		expected: false,
	}, {
		about: "mismatched port range and rule",
		rule:  network.MustNewIngressRule(proto_tcp, 80, 85),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   &proto_udp,
			PortRangeMin: &port_80,
			PortRangeMax: &port_80,
		},
		expected: false,
	}, {
		about: "default RemoteIPPrefix",
		rule:  network.MustNewIngressRule(proto_tcp, 80, 85),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   &port_80,
			PortRangeMax:   &port_85,
			RemoteIPPrefix: "0.0.0.0/0",
		},
		expected: true,
	}, {
		about: "matching RemoteIPPrefix",
		rule:  network.MustNewIngressRule(proto_tcp, 80, 85, "0.0.0.0/0", "192.168.1.0/24"),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   &port_80,
			PortRangeMax:   &port_85,
			RemoteIPPrefix: "192.168.1.0/24",
		},
		expected: true,
	}, {
		about: "non-matching RemoteIPPrefix",
		rule:  network.MustNewIngressRule(proto_tcp, 80, 85, "0.0.0.0/0", "192.168.1.0/24"),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   &port_80,
			PortRangeMax:   &port_85,
			RemoteIPPrefix: "192.168.100.0/24",
		},
		expected: false,
	}}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(SecGroupMatchesIngressRule(t.secGroupRule, t.rule), gc.Equals, t.expected)
	}
}

func (s *localTests) TestDetectRegionsNoRegionName(c *gc.C) {
	_, err := s.detectRegions(c)
	c.Assert(err, gc.ErrorMatches, "OS_REGION_NAME environment variable not set")
}

func (s *localTests) TestDetectRegionsNoAuthURL(c *gc.C) {
	s.PatchEnvironment("OS_REGION_NAME", "oceania")
	_, err := s.detectRegions(c)
	c.Assert(err, gc.ErrorMatches, "OS_AUTH_URL environment variable not set")
}

func (s *localTests) TestDetectRegions(c *gc.C) {
	s.PatchEnvironment("OS_REGION_NAME", "oceania")
	s.PatchEnvironment("OS_AUTH_URL", "http://keystone.internal")
	regions, err := s.detectRegions(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regions, jc.DeepEquals, []cloud.Region{
		{Name: "oceania", Endpoint: "http://keystone.internal"},
	})
}

func (s *localTests) detectRegions(c *gc.C) ([]cloud.Region, error) {
	provider, err := environs.Provider("openstack")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider, gc.Implements, new(environs.CloudRegionDetector))
	return provider.(environs.CloudRegionDetector).DetectRegions()
}

func (s *localTests) TestSchema(c *gc.C) {
	y := []byte(`
auth-types: [userpass, access-key]
endpoint: http://foo.com/openstack
regions: 
  one:
    endpoint: http://foo.com/bar
  two:
    endpoint: http://foo2.com/bar2
`[1:])
	var v interface{}
	err := yaml.Unmarshal(y, &v)
	c.Assert(err, jc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, jc.ErrorIsNil)

	p, err := environs.Provider("openstack")
	err = p.CloudSchema().Validate(v)
	c.Assert(err, jc.ErrorIsNil)
}

func (localTests) TestPingInvalidHost(c *gc.C) {
	tests := []string{
		"foo.com",
		"http://IHopeNoOneEverBuysThisVerySpecificJujuDomainName.com",
		"http://IHopeNoOneEverBuysThisVerySpecificJujuDomainName:77",
	}

	p, err := environs.Provider("openstack")
	c.Assert(err, jc.ErrorIsNil)
	for _, t := range tests {
		err = p.Ping(t)
		if err == nil {
			c.Errorf("ping %q: expected error, but got nil.", t)
			continue
		}
		expected := "No Openstack server running at " + t
		if err.Error() != expected {
			c.Errorf("ping %q: expected %q got %v", t, expected, err)
		}
	}
}
func (localTests) TestPingNoEndpoint(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer server.Close()
	p, err := environs.Provider("openstack")
	c.Assert(err, jc.ErrorIsNil)
	err = p.Ping(server.URL)
	c.Assert(err, gc.ErrorMatches, "No Openstack server running at "+server.URL)
}

func (localTests) TestPingInvalidResponse(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hi!")
	}))
	defer server.Close()
	p, err := environs.Provider("openstack")
	c.Assert(err, jc.ErrorIsNil)
	err = p.Ping(server.URL)
	c.Assert(err, gc.ErrorMatches, "No Openstack server running at "+server.URL)
}

func (localTests) TestPingOK(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This line is critical, the openstack provider will reject the message
		// if you return 200 like a mere mortal.
		w.WriteHeader(http.StatusMultipleChoices)
		fmt.Fprint(w, `
{
  "versions": {
    "values": [
      {
        "status": "stable",
        "updated": "2013-03-06T00:00:00Z",
        "media-types": [
          {
            "base": "application/json",
            "type": "application/vnd.openstack.identity-v3+json"
          },
          {
            "base": "application/xml",
            "type": "application/vnd.openstack.identity-v3+xml"
          }
        ],
        "id": "v3.0",
        "links": [
          {
            "href": "http://10.24.0.177:5000/v3/",
            "rel": "self"
          }
        ]
      },
      {
        "status": "stable",
        "updated": "2014-04-17T00:00:00Z",
        "media-types": [
          {
            "base": "application/json",
            "type": "application/vnd.openstack.identity-v2.0+json"
          },
          {
            "base": "application/xml",
            "type": "application/vnd.openstack.identity-v2.0+xml"
          }
        ],
        "id": "v2.0",
        "links": [
          {
            "href": "http://10.24.0.177:5000/v2.0/",
            "rel": "self"
          },
          {
            "href": "http://docs.openstack.org/api/openstack-identity-service/2.0/content/",
            "type": "text/html",
            "rel": "describedby"
          },
          {
            "href": "http://docs.openstack.org/api/openstack-identity-service/2.0/identity-dev-guide-2.0.pdf",
            "type": "application/pdf",
            "rel": "describedby"
          }
        ]
      }
    ]
  }
}
`)
	}))
	defer server.Close()
	p, err := environs.Provider("openstack")
	c.Assert(err, jc.ErrorIsNil)
	err = p.Ping(server.URL)
	c.Assert(err, jc.ErrorIsNil)
}

type providerUnitTests struct{}

var _ = gc.Suite(&providerUnitTests{})

func (s *providerUnitTests) checkIdentityClientVersionInvalid(c *gc.C, url string) {
	_, err := identityClientVersion(url)
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("version part of identity url %s not valid", url))
}

func (s *providerUnitTests) TestIdentityClientVersion_BadURLErrors(c *gc.C) {
	s.checkIdentityClientVersionInvalid(c, "https://keystone.internal/a")
	s.checkIdentityClientVersionInvalid(c, "https://keystone.internal/v")
	s.checkIdentityClientVersionInvalid(c, "https://keystone.internal/V")
	s.checkIdentityClientVersionInvalid(c, "https://keystone.internal/V/")
	s.checkIdentityClientVersionInvalid(c, "https://keystone.internal/100")

	_, err := identityClientVersion("abc123")
	c.Check(err, gc.ErrorMatches, `url abc123 is malformed`)

	_, err = identityClientVersion("https://keystone.internal/vot")
	c.Check(err, gc.ErrorMatches, `invalid major version number ot: .* parsing "ot": invalid syntax`)
}

func (s *providerUnitTests) TestIdentityClientVersion_ParsesGoodURL(c *gc.C) {
	version, err := identityClientVersion("https://keystone.internal/v2.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, 2)

	version, err = identityClientVersion("https://keystone.internal/v3.0/")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, 3)

	version, err = identityClientVersion("https://keystone.internal/v2/")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, 2)

	version, err = identityClientVersion("https://keystone.internal/V2/")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, 2)

	_, err = identityClientVersion("https://keystone.internal")
	c.Check(err, jc.ErrorIsNil)

	_, err = identityClientVersion("https://keystone.internal/")
	c.Check(err, jc.ErrorIsNil)
}
