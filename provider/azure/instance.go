// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/errors"
	"launchpad.net/gwacl"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

const AzureDomainName = "cloudapp.net"

type azureInstance struct {
	environ              *azureEnviron
	hostedService        *gwacl.HostedServiceDescriptor
	instanceId           instance.Id
	deploymentName       string
	roleName             string
	maskStateServerPorts bool

	mu           sync.Mutex
	roleInstance *gwacl.RoleInstance
}

// azureInstance implements Instance.
var _ instance.Instance = (*azureInstance)(nil)

// Id is specified in the Instance interface.
func (azInstance *azureInstance) Id() instance.Id {
	return azInstance.instanceId
}

// supportsLoadBalancing returns true iff the instance is
// not a legacy instance where endpoints may have been
// created without load balancing set names associated.
func (azInstance *azureInstance) supportsLoadBalancing() bool {
	v1Name := deploymentNameV1(azInstance.hostedService.ServiceName)
	return azInstance.deploymentName != v1Name
}

// Status is specified in the Instance interface.
func (azInstance *azureInstance) Status() string {
	azInstance.mu.Lock()
	defer azInstance.mu.Unlock()
	if azInstance.roleInstance == nil {
		return ""
	}
	return azInstance.roleInstance.InstanceStatus
}

func (azInstance *azureInstance) serviceName() string {
	return azInstance.hostedService.ServiceName
}

// Refresh is specified in the Instance interface.
func (azInstance *azureInstance) Refresh() error {
	return azInstance.apiCall(false, func(api *gwacl.ManagementAPI) error {
		d, err := api.GetDeployment(&gwacl.GetDeploymentRequest{
			ServiceName:    azInstance.serviceName(),
			DeploymentName: azInstance.deploymentName,
		})
		if err != nil {
			return err
		}
		// Look for the role instance.
		for _, role := range d.RoleInstanceList {
			if role.RoleName == azInstance.roleName {
				azInstance.mu.Lock()
				azInstance.roleInstance = &role
				azInstance.mu.Unlock()
				return nil
			}
		}
		return errors.NotFoundf("role instance %q", azInstance.roleName)
	})
}

// Addresses is specified in the Instance interface.
func (azInstance *azureInstance) Addresses() ([]network.Address, error) {
	var addrs []network.Address
	for i := 0; i < 2; i++ {
		if ip := azInstance.ipAddress(); ip != "" {
			addrs = append(addrs, network.Address{
				Value:       ip,
				Type:        network.IPv4Address,
				NetworkName: azInstance.environ.getVirtualNetworkName(),
				Scope:       network.ScopeCloudLocal,
			})
			break
		}
		if err := azInstance.Refresh(); err != nil {
			return nil, err
		}
	}
	name := fmt.Sprintf("%s.%s", azInstance.serviceName(), AzureDomainName)
	host := network.Address{
		Value:       name,
		Type:        network.HostName,
		NetworkName: "",
		Scope:       network.ScopePublic,
	}
	addrs = append(addrs, host)
	return addrs, nil
}

func (azInstance *azureInstance) ipAddress() string {
	azInstance.mu.Lock()
	defer azInstance.mu.Unlock()
	if azInstance.roleInstance == nil {
		// RoleInstance hasn't finished deploying.
		return ""
	}
	return azInstance.roleInstance.IPAddress
}

// OpenPorts is specified in the Instance interface.
func (azInstance *azureInstance) OpenPorts(machineId string, portRange []network.PortRange) error {
	return azInstance.apiCall(true, func(api *gwacl.ManagementAPI) error {
		return azInstance.openEndpoints(api, portRange)
	})
}

// apiCall wraps a call to the azure API, optionally locking the environment.
func (azInstance *azureInstance) apiCall(lock bool, f func(*gwacl.ManagementAPI) error) error {
	env := azInstance.environ
	api := env.getSnapshot().api
	if lock {
		env.Lock()
		defer env.Unlock()
	}
	return f(api)
}

// openEndpoints opens the endpoints in the Azure deployment.
func (azInstance *azureInstance) openEndpoints(api *gwacl.ManagementAPI, portRanges []network.PortRange) error {
	request := &gwacl.AddRoleEndpointsRequest{
		ServiceName:    azInstance.serviceName(),
		DeploymentName: azInstance.deploymentName,
		RoleName:       azInstance.roleName,
	}
	for _, portRange := range portRanges {
		name := fmt.Sprintf("%s%d-%d", portRange.Protocol, portRange.FromPort, portRange.ToPort)
		for port := portRange.FromPort; port <= portRange.ToPort; port++ {
			endpoint := gwacl.InputEndpoint{
				LocalPort: port,
				Name:      fmt.Sprintf("%s_range_%d", name, port),
				Port:      port,
				Protocol:  portRange.Protocol,
			}
			if azInstance.supportsLoadBalancing() {
				probePort := port
				if strings.ToUpper(endpoint.Protocol) == "UDP" {
					// Load balancing needs a TCP port to probe, or an HTTP
					// server port & path to query. For UDP, we just use the
					// machine's SSH agent port to test machine liveness.
					//
					// It probably doesn't make sense to load balance most UDP
					// protocols transparently, but that's an application level
					// concern.
					probePort = 22
				}
				endpoint.LoadBalancedEndpointSetName = name
				endpoint.LoadBalancerProbe = &gwacl.LoadBalancerProbe{
					Port:     probePort,
					Protocol: "TCP",
				}
			}
			request.InputEndpoints = append(request.InputEndpoints, endpoint)
		}
	}
	return api.AddRoleEndpoints(request)
}

// ClosePorts is specified in the Instance interface.
func (azInstance *azureInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return azInstance.apiCall(true, func(api *gwacl.ManagementAPI) error {
		return azInstance.closeEndpoints(api, ports)
	})
}

// closeEndpoints closes the endpoints in the Azure deployment.
func (azInstance *azureInstance) closeEndpoints(api *gwacl.ManagementAPI, portRanges []network.PortRange) error {
	request := &gwacl.RemoveRoleEndpointsRequest{
		ServiceName:    azInstance.serviceName(),
		DeploymentName: azInstance.deploymentName,
		RoleName:       azInstance.roleName,
	}
	for _, portRange := range portRanges {
		name := fmt.Sprintf("%s%d-%d", portRange.Protocol, portRange.FromPort, portRange.ToPort)
		for port := portRange.FromPort; port <= portRange.ToPort; port++ {
			request.InputEndpoints = append(request.InputEndpoints, gwacl.InputEndpoint{
				LocalPort:                   port,
				Name:                        fmt.Sprintf("%s_%d", name, port),
				Port:                        port,
				Protocol:                    portRange.Protocol,
				LoadBalancedEndpointSetName: name,
			})
		}
	}
	return api.RemoveRoleEndpoints(request)
}

// convertEndpointsToPorts converts a slice of gwacl.InputEndpoint into a slice of network.PortRange.
func convertEndpointsToPortRanges(endpoints []gwacl.InputEndpoint) []network.PortRange {
	// group ports by prefix on the endpoint name
	portSets := make(map[string][]network.Port)
	otherPorts := []network.Port{}
	for _, endpoint := range endpoints {
		port := network.Port{
			Protocol: strings.ToLower(endpoint.Protocol),
			Number:   endpoint.Port,
		}
		if strings.Contains(endpoint.Name, "_range_") {
			prefix := strings.Split(endpoint.Name, "_range_")[0]
			portSets[prefix] = append(portSets[prefix], port)
		} else {
			otherPorts = append(otherPorts, port)
		}
	}

	portRanges := []network.PortRange{}

	// convert port sets into port ranges
	for _, ports := range portSets {
		portRanges = append(portRanges, network.CollapsePorts(ports)...)
	}

	portRanges = append(portRanges, network.CollapsePorts(otherPorts)...)
	network.SortPortRanges(portRanges)
	return portRanges
}

// convertAndFilterEndpoints converts a slice of gwacl.InputEndpoint into a slice of network.PortRange
// and filters out the initial endpoints that every instance should have opened (ssh port, etc.).
func convertAndFilterEndpoints(endpoints []gwacl.InputEndpoint, env *azureEnviron, stateServer bool) []network.PortRange {
	return portRangeDiff(
		convertEndpointsToPortRanges(endpoints),
		convertEndpointsToPortRanges(env.getInitialEndpoints(stateServer)),
	)
}

// portRangeDiff returns all port ranges that are in a but not in b.
func portRangeDiff(A, B []network.PortRange) (missing []network.PortRange) {
next:
	for _, a := range A {
		for _, b := range B {
			if a == b {
				continue next
			}
		}
		missing = append(missing, a)
	}
	return
}

// Ports is specified in the Instance interface.
func (azInstance *azureInstance) Ports(machineId string) (ports []network.PortRange, err error) {
	err = azInstance.apiCall(false, func(api *gwacl.ManagementAPI) error {
		ports, err = azInstance.listPorts(api)
		return err
	})
	if ports != nil {
		network.SortPortRanges(ports)
	}
	return ports, err
}

// listPorts returns the slice of port ranges (network.PortRange)
// that this machine has opened. The returned list does not contain
// the "initial port ranges" (i.e. the port ranges every instance
// shoud have opened).
func (azInstance *azureInstance) listPorts(api *gwacl.ManagementAPI) ([]network.PortRange, error) {
	endpoints, err := api.ListRoleEndpoints(&gwacl.ListRoleEndpointsRequest{
		ServiceName:    azInstance.serviceName(),
		DeploymentName: azInstance.deploymentName,
		RoleName:       azInstance.roleName,
	})
	if err != nil {
		return nil, err
	}
	ports := convertAndFilterEndpoints(endpoints, azInstance.environ, azInstance.maskStateServerPorts)
	return ports, nil
}
