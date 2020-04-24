// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/sshclient"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

type FacadeSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestAddresses(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, arg)
		c.Check(id, gc.Equals, "")

		switch request {
		case "PublicAddress", "PrivateAddress":
			*result.(*params.SSHAddressResults) = params.SSHAddressResults{
				Results: []params.SSHAddressResult{
					{Address: "1.1.1.1"},
				},
			}

		case "AllAddresses":
			*result.(*params.SSHAddressesResults) = params.SSHAddressesResults{
				Results: []params.SSHAddressesResult{
					{Addresses: []string{"1.1.1.1", "2.2.2.2"}},
				},
			}
		}

		return nil
	})

	facade := sshclient.NewFacade(apiCaller)
	expectedArg := []interface{}{params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}}}

	public, err := facade.PublicAddress("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(public, gc.Equals, "1.1.1.1")
	stub.CheckCalls(c, []jujutesting.StubCall{{"SSHClient.PublicAddress", expectedArg}})
	stub.ResetCalls()

	private, err := facade.PrivateAddress("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(private, gc.Equals, "1.1.1.1")
	stub.CheckCalls(c, []jujutesting.StubCall{{"SSHClient.PrivateAddress", expectedArg}})
	stub.ResetCalls()

	addrs, err := facade.AllAddresses("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addrs, gc.DeepEquals, []string{"1.1.1.1", "2.2.2.2"})
	stub.CheckCalls(c, []jujutesting.StubCall{{"SSHClient.AllAddresses", expectedArg}})
}

func (s *FacadeSuite) TestAddressesError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	facade := sshclient.NewFacade(apiCaller)

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
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		serverError := common.ServerError(errors.New("boom"))

		switch request {
		case "PublicAddress", "PrivateAddress":
			*result.(*params.SSHAddressResults) = params.SSHAddressResults{
				Results: []params.SSHAddressResult{{Error: serverError}},
			}
		case "AllAddresses":
			*result.(*params.SSHAddressesResults) = params.SSHAddressesResults{
				Results: []params.SSHAddressesResult{{Error: serverError}},
			}
		}

		return nil
	})
	facade := sshclient.NewFacade(apiCaller)

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
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	facade := sshclient.NewFacade(apiCaller)
	expectedErr := "expected 1 result, got 0"

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
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		switch request {
		case "PublicAddress", "PrivateAddress":
			*result.(*params.SSHAddressResults) = params.SSHAddressResults{
				Results: []params.SSHAddressResult{
					{Address: "1.1.1.1"},
					{Address: "2.2.2.2"},
				},
			}
		case "AllAddresses":
			*result.(*params.SSHAddressesResults) = params.SSHAddressesResults{
				Results: []params.SSHAddressesResult{
					{Addresses: []string{"1.1.1.1"}},
					{Addresses: []string{"2.2.2.2"}},
				},
			}
		}
		return nil
	})
	facade := sshclient.NewFacade(apiCaller)
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
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, arg)
		c.Check(id, gc.Equals, "")
		*result.(*params.SSHPublicKeysResults) = params.SSHPublicKeysResults{
			Results: []params.SSHPublicKeysResult{{PublicKeys: []string{"rsa", "dsa"}}},
		}
		return nil
	})
	facade := sshclient.NewFacade(apiCaller)
	keys, err := facade.PublicKeys("foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(keys, gc.DeepEquals, []string{"rsa", "dsa"})
	stub.CheckCalls(c, []jujutesting.StubCall{{
		"SSHClient.PublicKeys",
		[]interface{}{params.Entities{[]params.Entity{{
			Tag: names.NewUnitTag("foo/0").String(),
		}}}},
	}})
}

func (s *FacadeSuite) TestPublicKeysError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	facade := sshclient.NewFacade(apiCaller)
	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestPublicKeysTargetError(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, arg)
		c.Check(id, gc.Equals, "")
		*result.(*params.SSHPublicKeysResults) = params.SSHPublicKeysResults{
			Results: []params.SSHPublicKeysResult{{Error: common.ServerError(errors.New("boom"))}},
		}
		return nil
	})
	facade := sshclient.NewFacade(apiCaller)
	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *FacadeSuite) TestPublicKeysMissingResults(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	facade := sshclient.NewFacade(apiCaller)
	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *FacadeSuite) TestPublicKeysExtraResults(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*result.(*params.SSHPublicKeysResults) = params.SSHPublicKeysResults{
			Results: []params.SSHPublicKeysResult{
				{PublicKeys: []string{"rsa"}},
				{PublicKeys: []string{"rsa"}},
			},
		}
		return nil
	})
	facade := sshclient.NewFacade(apiCaller)
	keys, err := facade.PublicKeys("foo/0")
	c.Check(keys, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *FacadeSuite) TestProxy(c *gc.C) {
	checkProxy(c, true)
	checkProxy(c, false)
}

func checkProxy(c *gc.C, useProxy bool) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, arg)
		*result.(*params.SSHProxyResult) = params.SSHProxyResult{
			UseProxy: useProxy,
		}
		return nil
	})
	facade := sshclient.NewFacade(apiCaller)
	result, err := facade.Proxy()
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, useProxy)
	stub.CheckCalls(c, []jujutesting.StubCall{{"SSHClient.Proxy", []interface{}{nil}}})
}

func (s *FacadeSuite) TestProxyError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	facade := sshclient.NewFacade(apiCaller)
	_, err := facade.Proxy()
	c.Check(err, gc.ErrorMatches, "boom")
}
