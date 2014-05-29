// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"fmt"
	"net/http"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
)

type instanceSuite struct {
	testing.BaseSuite
	env        *azureEnviron
	service    *gwacl.HostedService
	deployment *gwacl.Deployment
	role       *gwacl.Role
	instance   *azureInstance
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) SetUpTest(c *gc.C) {
	s.env = makeEnviron(c)
	s.service = makeDeployment(s.env, "service-name")
	s.deployment = &s.service.Deployments[0]
	s.deployment.Name = "deployment-one"
	s.role = &s.deployment.RoleList[0]
	s.role.RoleName = "role-one"
	inst, err := s.env.getInstance(s.service, s.role.RoleName)
	c.Assert(err, gc.IsNil)
	c.Assert(inst, gc.FitsTypeOf, &azureInstance{})
	s.instance = inst.(*azureInstance)
}

func configSetNetwork(role *gwacl.Role) *gwacl.ConfigurationSet {
	for i, configSet := range role.ConfigurationSets {
		if configSet.ConfigurationSetType == gwacl.CONFIG_SET_NETWORK {
			return &role.ConfigurationSets[i]
		}
	}
	return nil
}

// makeHostedServiceDescriptor creates a HostedServiceDescriptor with the
// given service name.
func makeHostedServiceDescriptor(name string) *gwacl.HostedServiceDescriptor {
	labelBase64 := base64.StdEncoding.EncodeToString([]byte("label"))
	return &gwacl.HostedServiceDescriptor{ServiceName: name, Label: labelBase64}
}

func (*instanceSuite) TestId(c *gc.C) {
	azInstance := azureInstance{instanceId: "whatever"}
	c.Check(azInstance.Id(), gc.Equals, instance.Id("whatever"))
}

func (*instanceSuite) TestStatus(c *gc.C) {
	var inst azureInstance
	c.Check(inst.Status(), gc.Equals, "")
	inst.roleInstance = &gwacl.RoleInstance{InstanceStatus: "anyoldthing"}
	c.Check(inst.Status(), gc.Equals, "anyoldthing")
}

func (*instanceSuite) TestDNSName(c *gc.C) {
	testService := makeHostedServiceDescriptor("cloud-service-name")
	azInstance := azureInstance{hostedService: testService}
	dnsName, err := azInstance.DNSName()
	c.Assert(err, gc.IsNil)
	c.Check(dnsName, gc.Equals, "cloud-service-name.cloudapp.net")
}

func (*instanceSuite) TestWaitDNSName(c *gc.C) {
	// An Azure instance gets its DNS name immediately, so there's no
	// waiting involved.
	testService := makeHostedServiceDescriptor("cloud-service-name")
	azInstance := azureInstance{hostedService: testService}
	dnsName, err := azInstance.WaitDNSName()
	c.Assert(err, gc.IsNil)
	c.Check(dnsName, gc.Equals, "cloud-service-name.cloudapp.net")
}

func makeInputEndpoint(port int, protocol string) gwacl.InputEndpoint {
	name := fmt.Sprintf("%s%d", protocol, port)
	probe := &gwacl.LoadBalancerProbe{Port: port, Protocol: "TCP"}
	if protocol == "udp" {
		// We just use port 22 (SSH) for the
		// probe when a UDP port is exposed.
		probe.Port = 22
	}
	return gwacl.InputEndpoint{
		LocalPort: port,
		Name:      name,
		LoadBalancedEndpointSetName: name,
		LoadBalancerProbe:           probe,
		Port:                        port,
		Protocol:                    protocol,
	}
}

func serialize(c *gc.C, object gwacl.AzureObject) []byte {
	xml, err := object.Serialize()
	c.Assert(err, gc.IsNil)
	return []byte(xml)
}

func prepareDeploymentInfoResponse(
	c *gc.C, dep gwacl.Deployment) []gwacl.DispatcherResponse {
	return []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(
			serialize(c, &dep), http.StatusOK, nil),
	}
}

func preparePortChangeConversation(c *gc.C, role *gwacl.Role) []gwacl.DispatcherResponse {
	persistentRole := &gwacl.PersistentVMRole{
		XMLNS:             gwacl.XMLNS,
		RoleName:          role.RoleName,
		ConfigurationSets: role.ConfigurationSets,
	}
	return []gwacl.DispatcherResponse{
		// GetRole returns a PersistentVMRole.
		gwacl.NewDispatcherResponse(serialize(c, persistentRole), http.StatusOK, nil),
		// UpdateRole expects a 200 response, that's all.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
}

// point is 1-indexed; it represents which request should fail.
func failPortChangeConversationAt(point int, responses []gwacl.DispatcherResponse) {
	responses[point-1] = gwacl.NewDispatcherResponse(
		nil, http.StatusInternalServerError, nil)
}

type expectedRequest struct {
	method     string
	urlpattern string
}

func assertPortChangeConversation(c *gc.C, record []*gwacl.X509Request, expected []expectedRequest) {
	c.Assert(record, gc.HasLen, len(expected))
	for index, request := range record {
		c.Check(request.Method, gc.Equals, expected[index].method)
		c.Check(request.URL, gc.Matches, expected[index].urlpattern)
	}
}

func (s *instanceSuite) TestAddresses(c *gc.C) {
	vnn := s.env.getVirtualNetworkName()
	responses := prepareDeploymentInfoResponse(c, gwacl.Deployment{
		RoleInstanceList: []gwacl.RoleInstance{{
			RoleName:  s.role.RoleName,
			IPAddress: "1.2.3.4",
		}},
		VirtualNetworkName: vnn,
	})
	gwacl.PatchManagementAPIResponses(responses)

	expected := []instance.Address{
		instance.Address{
			"1.2.3.4",
			instance.Ipv4Address,
			vnn,
			instance.NetworkCloudLocal,
		},
		instance.Address{
			s.service.ServiceName + "." + AZURE_DOMAIN_NAME,
			instance.HostName,
			"",
			instance.NetworkPublic,
		},
	}

	addrs, err := s.instance.Addresses()
	c.Check(err, gc.IsNil)
	c.Check(addrs, jc.SameContents, expected)
}

func (s *instanceSuite) TestOpenPorts(c *gc.C) {
	// Close the default ports.
	configSetNetwork((*gwacl.Role)(s.role)).InputEndpoints = nil

	responses := preparePortChangeConversation(c, s.role)
	record := gwacl.PatchManagementAPIResponses(responses)
	err := s.instance.OpenPorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})
	c.Assert(err, gc.IsNil)

	assertPortChangeConversation(c, *record, []expectedRequest{
		{"GET", ".*/deployments/deployment-one/roles/role-one"}, // GetRole
		{"PUT", ".*/deployments/deployment-one/roles/role-one"}, // UpdateRole
	})

	// A representative UpdateRole payload includes configuration for the
	// ports requested.
	role := &gwacl.PersistentVMRole{}
	err = role.Deserialize((*record)[1].Payload)
	c.Assert(err, gc.IsNil)
	c.Check(
		*configSetNetwork((*gwacl.Role)(role)).InputEndpoints,
		gc.DeepEquals,
		[]gwacl.InputEndpoint{
			makeInputEndpoint(79, "tcp"),
			makeInputEndpoint(587, "tcp"),
			makeInputEndpoint(9, "udp"),
		},
	)
}

func (s *instanceSuite) TestOpenPortsFailsWhenUnableToGetRole(c *gc.C) {
	responses := preparePortChangeConversation(c, s.role)
	failPortChangeConversationAt(1, responses) // 1st request, GetRole
	record := gwacl.PatchManagementAPIResponses(responses)
	err := s.instance.OpenPorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})
	c.Check(err, gc.ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
	c.Check(*record, gc.HasLen, 1)
}

func (s *instanceSuite) TestOpenPortsFailsWhenUnableToUpdateRole(c *gc.C) {
	responses := preparePortChangeConversation(c, s.role)
	failPortChangeConversationAt(2, responses) // 2nd request, UpdateRole
	record := gwacl.PatchManagementAPIResponses(responses)
	err := s.instance.OpenPorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})
	c.Check(err, gc.ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
	c.Check(*record, gc.HasLen, 2)
}

func (s *instanceSuite) TestClosePorts(c *gc.C) {
	type test struct {
		inputPorts  []instance.Port
		removePorts []instance.Port
		outputPorts []instance.Port
	}

	tests := []test{{
		inputPorts:  []instance.Port{{"tcp", 1}, {"tcp", 2}, {"udp", 3}},
		removePorts: nil,
		outputPorts: []instance.Port{{"tcp", 1}, {"tcp", 2}, {"udp", 3}},
	}, {
		inputPorts:  []instance.Port{{"tcp", 1}},
		removePorts: []instance.Port{{"udp", 1}},
		outputPorts: []instance.Port{{"tcp", 1}},
	}, {
		inputPorts:  []instance.Port{{"tcp", 1}, {"tcp", 2}, {"udp", 3}},
		removePorts: []instance.Port{{"tcp", 1}, {"tcp", 2}, {"udp", 3}},
		outputPorts: []instance.Port{},
	}, {
		inputPorts:  []instance.Port{{"tcp", 1}, {"tcp", 2}, {"udp", 3}},
		removePorts: []instance.Port{{"tcp", 99}},
		outputPorts: []instance.Port{{"tcp", 1}, {"tcp", 2}, {"udp", 3}},
	}}

	for i, test := range tests {
		c.Logf("test %d: %#v", i, test)

		inputEndpoints := make([]gwacl.InputEndpoint, len(test.inputPorts))
		for i, port := range test.inputPorts {
			inputEndpoints[i] = makeInputEndpoint(port.Number, port.Protocol)
		}
		configSetNetwork(s.role).InputEndpoints = &inputEndpoints
		responses := preparePortChangeConversation(c, s.role)
		record := gwacl.PatchManagementAPIResponses(responses)

		err := s.instance.ClosePorts("machine-id", test.removePorts)
		c.Assert(err, gc.IsNil)
		assertPortChangeConversation(c, *record, []expectedRequest{
			{"GET", ".*/deployments/deployment-one/roles/role-one"}, // GetRole
			{"PUT", ".*/deployments/deployment-one/roles/role-one"}, // UpdateRole
		})

		// The first UpdateRole removes all endpoints from the role's
		// configuration.
		roleOne := &gwacl.PersistentVMRole{}
		err = roleOne.Deserialize((*record)[1].Payload)
		c.Assert(err, gc.IsNil)
		endpoints := configSetNetwork((*gwacl.Role)(roleOne)).InputEndpoints
		if len(test.outputPorts) == 0 {
			c.Check(endpoints, gc.IsNil)
		} else {
			c.Check(endpoints, gc.NotNil)
			c.Check(convertAndFilterEndpoints(*endpoints, s.env, false), gc.DeepEquals, test.outputPorts)
		}
	}
}

func (s *instanceSuite) TestClosePortsFailsWhenUnableToGetRole(c *gc.C) {
	responses := preparePortChangeConversation(c, s.role)
	failPortChangeConversationAt(1, responses) // 1st request, GetRole
	record := gwacl.PatchManagementAPIResponses(responses)
	err := s.instance.ClosePorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})
	c.Check(err, gc.ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
	c.Check(*record, gc.HasLen, 1)
}

func (s *instanceSuite) TestClosePortsFailsWhenUnableToUpdateRole(c *gc.C) {
	responses := preparePortChangeConversation(c, s.role)
	failPortChangeConversationAt(2, responses) // 2nd request, UpdateRole
	record := gwacl.PatchManagementAPIResponses(responses)
	err := s.instance.ClosePorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})
	c.Check(err, gc.ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
	c.Check(*record, gc.HasLen, 2)
}

func (s *instanceSuite) TestConvertAndFilterEndpoints(c *gc.C) {
	endpoints := []gwacl.InputEndpoint{
		{
			LocalPort: 123,
			Protocol:  "udp",
			Name:      "test123",
			Port:      1123,
		},
		{
			LocalPort: 456,
			Protocol:  "tcp",
			Name:      "test456",
			Port:      44,
		}}
	endpoints = append(endpoints, s.env.getInitialEndpoints(true)...)
	expectedPorts := []instance.Port{
		{
			Number:   1123,
			Protocol: "udp",
		},
		{
			Number:   44,
			Protocol: "tcp",
		}}
	c.Check(convertAndFilterEndpoints(endpoints, s.env, true), gc.DeepEquals, expectedPorts)
}

func (s *instanceSuite) TestConvertAndFilterEndpointsEmptySlice(c *gc.C) {
	ports := convertAndFilterEndpoints([]gwacl.InputEndpoint{}, s.env, true)
	c.Check(ports, gc.HasLen, 0)
}

func (s *instanceSuite) TestPorts(c *gc.C) {
	s.testPorts(c, false)
	s.testPorts(c, true)
}

func (s *instanceSuite) testPorts(c *gc.C, maskStateServerPorts bool) {
	// Update the role's endpoints by hand.
	configSetNetwork(s.role).InputEndpoints = &[]gwacl.InputEndpoint{{
		LocalPort: 223,
		Protocol:  "udp",
		Name:      "test223",
		Port:      2123,
	}, {
		LocalPort: 123,
		Protocol:  "udp",
		Name:      "test123",
		Port:      1123,
	}, {
		LocalPort: 456,
		Protocol:  "tcp",
		Name:      "test456",
		Port:      4456,
	}, {
		LocalPort: s.env.Config().StatePort(),
		Protocol:  "tcp",
		Name:      "stateserver",
		Port:      s.env.Config().StatePort(),
	}, {
		LocalPort: s.env.Config().APIPort(),
		Protocol:  "tcp",
		Name:      "apiserver",
		Port:      s.env.Config().APIPort(),
	}}

	responses := preparePortChangeConversation(c, s.role)
	record := gwacl.PatchManagementAPIResponses(responses)
	s.instance.maskStateServerPorts = maskStateServerPorts
	ports, err := s.instance.Ports("machine-id")
	c.Assert(err, gc.IsNil)
	assertPortChangeConversation(c, *record, []expectedRequest{
		{"GET", ".*/deployments/deployment-one/roles/role-one"}, // GetRole
	})

	expected := []instance.Port{
		{Number: 4456, Protocol: "tcp"},
		{Number: 1123, Protocol: "udp"},
		{Number: 2123, Protocol: "udp"},
	}
	if !maskStateServerPorts {
		expected = append(expected, instance.Port{Number: s.env.Config().StatePort(), Protocol: "tcp"})
		expected = append(expected, instance.Port{Number: s.env.Config().APIPort(), Protocol: "tcp"})
		instance.SortPorts(expected)
	}
	c.Check(ports, gc.DeepEquals, expected)
}
