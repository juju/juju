// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"strings"

	"launchpad.net/gwacl"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/firewaller"
)

type azureInstance struct {
	// An instance contains an Azure Service (instance==service).
	gwacl.HostedServiceDescriptor
	environ *azureEnviron
}

// azureInstance implements Instance.
var _ instance.Instance = (*azureInstance)(nil)

// Id is specified in the Instance interface.
func (azInstance *azureInstance) Id() instance.Id {
	return instance.Id(azInstance.ServiceName)
}

// Status is specified in the Instance interface.
func (azInstance *azureInstance) Status() string {
	return azInstance.HostedServiceDescriptor.Status
}

var AZURE_DOMAIN_NAME = "cloudapp.net"

// Addresses is specified in the Instance interface.
func (azInstance *azureInstance) Addresses() ([]instance.Address, error) {
	logger.Errorf("azureInstance.Addresses not implemented")
	return nil, nil
}

// DNSName is specified in the Instance interface.
func (azInstance *azureInstance) DNSName() (string, error) {
	// For deployments in the Production slot, the instance's DNS name
	// is its service name, in the cloudapp.net domain.
	// (For Staging deployments it's all much weirder: they get random
	// names assigned, which somehow don't seem to resolve from the
	// outside.)
	name := fmt.Sprintf("%s.%s", azInstance.ServiceName, AZURE_DOMAIN_NAME)
	return name, nil
}

// WaitDNSName is specified in the Instance interface.
func (azInstance *azureInstance) WaitDNSName() (string, error) {
	return provider.WaitDNSName(azInstance)
}

// OpenPorts is specified in the Instance interface.
func (azInstance *azureInstance) OpenPorts(machineId string, ports []instance.Port) error {
	env := azInstance.environ

	context, err := env.getManagementAPI()
	if err != nil {
		return err
	}
	defer env.releaseManagementAPI(context)

	env.Lock()
	defer env.Unlock()

	return azInstance.openEndpoints(context, ports)
}

// openEndpoints opens the endpoints in the Azure deployment. The caller is
// responsible for locking and unlocking the environ and releasing the
// management context.
func (azInstance *azureInstance) openEndpoints(context *azureManagementContext, ports []instance.Port) error {
	deployments, err := context.ListAllDeployments(&gwacl.ListAllDeploymentsRequest{
		ServiceName: azInstance.ServiceName,
	})
	if err != nil {
		return err
	}

	for _, deployment := range deployments {
		for _, role := range deployment.RoleList {
			request := &gwacl.AddRoleEndpointsRequest{
				ServiceName:    azInstance.ServiceName,
				DeploymentName: deployment.Name,
				RoleName:       role.RoleName,
			}
			for _, port := range ports {
				request.InputEndpoints = append(
					request.InputEndpoints, gwacl.InputEndpoint{
						LocalPort: port.Number,
						Name:      fmt.Sprintf("%s%d", port.Protocol, port.Number),
						Port:      port.Number,
						Protocol:  port.Protocol,
					})
			}
			err := context.AddRoleEndpoints(request)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ClosePorts is specified in the Instance interface.
func (azInstance *azureInstance) ClosePorts(machineId string, ports []instance.Port) error {
	env := azInstance.environ

	context, err := env.getManagementAPI()
	if err != nil {
		return err
	}
	defer env.releaseManagementAPI(context)

	env.Lock()
	defer env.Unlock()

	return azInstance.closeEndpoints(context, ports)
}

// closeEndpoints closes the endpoints in the Azure deployment. The caller is
// responsible for locking and unlocking the environ and releasing the
// management context.
func (azInstance *azureInstance) closeEndpoints(context *azureManagementContext, ports []instance.Port) error {
	deployments, err := context.ListAllDeployments(&gwacl.ListAllDeploymentsRequest{
		ServiceName: azInstance.ServiceName,
	})
	if err != nil {
		return err
	}

	for _, deployment := range deployments {
		for _, role := range deployment.RoleList {
			request := &gwacl.RemoveRoleEndpointsRequest{
				ServiceName:    azInstance.ServiceName,
				DeploymentName: deployment.Name,
				RoleName:       role.RoleName,
			}
			for _, port := range ports {
				request.InputEndpoints = append(
					request.InputEndpoints, gwacl.InputEndpoint{
						LocalPort: port.Number,
						Name:      fmt.Sprintf("%s%d", port.Protocol, port.Number),
						Port:      port.Number,
						Protocol:  port.Protocol,
					})
			}
			err := context.RemoveRoleEndpoints(request)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// convertAndFilterEndpoints converts a slice of gwacl.InputEndpoint into a slice of instance.Port.
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
func convertAndFilterEndpoints(endpoints []gwacl.InputEndpoint, env *azureEnviron) []instance.Port {
	return firewaller.Diff(
		convertEndpointsToPorts(endpoints),
		convertEndpointsToPorts(env.getInitialEndpoints()))
}

// Ports is specified in the Instance interface.
func (azInstance *azureInstance) Ports(machineId string) ([]instance.Port, error) {
	env := azInstance.environ
	context, err := env.getManagementAPI()
	if err != nil {
		return nil, err
	}
	defer env.releaseManagementAPI(context)

	ports, err := azInstance.listPorts(context)
	if err != nil {
		return nil, err
	}
	state.SortPorts(ports)
	return ports, nil
}

// listPorts returns the slice of ports (instance.Port) that this machine
// has opened. The returned list does not contain the "initial ports"
// (i.e. the ports every instance shoud have opened). The caller is
// responsible for locking and unlocking the environ and releasing the
// management context.
func (azInstance *azureInstance) listPorts(context *azureManagementContext) ([]instance.Port, error) {
	deployments, err := context.ListAllDeployments(&gwacl.ListAllDeploymentsRequest{
		ServiceName: azInstance.ServiceName,
	})
	if err != nil {
		return nil, err
	}

	env := azInstance.environ
	switch {
	// Only zero or one deployment is a valid state (instance==service).
	case len(deployments) > 1:
		return nil, fmt.Errorf("more than one Azure deployment inside the service named %q", azInstance.ServiceName)
	case len(deployments) == 1:
		deployment := deployments[0]
		switch {
		// Only zero or one role is a valid state (instance==service).
		case len(deployment.RoleList) > 1:
			return nil, fmt.Errorf("more than one Azure role inside the deployment named %q", deployment.Name)
		case len(deployment.RoleList) == 1:
			role := deployment.RoleList[0]

			endpoints, err := context.ListRoleEndpoints(&gwacl.ListRoleEndpointsRequest{
				ServiceName:    azInstance.ServiceName,
				DeploymentName: deployment.Name,
				RoleName:       role.RoleName,
			})
			if err != nil {
				return nil, err
			}
			ports := convertAndFilterEndpoints(endpoints, env)
			return ports, nil
		}
		return nil, nil
	}
	return nil, nil
}
