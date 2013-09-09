// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider"
)

type maasInstance struct {
	maasObject *gomaasapi.MAASObject
	environ    *maasEnviron
}

var _ instance.Instance = (*maasInstance)(nil)

func (mi *maasInstance) String() string {
	return string(mi.Id())
}

func (mi *maasInstance) Id() instance.Id {
	// Use the node's 'resource_uri' value.
	return instance.Id(mi.maasObject.URI().String())
}

func (mi *maasInstance) Status() string {
	// MAAS does not track node status once they're allocated.
	// Since any instance that juju knows about will be an
	// allocated one, it doesn't make sense to report any
	// state unless we obtain it through some means other than
	// through the MAAS API.
	return ""
}

// refreshInstance refreshes the instance with the most up-to-date information
// from the MAAS server.
func (mi *maasInstance) refreshInstance() error {
	insts, err := mi.environ.Instances([]instance.Id{mi.Id()})
	if err != nil {
		return err
	}
	mi.maasObject = insts[0].(*maasInstance).maasObject
	return nil
}

func (mi *maasInstance) Addresses() ([]instance.Address, error) {
	name, err := mi.DNSName()
	if err != nil {
		return nil, err
	}
	host := instance.Address{name, instance.HostName, "", instance.NetworkPublic}
	addrs := []instance.Address{host}

	ips, err := mi.ipAddresses()
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		a := instance.Address{ip, instance.DeriveAddressType(ip), "", instance.NetworkUnknown}
		addrs = append(addrs, a)
	}

	return addrs, nil
}

func (mi *maasInstance) ipAddresses() ([]string, error) {
	// we have to do this the hard way, since maasObject doesn't have this built-in yet
	objs, err := mi.maasObject.GetMap()["ip_addresses"].GetArray()
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

func (mi *maasInstance) DNSName() (string, error) {
	// A MAAS instance has its DNS name immediately.
	return mi.maasObject.GetField("hostname")
}

func (mi *maasInstance) WaitDNSName() (string, error) {
	return provider.WaitDNSName(mi)
}

// MAAS does not do firewalling so these port methods do nothing.
func (mi *maasInstance) OpenPorts(machineId string, ports []instance.Port) error {
	logger.Debugf("unimplemented OpenPorts() called")
	return nil
}

func (mi *maasInstance) ClosePorts(machineId string, ports []instance.Port) error {
	logger.Debugf("unimplemented ClosePorts() called")
	return nil
}

func (mi *maasInstance) Ports(machineId string) ([]instance.Port, error) {
	logger.Debugf("unimplemented Ports() called")
	return []instance.Port{}, nil
}
