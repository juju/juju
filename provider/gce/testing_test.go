// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/testing"
)

var (
	ConfigAttrs = testing.FakeConfig().Merge(testing.Attrs{
		"type":           "gce",
		"private-key":    "seekrit",
		"client-id":      "static",
		"client-email":   "joe@mail.com",
		"region":         "home",
		"project-id":     "my-juju",
		"image-endpoint": "https://www.googleapis.com",
	})
)

type BaseSuite struct {
	gitjujutesting.IsolationSuite

	Config    *config.Config
	EnvConfig *environConfig
	FakeConn  *fakeConn
	Env       *environ

	Addresses    []network.Address
	BaseInstance *google.Instance
	Instance     *environInstance
	InstName     string

	Ports []network.PortRange
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Config = s.NewConfig(c, nil)
	s.EnvConfig = newEnvConfig(s.Config)
	s.FakeConn = &fakeConn{}
	uuid, _ := s.Config.UUID()
	s.Env = &environ{
		name: "google",
		uuid: uuid,
		ecfg: s.EnvConfig,
		gce:  s.FakeConn,
	}

	diskSpec := google.DiskSpec{
		SizeHintGB: 5,
		ImageURL:   "some/image/path",
		Boot:       true,
		Scratch:    false,
		Readonly:   false,
		AutoDelete: true,
	}
	metadata := map[string]string{
		"eggs": "steak",
	}
	s.Addresses = []network.Address{{
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}
	instanceSpec := google.InstanceSpec{
		ID:                "spam",
		Type:              "mtype",
		Disks:             []google.DiskSpec{diskSpec},
		Network:           google.NetworkSpec{Name: "somenetwork"},
		NetworkInterfaces: []string{"somenetif"},
		Metadata:          metadata,
		Tags:              []string{"spam"},
	}
	summary := google.InstanceSummary{
		ID:        "spam",
		ZoneName:  "a-zone",
		Status:    google.StatusRunning,
		Metadata:  metadata,
		Addresses: s.Addresses,
	}
	s.BaseInstance = google.NewInstance(summary, &instanceSpec)
	s.Instance = newInstance(s.BaseInstance, s.Env)
	s.InstName = "juju-" + s.Env.uuid + "-machine-spam"

	s.Ports = []network.PortRange{{
		FromPort: 80,
		ToPort:   80,
		Protocol: "tcp",
	}}

	s.PatchValue(&newConnection, func(*environConfig) gceConnection {
		return s.FakeConn
	})
}

func (s *BaseSuite) NewConfig(c *gc.C, updates testing.Attrs) *config.Config {
	var err error
	cfg := testing.EnvironConfig(c)
	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(updates)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *BaseSuite) UpdateConfig(c *gc.C, attrs map[string]interface{}) {
	cfg, err := s.Config.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.Config = cfg
	s.EnvConfig = newEnvConfig(cfg)
}

func (s *BaseSuite) CheckNoAPI(c *gc.C) {
	c.Check(s.FakeConn.Calls, gc.HasLen, 0)
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
		FuncName: "Instance",
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
		FuncName:     "Ports",
		FirewallName: fwname,
	})
	return fc.PortRanges, fc.err()
}

func (fc *fakeConn) OpenPorts(fwname string, ports []network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName:     "OpenPorts",
		FirewallName: fwname,
		PortRanges:   ports,
	})
	return fc.err()
}

func (fc *fakeConn) ClosePorts(fwname string, ports []network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeCall{
		FuncName:     "ClosePorts",
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
