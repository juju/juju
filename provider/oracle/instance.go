package oracle

import (
	"github.com/hoenirvili/go-oracle-cloud/response"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// oracleInstance represents the realization of amachine instate
// instance imlements the instance.Instance interface
type oracleInstance struct {
	// name of the instance, generated after the vm creation
	name string
	// status represents the status for a provider instance
	status instance.InstanceStatus
}

// newInstance returns a new instance.Instance implementation
// for the response.LaunchPlan
func newInstance(params response.LaunchPlan) (instance.Instance, error) {
	return nil, nil
}

// Id returns a provider generated indentifier for the Instance
func (o oracleInstance) Id() instance.Id {
	return instance.Id(o.name)
}

// Status represents the provider specific status for the instance
func (o oracleInstance) Status() instance.InstanceStatus {
	return o.status
}

// OpenPorts opens the given port ranges on the instance, which
// should have been started with the given machine id.
func (o oracleInstance) Addresses() ([]network.Address, error) {
	return nil, nil
}

// ClosePorts closes the given port ranges on the instance, which
// should have been started with the given machine id.
func (o oracleInstance) OpenPorts(machineId string, rules []network.IngressRule) error {
	return nil
}

// ClosePorts closes the given port ranges on the instance, which
// should have been started with the given machine id.
func (o oracleInstance) ClosePorts(machineId string, rules []network.IngressRule) error {
	return nil
}

// IngressRules returns the set of ingress rules for the instance,
// which should have been applied to the given machine id. The
// rules are returned as sorted by network.SortIngressRules().
// It is expected that there be only one ingress rule result for a given
// port range - the rule's SourceCIDRs will contain all applicable source
// address rules for that port range.
func IngressRule(machineId string) ([]network.IngressRule, error) {
	return nil, nil
}
