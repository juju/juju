package oracle

import (
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	names "gopkg.in/juju/names.v2"
)

//
// These methods here belong of the environ.Networking interface
// their are provided here to tell the juju if the oracle provider supports different network
// options
// Netowrking interface defines methods that environmnets with networking capabilities must implement.
//
// Together these implements also the NetworkingEnviron interface
//

// Subnet returns basic information about subnets known by the oracle provider for the environmnet
func (e oracleEnviron) Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	return nil, nil
}

// NetworkInterfaces requests information about the network interfaces on the given instance
func (e oracleEnviron) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	return nil, nil
}

// SupportsSpaces returns whether the current oracle environment supports
// spaces. The returned error satisfies errors.IsNotSupported(),
// unless a general API failure occurs.
func (e oracleEnviron) SupportsSpaces() (bool, error) {
	return false, nil
}

// SupportsSpaceDiscovery returns whether the current environment
// supports discovering spaces from the oracle provider. The returned error
// satisfies errors.IsNotSupported(), unless a general API failure occurs.
func (e oracleEnviron) SupportsSpaceDiscovery() (bool, error) {
	return false, nil
}

// Spaces returns a slice of network.SpaceInfo with info, including
// details of all associated subnets, about all spaces known to the
// oracle provider that have subnets available.
func (e oracleEnviron) Spaces() ([]network.SpaceInfo, error) {
	return nil, nil
}

// AllocateContainerAddresses allocates a static address for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
// network config including all allocated addresses on success.
func (e oracleEnviron) AllocateContainerAddresses(hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	return nil, nil
}

// ReleaseContainerAddresses releases the previously allocated
// addresses matching the interface details passed in.
func (e oracleEnviron) ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error {
	return nil
}
