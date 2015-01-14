// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	gitjujutesting "github.com/juju/testing"
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/gce/google"
)

type BaseSuite struct {
	gitjujutesting.IsolationSuite

	FakeConn *fakeConn
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.FakeConn = &fakeConn{}

	s.PatchValue(&connect, func(gceConnection, google.Auth) error {
		return nil
	})
}

type fakeCall struct {
	FuncName string

	Auth         google.Auth
	ID           string
	IDs          []string
	ZoneName     string
	ZoneNames    []string
	Prefix       string
	Statuses     []string
	InstanceSpec google.InstanceSpec
	FirewallName string
	PortRanges   []network.PortRange
	Region       string
}

type fakeConn struct {
	Calls []fakeCall

	Inst       *google.Instance
	Insts      []google.Instance
	PortRanges []network.PortRange
	Zones      []google.AvailabilityZone
	Err        error
	FailOnCall int
}

func (fc *fakeConn) err() error {
	if len(fc.Calls) != fc.FailOnCall+1 {
		return nil
	}
	return fc.Err
}

func (fc *fakeConn) Connect(auth google.Auth) error {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName: "",
		Auth:     auth,
	})
	return fc.err()
}

func (fc *fakeConn) VerifyCredentials() error {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName: "",
	})
	return fc.err()
}

func (fc *fakeConn) Instance(id, zone string) (*google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName: "",
		ID:       id,
		ZoneName: zone,
	})
	return fc.Inst, fc.err()
}

func (fc *fakeConn) Instances(prefix string, statuses ...string) ([]google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName: "",
		Prefix:   prefix,
		Statuses: statuses,
	})
	return fc.Insts, fc.err()
}

func (fc *fakeConn) AddInstance(spec google.InstanceSpec, zones []string) (*google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName:     "",
		InstanceSpec: spec,
		ZoneNames:    zones,
	})
	return fc.Inst, fc.err()
}

func (fc *fakeConn) RemoveInstances(prefix string, ids ...string) error {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName: "",
		Prefix:   prefix,
		IDs:      ids,
	})
	return fc.err()
}

func (fc *fakeConn) Ports(fwname string) ([]network.PortRange, error) {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName:     "",
		FirewallName: fwname,
	})
	return fc.PortRanges, fc.err()
}

func (fc *fakeConn) OpenPorts(fwname string, ports []network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName:     "",
		FirewallName: fwname,
		PortRanges:   ports,
	})
	return fc.err()
}

func (fc *fakeConn) ClosePorts(fwname string, ports []network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName:     "",
		FirewallName: fwname,
		PortRanges:   ports,
	})
	return fc.err()
}

func (fc *fakeConn) AvailabilityZones(region string) ([]google.AvailabilityZone, error) {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName: "",
		Region:   region,
	})
	return fc.Zones, fc.err()
}
