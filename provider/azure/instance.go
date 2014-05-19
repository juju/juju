// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/errors"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/worker/firewaller"
)

const AZURE_DOMAIN_NAME = "cloudapp.net"

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
	return azInstance.apiCall(false, func(c *azureManagementContext) error {
		d, err := c.GetDeployment(&gwacl.GetDeploymentRequest{
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
func (azInstance *azureInstance) Addresses() ([]instance.Address, error) {
	var addrs []instance.Address
	for i := 0; i < 2; i++ {
		if ip := azInstance.ipAddress(); ip != "" {
			addrs = append(addrs, instance.Address{
				Value:        ip,
				Type:         instance.Ipv4Address,
				NetworkName:  azInstance.environ.getVirtualNetworkName(),
				NetworkScope: instance.NetworkCloudLocal,
			})
			break
		}
		if err := azInstance.Refresh(); err != nil {
			return nil, err
		}
	}
	name, err := azInstance.DNSName()
	if err != nil {
		return nil, err
	}
	host := instance.Address{
		Value:        name,
		Type:         instance.HostName,
		NetworkName:  "",
		NetworkScope: instance.NetworkPublic,
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

// DNSName is specified in the Instance interface.
func (azInstance *azureInstance) DNSName() (string, error) {
	// For deployments in the Production slot, the instance's DNS name
	// is its service name, in the cloudapp.net domain.
	// (For Staging deployments it's all much weirder: they get random
	// names assigned, which somehow don't seem to resolve from the
	// outside.)
	name := fmt.Sprintf("%s.%s", azInstance.serviceName(), AZURE_DOMAIN_NAME)
	return name, nil
}

// WaitDNSName is specified in the Instance interface.
func (azInstance *azureInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(azInstance)
}

// OpenPorts is specified in the Instance interface.
func (azInstance *azureInstance) OpenPorts(machineId string, ports []instance.Port) error {
	return azInstance.apiCall(true, func(context *azureManagementContext) error {
		return azInstance.openEndpoints(context, ports)
	})
}

// apiCall wraps a call to the azure API to ensure it is properly disposed, optionally locking
// the environment
func (azInstance *azureInstance) apiCall(lock bool, f func(*azureManagementContext) error) error {
	env := azInstance.environ
	context, err := env.getManagementAPI()
	if err != nil {
		return err
	}
	defer env.releaseManagementAPI(context)
	if lock {
		env.Lock()
		defer env.Unlock()
	}
	return f(context)
}

// openEndpoints opens the endpoints in the Azure deployment. The caller is
// responsible for locking and unlocking the environ and releasing the
// management context.
func (azInstance *azureInstance) openEndpoints(context *azureManagementContext, ports []instance.Port) error {
	request := &gwacl.AddRoleEndpointsRequest{
		ServiceName:    azInstance.serviceName(),
		DeploymentName: azInstance.deploymentName,
		RoleName:       azInstance.roleName,
	}
	for _, port := range ports {
		name := fmt.Sprintf("%s%d", port.Protocol, port.Number)
		endpoint := gwacl.InputEndpoint{
			LocalPort: port.Number,
			Name:      name,
			Port:      port.Number,
			Protocol:  port.Protocol,
		}
		if azInstance.supportsLoadBalancing() {
			probePort := port.Number
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
	return context.AddRoleEndpoints(request)
}

// ClosePorts is specified in the Instance interface.
func (azInstance *azureInstance) ClosePorts(machineId string, ports []instance.Port) error {
	return azInstance.apiCall(true, func(context *azureManagementContext) error {
		return azInstance.closeEndpoints(context, ports)
	})
}

// closeEndpoints closes the endpoints in the Azure deployment. The caller is
// responsible for locking and unlocking the environ and releasing the
// management context.
func (azInstance *azureInstance) closeEndpoints(context *azureManagementContext, ports []instance.Port) error {
	request := &gwacl.RemoveRoleEndpointsRequest{
		ServiceName:    azInstance.serviceName(),
		DeploymentName: azInstance.deploymentName,
		RoleName:       azInstance.roleName,
	}
	for _, port := range ports {
		name := fmt.Sprintf("%s%d", port.Protocol, port.Number)
		request.InputEndpoints = append(request.InputEndpoints, gwacl.InputEndpoint{
			LocalPort:                   port.Number,
			Name:                        name,
			Port:                        port.Number,
			Protocol:                    port.Protocol,
			LoadBalancedEndpointSetName: name,
		})
	}
	return context.RemoveRoleEndpoints(request)
}

// convertEndpointsToPorts converts a slice of gwacl.InputEndpoint into a slice of instance.Port.
func convertEndpointsToPorts(endpoints []gwacl.InputEndpoint) []instance.Port {
	ports := []instance.Port{}
	for _, endpoint := range endpoints {
		ports = append(ports, instance.Port{
			Protocol: strings.ToLower(endpoint.Protocol),
			Number:   endpoint.Port,
		})
	}
	return ports
}

// convertAndFilterEndpoints converts a slice of gwacl.InputEndpoint into a slice of instance.Port
// and filters out the initial endpoints that every instance should have opened (ssh port, etc.).
func convertAndFilterEndpoints(endpoints []gwacl.InputEndpoint, env *azureEnviron, stateServer bool) []instance.Port {
	return firewaller.Diff(
		convertEndpointsToPorts(endpoints),
		convertEndpointsToPorts(env.getInitialEndpoints(stateServer)),
	)
}

// Ports is specified in the Instance interface.
func (azInstance *azureInstance) Ports(machineId string) (ports []instance.Port, err error) {
	err = azInstance.apiCall(false, func(context *azureManagementContext) error {
		ports, err = azInstance.listPorts(context)
		return err
	})
	if ports != nil {
		instance.SortPorts(ports)
	}
	return ports, err
}

// listPorts returns the slice of ports (instance.Port) that this machine
// has opened. The returned list does not contain the "initial ports"
// (i.e. the ports every instance shoud have opened). The caller is
// responsible for locking and unlocking the environ and releasing the
// management context.
func (azInstance *azureInstance) listPorts(context *azureManagementContext) ([]instance.Port, error) {
	endpoints, err := context.ListRoleEndpoints(&gwacl.ListRoleEndpointsRequest{
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
