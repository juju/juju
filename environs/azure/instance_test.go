// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"

	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"fmt"
	"launchpad.net/juju-core/instance"
	"net/http"
)

type InstanceSuite struct{}

var _ = Suite(new(InstanceSuite))

// makeHostedServiceDescriptor creates a HostedServiceDescriptor with the
// given service name.
func makeHostedServiceDescriptor(name string) *gwacl.HostedServiceDescriptor {
	labelBase64 := base64.StdEncoding.EncodeToString([]byte("label"))
	return &gwacl.HostedServiceDescriptor{ServiceName: name, Label: labelBase64}
}

func (*StorageSuite) TestId(c *C) {
	serviceName := "test-name"
	testService := makeHostedServiceDescriptor(serviceName)
	azInstance := azureInstance{*testService, nil}
	c.Check(azInstance.Id(), Equals, instance.Id(serviceName))
}

func (*StorageSuite) TestDNSName(c *C) {
	// An instance's DNS name is computed from its hosted-service name.
	host := "hostname"
	testService := makeHostedServiceDescriptor(host)
	azInstance := azureInstance{*testService, nil}
	dnsName, err := azInstance.DNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, host+"."+AZURE_DOMAIN_NAME)
}

func (*StorageSuite) TestWaitDNSName(c *C) {
	// An Azure instance gets its DNS name immediately, so there's no
	// waiting involved.
	host := "hostname"
	testService := makeHostedServiceDescriptor(host)
	azInstance := azureInstance{*testService, nil}
	dnsName, err := azInstance.WaitDNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, host+"."+AZURE_DOMAIN_NAME)
}

func makeRole(name string, endpoints ...gwacl.InputEndpoint) gwacl.Role {
	return gwacl.Role{
		RoleName: name,
		ConfigurationSets: []gwacl.ConfigurationSet{
			{
				ConfigurationSetType: gwacl.CONFIG_SET_NETWORK,
				InputEndpoints:       &endpoints,
			},
		},
	}
}

func makeDeployment(name string, roles ...gwacl.Role) gwacl.Deployment {
	return gwacl.Deployment{
		Name:     name,
		RoleList: roles,
	}
}

func makeInputEndpoint(port int, protocol string) gwacl.InputEndpoint {
	return gwacl.InputEndpoint{
		LocalPort: port,
		Name:      fmt.Sprintf("%s%d", protocol, port),
		Port:      port,
		Protocol:  protocol,
	}
}

func serialize(c *C, object gwacl.AzureObject) []byte {
	xml, err := object.Serialize()
	c.Assert(err, IsNil)
	return []byte(xml)
}

func prepareConversationForPortChanges(
	c *C, service *gwacl.HostedServiceDescriptor,
	deployments []gwacl.Deployment) *[]*gwacl.X509Request {
	// Construct the series of responses to expected requests.
	responses := []gwacl.DispatcherResponse{
		// First, GetHostedServiceProperties
		gwacl.NewDispatcherResponse(
			serialize(c, &gwacl.HostedService{
				Deployments:             deployments,
				HostedServiceDescriptor: *service,
				XMLNS: gwacl.XMLNS,
			}),
			http.StatusOK, nil),
	}
	for _, deployment := range deployments {
		for _, role := range deployment.RoleList {
			// GetRole returns a PersistentVMRole.
			persistentRole := &gwacl.PersistentVMRole{
				XMLNS:             gwacl.XMLNS,
				RoleName:          role.RoleName,
				ConfigurationSets: role.ConfigurationSets,
			}
			responses = append(responses, gwacl.NewDispatcherResponse(
				serialize(c, persistentRole), http.StatusOK, nil))
			// UpdateRole expects a 200 response, that's all.
			responses = append(responses,
				gwacl.NewDispatcherResponse(nil, http.StatusOK, nil))
		}
	}
	return gwacl.PatchManagementAPIResponses(responses)
}

type expectedRequest struct {
	method     string
	urlpattern string
}

func assertPortChangeConversation(c *C, record []*gwacl.X509Request, expected []expectedRequest) {
	c.Assert(record, HasLen, len(expected))
	for index, request := range record {
		c.Check(request.Method, Equals, expected[index].method)
		c.Check(request.URL, Matches, expected[index].urlpattern)
	}
}

func (*StorageSuite) TestOpenPorts(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	deployments := []gwacl.Deployment{
		makeDeployment("deployment-one", makeRole("role-one"), makeRole("role-two")),
		makeDeployment("deployment-two", makeRole("role-three")),
	}
	record := prepareConversationForPortChanges(c, service, deployments)
	azInstance := azureInstance{*service, makeEnviron(c)}

	err := azInstance.OpenPorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})

	c.Assert(err, IsNil)
	assertPortChangeConversation(c, *record, []expectedRequest{
		{"GET", ".*/services/hostedservices/service-name[?].*"},   // GetHostedServiceProperties
		{"GET", ".*/deployments/deployment-one/roles/role-one"},   // GetRole
		{"PUT", ".*/deployments/deployment-one/roles/role-one"},   // UpdateRole
		{"GET", ".*/deployments/deployment-one/roles/role-two"},   // GetRole
		{"PUT", ".*/deployments/deployment-one/roles/role-two"},   // UpdateRole
		{"GET", ".*/deployments/deployment-two/roles/role-three"}, // GetRole
		{"PUT", ".*/deployments/deployment-two/roles/role-three"}, // UpdateRole
	})

	// A representative UpdateRole payload includes configuration for the
	// ports requested.
	role := &gwacl.PersistentVMRole{}
	err = role.Deserialize((*record)[2].Payload)
	c.Assert(err, IsNil)
	c.Check(
		*(role.ConfigurationSets[0].InputEndpoints),
		DeepEquals, []gwacl.InputEndpoint{
			makeInputEndpoint(79, "tcp"),
			makeInputEndpoint(587, "tcp"),
			makeInputEndpoint(9, "udp"),
		})
}

func (*StorageSuite) TestOpenPortsFailsWhenUnableToGetServiceProperties(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	responses := []gwacl.DispatcherResponse{
		// GetHostedServiceProperties breaks.
		gwacl.NewDispatcherResponse(nil, http.StatusInternalServerError, nil),
	}
	record := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	azInstance := azureInstance{*service, env}

	err := azInstance.OpenPorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})

	c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
	c.Check(*record, HasLen, 1)
}

func (*StorageSuite) TestOpenPortsFailsWhenUnableToGetRole(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	deployments := []gwacl.Deployment{
		makeDeployment("deployment-one", makeRole("role-one")),
	}
	responses := []gwacl.DispatcherResponse{
		// First, GetHostedServiceProperties
		gwacl.NewDispatcherResponse(
			serialize(c, &gwacl.HostedService{
				Deployments:             deployments,
				HostedServiceDescriptor: *service,
				XMLNS: gwacl.XMLNS,
			}),
			http.StatusOK, nil),
		// Second, GetRole fails
		gwacl.NewDispatcherResponse(
			nil, http.StatusInternalServerError, nil),
	}
	record := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	azInstance := azureInstance{*service, env}

	err := azInstance.OpenPorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})

	c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
	c.Check(*record, HasLen, 2)
}

func (*StorageSuite) TestOpenPortsFailsWhenUnableToUpdateRole(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	deployments := []gwacl.Deployment{
		makeDeployment("deployment-one", makeRole("role-one")),
	}
	responses := []gwacl.DispatcherResponse{
		// First, GetHostedServiceProperties
		gwacl.NewDispatcherResponse(
			serialize(c, &gwacl.HostedService{
				Deployments:             deployments,
				HostedServiceDescriptor: *service,
				XMLNS: gwacl.XMLNS,
			}),
			http.StatusOK, nil),
		// Seconds, GetRole
		gwacl.NewDispatcherResponse(
			serialize(c, &gwacl.PersistentVMRole{
				XMLNS:    gwacl.XMLNS,
				RoleName: "role-one",
			}),
			http.StatusOK, nil),
		// Third, UpdateRole fails
		gwacl.NewDispatcherResponse(
			nil, http.StatusInternalServerError, nil),
	}
	record := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	azInstance := azureInstance{*service, env}

	err := azInstance.OpenPorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})

	c.Check(err, ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
	c.Check(*record, HasLen, 3)
}

func (*StorageSuite) TestClosePorts(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	deployments := []gwacl.Deployment{
		makeDeployment("deployment-one",
			makeRole("role-one",
				makeInputEndpoint(587, "tcp"),
			),
			makeRole("role-two",
				makeInputEndpoint(79, "tcp"),
				makeInputEndpoint(9, "udp"),
			)),
		makeDeployment("deployment-two",
			makeRole("role-three",
				makeInputEndpoint(9, "tcp"),
				makeInputEndpoint(9, "udp"),
			)),
	}
	record := prepareConversationForPortChanges(c, service, deployments)
	azInstance := azureInstance{*service, makeEnviron(c)}

	err := azInstance.ClosePorts("machine-id", []instance.Port{{"tcp", 587}, {"udp", 9}})

	c.Assert(err, IsNil)
	assertPortChangeConversation(c, *record, []expectedRequest{
		{"GET", ".*/services/hostedservices/service-name[?].*"},   // GetHostedServiceProperties
		{"GET", ".*/deployments/deployment-one/roles/role-one"},   // GetRole
		{"PUT", ".*/deployments/deployment-one/roles/role-one"},   // UpdateRole
		{"GET", ".*/deployments/deployment-one/roles/role-two"},   // GetRole
		{"PUT", ".*/deployments/deployment-one/roles/role-two"},   // UpdateRole
		{"GET", ".*/deployments/deployment-two/roles/role-three"}, // GetRole
		{"PUT", ".*/deployments/deployment-two/roles/role-three"}, // UpdateRole
	})

	// The first UpdateRole removes all endpoints from the role's
	// configuration.
	roleOne := &gwacl.PersistentVMRole{}
	err = roleOne.Deserialize((*record)[2].Payload)
	c.Assert(err, IsNil)
	c.Check(roleOne.ConfigurationSets[0].InputEndpoints, IsNil)

	// The second UpdateRole removes all but 79/TCP.
	roleTwo := &gwacl.PersistentVMRole{}
	err = roleTwo.Deserialize((*record)[4].Payload)
	c.Assert(err, IsNil)
	c.Check(roleTwo.ConfigurationSets[0].InputEndpoints, DeepEquals,
		&[]gwacl.InputEndpoint{makeInputEndpoint(79, "tcp")})

	// The third UpdateRole removes all but 9/TCP.
	roleThree := &gwacl.PersistentVMRole{}
	err = roleThree.Deserialize((*record)[6].Payload)
	c.Assert(err, IsNil)
	c.Check(roleThree.ConfigurationSets[0].InputEndpoints, DeepEquals,
		&[]gwacl.InputEndpoint{makeInputEndpoint(9, "tcp")})
}

func (*StorageSuite) TestClosePortsFailsWhenUnableToGetServiceProperties(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	responses := []gwacl.DispatcherResponse{
		// GetHostedServiceProperties breaks.
		gwacl.NewDispatcherResponse(nil, http.StatusInternalServerError, nil),
	}
	record := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	azInstance := azureInstance{*service, env}

	err := azInstance.ClosePorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})

	c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
	c.Check(*record, HasLen, 1)
}

func (*StorageSuite) TestClosePortsFailsWhenUnableToGetRole(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	deployments := []gwacl.Deployment{
		makeDeployment("deployment-one", makeRole("role-one")),
	}
	responses := []gwacl.DispatcherResponse{
		// First, GetHostedServiceProperties
		gwacl.NewDispatcherResponse(
			serialize(c, &gwacl.HostedService{
				Deployments:             deployments,
				HostedServiceDescriptor: *service,
				XMLNS: gwacl.XMLNS,
			}),
			http.StatusOK, nil),
		// Second, GetRole fails
		gwacl.NewDispatcherResponse(
			nil, http.StatusInternalServerError, nil),
	}
	record := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	azInstance := azureInstance{*service, env}

	err := azInstance.ClosePorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})

	c.Check(err, ErrorMatches, "GET request failed [(]500: Internal Server Error[)]")
	c.Check(*record, HasLen, 2)
}

func (*StorageSuite) TestClosePortsFailsWhenUnableToUpdateRole(c *C) {
	service := makeHostedServiceDescriptor("service-name")
	deployments := []gwacl.Deployment{
		makeDeployment("deployment-one", makeRole("role-one")),
	}
	responses := []gwacl.DispatcherResponse{
		// First, GetHostedServiceProperties
		gwacl.NewDispatcherResponse(
			serialize(c, &gwacl.HostedService{
				Deployments:             deployments,
				HostedServiceDescriptor: *service,
				XMLNS: gwacl.XMLNS,
			}),
			http.StatusOK, nil),
		// Seconds, GetRole
		gwacl.NewDispatcherResponse(
			serialize(c, &gwacl.PersistentVMRole{
				XMLNS:    gwacl.XMLNS,
				RoleName: "role-one",
			}),
			http.StatusOK, nil),
		// Third, UpdateRole fails
		gwacl.NewDispatcherResponse(
			nil, http.StatusInternalServerError, nil),
	}
	record := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	azInstance := azureInstance{*service, env}

	err := azInstance.ClosePorts("machine-id", []instance.Port{
		{"tcp", 79}, {"tcp", 587}, {"udp", 9},
	})

	c.Check(err, ErrorMatches, "PUT request failed [(]500: Internal Server Error[)]")
	c.Check(*record, HasLen, 3)
}
