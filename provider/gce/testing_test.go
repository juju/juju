// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
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
		"uuid":           "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
	})
)

type BaseSuite struct {
	gitjujutesting.IsolationSuite

	Config    *config.Config
	EnvConfig *environConfig
	FakeConn  *fakeConn
	Env       *environ
	Prefix    string

	Addresses     []network.Address
	BaseInstance  *google.Instance
	Instance      *environInstance
	InstName      string
	StartInstArgs environs.StartInstanceParams

	Ports []network.PortRange

	FakeImages *fakeImages
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.initEnv(c)
	s.initInst(c)
	s.initNet(c)

	s.FakeImages = &fakeImages{}

	// Patch out all expensive external deps.
	s.PatchValue(&newConnection, func(*environConfig) gceConnection {
		return s.FakeConn
	})
	s.PatchValue(&supportedArchitectures, s.FakeImages.SupportedArchitectures)
}

func (s *BaseSuite) initEnv(c *gc.C) {
	s.FakeConn = &fakeConn{}
	s.Env = &environ{
		name: "google",
		gce:  s.FakeConn,
	}
	cfg := s.NewConfig(c, nil)
	s.setConfig(cfg)
}

func (s *BaseSuite) initInst(c *gc.C) {
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
		ZoneName:  "home-zone",
		Status:    google.StatusRunning,
		Metadata:  metadata,
		Addresses: s.Addresses,
	}
	s.BaseInstance = google.NewInstance(summary, &instanceSpec)
	s.Instance = newInstance(s.BaseInstance, s.Env)
	s.InstName = s.Prefix + "machine-spam"
	s.StartInstArgs = environs.StartInstanceParams{
	//Placement: "",
	//DistributionGroup: nil,
	}
}

func (s *BaseSuite) initNet(c *gc.C) {
	s.Ports = []network.PortRange{{
		FromPort: 80,
		ToPort:   80,
		Protocol: "tcp",
	}}
}

func (s *BaseSuite) setConfig(cfg *config.Config) {
	s.Config = cfg
	s.EnvConfig = newEnvConfig(cfg)
	uuid, _ := cfg.UUID()
	s.Env.uuid = uuid
	s.Env.ecfg = s.EnvConfig
	s.Prefix = "juju-" + uuid + "-"
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
	s.setConfig(cfg)
}

func (s *BaseSuite) CheckNoAPI(c *gc.C) {
	c.Check(s.FakeConn.Calls, gc.HasLen, 0)
}

// TODO(ericsnow) Add a method to patch tools metadata?

// TODO(ericsnow) Move fakeCallArgs, fakeCall, and fake to the testing repo?

type fakeCallArgs map[string]interface{}

type fakeCall struct {
	funcName string
	args     fakeCallArgs
}

type fake struct {
	calls []fakeCall

	Err        error
	FailOnCall int
}

func (f *fake) err() error {
	if len(f.calls) != f.FailOnCall+1 {
		return nil
	}
	return f.Err
}

func (f *fake) addCall(funcName string, args fakeCallArgs) {
	f.calls = append(f.calls, fakeCall{
		funcName: funcName,
		args:     args,
	})
}

func (f *fake) CheckCalls(c *gc.C, expected ...fakeCall) {
	c.Check(f.calls, jc.DeepEquals, expected)
}

type fakeImages struct {
	fake

	Arches []string
}

func (fi *fakeImages) SupportedArchitectures(env environs.Environ, cons *imagemetadata.ImageConstraint) ([]string, error) {
	fi.addCall("SupportedArchitectures", fakeCallArgs{
		"env":  env,
		"cons": cons,
	})
	return fi.Arches, fi.err()
}

// TODO(ericsnow) Refactor fakeConnCall and fakeConn to embed fakeCall and fake.

type fakeConnCall struct {
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
	Calls []fakeConnCall

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
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "",
		Auth:     auth,
	})
	return fc.err()
}

func (fc *fakeConn) VerifyCredentials() error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "",
	})
	return fc.err()
}

func (fc *fakeConn) Instance(id, zone string) (*google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Instance",
		ID:       id,
		ZoneName: zone,
	})
	return fc.Inst, fc.err()
}

func (fc *fakeConn) Instances(prefix string, statuses ...string) ([]google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Instances",
		Prefix:   prefix,
		Statuses: statuses,
	})
	return fc.Insts, fc.err()
}

func (fc *fakeConn) AddInstance(spec google.InstanceSpec, zones []string) (*google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "",
		InstanceSpec: spec,
		ZoneNames:    zones,
	})
	return fc.Inst, fc.err()
}

func (fc *fakeConn) RemoveInstances(prefix string, ids ...string) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "",
		Prefix:   prefix,
		IDs:      ids,
	})
	return fc.err()
}

func (fc *fakeConn) Ports(fwname string) ([]network.PortRange, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "Ports",
		FirewallName: fwname,
	})
	return fc.PortRanges, fc.err()
}

func (fc *fakeConn) OpenPorts(fwname string, ports []network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "OpenPorts",
		FirewallName: fwname,
		PortRanges:   ports,
	})
	return fc.err()
}

func (fc *fakeConn) ClosePorts(fwname string, ports []network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "ClosePorts",
		FirewallName: fwname,
		PortRanges:   ports,
	})
	return fc.err()
}

func (fc *fakeConn) AvailabilityZones(region string) ([]google.AvailabilityZone, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "AvailabilityZones",
		Region:   region,
	})
	return fc.Zones, fc.err()
}
