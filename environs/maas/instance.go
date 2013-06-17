// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/params"
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

func (instance *maasInstance) DNSName() (string, error) {
	hostname, err := (*instance.maasObject).GetField("hostname")
	if err != nil {
		return "", err
	}
	return hostname, nil
}

func (instance *maasInstance) WaitDNSName() (string, error) {
	// A MAAS nodes gets his DNS name when it's created.  WaitDNSName,
	// (same as DNSName) just returns the hostname of the node.
	return instance.DNSName()
}

// MAAS does not do firewalling so these port methods do nothing.
func (instance *maasInstance) OpenPorts(machineId string, ports []params.Port) error {
	log.Debugf("environs/maas: unimplemented OpenPorts() called")
	return nil
}

func (instance *maasInstance) ClosePorts(machineId string, ports []params.Port) error {
	log.Debugf("environs/maas: unimplemented ClosePorts() called")
	return nil
}

func (instance *maasInstance) Ports(machineId string) ([]params.Port, error) {
	log.Debugf("environs/maas: unimplemented Ports() called")
	return []params.Port{}, nil
}
