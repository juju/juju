// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"io/ioutil"
	"net"
	"path/filepath"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/provisioner"
)

type fakeAddr struct{ value string }

func (f *fakeAddr) Network() string { return "net" }
func (f *fakeAddr) String() string {
	if f.value != "" {
		return f.value
	}
	return "fakeAddr"
}

var _ net.Addr = (*fakeAddr)(nil)

type fakeAPI struct {
	*gitjujutesting.Stub

	fakeContainerConfig params.ContainerConfig
	fakeInterfaceInfo   network.InterfaceInfo
}

var _ provisioner.APICalls = (*fakeAPI)(nil)

var fakeInterfaceInfo network.InterfaceInfo = network.InterfaceInfo{
	DeviceIndex:    0,
	MACAddress:     "aa:bb:cc:dd:ee:ff",
	CIDR:           "0.1.2.0/24",
	InterfaceName:  "dummy0",
	Address:        network.NewAddress("0.1.2.3"),
	GatewayAddress: network.NewAddress("0.1.2.1"),
}

var fakeContainerConfig = params.ContainerConfig{
	UpdateBehavior:          &params.UpdateBehavior{true, true},
	ProviderType:            "fake",
	AuthorizedKeys:          coretesting.FakeAuthKeys,
	SSLHostnameVerification: true,
}

func NewFakeAPI() *fakeAPI {
	return &fakeAPI{
		Stub:                &gitjujutesting.Stub{},
		fakeContainerConfig: fakeContainerConfig,
		fakeInterfaceInfo:   fakeInterfaceInfo,
	}
}

func (f *fakeAPI) ContainerConfig() (params.ContainerConfig, error) {
	f.MethodCall(f, "ContainerConfig")
	if err := f.NextErr(); err != nil {
		return params.ContainerConfig{}, err
	}
	return f.fakeContainerConfig, nil
}

func (f *fakeAPI) PrepareContainerInterfaceInfo(tag names.MachineTag) ([]network.InterfaceInfo, error) {
	f.MethodCall(f, "PrepareContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []network.InterfaceInfo{f.fakeInterfaceInfo}, nil
}

func (f *fakeAPI) GetContainerInterfaceInfo(tag names.MachineTag) ([]network.InterfaceInfo, error) {
	f.MethodCall(f, "GetContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []network.InterfaceInfo{f.fakeInterfaceInfo}, nil
}

func (f *fakeAPI) ReleaseContainerAddresses(tag names.MachineTag) error {
	f.MethodCall(f, "ReleaseContainerAddresses", tag)
	if err := f.NextErr(); err != nil {
		return err
	}
	return nil
}

type patcher interface {
	PatchValue(destination, source interface{})
}

func patchResolvConf(s patcher, c *gc.C) {
	fakeResolvConf := filepath.Join(c.MkDir(), "resolv.conf")
	err := ioutil.WriteFile(fakeResolvConf, []byte("nameserver ns1.dummy\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(provisioner.ResolvConf, fakeResolvConf)
}
