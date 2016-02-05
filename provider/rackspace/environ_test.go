// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace_test

import (
	"io"
	"os"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/rackspace"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/utils/ssh"
)

type environSuite struct {
	testing.BaseSuite
	environ      environs.Environ
	innerEnviron *fakeEnviron
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.innerEnviron = new(fakeEnviron)
	s.environ = rackspace.NewEnviron(s.innerEnviron)
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	s.PatchValue(rackspace.Bootstrap, func(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
		return s.innerEnviron.Bootstrap(ctx, args)
	})
	s.environ.Bootstrap(nil, environs.BootstrapParams{})
	c.Check(s.innerEnviron.Pop().name, gc.Equals, "Bootstrap")
}

func (s *environSuite) TestStartInstance(c *gc.C) {
	configurator := &fakeConfigurator{}
	s.PatchValue(rackspace.WaitSSH, func(stdErr io.Writer, interrupted <-chan os.Signal, client ssh.Client, checkHostScript string, inst common.Addresser, timeout config.SSHTimeoutOpts) (addr string, err error) {
		addresses, err := inst.Addresses()
		if err != nil {
			return "", err
		}
		return addresses[0].Value, nil
	})
	s.PatchValue(rackspace.NewInstanceConfigurator, func(host string) common.InstanceConfigurator {
		return configurator
	})
	config, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "some-name",
		"type":            "some-type",
		"authorized-keys": "key",
	})
	c.Assert(err, gc.IsNil)
	err = s.environ.SetConfig(config)
	c.Assert(err, gc.IsNil)
	_, err = s.environ.StartInstance(environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{
			Config: config,
		},
		Tools: tools.List{&tools.Tools{
			Version: version.Binary{Series: "trusty"},
		}},
	})
	c.Check(err, gc.IsNil)
	c.Check(s.innerEnviron.Pop().name, gc.Equals, "StartInstance")
	dropParams := configurator.Pop()
	c.Check(dropParams.name, gc.Equals, "DropAllPorts")
	c.Check(dropParams.params[1], gc.Equals, "1.1.1.1")
}

type methodCall struct {
	name   string
	params []interface{}
}

type fakeEnviron struct {
	config      *config.Config
	methodCalls []methodCall
}

func (p *fakeEnviron) Push(name string, params ...interface{}) {
	p.methodCalls = append(p.methodCalls, methodCall{name, params})
}

func (p *fakeEnviron) Pop() methodCall {
	m := p.methodCalls[0]
	p.methodCalls = p.methodCalls[1:]
	return m
}

func (p *fakeEnviron) Open(cfg *config.Config) (environs.Environ, error) {
	p.Push("Open", cfg)
	return nil, nil
}

func (e *fakeEnviron) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	e.Push("Bootstrap", ctx, params)
	return nil, nil
}

func (e *fakeEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	e.Push("StartInstance", args)
	return &environs.StartInstanceResult{
		Instance: &fakeInstance{},
	}, nil
}

func (e *fakeEnviron) StopInstances(ids ...instance.Id) error {
	e.Push("StopInstances", ids)
	return nil
}

func (e *fakeEnviron) AllInstances() ([]instance.Instance, error) {
	e.Push("AllInstances")
	return nil, nil
}

func (e *fakeEnviron) MaintainInstance(args environs.StartInstanceParams) error {
	e.Push("MaintainInstance", args)
	return nil
}

func (e *fakeEnviron) Config() *config.Config {
	return e.config
}

func (e *fakeEnviron) SupportedArchitectures() ([]string, error) {
	e.Push("SupportedArchitectures")
	return nil, nil
}

func (e *fakeEnviron) SupportsUnitPlacement() error {
	e.Push("SupportsUnitPlacement")
	return nil
}

func (e *fakeEnviron) ConstraintsValidator() (constraints.Validator, error) {
	e.Push("ConstraintsValidator")
	return nil, nil
}

func (e *fakeEnviron) SetConfig(cfg *config.Config) error {
	e.config = cfg
	return nil
}

func (e *fakeEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	e.Push("Instances", ids)
	return []instance.Instance{&fakeInstance{}}, nil
}

func (e *fakeEnviron) ControllerInstances() ([]instance.Id, error) {
	e.Push("ControllerInstances")
	return nil, nil
}

func (e *fakeEnviron) Destroy() error {
	e.Push("Destroy")
	return nil
}

func (e *fakeEnviron) OpenPorts(ports []network.PortRange) error {
	e.Push("OpenPorts", ports)
	return nil
}

func (e *fakeEnviron) ClosePorts(ports []network.PortRange) error {
	e.Push("ClosePorts", ports)
	return nil
}

func (e *fakeEnviron) Ports() ([]network.PortRange, error) {
	e.Push("Ports")
	return nil, nil
}

func (e *fakeEnviron) Provider() environs.EnvironProvider {
	e.Push("Provider")
	return nil
}

func (e *fakeEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	e.Push("PrecheckInstance", series, cons, placement)
	return nil
}

type fakeConfigurator struct {
	methodCalls []methodCall
}

func (p *fakeConfigurator) Push(name string, params ...interface{}) {
	p.methodCalls = append(p.methodCalls, methodCall{name, params})
}

func (p *fakeConfigurator) Pop() methodCall {
	m := p.methodCalls[0]
	p.methodCalls = p.methodCalls[1:]
	return m
}

func (e *fakeConfigurator) DropAllPorts(exceptPorts []int, addr string) error {
	e.Push("DropAllPorts", exceptPorts, addr)
	return nil
}

func (e *fakeConfigurator) ConfigureExternalIpAddress(apiPort int) error {
	e.Push("ConfigureExternalIpAddress", apiPort)
	return nil
}

func (e *fakeConfigurator) ChangePorts(ipAddress string, insert bool, ports []network.PortRange) error {
	e.Push("ChangePorts", ipAddress, insert, ports)
	return nil
}

func (e *fakeConfigurator) FindOpenPorts() ([]network.PortRange, error) {
	e.Push("FindOpenPorts")
	return nil, nil
}

func (e *fakeConfigurator) AddIpAddress(nic string, addr string) error {
	e.Push("AddIpAddress", nic, addr)
	return nil
}

func (e *fakeConfigurator) ReleaseIpAddress(addr string) error {
	e.Push("AddIpAddress", addr)
	return nil
}

type fakeInstance struct {
	methodCalls []methodCall
}

func (p *fakeInstance) Push(name string, params ...interface{}) {
	p.methodCalls = append(p.methodCalls, methodCall{name, params})
}

func (p *fakeInstance) Pop() methodCall {
	m := p.methodCalls[0]
	p.methodCalls = p.methodCalls[1:]
	return m
}

func (e *fakeInstance) Id() instance.Id {
	e.Push("Id")
	return instance.Id("")
}

func (e *fakeInstance) Status() string {
	e.Push("Status")
	return ""
}

func (e *fakeInstance) Refresh() error {
	e.Push("Refresh")
	return nil
}

func (e *fakeInstance) Addresses() ([]network.Address, error) {
	e.Push("Addresses")
	return []network.Address{network.Address{
		Value: "1.1.1.1",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}}, nil
}

func (e *fakeInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	e.Push("OpenPorts", machineId, ports)
	return nil
}

func (e *fakeInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	e.Push("ClosePorts", machineId, ports)
	return nil
}

func (e *fakeInstance) Ports(machineId string) ([]network.PortRange, error) {
	e.Push("Ports", machineId)
	return nil, nil
}
