// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/envcontext"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/common"
)

type vpcSuite struct {
	testing.IsolationSuite

	stubAPI *stubVPCAPIClient

	cloudCallCtx envcontext.ProviderCallContext
}

var _ = gc.Suite(&vpcSuite{})

func (s *vpcSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stubAPI = &stubVPCAPIClient{Stub: &testing.Stub{}}
	s.cloudCallCtx = envcontext.WithoutCredentialInvalidator(context.Background())
}

func (s *vpcSuite) TestValidateBootstrapVPCUnexpectedError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	err := validateBootstrapVPC(context.Background(), s.stubAPI, "region", anyVPCID, false, envtesting.BootstrapTestContext(c))
	s.checkErrorMatchesCannotVerifyVPC(c, err)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateBootstrapVPCCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))
	err := validateBootstrapVPC(context.Background(), s.stubAPI, "region", anyVPCID, false, envtesting.BootstrapTestContext(c))
	s.checkErrorMatchesCannotVerifyVPC(c, err)
	c.Check(err, jc.ErrorIs, common.ErrorCredentialNotValid)
}

func (*vpcSuite) checkErrorMatchesCannotVerifyVPC(c *gc.C, err error) {
	expectedError := `Juju could not verify whether the given vpc-id(.|\n)*AWS failed!`
	c.Check(err, gc.ErrorMatches, expectedError)
}

func (s *vpcSuite) TestValidateModelVPCUnexpectedError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	err := validateModelVPC(context.Background(), s.stubAPI, "model", anyVPCID)
	s.checkErrorMatchesCannotVerifyVPC(c, err)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateModelVPCNotUsableError(c *gc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	err := validateModelVPC(context.Background(), s.stubAPI, "model", "foo")
	c.Check(err, jc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateModelVPCCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("foo: %w", common.ErrorCredentialNotValid))
	err := validateModelVPC(context.Background(), s.stubAPI, "model", "foo")
	expectedError := `Juju could not verify whether the given vpc-id(.|\n)*`
	c.Check(err, gc.ErrorMatches, expectedError)
	c.Check(err, jc.ErrorIs, common.ErrorCredentialNotValid)
}

func (s *vpcSuite) TestValidateModelVPCIDNotSetOrNone(c *gc.C) {
	const emptyVPCID = ""
	err := validateModelVPC(context.Background(), s.stubAPI, "model", emptyVPCID)
	c.Check(err, jc.ErrorIsNil)

	err = validateModelVPC(context.Background(), s.stubAPI, "model", vpcIDNone)
	c.Check(err, jc.ErrorIsNil)

	s.stubAPI.CheckNoCalls(c)
}

// NOTE: validateVPC tests only verify expected error types for all code paths,
// but do not check passed API arguments or exact error messages, as those are
// extensively tested separately below.

func (s *vpcSuite) TestValidateVPCWithEmptyVPCIDOrNilAPIClient(c *gc.C) {
	err := validateVPC(context.Background(), s.stubAPI, "")
	c.Assert(err, gc.ErrorMatches, "invalid arguments: empty VPC ID or nil client")

	err = validateVPC(context.Background(), nil, anyVPCID)
	c.Assert(err, gc.ErrorMatches, "invalid arguments: empty VPC ID or nil client")

	s.stubAPI.CheckNoCalls(c)
}

func (s *vpcSuite) TestValidateVPCWhenVPCIDNotFound(c *gc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	err := validateVPC(context.Background(), s.stubAPI, anyVPCID)
	c.Check(err, jc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoSubnets(c *gc.C) {
	s.stubAPI.SetVPCsResponse(1, types.VpcStateAvailable, notDefaultVPC)
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	err := validateVPC(context.Background(), s.stubAPI, anyVPCID)
	c.Check(err, jc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext")
}
func (s *vpcSuite) TestValidateVPCWhenVPCNotAvailable(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetVPCsResponse(1, "bad-state", notDefaultVPC)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoPublicSubnets(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetSubnetsResponse(1, anyZone, noPublicIPOnLaunch)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "DescribeSubnetsWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoGateway(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetGatewaysResponse(noResults, anyState)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "DescribeInternetGatewaysWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoAttachedGateway(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetGatewaysResponse(1, "pending")

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "DescribeInternetGatewaysWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoRouteTables(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse() // no route tables at all

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoMainRouteTable(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, notMainRouteTable, nil, nil),
	)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasMainRouteTableWithoutRoutes(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, mainRouteTable, nil, nil),
	)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateVPCSuccess(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()

	err := validateVPC(context.Background(), s.stubAPI, anyVPCID)
	c.Assert(err, jc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext", "DescribeInternetGatewaysWithContext", "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateModelVPCSuccess(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()

	err := validateModelVPC(context.Background(), s.stubAPI, "model", anyVPCID)
	c.Assert(err, jc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext", "DescribeInternetGatewaysWithContext", "DescribeRouteTablesWithContext")
	c.Check(c.GetTestLog(), jc.Contains, `INFO juju.provider.ec2 Using VPC "vpc-anything" for model "model"`)
}

func (s *vpcSuite) TestValidateModelVPCNotRecommendedStillOK(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetSubnetsResponse(1, anyZone, noPublicIPOnLaunch)

	err := validateModelVPC(context.Background(), s.stubAPI, "model", anyVPCID)
	c.Assert(err, jc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, `INFO juju.provider.ec2 Juju will use, but does not recommend `+
		`using VPC "vpc-anything": VPC contains no public subnets`)
	c.Check(testLog, jc.Contains, `INFO juju.provider.ec2 Using VPC "vpc-anything" for model "model"`)
}

func (s *vpcSuite) TestGetVPCByIDWithMissingID(c *gc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	vpc, err := getVPCByID(context.Background(), s.stubAPI, "foo")
	c.Check(err, jc.ErrorIs, errorVPCNotUsable)
	c.Check(vpc, gc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "foo")
}

func (s *vpcSuite) TestGetVPCByIDUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	vpc, err := getVPCByID(context.Background(), s.stubAPI, "bar")
	c.Assert(err, gc.ErrorMatches, `unexpected AWS response getting VPC "bar": AWS failed!`)
	c.Check(vpc, gc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "bar")
}

func (s *vpcSuite) TestGetVPCByIDCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))

	vpc, err := getVPCByID(context.Background(), s.stubAPI, "bar")
	c.Check(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(vpc, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCByIDNoResults(c *gc.C) {
	s.stubAPI.SetVPCsResponse(noResults, anyState, notDefaultVPC)

	_, err := getVPCByID(context.Background(), s.stubAPI, "vpc-42")
	c.Assert(err, gc.ErrorMatches, `VPC "vpc-42" not found`)
	c.Check(err, jc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-42")
}

func (s *vpcSuite) TestGetVPCByIDMultipleResults(c *gc.C) {
	s.stubAPI.SetVPCsResponse(5, anyState, notDefaultVPC)

	vpc, err := getVPCByID(context.Background(), s.stubAPI, "vpc-33")
	c.Assert(err, gc.ErrorMatches, "expected 1 result from AWS, got 5")
	c.Check(vpc, gc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-33")
}

func (s *vpcSuite) TestGetVPCByIDSuccess(c *gc.C) {
	s.stubAPI.SetVPCsResponse(1, anyState, notDefaultVPC)

	vpc, err := getVPCByID(context.Background(), s.stubAPI, "vpc-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*vpc, jc.DeepEquals, s.stubAPI.vpcsResponse.Vpcs[0])

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-1")
}

func (s *vpcSuite) TestIsVPCNotFoundError(c *gc.C) {
	c.Check(isVPCNotFoundError(nil), jc.IsFalse)

	nonEC2Error := errors.New("boom")
	c.Check(isVPCNotFoundError(nonEC2Error), jc.IsFalse)

	ec2Error := makeEC2Error("code", "bad stuff")
	c.Check(isVPCNotFoundError(ec2Error), jc.IsFalse)

	ec2Error = makeVPCNotFoundError("some-id")
	c.Check(isVPCNotFoundError(ec2Error), jc.IsTrue)
}

func (s *vpcSuite) TestCheckVPCIsAvailable(c *gc.C) {
	availableVPC := makeEC2VPC(anyVPCID, types.VpcStateAvailable)
	c.Check(checkVPCIsAvailable(&availableVPC), jc.ErrorIsNil)

	defaultVPC := makeEC2VPC(anyVPCID, types.VpcStateAvailable)
	defaultVPC.IsDefault = aws.Bool(true)
	c.Check(checkVPCIsAvailable(&defaultVPC), jc.ErrorIsNil)

	notAvailableVPC := makeEC2VPC(anyVPCID, types.VpcStatePending)
	err := checkVPCIsAvailable(&notAvailableVPC)
	c.Assert(err, gc.ErrorMatches, `VPC ".*" has unexpected state "pending"`)
	c.Check(err, jc.ErrorIs, errorVPCNotRecommended)
}

func (s *vpcSuite) TestGetVPCSubnetUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	subnets, err := getVPCSubnets(context.Background(), s.stubAPI, anyVPCID)
	c.Assert(err, gc.ErrorMatches, `unexpected AWS response getting subnets of VPC "vpc-anything": AWS failed!`)
	c.Check(subnets, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCSubnetCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))

	subnets, err := getVPCSubnets(context.Background(), s.stubAPI, anyVPCID)
	c.Check(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(subnets, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCSubnetsNoResults(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	subnets, err := getVPCSubnets(context.Background(), s.stubAPI, anyVPCID)
	c.Check(err, jc.ErrorIs, errorVPCNotUsable)
	c.Check(subnets, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCSubnetsSuccess(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, noPublicIPOnLaunch)

	subnets, err := getVPCSubnets(context.Background(), s.stubAPI, anyVPCID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets, jc.DeepEquals, s.stubAPI.subnetsResponse.Subnets)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPCID)
}

func (s *vpcSuite) TestFindFirstPublicSubnetSuccess(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, withPublicIPOnLaunch)
	s.stubAPI.subnetsResponse.Subnets[0].MapPublicIpOnLaunch = aws.Bool(false)

	subnet, err := findFirstPublicSubnet(s.stubAPI.subnetsResponse.Subnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*subnet, jc.DeepEquals, s.stubAPI.subnetsResponse.Subnets[1])
}

func (s *vpcSuite) TestFindFirstPublicSubnetNoneFound(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, noPublicIPOnLaunch)

	subnet, err := findFirstPublicSubnet(s.stubAPI.subnetsResponse.Subnets)
	c.Assert(err, gc.ErrorMatches, "VPC contains no public subnets")
	c.Check(err, jc.ErrorIs, errorVPCNotRecommended)
	c.Check(subnet, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCInternetGatewayNoResults(c *gc.C) {
	s.stubAPI.SetGatewaysResponse(noResults, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(context.Background(), s.stubAPI, &anyVPC)
	c.Assert(err, gc.ErrorMatches, `VPC has no Internet Gateway attached`)
	c.Check(err, jc.ErrorIs, errorVPCNotRecommended)
	c.Check(gateway, gc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCInternetGatewayUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(context.Background(), s.stubAPI, &anyVPC)
	c.Assert(err, gc.ErrorMatches, `unexpected AWS response getting Internet Gateway of VPC "vpc-anything": AWS failed!`)
	c.Check(gateway, gc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCInternetGatewayCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(context.Background(), s.stubAPI, &anyVPC)
	c.Check(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(gateway, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCInternetGatewayMultipleResults(c *gc.C) {
	s.stubAPI.SetGatewaysResponse(3, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(context.Background(), s.stubAPI, &anyVPC)
	c.Assert(err, gc.ErrorMatches, "expected 1 result from AWS, got 3")
	c.Check(gateway, gc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCInternetGatewaySuccess(c *gc.C) {
	s.stubAPI.SetGatewaysResponse(1, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(context.Background(), s.stubAPI, &anyVPC)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*gateway, jc.DeepEquals, s.stubAPI.gatewaysResponse.InternetGateways[0])

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestCheckInternetGatewayIsAvailable(c *gc.C) {
	availableIGW := makeEC2InternetGateway(anyGatewayID, attachmentStatusAvailable)
	c.Check(checkInternetGatewayIsAvailable(&availableIGW), jc.ErrorIsNil)

	pendingIGW := makeEC2InternetGateway(anyGatewayID, "pending")
	err := checkInternetGatewayIsAvailable(&pendingIGW)
	c.Assert(err, gc.ErrorMatches, `VPC has Internet Gateway "igw-anything" in unexpected state "pending"`)
	c.Check(err, jc.ErrorIs, errorVPCNotRecommended)
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesWithNoDefaultRoute(c *gc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	c.Check(table.Routes, gc.HasLen, 0) // no routes at all

	checkFailed := func() {
		err := checkVPCRouteTableRoutes(context.Background(), &vpc, &table, &gateway)
		c.Assert(err, gc.ErrorMatches, `missing default route via gateway "igw-anything"`)
		c.Check(err, jc.ErrorIs, errorVPCNotRecommended)
	}
	checkFailed()

	cidrBlock := aws.ToString(vpc.CidrBlock)
	table.Routes = makeEC2Routes(aws.ToString(gateway.InternetGatewayId), cidrBlock, "blackhole", 3) // inactive routes only
	checkFailed()

	table.Routes = makeEC2Routes("", cidrBlock, types.RouteStateActive, 1) // local and 1 extra route
	checkFailed()

	table.Routes = makeEC2Routes("", cidrBlock, types.RouteStateActive, 0) // local route only
	checkFailed()
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesWithDefaultButNoLocalRoutes(c *gc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	gatewayID := aws.ToString(gateway.InternetGatewayId)
	table.Routes = makeEC2Routes(gatewayID, "", types.RouteStateActive, 3) // default and 3 extra routes; no local route

	checkFailed := func() {
		err := checkVPCRouteTableRoutes(context.Background(), &vpc, &table, &gateway)
		c.Assert(err, gc.ErrorMatches, `missing local route with destination "0.1.0.0/16"`)
		c.Check(err, jc.ErrorIs, errorVPCNotRecommended)
	}
	checkFailed()

	table.Routes = makeEC2Routes(gatewayID, "", types.RouteStateActive, 0) // only default route
	checkFailed()
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesSuccess(c *gc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	table.Routes = makeEC2Routes(aws.ToString(gateway.InternetGatewayId), aws.ToString(vpc.CidrBlock), types.RouteStateActive, 3) // default, local and 3 extra routes

	err := checkVPCRouteTableRoutes(context.Background(), &vpc, &table, &gateway)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *vpcSuite) TestFindDefaultVPCIDUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	vpcID, err := findDefaultVPCID(context.Background(), s.stubAPI)
	c.Assert(err, gc.ErrorMatches, "unexpected AWS response getting default-vpc account attribute: AWS failed!")
	c.Check(vpcID, gc.Equals, "")

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestFindDefaultVPCIDCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))
	_, err := findDefaultVPCID(context.Background(), s.stubAPI)
	c.Check(err, jc.ErrorIs, common.ErrorCredentialNotValid)
}

func (s *vpcSuite) TestFindDefaultVPCIDNoAttributeOrNoValue(c *gc.C) {
	s.stubAPI.SetAttributesResponse(nil) // no attributes at all

	checkFailed := func() {
		vpcID, err := findDefaultVPCID(context.Background(), s.stubAPI)
		c.Assert(err, gc.ErrorMatches, "default-vpc account attribute not found")
		c.Check(err, jc.ErrorIs, errors.NotFound)
		c.Check(vpcID, gc.Equals, "")

		s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
	}
	checkFailed()

	s.stubAPI.SetAttributesResponse(map[string][]string{
		"any-attribute": nil, // no values
	})
	checkFailed()

	s.stubAPI.SetAttributesResponse(map[string][]string{
		"not-default-vpc-attribute": {"foo", "bar"}, // wrong name
	})
	checkFailed()

	s.stubAPI.SetAttributesResponse(map[string][]string{
		"default-vpc": nil, // name ok, no values
	})
	checkFailed()

	s.stubAPI.SetAttributesResponse(map[string][]string{
		"default-vpc": {}, // name ok, empty values
	})
	checkFailed()
}

func (s *vpcSuite) TestFindDefaultVPCIDWithExplicitNoneValue(c *gc.C) {
	s.stubAPI.SetAttributesResponse(map[string][]string{
		"default-vpc": {"none"},
	})

	vpcID, err := findDefaultVPCID(context.Background(), s.stubAPI)
	c.Assert(err, gc.ErrorMatches, "default VPC not found")
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(vpcID, gc.Equals, "")

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestFindDefaultVPCIDSuccess(c *gc.C) {
	s.stubAPI.SetAttributesResponse(map[string][]string{
		"default-vpc": {"vpc-foo", "vpc-bar"},
	})

	vpcID, err := findDefaultVPCID(context.Background(), s.stubAPI)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(vpcID, gc.Equals, "vpc-foo") // always the first value is used.

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("too cloudy"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnetIDs, err := getVPCSubnetsForAvailabilityZone(context.Background(), s.stubAPI, vpcID, anyZone, nil)
	c.Assert(err, gc.ErrorMatches, `cannot get VPC "vpc-anything" subnets: unexpected AWS .*: too cloudy`)
	c.Check(subnetIDs, gc.HasLen, 0)

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: too cloudy", common.ErrorCredentialNotValid))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnetIDs, err := getVPCSubnetsForAvailabilityZone(context.Background(), s.stubAPI, vpcID, anyZone, nil)
	c.Assert(err, gc.ErrorMatches, `cannot get VPC "vpc-anything" subnets: unexpected AWS .*: too cloudy`)
	c.Check(subnetIDs, gc.IsNil)
	c.Check(err, jc.ErrorIs, common.ErrorCredentialNotValid)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneNoSubnetsAtAll(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnetIDs, err := getVPCSubnetsForAvailabilityZone(context.Background(), s.stubAPI, vpcID, anyZone, nil)
	c.Assert(err, gc.ErrorMatches, `VPC "vpc-anything" has no subnets in AZ "any-zone": subnets for VPC .* not found`)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(subnetIDs, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneNoSubnetsInAZ(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, "other-zone", noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnets, err := getVPCSubnetsForAvailabilityZone(context.Background(), s.stubAPI, vpcID, "given-zone", nil)
	c.Assert(err, gc.ErrorMatches, `VPC "vpc-anything" has no subnets in AZ "given-zone"`)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(subnets, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsToZones(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(4, "my-zone", noPublicIPOnLaunch)
	// Simulate we used --constraints spaces=foo, which contains subnet-1 and
	// subnet-3 out of the 4 subnets in AZ my-zone (see the related bug
	// http://pad.lv/1609343).
	allowedSubnetIDs := []corenetwork.Id{"subnet-1", "subnet-3"}

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnets, err := getVPCSubnetsForAvailabilityZone(context.Background(), s.stubAPI, vpcID, "my-zone", allowedSubnetIDs)
	c.Assert(err, jc.ErrorIsNil)
	subnetIDs := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		subnetIDs = append(subnetIDs, *subnet.SubnetId)
	}
	c.Check(subnetIDs, jc.DeepEquals, []string{"subnet-1", "subnet-3"})

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneSuccess(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(2, "my-zone", noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnets, err := getVPCSubnetsForAvailabilityZone(context.Background(), s.stubAPI, vpcID, "my-zone", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Result slice of IDs is always sorted.
	subnetIDs := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		subnetIDs = append(subnetIDs, *subnet.SubnetId)
	}
	c.Check(subnetIDs, jc.DeepEquals, []string{"subnet-0", "subnet-1"})

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

const (
	notDefaultVPC = false

	notMainRouteTable = false
	mainRouteTable    = true

	noResults = 0

	anyState     = "any state"
	anyVPCID     = "vpc-anything"
	anyGatewayID = "igw-anything"
	anyTableID   = "rtb-anything"
	anyZone      = "any-zone"

	noPublicIPOnLaunch   = false
	withPublicIPOnLaunch = true
)

type stubVPCAPIClient struct {
	*testing.Stub

	attributesResponse  *ec2.DescribeAccountAttributesOutput
	vpcsResponse        *ec2.DescribeVpcsOutput
	subnetsResponse     *ec2.DescribeSubnetsOutput
	gatewaysResponse    *ec2.DescribeInternetGatewaysOutput
	routeTablesResponse *ec2.DescribeRouteTablesOutput
}

// AccountAttributes implements vpcAPIClient and is used to test finding the
// default VPC from the "default-vpc"" attribute.
func (s *stubVPCAPIClient) DescribeAccountAttributes(_ context.Context, in *ec2.DescribeAccountAttributesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAccountAttributesOutput, error) {
	s.Stub.AddCall("DescribeAccountAttributesWithContext", makeArgsFromNames(in.AttributeNames...)...)
	return s.attributesResponse, s.Stub.NextErr()
}

// VPCs implements vpcAPIClient and is used to test getting the details of a
// VPC.
func (s *stubVPCAPIClient) DescribeVpcs(_ context.Context, in *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	s.Stub.AddCall("DescribeVpcsWithContext", in.VpcIds, in.Filters)
	return s.vpcsResponse, s.Stub.NextErr()
}

// Subnets implements vpcAPIClient and is used to test getting a VPC's subnets.
func (s *stubVPCAPIClient) DescribeSubnets(_ context.Context, in *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	s.Stub.AddCall("DescribeSubnetsWithContext", in.SubnetIds, in.Filters)
	return s.subnetsResponse, s.Stub.NextErr()
}

// InternetGateways implements vpcAPIClient and is used to test getting the
// attached IGW of a VPC.
func (s *stubVPCAPIClient) DescribeInternetGateways(_ context.Context, in *ec2.DescribeInternetGatewaysInput, _ ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error) {
	s.Stub.AddCall("DescribeInternetGatewaysWithContext", in.InternetGatewayIds, in.Filters)
	return s.gatewaysResponse, s.Stub.NextErr()
}

// RouteTables implements vpcAPIClient and is used to test getting all route
// tables of a VPC, alond with their routes.
func (s *stubVPCAPIClient) DescribeRouteTables(_ context.Context, in *ec2.DescribeRouteTablesInput, _ ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	s.Stub.AddCall("DescribeRouteTablesWithContext", in.RouteTableIds, in.Filters)
	return s.routeTablesResponse, s.Stub.NextErr()
}

func (s *stubVPCAPIClient) SetAttributesResponse(attributeNameToValues map[string][]string) {
	s.attributesResponse = &ec2.DescribeAccountAttributesOutput{
		AccountAttributes: make([]types.AccountAttribute, 0, len(attributeNameToValues)),
	}

	for name, values := range attributeNameToValues {
		attributeValues := make([]types.AccountAttributeValue, len(values))
		for i, v := range values {
			attributeValues[i] = types.AccountAttributeValue{
				AttributeValue: aws.String(v),
			}
		}
		attribute := types.AccountAttribute{
			AttributeName:   aws.String(name),
			AttributeValues: attributeValues,
		}
		s.attributesResponse.AccountAttributes = append(s.attributesResponse.AccountAttributes, attribute)
	}
}
func (s *stubVPCAPIClient) CheckSingleAccountAttributesCall(c *gc.C, attributeNames ...types.AccountAttributeName) {
	s.Stub.CheckCallNames(c, "DescribeAccountAttributesWithContext")
	s.Stub.CheckCall(c, 0, "DescribeAccountAttributesWithContext", makeArgsFromNames(attributeNames...)...)
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetVPCsResponse(numResults int, state types.VpcState, isDefault bool) {
	s.vpcsResponse = &ec2.DescribeVpcsOutput{
		Vpcs: make([]types.Vpc, numResults),
	}

	for i := range s.vpcsResponse.Vpcs {
		id := fmt.Sprintf("vpc-%d", i)
		vpc := makeEC2VPC(id, state)
		vpc.IsDefault = aws.Bool(isDefault)
		s.vpcsResponse.Vpcs[i] = vpc
	}
}

func (s *stubVPCAPIClient) CheckSingleVPCsCall(c *gc.C, vpcID string) {
	s.Stub.CheckCallNames(c, "DescribeVpcsWithContext")
	s.Stub.CheckCall(c, 0, "DescribeVpcsWithContext", []string{vpcID}, ([]types.Filter)(nil))
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetSubnetsResponse(numResults int, zone string, mapPublicIpOnLaunch bool) {
	s.subnetsResponse = &ec2.DescribeSubnetsOutput{
		Subnets: make([]types.Subnet, numResults),
	}

	for i := range s.subnetsResponse.Subnets {
		s.subnetsResponse.Subnets[i] = types.Subnet{
			SubnetId:            aws.String(fmt.Sprintf("subnet-%d", i)),
			VpcId:               aws.String(anyVPCID),
			State:               anyState,
			AvailabilityZone:    aws.String(zone),
			CidrBlock:           aws.String(fmt.Sprintf("0.1.%d.0/20", i)),
			MapPublicIpOnLaunch: aws.Bool(mapPublicIpOnLaunch),
		}
	}
}

func (s *stubVPCAPIClient) CheckSingleSubnetsCall(c *gc.C, vpcID string) {
	var nilIDs []string
	filter := makeFilter("vpc-id", vpcID)

	s.Stub.CheckCallNames(c, "DescribeSubnetsWithContext")
	s.Stub.CheckCall(c, 0, "DescribeSubnetsWithContext", nilIDs, []types.Filter{filter})
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetGatewaysResponse(numResults int, status types.AttachmentStatus) {
	s.gatewaysResponse = &ec2.DescribeInternetGatewaysOutput{
		InternetGateways: make([]types.InternetGateway, numResults),
	}

	for i := range s.gatewaysResponse.InternetGateways {
		id := fmt.Sprintf("igw-%d", i)
		gateway := makeEC2InternetGateway(id, status)
		s.gatewaysResponse.InternetGateways[i] = gateway
	}
}

func (s *stubVPCAPIClient) CheckSingleInternetGatewaysCall(c *gc.C, vpcID string) {
	var nilIDs []string
	filter := makeFilter("attachment.vpc-id", vpcID)

	s.Stub.CheckCallNames(c, "DescribeInternetGatewaysWithContext")
	s.Stub.CheckCall(c, 0, "DescribeInternetGatewaysWithContext", nilIDs, []types.Filter{filter})
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetRouteTablesResponse(tables ...types.RouteTable) {
	s.routeTablesResponse = &ec2.DescribeRouteTablesOutput{
		RouteTables: make([]types.RouteTable, len(tables)),
	}

	for i := range s.routeTablesResponse.RouteTables {
		s.routeTablesResponse.RouteTables[i] = tables[i]
	}
}

func (s *stubVPCAPIClient) CheckSingleRouteTablesCall(c *gc.C, vpcID string) {
	var nilIDs []string
	filter := makeFilter("vpc-id", vpcID)

	s.Stub.CheckCallNames(c, "DescribeRouteTablesWithContext")
	s.Stub.CheckCall(c, 0, "DescribeRouteTablesWithContext", nilIDs, []types.Filter{filter})
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) PrepareValidateVPCResponses() {
	s.SetVPCsResponse(1, types.VpcStateAvailable, notDefaultVPC)
	s.vpcsResponse.Vpcs[0].CidrBlock = aws.String("0.1.0.0/16")
	s.SetSubnetsResponse(1, anyZone, withPublicIPOnLaunch)
	s.SetGatewaysResponse(1, attachmentStatusAvailable)
	onlyDefaultAndLocalRoutes := makeEC2Routes(
		aws.ToString(s.gatewaysResponse.InternetGateways[0].InternetGatewayId),
		aws.ToString(s.vpcsResponse.Vpcs[0].CidrBlock),
		types.RouteStateActive,
		0, // no extra routes
	)
	s.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, mainRouteTable, nil, onlyDefaultAndLocalRoutes),
	)
}

func (s *stubVPCAPIClient) CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c *gc.C, ctx envcontext.ProviderCallContext, lastExpectedCallName string) {
	err := validateVPC(ctx, s, anyVPCID)
	c.Assert(err, jc.ErrorIs, errorVPCNotRecommended)

	allCalls := []string{"DescribeVpcsWithContext", "DescribeSubnetsWithContext", "DescribeInternetGatewaysWithContext", "DescribeRouteTablesWithContext"}
	var expectedCalls []string
	for i := range allCalls {
		expectedCalls = append(expectedCalls, allCalls[i])
		if allCalls[i] == lastExpectedCallName {
			break
		}
	}
	s.CheckCallNames(c, expectedCalls...)
}

func makeEC2VPC(vpcID string, state types.VpcState) types.Vpc {
	return types.Vpc{
		VpcId: aws.String(vpcID),
		State: state,
	}
}

func makeEC2InternetGateway(gatewayID string, attachmentState types.AttachmentStatus) types.InternetGateway {
	return types.InternetGateway{
		InternetGatewayId: aws.String(gatewayID),
		Attachments: []types.InternetGatewayAttachment{{
			VpcId: aws.String(anyVPCID),
			State: attachmentState,
		}},
	}
}

func makeEC2RouteTable(tableID string, isMain bool, associatedSubnetIDs []string, routes []types.Route) types.RouteTable {
	table := types.RouteTable{
		RouteTableId: aws.String(tableID),
		VpcId:        aws.String(anyVPCID),
		Routes:       routes,
	}

	if isMain {
		table.Associations = []types.RouteTableAssociation{{
			RouteTableAssociationId: aws.String("rtbassoc-main"),
			RouteTableId:            aws.String(tableID),
			Main:                    aws.Bool(true),
		}}
	} else {
		table.Associations = make([]types.RouteTableAssociation, len(associatedSubnetIDs))
		for i := range associatedSubnetIDs {
			table.Associations[i] = types.RouteTableAssociation{
				RouteTableAssociationId: aws.String(fmt.Sprintf("rtbassoc-%d", i)),
				RouteTableId:            aws.String(tableID),
				SubnetId:                aws.String(associatedSubnetIDs[i]),
			}
		}
	}
	return table
}

func makeEC2Routes(
	defaultRouteGatewayID,
	localRouteCIDRBlock string,
	state types.RouteState,
	numExtraRoutes int,
) []types.Route {
	var routes []types.Route

	if defaultRouteGatewayID != "" {
		routes = append(routes, types.Route{
			DestinationCidrBlock: aws.String(defaultRouteIpv4CIDRBlock),
			GatewayId:            aws.String(defaultRouteGatewayID),
			State:                state,
		})
	}

	if localRouteCIDRBlock != "" {
		routes = append(routes, types.Route{
			DestinationCidrBlock: aws.String(localRouteCIDRBlock),
			GatewayId:            aws.String(localRouteGatewayID),
			State:                state,
		})
	}

	if numExtraRoutes > 0 {
		for i := 0; i < numExtraRoutes; i++ {
			routes = append(routes, types.Route{
				DestinationCidrBlock: aws.String(fmt.Sprintf("0.1.%d.0/24", i)),
				State:                state,
			})
		}
	}

	return routes
}

func prepareCheckVPCRouteTableRoutesArgs() (types.Vpc, types.RouteTable, types.InternetGateway) {
	anyVPC := makeEC2VPC(anyVPCID, anyState)
	anyVPC.CidrBlock = aws.String("0.1.0.0/16")
	anyTable := makeEC2RouteTable(anyTableID, notMainRouteTable, nil, nil)
	anyGateway := makeEC2InternetGateway(anyGatewayID, anyState)

	return anyVPC, anyTable, anyGateway
}

func makeEC2Error(code, message string) error {
	return &smithy.GenericAPIError{
		Code:    code,
		Message: message,
	}
}

func makeVPCNotFoundError(vpcID string) error {
	return makeEC2Error(
		"InvalidVpcID.NotFound",
		fmt.Sprintf("The vpc ID '%s' does not exist", vpcID),
	)
}

func makeArgsFromNames(names ...types.AccountAttributeName) []interface{} {
	args := make([]interface{}, len(names))
	for i := range names {
		args[i] = names[i]
	}
	return args
}
