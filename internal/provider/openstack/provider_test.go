// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/go-goose/goose/v5/identity"
	"github.com/go-goose/goose/v5/neutron"
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/testhelpers"
)

// localTests contains tests which do not require a live service or test double to run.
type localTests struct {
	testhelpers.IsolationSuite
}

func TestLocalTests(t *stdtesting.T) {
	tc.Run(t, &localTests{})
}

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

func (t *localTests) TestGetServerAddresses(c *tc.C) {
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
		addr := InstanceAddress(c, t.floatingIP, addresses)
		c.Check(addr, tc.Equals, t.expected)
	}
}

func (*localTests) TestPortsToRuleInfo(c *tc.C) {
	groupId := "groupid"
	testCases := []struct {
		about    string
		rules    firewall.IngressRules
		expected []neutron.RuleInfoV2
	}{{
		about: "single port",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))},
		expected: []neutron.RuleInfoV2{
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   80,
				PortRangeMax:   80,
				RemoteIPPrefix: "0.0.0.0/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv4",
			},
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   80,
				PortRangeMax:   80,
				RemoteIPPrefix: "::/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv6",
			},
		},
	}, {
		about: "multiple ports",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp"))},
		expected: []neutron.RuleInfoV2{
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   80,
				PortRangeMax:   82,
				RemoteIPPrefix: "0.0.0.0/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv4",
			},
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   80,
				PortRangeMax:   82,
				RemoteIPPrefix: "::/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv6",
			},
		},
	}, {
		about: "multiple port ranges",
		rules: firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-120/tcp")),
		},
		expected: []neutron.RuleInfoV2{
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   80,
				PortRangeMax:   82,
				RemoteIPPrefix: "0.0.0.0/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv4",
			}, {
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   100,
				PortRangeMax:   120,
				RemoteIPPrefix: "0.0.0.0/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv4",
			}, {
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   80,
				PortRangeMax:   82,
				RemoteIPPrefix: "::/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv6",
			}, {
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMin:   100,
				PortRangeMax:   120,
				RemoteIPPrefix: "::/0",
				ParentGroupId:  groupId,
				EthernetType:   "IPv6",
			},
		},
	}, {
		about: "source range",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), "192.168.1.0/24", "0.0.0.0/0")},
		expected: []neutron.RuleInfoV2{{
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   100,
			RemoteIPPrefix: "192.168.1.0/24",
			ParentGroupId:  groupId,
			EthernetType:   "IPv4",
		}, {
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   100,
			RemoteIPPrefix: "0.0.0.0/0",
			ParentGroupId:  groupId,
			EthernetType:   "IPv4",
		}},
	}, {
		about: "IPV4 and IPV6 CIDRs",
		rules: firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), "192.168.1.0/24", "2002::1234:abcd:ffff:c0a8:101/64")},
		expected: []neutron.RuleInfoV2{{
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   100,
			RemoteIPPrefix: "192.168.1.0/24",
			ParentGroupId:  groupId,
			EthernetType:   "IPv4",
		}, {
			Direction:      "ingress",
			IPProtocol:     "tcp",
			PortRangeMin:   80,
			PortRangeMax:   100,
			RemoteIPPrefix: "2002::1234:abcd:ffff:c0a8:101/64",
			ParentGroupId:  groupId,
			EthernetType:   "IPv6",
		}},
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		rules := PortsToRuleInfo(groupId, t.rules)
		c.Check(len(rules), tc.Equals, len(t.expected))
		c.Check(rules, tc.SameContents, t.expected)
	}
}

func (*localTests) TestSecGroupMatchesIngressRule(c *tc.C) {
	proto_tcp := "tcp"
	proto_udp := "udp"
	port_80 := 80
	port_85 := 85

	testCases := []struct {
		about        string
		rule         firewall.IngressRule
		secGroupRule neutron.SecurityGroupRuleV2
		expected     bool
	}{{
		about: "single port",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   &proto_tcp,
			PortRangeMin: &port_80,
			PortRangeMax: &port_80,
			EthernetType: "IPv4",
		},
		expected: true,
	}, {
		about: "multiple port",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp")),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   &proto_tcp,
			PortRangeMin: &port_80,
			PortRangeMax: &port_85,
			EthernetType: "IPv4",
		},
		expected: true,
	}, {
		about: "nil rule components",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp")),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   nil,
			PortRangeMin: nil,
			PortRangeMax: nil,
			EthernetType: "IPv4",
		},
		expected: false,
	}, {
		about: "nil rule component: PortRangeMin",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   nil,
			PortRangeMax:   &port_85,
			RemoteIPPrefix: "192.168.100.0/24",
			EthernetType:   "IPv4",
		},
		expected: false,
	}, {
		about: "nil rule component: PortRangeMax",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   &port_85,
			PortRangeMax:   nil,
			RemoteIPPrefix: "192.168.100.0/24",
			EthernetType:   "IPv4",
		},
		expected: false,
	}, {
		about: "mismatched port range and rule",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp")),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:   &proto_udp,
			PortRangeMin: &port_80,
			PortRangeMax: &port_80,
			EthernetType: "IPv4",
		},
		expected: false,
	}, {
		about: "default RemoteIPPrefix",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp")),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   &port_80,
			PortRangeMax:   &port_85,
			RemoteIPPrefix: "0.0.0.0/0",
			EthernetType:   "IPv4",
		},
		expected: true,
	}, {
		about: "matching RemoteIPPrefix",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   &port_80,
			PortRangeMax:   &port_85,
			RemoteIPPrefix: "192.168.1.0/24",
			EthernetType:   "IPv4",
		},
		expected: true,
	}, {
		about: "non-matching RemoteIPPrefix",
		rule:  firewall.NewIngressRule(network.MustParsePortRange("80-85/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		secGroupRule: neutron.SecurityGroupRuleV2{
			IPProtocol:     &proto_tcp,
			PortRangeMin:   &port_80,
			PortRangeMax:   &port_85,
			RemoteIPPrefix: "192.168.100.0/24",
			EthernetType:   "IPv4",
		},
		expected: false,
	}}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(SecGroupMatchesIngressRule(t.secGroupRule, t.rule), tc.Equals, t.expected)
	}
}

func (s *localTests) TestDetectRegionsNoRegionName(c *tc.C) {
	_, err := s.detectRegions(c)
	c.Assert(err, tc.ErrorMatches, "OS_REGION_NAME environment variable not set")
}

func (s *localTests) TestDetectRegionsNoAuthURL(c *tc.C) {
	s.PatchEnvironment("OS_REGION_NAME", "oceania")
	_, err := s.detectRegions(c)
	c.Assert(err, tc.ErrorMatches, "OS_AUTH_URL environment variable not set")
}

func (s *localTests) TestDetectRegions(c *tc.C) {
	s.PatchEnvironment("OS_REGION_NAME", "oceania")
	s.PatchEnvironment("OS_AUTH_URL", "http://keystone.internal")
	regions, err := s.detectRegions(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(regions, tc.DeepEquals, []cloud.Region{
		{Name: "oceania", Endpoint: "http://keystone.internal"},
	})
}

func (s *localTests) detectRegions(c *tc.C) ([]cloud.Region, error) {
	provider, err := environs.Provider("openstack")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provider, tc.Implements, new(environs.CloudRegionDetector))
	return provider.(environs.CloudRegionDetector).DetectRegions()
}

func (s *localTests) TestSchema(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, tc.ErrorIsNil)

	p, err := environs.Provider("openstack")
	c.Assert(err, tc.ErrorIsNil)
	err = p.CloudSchema().Validate(v)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *localTests) TestPingInvalidHost(c *tc.C) {
	tests := []string{
		"foo.com",
		"http://IHopeNoOneEverBuysThisVerySpecificJujuDomainName.com",
		"http://IHopeNoOneEverBuysThisVerySpecificJujuDomainName:77",
	}

	p, err := environs.Provider("openstack")
	c.Assert(err, tc.ErrorIsNil)
	callCtx := c.Context()
	for _, t := range tests {
		err = p.Ping(callCtx, t)
		if err == nil {
			c.Errorf("ping %q: expected error, but got nil.", t)
			continue
		}
		c.Check(err, tc.ErrorMatches, "(?m)No Openstack server running at "+t+".*")
	}
}
func (s *localTests) TestPingNoEndpoint(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer server.Close()
	p, err := environs.Provider("openstack")
	c.Assert(err, tc.ErrorIsNil)
	err = p.Ping(c.Context(), server.URL)
	c.Assert(err, tc.ErrorMatches, "(?m)No Openstack server running at "+server.URL+".*")
}

func (s *localTests) TestPingInvalidResponse(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hi!")
	}))
	defer server.Close()
	p, err := environs.Provider("openstack")
	c.Assert(err, tc.ErrorIsNil)
	err = p.Ping(c.Context(), server.URL)
	c.Assert(err, tc.ErrorMatches, "(?m)No Openstack server running at "+server.URL+".*")
}

func (s *localTests) TestPingOKCACertificate(c *tc.C) {
	server := httptest.NewTLSServer(handlerFunc)
	defer server.Close()
	pingOk(c, server)
}

func (s *localTests) TestPingOK(c *tc.C) {
	server := httptest.NewServer(handlerFunc)
	defer server.Close()
	pingOk(c, server)
}

func pingOk(c *tc.C, server *httptest.Server) {
	p, err := environs.Provider("openstack")
	c.Assert(err, tc.ErrorIsNil)
	err = p.Ping(c.Context(), server.URL)
	c.Assert(err, tc.ErrorIsNil)
}

var handlerFunc = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// This line is critical, the openstack provider will reject the message
	// if you return 200 like a mere mortal.
	w.WriteHeader(http.StatusMultipleChoices)
	_, _ = fmt.Fprint(w, `
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
})

type providerUnitTests struct {
	testhelpers.IsolationSuite
}

func TestProviderUnitTests(t *stdtesting.T) {
	tc.Run(t, &providerUnitTests{})
}

func checkIdentityClientVersionInvalid(c *tc.C, url string) {
	_, err := identityClientVersion(url)
	c.Check(err, tc.ErrorMatches, fmt.Sprintf("version part of identity url %s not valid", url))
}

func checkIdentityClientVersion(c *tc.C, url string, expversion int) {
	version, err := identityClientVersion(url)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version, tc.Equals, expversion)
}
func (s *providerUnitTests) TestIdentityClientVersion_BadURLErrors(c *tc.C) {
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/a")
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/v")
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/V")
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/V/")
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/100")
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/vot")
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/identity/vot")
	checkIdentityClientVersionInvalid(c, "https://keystone.internal/identity/2")

	_, err := identityClientVersion("abc123")
	c.Check(err, tc.ErrorMatches, `url abc123 is malformed`)
}

func (s *providerUnitTests) TestIdentityClientVersion_ParsesGoodURL(c *tc.C) {
	checkIdentityClientVersion(c, "https://keystone.internal/v2.0", 2)
	checkIdentityClientVersion(c, "https://keystone.internal/v3.0/", 3)
	checkIdentityClientVersion(c, "https://keystone.internal/v2/", 2)
	checkIdentityClientVersion(c, "https://keystone.internal/V2/", 2)
	checkIdentityClientVersion(c, "https://keystone.internal/internal/V2/", 2)
	checkIdentityClientVersion(c, "https://keystone.internal/internal/v3.0/", 3)
	checkIdentityClientVersion(c, "https://keystone.internal/internal/v3.2///", 3)
	checkIdentityClientVersion(c, "https://keystone.internal", -1)
	checkIdentityClientVersion(c, "https://keystone.internal/", -1)
}

func (s *providerUnitTests) TestNewCredentialsWithVersion3(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"version":     "3",
		"username":    "user",
		"password":    "secret",
		"tenant-name": "someTenant",
		"tenant-id":   "someID",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	cred, authmode, err := newCredentials(clouldSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cred, tc.Equals, identity.Credentials{
		URL:           "http://endpoint",
		User:          "user",
		Secrets:       "secret",
		Region:        "openstack_region",
		TenantName:    "someTenant",
		TenantID:      "someID",
		Version:       3,
		Domain:        "",
		UserDomain:    "",
		ProjectDomain: "",
	})
	c.Check(authmode, tc.Equals, identity.AuthUserPassV3)
}

func (s *providerUnitTests) TestNewCredentialsWithFaultVersion(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"version":     "abc",
		"username":    "user",
		"password":    "secret",
		"tenant-name": "someTenant",
		"tenant-id":   "someID",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	_, _, err := newCredentials(clouldSpec)
	c.Assert(err, tc.ErrorMatches,
		"cred.Version is not a valid integer type : strconv.Atoi: parsing \"abc\": invalid syntax")
}

func (s *providerUnitTests) TestNewCredentialsWithoutVersion(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":    "user",
		"password":    "secret",
		"tenant-name": "someTenant",
		"tenant-id":   "someID",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	cred, authmode, err := newCredentials(clouldSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cred, tc.Equals, identity.Credentials{
		URL:           "http://endpoint",
		User:          "user",
		Secrets:       "secret",
		Region:        "openstack_region",
		TenantName:    "someTenant",
		TenantID:      "someID",
		Domain:        "",
		UserDomain:    "",
		ProjectDomain: "",
	})
	c.Check(authmode, tc.Equals, identity.AuthUserPass)
}

func (s *providerUnitTests) TestNewCredentialsWithFaultVersionAndProjectDomainName(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"version":             "abc",
		"username":            "user",
		"password":            "secret",
		"tenant-name":         "someTenant",
		"tenant-id":           "someID",
		"project-domain-name": "openstack_projectdomain",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	_, _, err := newCredentials(clouldSpec)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches,
		"cred.Version is not a valid integer type : strconv.Atoi: parsing \"abc\": invalid syntax")
}
func (s *providerUnitTests) TestNewCredentialsWithoutVersionWithProjectDomain(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":            "user",
		"password":            "secret",
		"tenant-name":         "someTenant",
		"tenant-id":           "someID",
		"project-domain-name": "openstack_projectdomain",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	cred, authmode, err := newCredentials(clouldSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cred, tc.Equals, identity.Credentials{
		URL:           "http://endpoint",
		User:          "user",
		Secrets:       "secret",
		Region:        "openstack_region",
		TenantName:    "someTenant",
		TenantID:      "someID",
		Domain:        "",
		UserDomain:    "",
		ProjectDomain: "openstack_projectdomain",
	})
	c.Check(authmode, tc.Equals, identity.AuthUserPassV3)
}

func (s *providerUnitTests) TestNewCredentialsWithoutVersionWithUserDomain(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":         "user",
		"password":         "secret",
		"tenant-name":      "someTenant",
		"tenant-id":        "someID",
		"user-domain-name": "openstack_userdomain",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	cred, authmode, err := newCredentials(clouldSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cred, tc.Equals, identity.Credentials{
		URL:           "http://endpoint",
		User:          "user",
		Secrets:       "secret",
		Region:        "openstack_region",
		TenantName:    "someTenant",
		TenantID:      "someID",
		Version:       0,
		Domain:        "",
		UserDomain:    "openstack_userdomain",
		ProjectDomain: "",
	})
	c.Check(authmode, tc.Equals, identity.AuthUserPassV3)
}

func (s *providerUnitTests) TestNewCredentialsWithVersion2(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"version":     "2",
		"username":    "user",
		"password":    "secret",
		"tenant-name": "someTenant",
		"tenant-id":   "someID",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	cred, authmode, err := newCredentials(clouldSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cred, tc.Equals, identity.Credentials{
		URL:           "http://endpoint",
		User:          "user",
		Secrets:       "secret",
		Region:        "openstack_region",
		TenantName:    "someTenant",
		TenantID:      "someID",
		Version:       2,
		Domain:        "",
		UserDomain:    "",
		ProjectDomain: "",
	})
	c.Check(authmode, tc.Equals, identity.AuthUserPass)
}

func (s *providerUnitTests) TestNewCredentialsWithVersion2AndDomain(c *tc.C) {
	creds := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"version":             "2",
		"username":            "user",
		"password":            "secret",
		"tenant-name":         "someTenant",
		"tenant-id":           "someID",
		"project-domain-name": "openstack_projectdomain",
	})
	clouldSpec := environscloudspec.CloudSpec{
		Type:       "openstack",
		Region:     "openstack_region",
		Name:       "openstack",
		Endpoint:   "http://endpoint",
		Credential: &creds,
	}
	cred, authmode, err := newCredentials(clouldSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cred, tc.Equals, identity.Credentials{
		URL:           "http://endpoint",
		User:          "user",
		Secrets:       "secret",
		Region:        "openstack_region",
		TenantName:    "someTenant",
		TenantID:      "someID",
		Version:       2,
		Domain:        "",
		UserDomain:    "",
		ProjectDomain: "openstack_projectdomain",
	})
	c.Check(authmode, tc.Equals, identity.AuthUserPass)
}

func (s *providerUnitTests) TestNetworksForInstance(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	netID := "network-id-foo"

	mockNetworking := NewMockNetworking(ctrl)
	mockNetworking.EXPECT().ResolveNetworks(netID, false).Return([]neutron.NetworkV2{{Id: netID}}, nil)

	netCfg := NewMockNetworkingConfig(ctrl)

	siParams := environs.StartInstanceParams{
		AvailabilityZone: "eu-west-az",
	}

	result, err := envWithNetworking(mockNetworking, netID).networksForInstance(c.Context(), siParams, netCfg)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []nova.ServerNetworks{
		{
			NetworkId: netID,
			FixedIp:   "",
			PortId:    "",
		},
	})
}

func (s *providerUnitTests) TestNetworksForInstanceNoConfigMultiNet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockNetworking := NewMockNetworking(ctrl)
	mockNetworking.EXPECT().ResolveNetworks("", false).Return([]neutron.NetworkV2{
		{Id: "network-id-foo"},
		{Id: "network-id-bar"},
	}, nil)

	netCfg := NewMockNetworkingConfig(ctrl)

	siParams := environs.StartInstanceParams{
		AvailabilityZone: "eu-west-az",
	}

	result, err := envWithNetworking(mockNetworking, "").networksForInstance(c.Context(), siParams, netCfg)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []nova.ServerNetworks{
		{NetworkId: "network-id-foo"},
		{NetworkId: "network-id-bar"},
	})
}

func (s *providerUnitTests) TestNetworksForInstanceMultiConfigMultiNet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockNetworking := NewMockNetworking(ctrl)
	mockNetworking.EXPECT().ResolveNetworks("network-id-foo", false).Return([]neutron.NetworkV2{{
		Id: "network-id-foo"}}, nil)
	mockNetworking.EXPECT().ResolveNetworks("network-id-bar", false).Return([]neutron.NetworkV2{{
		Id: "network-id-bar"}}, nil)

	netCfg := NewMockNetworkingConfig(ctrl)

	siParams := environs.StartInstanceParams{
		AvailabilityZone: "eu-west-az",
	}

	result, err := envWithNetworking(mockNetworking, "network-id-foo,network-id-bar").networksForInstance(c.Context(), siParams, netCfg)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []nova.ServerNetworks{
		{NetworkId: "network-id-foo"},
		{NetworkId: "network-id-bar"},
	})
}

func (s *providerUnitTests) TestNetworksForInstanceWithAZ(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	netID := "network-id-foo"

	mockNetworking := NewMockNetworking(ctrl)
	mockNetworking.EXPECT().ResolveNetworks(netID, false).Return([]neutron.NetworkV2{{
		Id:        netID,
		SubnetIds: []string{"subnet-foo"},
	}}, nil)

	mockNetworking.EXPECT().CreatePort("", netID, network.Id("subnet-foo")).Return(
		&neutron.PortV2{
			FixedIPs: []neutron.PortFixedIPsV2{{
				IPAddress: "10.10.10.1",
				SubnetID:  "subnet-id",
			}},
			Id:         "port-id",
			MACAddress: "mac-address",
		}, nil)

	netCfg := NewMockNetworkingConfig(ctrl)
	netCfg.EXPECT().AddNetworkConfig(network.InterfaceInfos{{
		InterfaceName: "eth0",
		MACAddress:    "mac-address",
		Addresses:     network.NewMachineAddresses([]string{"10.10.10.1"}).AsProviderAddresses(),
		ConfigType:    network.ConfigDHCP,
		Origin:        network.OriginProvider,
	}}).Return(nil)

	siParams := environs.StartInstanceParams{
		AvailabilityZone: "eu-west-az",
		SubnetsToZones:   []map[network.Id][]string{{"subnet-foo": {"eu-west-az", "eu-east-az"}}},
	}

	result, err := envWithNetworking(mockNetworking, netID).networksForInstance(c.Context(), siParams, netCfg)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []nova.ServerNetworks{
		{
			NetworkId: netID,
			PortId:    "port-id",
		},
	})
}

func (s *providerUnitTests) TestNetworksForInstanceWithAZNoConfigMultiNet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockNetworking := NewMockNetworking(ctrl)
	mockNetworking.EXPECT().ResolveNetworks("", false).Return([]neutron.NetworkV2{
		{
			Id:        "network-id-foo",
			SubnetIds: []string{"subnet-foo"},
		},
		{
			Id:        "network-id-bar",
			SubnetIds: []string{"subnet-bar"},
		},
	}, nil)

	mockNetworking.EXPECT().CreatePort("", "network-id-foo", network.Id("subnet-foo")).Return(
		&neutron.PortV2{
			FixedIPs: []neutron.PortFixedIPsV2{{
				IPAddress: "10.10.10.1",
				SubnetID:  "subnet-foo",
			}},
			Id:         "port-id-foo",
			MACAddress: "mac-address-foo",
		}, nil)

	mockNetworking.EXPECT().CreatePort("", "network-id-bar", network.Id("subnet-bar")).Return(
		&neutron.PortV2{
			FixedIPs: []neutron.PortFixedIPsV2{{
				IPAddress: "10.10.20.1",
				SubnetID:  "subnet-bar",
			}},
			Id:         "port-id-bar",
			MACAddress: "mac-address-bar",
		}, nil)

	netCfg := NewMockNetworkingConfig(ctrl)
	netCfg.EXPECT().AddNetworkConfig(network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			MACAddress:    "mac-address-foo",
			Addresses:     network.NewMachineAddresses([]string{"10.10.10.1"}).AsProviderAddresses(),
			ConfigType:    network.ConfigDHCP,
			Origin:        network.OriginProvider,
		},
		{
			InterfaceName: "eth1",
			MACAddress:    "mac-address-bar",
			Addresses:     network.NewMachineAddresses([]string{"10.10.20.1"}).AsProviderAddresses(),
			ConfigType:    network.ConfigDHCP,
			Origin:        network.OriginProvider,
		},
	}).Return(nil)

	siParams := environs.StartInstanceParams{
		AvailabilityZone: "eu-west-az",
		SubnetsToZones: []map[network.Id][]string{
			{"subnet-foo": {"eu-west-az", "eu-east-az"}},
			{"subnet-bar": {"eu-west-az", "eu-east-az"}},
		},
	}

	result, err := envWithNetworking(mockNetworking, "").networksForInstance(c.Context(), siParams, netCfg)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []nova.ServerNetworks{
		{
			NetworkId: "network-id-foo",
			PortId:    "port-id-foo",
		},
		{
			NetworkId: "network-id-bar",
			PortId:    "port-id-bar",
		},
	})
}

func (s *providerUnitTests) TestNetworksForInstanceWithNoMatchingAZ(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	netID := "network-id-foo"

	mockNetworking := NewMockNetworking(ctrl)
	mockNetworking.EXPECT().ResolveNetworks(netID, false).Return([]neutron.NetworkV2{{
		Id:        netID,
		SubnetIds: []string{"subnet-foo"},
	}}, nil)

	netCfg := NewMockNetworkingConfig(ctrl)

	siParams := environs.StartInstanceParams{
		AvailabilityZone: "us-east-az",
		SubnetsToZones:   []map[network.Id][]string{{"subnet-foo": {"eu-west-az", "eu-east-az"}}},
		Constraints: constraints.Value{
			Spaces: &[]string{"eu-west-az"},
		},
	}

	_, err := envWithNetworking(mockNetworking, netID).networksForInstance(c.Context(), siParams, netCfg)
	c.Assert(err, tc.ErrorMatches, "determining subnets in zone \"us-east-az\": subnets in AZ \"us-east-az\" not found")
}

func (s *providerUnitTests) TestNetworksForInstanceNoSubnetAZsStillConsidered(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	netID := "network-id-foo"

	mockNetworking := NewMockNetworking(ctrl)
	exp := mockNetworking.EXPECT()

	exp.ResolveNetworks(netID, false).Return([]neutron.NetworkV2{{
		Id:        netID,
		SubnetIds: []string{"subnet-foo", "subnet-with-az"},
	}}, nil)

	exp.CreatePort("", netID, network.Id("subnet-foo")).Return(
		&neutron.PortV2{
			FixedIPs: []neutron.PortFixedIPsV2{{
				IPAddress: "10.10.10.1",
				SubnetID:  "subnet-id",
			}},
			Id:         "port-id",
			MACAddress: "mac-address",
		}, nil)

	netCfg := NewMockNetworkingConfig(ctrl)
	netCfg.EXPECT().AddNetworkConfig(network.InterfaceInfos{{
		InterfaceName: "eth0",
		MACAddress:    "mac-address",
		Addresses:     network.NewMachineAddresses([]string{"10.10.10.1"}).AsProviderAddresses(),
		ConfigType:    network.ConfigDHCP,
		Origin:        network.OriginProvider,
	}}).Return(nil)

	siParams := environs.StartInstanceParams{
		AvailabilityZone: "eu-west-az",
		SubnetsToZones: []map[network.Id][]string{{
			"subnet-foo":     {},
			"subnet-with-az": {"some-non-matching-zone"},
		}},
	}

	result, err := envWithNetworking(mockNetworking, netID).networksForInstance(c.Context(), siParams, netCfg)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []nova.ServerNetworks{
		{
			NetworkId: netID,
			PortId:    "port-id",
		},
	})
}

func envWithNetworking(net Networking, netCfg string) *Environ {
	return &Environ{
		ecfgUnlocked: &environConfig{
			attrs: map[string]interface{}{NetworkKey: netCfg},
		},
		networking: net,
	}
}
