// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testhelpers"
)

type vpcSuite struct {
	testhelpers.IsolationSuite

	stubAPI *stubVPCAPIClient
}

func TestVpcSuite(t *testing.T) {
	tc.Run(t, &vpcSuite{})
}

func (s *vpcSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stubAPI = &stubVPCAPIClient{Stub: &testhelpers.Stub{}}
}

func (s *vpcSuite) TestValidateBootstrapVPCUnexpectedError(c *tc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	err := validateBootstrapVPC(c.Context(), s.stubAPI, "region", anyVPCID, false, envtesting.BootstrapTestContext(c))
	s.checkErrorMatchesCannotVerifyVPC(c, err)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateBootstrapVPCCredentialError(c *tc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))
	err := validateBootstrapVPC(c.Context(), s.stubAPI, "region", anyVPCID, false, envtesting.BootstrapTestContext(c))
	s.checkErrorMatchesCannotVerifyVPC(c, err)
	c.Check(err, tc.ErrorIs, common.ErrorCredentialNotValid)
}

func (*vpcSuite) checkErrorMatchesCannotVerifyVPC(c *tc.C, err error) {
	expectedError := `Juju could not verify whether the given vpc-id(.|\n)*AWS failed!`
	c.Check(err, tc.ErrorMatches, expectedError)
}

func (s *vpcSuite) TestValidateModelVPCUnexpectedError(c *tc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	err := validateModelVPC(c.Context(), s.stubAPI, "model", anyVPCID)
	s.checkErrorMatchesCannotVerifyVPC(c, err)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateModelVPCNotUsableError(c *tc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	err := validateModelVPC(c.Context(), s.stubAPI, "model", "foo")
	c.Check(err, tc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateModelVPCCredentialError(c *tc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("foo: %w", common.ErrorCredentialNotValid))
	err := validateModelVPC(c.Context(), s.stubAPI, "model", "foo")
	expectedError := `Juju could not verify whether the given vpc-id(.|\n)*`
	c.Check(err, tc.ErrorMatches, expectedError)
	c.Check(err, tc.ErrorIs, common.ErrorCredentialNotValid)
}

func (s *vpcSuite) TestValidateModelVPCIDNotSetOrNone(c *tc.C) {
	const emptyVPCID = ""
	err := validateModelVPC(c.Context(), s.stubAPI, "model", emptyVPCID)
	c.Check(err, tc.ErrorIsNil)

	err = validateModelVPC(c.Context(), s.stubAPI, "model", vpcIDNone)
	c.Check(err, tc.ErrorIsNil)

	s.stubAPI.CheckNoCalls(c)
}

// NOTE: validateVPC tests only verify expected error types for all code paths,
// but do not check passed API arguments or exact error messages, as those are
// extensively tested separately below.

func (s *vpcSuite) TestValidateVPCWithEmptyVPCIDOrNilAPIClient(c *tc.C) {
	err := validateVPC(c.Context(), s.stubAPI, "")
	c.Assert(err, tc.ErrorMatches, "invalid arguments: empty VPC ID or nil client")

	err = validateVPC(c.Context(), nil, anyVPCID)
	c.Assert(err, tc.ErrorMatches, "invalid arguments: empty VPC ID or nil client")

	s.stubAPI.CheckNoCalls(c)
}

func (s *vpcSuite) TestValidateVPCWhenVPCIDNotFound(c *tc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	err := validateVPC(c.Context(), s.stubAPI, anyVPCID)
	c.Check(err, tc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoSubnets(c *tc.C) {
	s.stubAPI.SetVPCsResponse(1, types.VpcStateAvailable, notDefaultVPC)
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	err := validateVPC(c.Context(), s.stubAPI, anyVPCID)
	c.Check(err, tc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext")
}
func (s *vpcSuite) TestValidateVPCWhenVPCNotAvailable(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetVPCsResponse(1, "bad-state", notDefaultVPC)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, "DescribeVpcsWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoPublicSubnets(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetSubnetsResponse(1, anyZone, noPublicIPOnLaunch)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, "DescribeSubnetsWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoGateway(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetGatewaysResponse(noResults, anyState)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, "DescribeInternetGatewaysWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoAttachedGateway(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetGatewaysResponse(1, "pending")

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, "DescribeInternetGatewaysWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoRouteTables(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse() // no route tables at all

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoMainRouteTable(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, notMainRouteTable, nil, nil),
	)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasMainRouteTableWithoutRoutes(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, mainRouteTable, nil, nil),
	)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateVPCSuccess(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()

	err := validateVPC(c.Context(), s.stubAPI, anyVPCID)
	c.Assert(err, tc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext", "DescribeInternetGatewaysWithContext", "DescribeRouteTablesWithContext")
}

func (s *vpcSuite) TestValidateModelVPCSuccess(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()

	err := validateModelVPC(c.Context(), s.stubAPI, "model", anyVPCID)
	c.Assert(err, tc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext", "DescribeInternetGatewaysWithContext", "DescribeRouteTablesWithContext")
	//c.Check(c.GetTestLog(), tc.Contains, `INFO juju.provider.ec2 Using VPC "vpc-anything" for model "model"`)
}

func (s *vpcSuite) TestValidateModelVPCNotRecommendedStillOK(c *tc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetSubnetsResponse(1, anyZone, noPublicIPOnLaunch)

	err := validateModelVPC(c.Context(), s.stubAPI, "model", anyVPCID)
	c.Assert(err, tc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "DescribeVpcsWithContext", "DescribeSubnetsWithContext")
	//testLog := c.GetTestLog()
	//c.Check(testLog, tc.Contains, `INFO juju.provider.ec2 Juju will use, but does not recommend `+
	//	`using VPC "vpc-anything": VPC contains no public subnets`)
	//c.Check(testLog, tc.Contains, `INFO juju.provider.ec2 Using VPC "vpc-anything" for model "model"`)
}

func (s *vpcSuite) TestGetVPCByIDWithMissingID(c *tc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	vpc, err := getVPCByID(c.Context(), s.stubAPI, "foo")
	c.Check(err, tc.ErrorIs, errorVPCNotUsable)
	c.Check(vpc, tc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "foo")
}

func (s *vpcSuite) TestGetVPCByIDUnexpectedAWSError(c *tc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	vpc, err := getVPCByID(c.Context(), s.stubAPI, "bar")
	c.Assert(err, tc.ErrorMatches, `unexpected AWS response getting VPC "bar": AWS failed!`)
	c.Check(vpc, tc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "bar")
}

func (s *vpcSuite) TestGetVPCByIDCredentialError(c *tc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))

	vpc, err := getVPCByID(c.Context(), s.stubAPI, "bar")
	c.Check(err, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(vpc, tc.IsNil)
}

func (s *vpcSuite) TestGetVPCByIDNoResults(c *tc.C) {
	s.stubAPI.SetVPCsResponse(noResults, anyState, notDefaultVPC)

	_, err := getVPCByID(c.Context(), s.stubAPI, "vpc-42")
	c.Assert(err, tc.ErrorMatches, `VPC "vpc-42" not found`)
	c.Check(err, tc.ErrorIs, errorVPCNotUsable)

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-42")
}

func (s *vpcSuite) TestGetVPCByIDMultipleResults(c *tc.C) {
	s.stubAPI.SetVPCsResponse(5, anyState, notDefaultVPC)

	vpc, err := getVPCByID(c.Context(), s.stubAPI, "vpc-33")
	c.Assert(err, tc.ErrorMatches, "expected 1 result from AWS, got 5")
	c.Check(vpc, tc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-33")
}

func (s *vpcSuite) TestGetVPCByIDSuccess(c *tc.C) {
	s.stubAPI.SetVPCsResponse(1, anyState, notDefaultVPC)

	vpc, err := getVPCByID(c.Context(), s.stubAPI, "vpc-1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*vpc, tc.DeepEquals, s.stubAPI.vpcsResponse.Vpcs[0])

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-1")
}

func (s *vpcSuite) TestIsVPCNotFoundError(c *tc.C) {
	c.Check(isVPCNotFoundError(nil), tc.IsFalse)

	nonEC2Error := errors.New("boom")
	c.Check(isVPCNotFoundError(nonEC2Error), tc.IsFalse)

	ec2Error := makeEC2Error("code", "bad stuff")
	c.Check(isVPCNotFoundError(ec2Error), tc.IsFalse)

	ec2Error = makeVPCNotFoundError("some-id")
	c.Check(isVPCNotFoundError(ec2Error), tc.IsTrue)
}

func (s *vpcSuite) TestCheckVPCIsAvailable(c *tc.C) {
	availableVPC := makeEC2VPC(anyVPCID, types.VpcStateAvailable)
	c.Check(checkVPCIsAvailable(&availableVPC), tc.ErrorIsNil)

	defaultVPC := makeEC2VPC(anyVPCID, types.VpcStateAvailable)
	defaultVPC.IsDefault = aws.Bool(true)
	c.Check(checkVPCIsAvailable(&defaultVPC), tc.ErrorIsNil)

	notAvailableVPC := makeEC2VPC(anyVPCID, types.VpcStatePending)
	err := checkVPCIsAvailable(&notAvailableVPC)
	c.Assert(err, tc.ErrorMatches, `VPC ".*" has unexpected state "pending"`)
	c.Check(err, tc.ErrorIs, errorVPCNotRecommended)
}

func (s *vpcSuite) TestGetVPCSubnetUnexpectedAWSError(c *tc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	subnets, err := getVPCSubnets(c.Context(), s.stubAPI, anyVPCID)
	c.Assert(err, tc.ErrorMatches, `unexpected AWS response getting subnets of VPC "vpc-anything": AWS failed!`)
	c.Check(subnets, tc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCSubnetCredentialError(c *tc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))

	subnets, err := getVPCSubnets(c.Context(), s.stubAPI, anyVPCID)
	c.Check(err, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(subnets, tc.IsNil)
}

func (s *vpcSuite) TestGetVPCSubnetsNoResults(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	subnets, err := getVPCSubnets(c.Context(), s.stubAPI, anyVPCID)
	c.Check(err, tc.ErrorIs, errorVPCNotUsable)
	c.Check(subnets, tc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCSubnetsSuccess(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, noPublicIPOnLaunch)

	subnets, err := getVPCSubnets(c.Context(), s.stubAPI, anyVPCID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.DeepEquals, s.stubAPI.subnetsResponse.Subnets)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPCID)
}

func (s *vpcSuite) TestFindFirstPublicSubnetSuccess(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, withPublicIPOnLaunch)
	s.stubAPI.subnetsResponse.Subnets[0].MapPublicIpOnLaunch = aws.Bool(false)

	subnet, err := findFirstPublicSubnet(s.stubAPI.subnetsResponse.Subnets)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*subnet, tc.DeepEquals, s.stubAPI.subnetsResponse.Subnets[1])
}

func (s *vpcSuite) TestFindFirstPublicSubnetNoneFound(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, noPublicIPOnLaunch)

	subnet, err := findFirstPublicSubnet(s.stubAPI.subnetsResponse.Subnets)
	c.Assert(err, tc.ErrorMatches, "VPC contains no public subnets")
	c.Check(err, tc.ErrorIs, errorVPCNotRecommended)
	c.Check(subnet, tc.IsNil)
}

func (s *vpcSuite) TestGetVPCInternetGatewayNoResults(c *tc.C) {
	s.stubAPI.SetGatewaysResponse(noResults, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(c.Context(), s.stubAPI, &anyVPC)
	c.Assert(err, tc.ErrorMatches, `VPC has no Internet Gateway attached`)
	c.Check(err, tc.ErrorIs, errorVPCNotRecommended)
	c.Check(gateway, tc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCInternetGatewayUnexpectedAWSError(c *tc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(c.Context(), s.stubAPI, &anyVPC)
	c.Assert(err, tc.ErrorMatches, `unexpected AWS response getting Internet Gateway of VPC "vpc-anything": AWS failed!`)
	c.Check(gateway, tc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCInternetGatewayCredentialError(c *tc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(c.Context(), s.stubAPI, &anyVPC)
	c.Check(err, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(gateway, tc.IsNil)
}

func (s *vpcSuite) TestGetVPCInternetGatewayMultipleResults(c *tc.C) {
	s.stubAPI.SetGatewaysResponse(3, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(c.Context(), s.stubAPI, &anyVPC)
	c.Assert(err, tc.ErrorMatches, "expected 1 result from AWS, got 3")
	c.Check(gateway, tc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestGetVPCInternetGatewaySuccess(c *tc.C) {
	s.stubAPI.SetGatewaysResponse(1, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(c.Context(), s.stubAPI, &anyVPC)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*gateway, tc.DeepEquals, s.stubAPI.gatewaysResponse.InternetGateways[0])

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPCID)
}

func (s *vpcSuite) TestCheckInternetGatewayIsAvailable(c *tc.C) {
	availableIGW := makeEC2InternetGateway(anyGatewayID, attachmentStatusAvailable)
	c.Check(checkInternetGatewayIsAvailable(&availableIGW), tc.ErrorIsNil)

	pendingIGW := makeEC2InternetGateway(anyGatewayID, "pending")
	err := checkInternetGatewayIsAvailable(&pendingIGW)
	c.Assert(err, tc.ErrorMatches, `VPC has Internet Gateway "igw-anything" in unexpected state "pending"`)
	c.Check(err, tc.ErrorIs, errorVPCNotRecommended)
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesWithNoDefaultRoute(c *tc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	c.Check(table.Routes, tc.HasLen, 0) // no routes at all

	checkFailed := func() {
		err := checkVPCRouteTableRoutes(c.Context(), &vpc, &table, &gateway)
		c.Assert(err, tc.ErrorMatches, `missing default route via gateway "igw-anything"`)
		c.Check(err, tc.ErrorIs, errorVPCNotRecommended)
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

func (s *vpcSuite) TestCheckVPCRouteTableRoutesWithDefaultButNoLocalRoutes(c *tc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	gatewayID := aws.ToString(gateway.InternetGatewayId)
	table.Routes = makeEC2Routes(gatewayID, "", types.RouteStateActive, 3) // default and 3 extra routes; no local route

	checkFailed := func() {
		err := checkVPCRouteTableRoutes(c.Context(), &vpc, &table, &gateway)
		c.Assert(err, tc.ErrorMatches, `missing local route with destination "0.1.0.0/16"`)
		c.Check(err, tc.ErrorIs, errorVPCNotRecommended)
	}
	checkFailed()

	table.Routes = makeEC2Routes(gatewayID, "", types.RouteStateActive, 0) // only default route
	checkFailed()
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesSuccess(c *tc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	table.Routes = makeEC2Routes(aws.ToString(gateway.InternetGatewayId), aws.ToString(vpc.CidrBlock), types.RouteStateActive, 3) // default, local and 3 extra routes

	err := checkVPCRouteTableRoutes(c.Context(), &vpc, &table, &gateway)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *vpcSuite) TestFindDefaultVPCIDUnexpectedAWSError(c *tc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	vpcID, err := findDefaultVPCID(c.Context(), s.stubAPI)
	c.Assert(err, tc.ErrorMatches, "unexpected AWS response getting default-vpc account attribute: AWS failed!")
	c.Check(vpcID, tc.Equals, "")

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestFindDefaultVPCIDCredentialError(c *tc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: AWS failed!", common.ErrorCredentialNotValid))
	_, err := findDefaultVPCID(c.Context(), s.stubAPI)
	c.Check(err, tc.ErrorIs, common.ErrorCredentialNotValid)
}

func (s *vpcSuite) TestFindDefaultVPCIDNoAttributeOrNoValue(c *tc.C) {
	s.stubAPI.SetAttributesResponse(nil) // no attributes at all

	checkFailed := func() {
		vpcID, err := findDefaultVPCID(c.Context(), s.stubAPI)
		c.Assert(err, tc.ErrorMatches, "default-vpc account attribute not found")
		c.Check(err, tc.ErrorIs, errors.NotFound)
		c.Check(vpcID, tc.Equals, "")

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

func (s *vpcSuite) TestFindDefaultVPCIDWithExplicitNoneValue(c *tc.C) {
	s.stubAPI.SetAttributesResponse(map[string][]string{
		"default-vpc": {"none"},
	})

	vpcID, err := findDefaultVPCID(c.Context(), s.stubAPI)
	c.Assert(err, tc.ErrorMatches, "default VPC not found")
	c.Check(err, tc.ErrorIs, errors.NotFound)
	c.Check(vpcID, tc.Equals, "")

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestFindDefaultVPCIDSuccess(c *tc.C) {
	s.stubAPI.SetAttributesResponse(map[string][]string{
		"default-vpc": {"vpc-foo", "vpc-bar"},
	})

	vpcID, err := findDefaultVPCID(c.Context(), s.stubAPI)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vpcID, tc.Equals, "vpc-foo") // always the first value is used.

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsError(c *tc.C) {
	s.stubAPI.SetErrors(errors.New("too cloudy"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnetIDs, err := getVPCSubnetsForAvailabilityZone(c.Context(), s.stubAPI, vpcID, anyZone, nil)
	c.Assert(err, tc.ErrorMatches, `cannot get VPC "vpc-anything" subnets: unexpected AWS .*: too cloudy`)
	c.Check(subnetIDs, tc.HasLen, 0)

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsCredentialError(c *tc.C) {
	s.stubAPI.SetErrors(fmt.Errorf("%w: too cloudy", common.ErrorCredentialNotValid))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnetIDs, err := getVPCSubnetsForAvailabilityZone(c.Context(), s.stubAPI, vpcID, anyZone, nil)
	c.Assert(err, tc.ErrorMatches, `cannot get VPC "vpc-anything" subnets: unexpected AWS .*: too cloudy`)
	c.Check(subnetIDs, tc.IsNil)
	c.Check(err, tc.ErrorIs, common.ErrorCredentialNotValid)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneNoSubnetsAtAll(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnetIDs, err := getVPCSubnetsForAvailabilityZone(c.Context(), s.stubAPI, vpcID, anyZone, nil)
	c.Assert(err, tc.ErrorMatches, `VPC "vpc-anything" has no subnets in AZ "any-zone": subnets for VPC .* not found`)
	c.Check(err, tc.ErrorIs, errors.NotFound)
	c.Check(subnetIDs, tc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneNoSubnetsInAZ(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(3, "other-zone", noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnets, err := getVPCSubnetsForAvailabilityZone(c.Context(), s.stubAPI, vpcID, "given-zone", nil)
	c.Assert(err, tc.ErrorMatches, `VPC "vpc-anything" has no subnets in AZ "given-zone"`)
	c.Check(err, tc.ErrorIs, errors.NotFound)
	c.Check(subnets, tc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsToZones(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(4, "my-zone", noPublicIPOnLaunch)
	// Simulate we used --constraints spaces=foo, which contains subnet-1 and
	// subnet-3 out of the 4 subnets in AZ my-zone (see the related bug
	// http://pad.lv/1609343).
	allowedSubnetIDs := []corenetwork.Id{"subnet-1", "subnet-3"}

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnets, err := getVPCSubnetsForAvailabilityZone(c.Context(), s.stubAPI, vpcID, "my-zone", allowedSubnetIDs)
	c.Assert(err, tc.ErrorIsNil)
	subnetIDs := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		subnetIDs = append(subnetIDs, *subnet.SubnetId)
	}
	c.Check(subnetIDs, tc.DeepEquals, []string{"subnet-1", "subnet-3"})

	s.stubAPI.CheckSingleSubnetsCall(c, vpcID)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneSuccess(c *tc.C) {
	s.stubAPI.SetSubnetsResponse(2, "my-zone", noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	vpcID := aws.ToString(anyVPC.VpcId)
	subnets, err := getVPCSubnetsForAvailabilityZone(c.Context(), s.stubAPI, vpcID, "my-zone", nil)
	c.Assert(err, tc.ErrorIsNil)
	// Result slice of IDs is always sorted.
	subnetIDs := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		subnetIDs = append(subnetIDs, *subnet.SubnetId)
	}
	c.Check(subnetIDs, tc.DeepEquals, []string{"subnet-0", "subnet-1"})

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
	*testhelpers.Stub

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
func (s *stubVPCAPIClient) CheckSingleAccountAttributesCall(c *tc.C, attributeNames ...types.AccountAttributeName) {
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

func (s *stubVPCAPIClient) CheckSingleVPCsCall(c *tc.C, vpcID string) {
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

func (s *stubVPCAPIClient) CheckSingleSubnetsCall(c *tc.C, vpcID string) {
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

func (s *stubVPCAPIClient) CheckSingleInternetGatewaysCall(c *tc.C, vpcID string) {
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

func (s *stubVPCAPIClient) CheckSingleRouteTablesCall(c *tc.C, vpcID string) {
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

func (s *stubVPCAPIClient) CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c *tc.C, lastExpectedCallName string) {
	err := validateVPC(c.Context(), s, anyVPCID)
	c.Assert(err, tc.ErrorIs, errorVPCNotRecommended)

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
