// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/sshclient"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type FacadeSuite struct {
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestAddresses(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHAddressResults)
	ress1 := params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Address: "1.1.1.1"},
		},
	}

	res2 := new(params.SSHAddressesResults)
	ress2 := params.SSHAddressesResults{
		Results: []params.SSHAddressesResult{
			{Addresses: []string{"1.1.1.1", "2.2.2.2"}},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicAddress", expectedArg, res).SetArg(2, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("PrivateAddress", expectedArg, res).SetArg(2, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("AllAddresses", expectedArg, res2).SetArg(2, ress2).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(public, gc.Equals, "1.1.1.1")

	private, err := facade.PrivateAddress("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(private, gc.Equals, "1.1.1.1")

	addrs, err := facade.AllAddresses("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addrs, gc.DeepEquals, []string{"1.1.1.1", "2.2.2.2"})

}

func (s *FacadeSuite) TestAddressesError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHAddressResults)
	ress1 := params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Address: "1.1.1.1"},
		},
	}

	res2 := new(params.SSHAddressesResults)
	ress2 := params.SSHAddressesResults{
		Results: []params.SSHAddressesResult{
			{Addresses: []string{"1.1.1.1", "2.2.2.2"}},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicAddress", expectedArg, res).SetArg(2, ress1).Return(errors.New("boom"))
	mockFacadeCaller.EXPECT().FacadeCall("PrivateAddress", expectedArg, res).SetArg(2, ress1).Return(errors.New("boom"))
	mockFacadeCaller.EXPECT().FacadeCall("AllAddresses", expectedArg, res2).SetArg(2, ress2).Return(errors.New("boom"))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress("foo/0")
	c.Check(public, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")

	private, err := facade.PrivateAddress("foo/0")
	c.Check(private, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")

	addrs, err := facade.AllAddresses("foo/0")
	c.Check(addrs, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestAddressesTargetError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverError := apiservererrors.ServerError(errors.New("boom"))
	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHAddressResults)
	ress1 := params.SSHAddressResults{
		Results: []params.SSHAddressResult{{Error: serverError}},
	}

	res2 := new(params.SSHAddressesResults)
	ress2 := params.SSHAddressesResults{
		Results: []params.SSHAddressesResult{{Error: serverError}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicAddress", expectedArg, res).SetArg(2, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("PrivateAddress", expectedArg, res).SetArg(2, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("AllAddresses", expectedArg, res2).SetArg(2, ress2).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress("foo/0")
	c.Check(public, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")

	private, err := facade.PrivateAddress("foo/0")
	c.Check(private, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")

	addrs, err := facade.AllAddresses("foo/0")
	c.Check(addrs, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestAddressesMissingResults(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHAddressResults)
	res2 := new(params.SSHAddressesResults)
	expectedErr := "expected 1 result, got 0"
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicAddress", expectedArg, res).Return(errors.New(expectedErr))
	mockFacadeCaller.EXPECT().FacadeCall("PrivateAddress", expectedArg, res).Return(errors.New(expectedErr))
	mockFacadeCaller.EXPECT().FacadeCall("AllAddresses", expectedArg, res2).Return(errors.New(expectedErr))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress("foo/0")
	c.Check(public, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, expectedErr)

	private, err := facade.PrivateAddress("foo/0")
	c.Check(private, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, expectedErr)

	addrs, err := facade.AllAddresses("foo/0")
	c.Check(addrs, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expectedErr)
}

func (s *FacadeSuite) TestAddressesExtraResults(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHAddressResults)
	ress1 := params.SSHAddressResults{
		Results: []params.SSHAddressResult{
			{Address: "1.1.1.1"},
			{Address: "2.2.2.2"},
		},
	}

	res2 := new(params.SSHAddressesResults)
	ress2 := params.SSHAddressesResults{
		Results: []params.SSHAddressesResult{
			{Addresses: []string{"1.1.1.1"}},
			{Addresses: []string{"2.2.2.2"}},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicAddress", expectedArg, res).SetArg(2, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("PrivateAddress", expectedArg, res).SetArg(2, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("AllAddresses", expectedArg, res2).SetArg(2, ress2).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)
	expectedErr := "expected 1 result, got 2"

	public, err := facade.PublicAddress("foo/0")
	c.Check(public, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, expectedErr)

	private, err := facade.PrivateAddress("foo/0")
	c.Check(private, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, expectedErr)

	addrs, err := facade.AllAddresses("foo/0")
	c.Check(addrs, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expectedErr)
}

func (s *FacadeSuite) TestPublicKeys(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)
	ress := params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{{PublicKeys: []string{"rsa", "dsa"}}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicKeys", expectedArg, res).SetArg(2, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	keys, err := facade.PublicKeys("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(keys, gc.DeepEquals, []string{"rsa", "dsa"})
}

func (s *FacadeSuite) TestPublicKeysError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicKeys", gomock.Any(), gomock.Any()).Return(errors.New("boom"))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)
	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestPublicKeysTargetError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)
	ress := params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{{Error: apiservererrors.ServerError(errors.New("boom"))}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicKeys", expectedArg, res).SetArg(2, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)
	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestPublicKeysMissingResults(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicKeys", expectedArg, res).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *FacadeSuite) TestPublicKeysExtraResults(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)
	ress := params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{
			{PublicKeys: []string{"rsa"}},
			{PublicKeys: []string{"rsa"}},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicKeys", expectedArg, res).SetArg(2, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *FacadeSuite) TestProxy(c *gc.C) {
	checkProxy(c, true)
	checkProxy(c, false)
}

func checkProxy(c *gc.C, useProxy bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res := new(params.SSHProxyResult)
	ress := params.SSHProxyResult{
		UseProxy: useProxy,
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Proxy", nil, res).SetArg(2, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	result, err := facade.Proxy()
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, useProxy)
}

func (s *FacadeSuite) TestProxyError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Proxy", gomock.Any(), gomock.Any()).Return(errors.New("boom"))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	_, err := facade.Proxy()
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestModelCredentialForSSH(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res := new(params.CloudSpecResult)
	ress := params.CloudSpecResult{
		Result: &params.CloudSpec{
			Type:             "type",
			Name:             "name",
			Region:           "region",
			Endpoint:         "endpoint",
			IdentityEndpoint: "identity-endpoint",
			StorageEndpoint:  "storage-endpoint",
			Credential: &params.CloudCredential{
				AuthType: "auth-type",
				Attributes: map[string]string{
					k8scloud.CredAttrUsername: "",
					k8scloud.CredAttrPassword: "",
					k8scloud.CredAttrToken:    "token",
				},
			},
			CACertificates: []string{testing.CACert},
			SkipTLSVerify:  true,
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelCredentialForSSH", nil, res).SetArg(2, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	spec, err := facade.ModelCredentialForSSH()
	c.Assert(err, jc.ErrorIsNil)

	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{
			k8scloud.CredAttrUsername: "",
			k8scloud.CredAttrPassword: "",
			k8scloud.CredAttrToken:    "token",
		},
	)
	cloudSpec := environscloudspec.CloudSpec{
		Type:             "type",
		Name:             "name",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		Credential:       &credential,
		CACertificates:   []string{testing.CACert},
		SkipTLSVerify:    true,
	}
	c.Assert(spec, gc.DeepEquals, cloudSpec)
}

func (s *FacadeSuite) TestVirtualHostname(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.VirtualHostnameTargetArg{
		Tag: names.NewUnitTag("foo/0").String(),
	}

	res := new(params.SSHAddressResult)
	ress1 := params.SSHAddressResult{
		Address: "1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("VirtualHostname", expectedArg, res).SetArg(2, ress1).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	virtualHostname, err := facade.VirtualHostname("foo/0", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(virtualHostname, gc.Equals, "1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
}

func (s *FacadeSuite) TestVirtualHostnameError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.VirtualHostnameTargetArg{
		Tag: names.NewUnitTag("foo/0").String(),
	}

	res := new(params.SSHAddressResult)
	ress1 := params.SSHAddressResult{
		Error: apiservererrors.ServerError(errors.New("boom")),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("VirtualHostname", expectedArg, res).SetArg(2, ress1).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	_, err := facade.VirtualHostname("foo/0", nil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestPublicHostKeyForTarget(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.SSHVirtualHostKeyRequestArg{
		Hostname: "virtual-hostname",
	}

	res := new(params.PublicSSHHostKeyResult)
	ress1 := params.PublicSSHHostKeyResult{
		PublicKey: []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3"),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("PublicHostKeyForTarget", expectedArg, res).SetArg(2, ress1).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	result, err := facade.PublicHostKeyForTarget("virtual-hostname")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.PublicKey, gc.DeepEquals, []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3"))
}
