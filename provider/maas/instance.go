// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type maasInstance struct {
	maasObject *gomaasapi.MAASObject
}

var _ instance.Instance = (*maasInstance)(nil)

// Override for testing.
var resolveHostnames = func(addrs []network.Address) []network.Address {
	return network.ResolvableHostnames(addrs)
}

func (mi *maasInstance) String() string {
	hostname, err := mi.hostname()
	if err != nil {
		// This is meant to be impossible, but be paranoid.
		hostname = fmt.Sprintf("<DNSName failed: %q>", err)
	}
	return fmt.Sprintf("%s:%s", hostname, mi.Id())
}

func (mi *maasInstance) Id() instance.Id {
	return maasObjectId(mi.maasObject)
}

func maasObjectId(maasObject *gomaasapi.MAASObject) instance.Id {
	// Use the node's 'resource_uri' value.
	return instance.Id(maasObject.URI().String())
}

func (mi *maasInstance) Status() string {
	// MAAS does not track node status once they're allocated.
	// Since any instance that juju knows about will be an
	// allocated one, it doesn't make sense to report any
	// state unless we obtain it through some means other than
	// through the MAAS API.
	return ""
}

func (mi *maasInstance) Addresses() ([]network.Address, error) {
	name, err := mi.hostname()
	if err != nil {
		return nil, err
	}
	// MAAS prefers to use the dns name for intra-node communication.
	// When Juju looks up the address to use for communicating between
	// nodes, it looks up the address by scope. So we add a cloud
	// local address for that purpose.
	addrs := network.NewAddresses(name, name)
	addrs[0].Scope = network.ScopePublic
	addrs[1].Scope = network.ScopeCloudLocal

	// Append any remaining IP addresses after the preferred ones.
	ips, err := mi.ipAddresses()
	if err != nil {
		return nil, err
	}
	addrs = append(addrs, network.NewAddresses(ips...)...)

	// Although we would prefer a DNS name there's no point
	// returning unresolvable names because activities like 'juju
	// ssh 0' will instantly fail.
	return resolveHostnames(addrs), nil
}

func (mi *maasInstance) ipAddresses() ([]string, error) {
	// we have to do this the hard way, since maasObject doesn't have this built-in yet
	addressArray := mi.maasObject.GetMap()["ip_addresses"]
	if addressArray.IsNil() {
		// Older MAAS versions do not return ip_addresses.
		return nil, nil
	}
	objs, err := addressArray.GetArray()
	if err != nil {
		return nil, err
	}
	ips := make([]string, len(objs))
	for i, obj := range objs {
		s, err := obj.GetString()
		if err != nil {
			return nil, err
		}
		ips[i] = s
	}
	return ips, nil
}

func (mi *maasInstance) architecture() (arch, subarch string, err error) {
	// MAAS may return an architecture of the form, for example,
	// "amd64/generic"; we only care about the major part.
	arch, err = mi.maasObject.GetField("architecture")
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(arch, "/", 2)
	arch = parts[0]
	if len(parts) == 2 {
		subarch = parts[1]
	}
	return arch, subarch, nil
}

func (mi *maasInstance) zone() string {
	zone, _ := mi.maasObject.GetField("zone")
	return zone
}

func (mi *maasInstance) cpuCount() (uint64, error) {
	count, err := mi.maasObject.GetMap()["cpu_count"].GetFloat64()
	if err != nil {
		return 0, err
	}
	return uint64(count), nil
}

func (mi *maasInstance) memory() (uint64, error) {
	mem, err := mi.maasObject.GetMap()["memory"].GetFloat64()
	if err != nil {
		return 0, err
	}
	return uint64(mem), nil
}

func (mi *maasInstance) tagNames() ([]string, error) {
	obj := mi.maasObject.GetMap()["tag_names"]
	if obj.IsNil() {
		return nil, errors.NotFoundf("tag_names")
	}
	array, err := obj.GetArray()
	if err != nil {
		return nil, err
	}
	tags := make([]string, len(array))
	for i, obj := range array {
		tag, err := obj.GetString()
		if err != nil {
			return nil, err
		}
		tags[i] = tag
	}
	return tags, nil
}

func (mi *maasInstance) hardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	nodeArch, _, err := mi.architecture()
	if err != nil {
		return nil, errors.Annotate(err, "error determining architecture")
	}
	nodeCpuCount, err := mi.cpuCount()
	if err != nil {
		return nil, errors.Annotate(err, "error determining cpu count")
	}
	nodeMemoryMB, err := mi.memory()
	if err != nil {
		return nil, errors.Annotate(err, "error determining available memory")
	}
	zone := mi.zone()
	hc := &instance.HardwareCharacteristics{
		Arch:             &nodeArch,
		CpuCores:         &nodeCpuCount,
		Mem:              &nodeMemoryMB,
		AvailabilityZone: &zone,
	}
	nodeTags, err := mi.tagNames()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Annotate(err, "error determining tag names")
	}
	if len(nodeTags) > 0 {
		hc.Tags = &nodeTags
	}
	return hc, nil
}

func (mi *maasInstance) hostname() (string, error) {
	// A MAAS instance has its DNS name immediately.
	return mi.maasObject.GetField("hostname")
}

// MAAS does not do firewalling so these port methods do nothing.
func (mi *maasInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	logger.Debugf("unimplemented OpenPorts() called")
	return nil
}

func (mi *maasInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	logger.Debugf("unimplemented ClosePorts() called")
	return nil
}

func (mi *maasInstance) Ports(machineId string) ([]network.PortRange, error) {
	logger.Debugf("unimplemented Ports() called")
	return nil, nil
}
