// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace_test

import (
	"context"
	"io"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/ssh"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	gooseerrors "gopkg.in/goose.v2/errors"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/rackspace"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type environSuite struct {
	testing.BaseSuite
	environ      environs.Environ
	innerEnviron *fakeEnviron

	callCtx           *envcontext.CloudCallContext
	invalidCredential bool
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.innerEnviron = new(fakeEnviron)
	s.environ = rackspace.NewEnviron(s.innerEnviron)
	s.callCtx = &envcontext.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.invalidCredential = true
			return nil
		},
	}
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.invalidCredential = false
	s.BaseSuite.TearDownTest(c)
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	s.PatchValue(rackspace.Bootstrap, func(ctx environs.BootstrapContext, env environs.Environ, callCtx envcontext.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
		return s.innerEnviron.Bootstrap(ctx, callCtx, args)
	})
	s.environ.Bootstrap(nil, s.callCtx, environs.BootstrapParams{
		ControllerConfig: testing.FakeControllerConfig(),
	})
	c.Check(s.innerEnviron.Pop().name, gc.Equals, "Bootstrap")
}

func (s *environSuite) TestStartInstance(c *gc.C) {
	configurator := &fakeConfigurator{}
	s.PatchValue(rackspace.WaitSSH, func(
		ctx context.Context,
		stdErr io.Writer,
		client ssh.Client,
		checkHostScript string,
		inst common.InstanceRefresher,
		callCtx envcontext.ProviderCallContext,
		timeout environs.BootstrapDialOpts,
		hostSSHOptions common.HostSSHOptionsFunc,
	) (addr string, err error) {
		addresses, err := inst.Addresses(s.callCtx)
		if err != nil {
			return "", err
		}
		return addresses[0].Value, nil
	})
	s.PatchValue(rackspace.NewInstanceConfigurator, func(host string) common.InstanceConfigurator {
		return configurator
	})
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "some-name",
		"type":            "some-type",
		"uuid":            testing.ModelTag.Id(),
		"controller-uuid": testing.ControllerTag.Id(),
		"authorized-keys": "key",
	})
	c.Assert(err, gc.IsNil)
	err = s.environ.SetConfig(cfg)
	c.Assert(err, gc.IsNil)
	_, err = s.environ.StartInstance(s.callCtx, environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{},
		Tools: tools.List{&tools.Tools{
			Version: version.Binary{Release: "ubuntu"},
		}},
	})
	c.Check(err, gc.IsNil)
	c.Check(s.innerEnviron.Pop().name, gc.Equals, "StartInstance")
	dropParams := configurator.Pop()
	c.Check(dropParams.name, gc.Equals, "DropAllPorts")
	c.Check(dropParams.params[1], gc.Equals, "1.1.1.1")
}

var testUnauthorisedGooseError = gooseerrors.NewUnauthorisedf(nil, "", "invalid auth")

func (s *environSuite) TestStartInstanceInvalidCredential(c *gc.C) {
	configurator := &fakeConfigurator{
		dropAllPortsF: func(exceptPorts []int, addr string) error {
			return testUnauthorisedGooseError
		},
	}
	s.PatchValue(rackspace.WaitSSH, func(
		ctx context.Context,
		stdErr io.Writer,
		client ssh.Client,
		checkHostScript string,
		inst common.InstanceRefresher,
		callCtx envcontext.ProviderCallContext,
		timeout environs.BootstrapDialOpts,
		hostSSHOptions common.HostSSHOptionsFunc,
	) (addr string, err error) {
		addresses, err := inst.Addresses(s.callCtx)
		if err != nil {
			return "", err
		}
		return addresses[0].Value, nil
	})
	s.PatchValue(rackspace.NewInstanceConfigurator, func(host string) common.InstanceConfigurator {
		return configurator
	})
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "some-name",
		"type":            "some-type",
		"uuid":            testing.ModelTag.Id(),
		"controller-uuid": testing.ControllerTag.Id(),
		"authorized-keys": "key",
	})
	c.Assert(err, gc.IsNil)
	err = s.environ.SetConfig(cfg)
	c.Assert(err, gc.IsNil)

	c.Assert(s.invalidCredential, jc.IsFalse)
	_, err = s.environ.StartInstance(s.callCtx, environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{},
		Tools: tools.List{&tools.Tools{
			Version: version.Binary{Release: "ubuntu"},
		}},
	})
	c.Assert(s.invalidCredential, jc.IsTrue)
	c.Check(err, gc.ErrorMatches, "invalid auth")
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

func (e *fakeEnviron) Create(callCtx envcontext.ProviderCallContext, args environs.CreateParams) error {
	e.Push("Create", callCtx, args)
	return nil
}

func (e *fakeEnviron) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	e.Push("PrepareForBootstrap", ctx)
	return nil
}

func (e *fakeEnviron) Bootstrap(ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	e.Push("Bootstrap", ctx, callCtx, params)
	return nil, nil
}

func (e *fakeEnviron) StartInstance(callCtx envcontext.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	e.Push("StartInstance", callCtx, args)
	return &environs.StartInstanceResult{
		Instance: &fakeInstance{},
	}, nil
}

func (e *fakeEnviron) StopInstances(callCtx envcontext.ProviderCallContext, ids ...instance.Id) error {
	e.Push("StopInstances", callCtx, ids)
	return nil
}

func (e *fakeEnviron) AllInstances(callCtx envcontext.ProviderCallContext) ([]instances.Instance, error) {
	e.Push("AllInstances", callCtx)
	return nil, nil
}

func (e *fakeEnviron) AllRunningInstances(callCtx envcontext.ProviderCallContext) ([]instances.Instance, error) {
	e.Push("AllRunningInstances", callCtx)
	return nil, nil
}

func (e *fakeEnviron) Config() *config.Config {
	return e.config
}

func (e *fakeEnviron) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	e.Push("ConstraintsValidator", ctx)
	return nil, nil
}

func (e *fakeEnviron) SetConfig(cfg *config.Config) error {
	e.config = cfg
	return nil
}

func (e *fakeEnviron) Instances(callCtx envcontext.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	e.Push("Instances", callCtx, ids)
	return []instances.Instance{&fakeInstance{}}, nil
}

func (e *fakeEnviron) ControllerInstances(callCtx envcontext.ProviderCallContext, st string) ([]instance.Id, error) {
	e.Push("ControllerInstances", callCtx, st)
	return nil, nil
}

func (e *fakeEnviron) AdoptResources(callCtx envcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	e.Push("AdoptResources", callCtx, controllerUUID, fromVersion)
	return nil
}

func (e *fakeEnviron) Destroy(callCtx envcontext.ProviderCallContext) error {
	e.Push("Destroy", callCtx)
	return nil
}

func (e *fakeEnviron) DestroyController(callCtx envcontext.ProviderCallContext, controllerUUID string) error {
	e.Push("Destroy", callCtx, controllerUUID)
	return nil
}

func (e *fakeEnviron) OpenPorts(callCtx envcontext.ProviderCallContext, rules firewall.IngressRules) error {
	e.Push("OpenPorts", callCtx, rules)
	return nil
}

func (e *fakeEnviron) ClosePorts(callCtx envcontext.ProviderCallContext, rules firewall.IngressRules) error {
	e.Push("ClosePorts", callCtx, rules)
	return nil
}

func (e *fakeEnviron) IngressRules(callCtx envcontext.ProviderCallContext) (firewall.IngressRules, error) {
	e.Push("Ports", callCtx)
	return nil, nil
}

func (e *fakeEnviron) Provider() environs.EnvironProvider {
	e.Push("Provider")
	return nil
}

func (e *fakeEnviron) PrecheckInstance(callCtx envcontext.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	e.Push("PrecheckInstance", callCtx, args)
	return nil
}

func (e *fakeEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	e.Push("StorageProviderTypes")
	return nil, nil
}

func (e *fakeEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	e.Push("StorageProvider", t)
	return nil, errors.NotImplementedf("StorageProvider")
}

func (e *fakeEnviron) InstanceTypes(envcontext.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, nil
}

type fakeConfigurator struct {
	methodCalls []methodCall

	dropAllPortsF func(exceptPorts []int, addr string) error
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
	if e.dropAllPortsF != nil {
		return e.dropAllPortsF(exceptPorts, addr)
	}
	return nil
}

func (e *fakeConfigurator) ConfigureExternalIpAddress(apiPort int) error {
	e.Push("ConfigureExternalIpAddress", apiPort)
	return nil
}

func (e *fakeConfigurator) ChangeIngressRules(ipAddress string, insert bool, rules firewall.IngressRules) error {
	e.Push("ChangeIngressRules", ipAddress, insert, rules)
	return nil
}

func (e *fakeConfigurator) FindIngressRules() (firewall.IngressRules, error) {
	e.Push("FindIngressRules")
	return nil, nil
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

func (e *fakeInstance) Status(callCtx envcontext.ProviderCallContext) instance.Status {
	e.Push("Status", callCtx)
	return instance.Status{
		Status:  status.Provisioning,
		Message: "a message",
	}
}

func (e *fakeInstance) Refresh(callCtx envcontext.ProviderCallContext) error {
	e.Push("Refresh", callCtx)
	return nil
}

func (e *fakeInstance) Addresses(callCtx envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	e.Push("Addresses", callCtx)
	return []network.ProviderAddress{{
		MachineAddress: network.MachineAddress{
			Value: "1.1.1.1",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
	}}, nil
}

func (e *fakeInstance) OpenPorts(callCtx envcontext.ProviderCallContext, machineId string, ports firewall.IngressRules) error {
	e.Push("OpenPorts", callCtx, machineId, ports)
	return nil
}

func (e *fakeInstance) ClosePorts(callCtx envcontext.ProviderCallContext, machineId string, ports firewall.IngressRules) error {
	e.Push("ClosePorts", callCtx, machineId, ports)
	return nil
}

func (e *fakeInstance) IngressRules(callCtx envcontext.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	e.Push("Ports", callCtx, machineId)
	return nil, nil
}
