// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/sshclient"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
}

func TestFacadeSuite(t *stdtesting.T) { tc.Run(t, &FacadeSuite{}) }
func (s *FacadeSuite) TestAddresses(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicAddress", expectedArg, res).SetArg(3, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PrivateAddress", expectedArg, res).SetArg(3, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AllAddresses", expectedArg, res2).SetArg(3, ress2).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(public, tc.Equals, "1.1.1.1")

	private, err := facade.PrivateAddress(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(private, tc.Equals, "1.1.1.1")

	addrs, err := facade.AllAddresses(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, []string{"1.1.1.1", "2.2.2.2"})

}

func (s *FacadeSuite) TestAddressesError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicAddress", expectedArg, res).SetArg(3, ress1).Return(errors.New("boom"))
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PrivateAddress", expectedArg, res).SetArg(3, ress1).Return(errors.New("boom"))
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AllAddresses", expectedArg, res2).SetArg(3, ress2).Return(errors.New("boom"))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress(c.Context(), "foo/0")
	c.Check(public, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "boom")

	private, err := facade.PrivateAddress(c.Context(), "foo/0")
	c.Check(private, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "boom")

	addrs, err := facade.AllAddresses(c.Context(), "foo/0")
	c.Check(addrs, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestAddressesTargetError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverError := apiservererrors.ServerError(errors.New("boom"))
	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicAddress", expectedArg, res).SetArg(3, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PrivateAddress", expectedArg, res).SetArg(3, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AllAddresses", expectedArg, res2).SetArg(3, ress2).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress(c.Context(), "foo/0")
	c.Check(public, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "boom")

	private, err := facade.PrivateAddress(c.Context(), "foo/0")
	c.Check(private, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "boom")

	addrs, err := facade.AllAddresses(c.Context(), "foo/0")
	c.Check(addrs, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestAddressesMissingResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHAddressResults)
	res2 := new(params.SSHAddressesResults)
	expectedErr := "expected 1 result, got 0"
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicAddress", expectedArg, res).Return(errors.New(expectedErr))
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PrivateAddress", expectedArg, res).Return(errors.New(expectedErr))
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AllAddresses", expectedArg, res2).Return(errors.New(expectedErr))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	public, err := facade.PublicAddress(c.Context(), "foo/0")
	c.Check(public, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, expectedErr)

	private, err := facade.PrivateAddress(c.Context(), "foo/0")
	c.Check(private, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, expectedErr)

	addrs, err := facade.AllAddresses(c.Context(), "foo/0")
	c.Check(addrs, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expectedErr)
}

func (s *FacadeSuite) TestAddressesExtraResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicAddress", expectedArg, res).SetArg(3, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PrivateAddress", expectedArg, res).SetArg(3, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AllAddresses", expectedArg, res2).SetArg(3, ress2).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)
	expectedErr := "expected 1 result, got 2"

	public, err := facade.PublicAddress(c.Context(), "foo/0")
	c.Check(public, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, expectedErr)

	private, err := facade.PrivateAddress(c.Context(), "foo/0")
	c.Check(private, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, expectedErr)

	addrs, err := facade.AllAddresses(c.Context(), "foo/0")
	c.Check(addrs, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expectedErr)
}

func (s *FacadeSuite) TestPublicKeys(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)
	ress := params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{{PublicKeys: []string{"rsa", "dsa"}}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicKeys", expectedArg, res).SetArg(3, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	keys, err := facade.PublicKeys(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{"rsa", "dsa"})
}

func (s *FacadeSuite) TestPublicKeysError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicKeys", gomock.Any(), gomock.Any()).Return(errors.New("boom"))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)
	keys, err := facade.PublicKeys(c.Context(), "foo/0")
	c.Check(keys, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestPublicKeysTargetError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)
	ress := params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{{Error: apiservererrors.ServerError(errors.New("boom"))}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicKeys", expectedArg, res).SetArg(3, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)
	keys, err := facade.PublicKeys(c.Context(), "foo/0")
	c.Check(keys, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestPublicKeysMissingResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicKeys", expectedArg, res).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	keys, err := facade.PublicKeys(c.Context(), "foo/0")
	c.Check(keys, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 0")
}

func (s *FacadeSuite) TestPublicKeysExtraResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedArg := params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
	}}}

	res := new(params.SSHPublicKeysResults)
	ress := params.SSHPublicKeysResults{
		Results: []params.SSHPublicKeysResult{
			{PublicKeys: []string{"rsa"}},
			{PublicKeys: []string{"rsa"}},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "PublicKeys", expectedArg, res).SetArg(3, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	keys, err := facade.PublicKeys(c.Context(), "foo/0")
	c.Check(keys, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *FacadeSuite) TestProxy(c *tc.C) {
	checkProxy(c, true)
	checkProxy(c, false)
}

func checkProxy(c *tc.C, useProxy bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res := new(params.SSHProxyResult)
	ress := params.SSHProxyResult{
		UseProxy: useProxy,
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Proxy", nil, res).SetArg(3, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	result, err := facade.Proxy(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, useProxy)
}

func (s *FacadeSuite) TestProxyError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Proxy", gomock.Any(), gomock.Any()).Return(errors.New("boom"))
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	_, err := facade.Proxy(c.Context())
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestModelCredentialForSSH(c *tc.C) {
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
					"username": "",
					"password": "",
					"Token":    "token",
				},
			},
			CACertificates: []string{testing.CACert},
			SkipTLSVerify:  true,
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelCredentialForSSH", nil, res).SetArg(3, ress).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	spec, err := facade.ModelCredentialForSSH(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{
			"username": "",
			"password": "",
			"Token":    "token",
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
	c.Assert(spec, tc.DeepEquals, cloudSpec)
}

func (s *FacadeSuite) TestVirtualHostname(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "VirtualHostname", expectedArg, res).SetArg(3, ress1).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	virtualHostname, err := facade.VirtualHostname(c.Context(), "foo/0", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(virtualHostname, tc.Equals, "1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
}

func (s *FacadeSuite) TestVirtualHostnameError(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "VirtualHostname", expectedArg, res).SetArg(3, ress1).Return(nil)
	facade := sshclient.NewFacadeFromCaller(mockFacadeCaller)

	_, err := facade.VirtualHostname(c.Context(), "foo/0", nil)
	c.Check(err, tc.ErrorMatches, "boom")
}
