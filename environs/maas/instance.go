// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
)

type maasInstance struct {
	maasObject *gomaasapi.MAASObject
	environ    *maasEnviron
}

var _ instance.Instance = (*maasInstance)(nil)

func (mi *maasInstance) Id() instance.Id {
	// Use the node's 'resource_uri' value.
	return instance.Id((*mi.maasObject).URI().String())
}

// refreshInstance refreshes the instance with the most up-to-date information
// from the MAAS server.
func (mi *maasInstance) refreshInstance() error {
	insts, err := mi.environ.Instances([]instance.Id{mi.Id()})
	if err != nil {
		return err
	}
	newMaasObject := insts[0].(*maasInstance).maasObject
	mi.maasObject = newMaasObject
	return nil
}

func (mi *maasInstance) Addresses() ([]instance.Address, error) {
	logger.Errorf("maasInstance.Address not implemented")
	return nil, nil
}

func (mi *maasInstance) DNSName() (string, error) {
	// A MAAS instance has its DNS name immediately.
	hostname, err := (*mi.maasObject).GetField("hostname")
	if err != nil {
		return "", err
	}
	return hostname, nil
}

func (mi *maasInstance) WaitDNSName() (string, error) {
	return environs.WaitDNSName(mi)
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
