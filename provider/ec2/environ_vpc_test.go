// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/amz.v3/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/common"
)

type vpcSuite struct {
	testing.IsolationSuite

	stubAPI *stubVPCAPIClient

	cloudCallCtx context.ProviderCallContext
}

var _ = gc.Suite(&vpcSuite{})

func (s *vpcSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stubAPI = &stubVPCAPIClient{Stub: &testing.Stub{}}
	s.cloudCallCtx = context.NewCloudCallContext()
}

func (s *vpcSuite) TestValidateBootstrapVPCUnexpectedError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	err := validateBootstrapVPC(s.stubAPI, s.cloudCallCtx, "region", anyVPCID, false, envtesting.BootstrapContext(c))
	s.checkErrorMatchesCannotVerifyVPC(c, err)

	s.stubAPI.CheckCallNames(c, "VPCs")
}

func (s *vpcSuite) TestValidateBootstrapVPCCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("AWS failed!"))
	err := validateBootstrapVPC(s.stubAPI, s.cloudCallCtx, "region", anyVPCID, false, envtesting.BootstrapContext(c))
	s.checkErrorMatchesCannotVerifyVPC(c, err)
	c.Check(err, jc.Satisfies, common.IsCredentialNotValid)
}

func (*vpcSuite) checkErrorMatchesCannotVerifyVPC(c *gc.C, err error) {
	expectedError := `Juju could not verify whether the given vpc-id(.|\n)*AWS failed!`
	c.Check(err, gc.ErrorMatches, expectedError)
}

func (s *vpcSuite) TestValidateModelVPCUnexpectedError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	err := validateModelVPC(s.stubAPI, s.cloudCallCtx, "model", anyVPCID)
	s.checkErrorMatchesCannotVerifyVPC(c, err)

	s.stubAPI.CheckCallNames(c, "VPCs")
}

func (s *vpcSuite) TestValidateModelVPCNotUsableError(c *gc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	err := validateModelVPC(s.stubAPI, s.cloudCallCtx, "model", "foo")
	expectedError := `Juju cannot use the given vpc-id for the model being added(.|\n)*vpc ID 'foo' does not exist.*`
	c.Check(err, gc.ErrorMatches, expectedError)
	c.Check(err, jc.Satisfies, isVPCNotUsableError)

	s.stubAPI.CheckCallNames(c, "VPCs")
}

func (s *vpcSuite) TestValidateModelVPCCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("foo"))
	err := validateModelVPC(s.stubAPI, s.cloudCallCtx, "model", "foo")
	expectedError := `Juju could not verify whether the given vpc-id(.|\n)*`
	c.Check(err, gc.ErrorMatches, expectedError)
	c.Check(err, jc.Satisfies, common.IsCredentialNotValid)
}

func (s *vpcSuite) TestValidateModelVPCIDNotSetOrNone(c *gc.C) {
	const emptyVPCID = ""
	err := validateModelVPC(s.stubAPI, s.cloudCallCtx, "model", emptyVPCID)
	c.Check(err, jc.ErrorIsNil)

	err = validateModelVPC(s.stubAPI, s.cloudCallCtx, "model", vpcIDNone)
	c.Check(err, jc.ErrorIsNil)

	s.stubAPI.CheckNoCalls(c)
}

// NOTE: validateVPC tests only verify expected error types for all code paths,
// but do not check passed API arguments or exact error messages, as those are
// extensively tested separately below.

func (s *vpcSuite) TestValidateVPCWithEmptyVPCIDOrNilAPIClient(c *gc.C) {
	err := validateVPC(s.stubAPI, s.cloudCallCtx, "")
	c.Assert(err, gc.ErrorMatches, "invalid arguments: empty VPC ID or nil client")

	err = validateVPC(nil, s.cloudCallCtx, anyVPCID)
	c.Assert(err, gc.ErrorMatches, "invalid arguments: empty VPC ID or nil client")

	s.stubAPI.CheckNoCalls(c)
}

func (s *vpcSuite) TestValidateVPCWhenVPCIDNotFound(c *gc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	err := validateVPC(s.stubAPI, s.cloudCallCtx, anyVPCID)
	c.Check(err, jc.Satisfies, isVPCNotUsableError)

	s.stubAPI.CheckCallNames(c, "VPCs")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoSubnets(c *gc.C) {
	s.stubAPI.SetVPCsResponse(1, availableState, notDefaultVPC)
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	err := validateVPC(s.stubAPI, s.cloudCallCtx, anyVPCID)
	c.Check(err, jc.Satisfies, isVPCNotUsableError)

	s.stubAPI.CheckCallNames(c, "VPCs", "Subnets")
}
func (s *vpcSuite) TestValidateVPCWhenVPCNotAvailable(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetVPCsResponse(1, "bad-state", notDefaultVPC)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "VPCs")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoPublicSubnets(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetSubnetsResponse(1, anyZone, noPublicIPOnLaunch)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "Subnets")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoGateway(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetGatewaysResponse(noResults, anyState)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "InternetGateways")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoAttachedGateway(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetGatewaysResponse(1, "pending")

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "InternetGateways")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoRouteTables(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse() // no route tables at all

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "RouteTables")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasNoMainRouteTable(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, notMainRouteTable, nil, nil),
	)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "RouteTables")
}

func (s *vpcSuite) TestValidateVPCWhenVPCHasMainRouteTableWithoutRoutes(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, mainRouteTable, nil, nil),
	)

	s.stubAPI.CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c, s.cloudCallCtx, "RouteTables")
}

func (s *vpcSuite) TestValidateVPCSuccess(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()

	err := validateVPC(s.stubAPI, s.cloudCallCtx, anyVPCID)
	c.Assert(err, jc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "VPCs", "Subnets", "InternetGateways", "RouteTables")
}

func (s *vpcSuite) TestValidateModelVPCSuccess(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()

	err := validateModelVPC(s.stubAPI, s.cloudCallCtx, "model", anyVPCID)
	c.Assert(err, jc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "VPCs", "Subnets", "InternetGateways", "RouteTables")
	c.Check(c.GetTestLog(), jc.Contains, `INFO juju.provider.ec2 Using VPC "vpc-anything" for model "model"`)
}

func (s *vpcSuite) TestValidateModelVPCNotRecommendedStillOK(c *gc.C) {
	s.stubAPI.PrepareValidateVPCResponses()
	s.stubAPI.SetSubnetsResponse(1, anyZone, noPublicIPOnLaunch)

	err := validateModelVPC(s.stubAPI, s.cloudCallCtx, "model", anyVPCID)
	c.Assert(err, jc.ErrorIsNil)

	s.stubAPI.CheckCallNames(c, "VPCs", "Subnets")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, `INFO juju.provider.ec2 Juju will use, but does not recommend `+
		`using VPC "vpc-anything": VPC contains no public subnets`)
	c.Check(testLog, jc.Contains, `INFO juju.provider.ec2 Using VPC "vpc-anything" for model "model"`)
}

func (s *vpcSuite) TestGetVPCByIDWithMissingID(c *gc.C) {
	s.stubAPI.SetErrors(makeVPCNotFoundError("foo"))

	vpc, err := getVPCByID(s.stubAPI, s.cloudCallCtx, "foo")
	c.Assert(err, gc.ErrorMatches, `The vpc ID 'foo' does not exist \(InvalidVpcID.NotFound\)`)
	c.Check(err, jc.Satisfies, isVPCNotUsableError)
	c.Check(vpc, gc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "foo")
}

func (s *vpcSuite) TestGetVPCByIDUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	vpc, err := getVPCByID(s.stubAPI, s.cloudCallCtx, "bar")
	c.Assert(err, gc.ErrorMatches, `unexpected AWS response getting VPC "bar": AWS failed!`)
	c.Check(vpc, gc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "bar")
}

func (s *vpcSuite) TestGetVPCByIDCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("AWS failed!"))

	vpc, err := getVPCByID(s.stubAPI, s.cloudCallCtx, "bar")
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Check(vpc, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCByIDNoResults(c *gc.C) {
	s.stubAPI.SetVPCsResponse(noResults, anyState, notDefaultVPC)

	vpc, err := getVPCByID(s.stubAPI, s.cloudCallCtx, "vpc-42")
	c.Assert(err, gc.ErrorMatches, `VPC "vpc-42" not found`)
	c.Check(err, jc.Satisfies, isVPCNotUsableError)
	c.Check(vpc, gc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-42")
}

func (s *vpcSuite) TestGetVPCByIDMultipleResults(c *gc.C) {
	s.stubAPI.SetVPCsResponse(5, anyState, notDefaultVPC)

	vpc, err := getVPCByID(s.stubAPI, s.cloudCallCtx, "vpc-33")
	c.Assert(err, gc.ErrorMatches, "expected 1 result from AWS, got 5")
	c.Check(vpc, gc.IsNil)

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-33")
}

func (s *vpcSuite) TestGetVPCByIDSuccess(c *gc.C) {
	s.stubAPI.SetVPCsResponse(1, anyState, notDefaultVPC)

	vpc, err := getVPCByID(s.stubAPI, s.cloudCallCtx, "vpc-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(vpc, jc.DeepEquals, &s.stubAPI.vpcsResponse.VPCs[0])

	s.stubAPI.CheckSingleVPCsCall(c, "vpc-1")
}

func (s *vpcSuite) TestIsVPCNotFoundError(c *gc.C) {
	c.Check(isVPCNotFoundError(nil), jc.IsFalse)

	nonEC2Error := errors.New("boom")
	c.Check(isVPCNotFoundError(nonEC2Error), jc.IsFalse)

	ec2Error := makeEC2Error(444, "code", "bad stuff", "req-id")
	c.Check(isVPCNotFoundError(ec2Error), jc.IsFalse)

	ec2Error = makeVPCNotFoundError("some-id")
	c.Check(isVPCNotFoundError(ec2Error), jc.IsTrue)
}

func (s *vpcSuite) TestCheckVPCIsAvailable(c *gc.C) {
	availableVPC := makeEC2VPC(anyVPCID, availableState)
	c.Check(checkVPCIsAvailable(availableVPC), jc.ErrorIsNil)

	defaultVPC := makeEC2VPC(anyVPCID, availableState)
	defaultVPC.IsDefault = true
	c.Check(checkVPCIsAvailable(defaultVPC), jc.ErrorIsNil)

	notAvailableVPC := makeEC2VPC(anyVPCID, anyState)
	err := checkVPCIsAvailable(notAvailableVPC)
	c.Assert(err, gc.ErrorMatches, `VPC has unexpected state "any state"`)
	c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
}

func (s *vpcSuite) TestGetVPCSubnetUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnets, err := getVPCSubnets(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, gc.ErrorMatches, `unexpected AWS response getting subnets of VPC "vpc-anything": AWS failed!`)
	c.Check(subnets, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCSubnetCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnets, err := getVPCSubnets(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Check(subnets, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCSubnetsNoResults(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnets, err := getVPCSubnets(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, gc.ErrorMatches, `no subnets found for VPC "vpc-anything"`)
	c.Check(err, jc.Satisfies, isVPCNotUsableError)
	c.Check(subnets, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCSubnetsSuccess(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnets, err := getVPCSubnets(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets, jc.DeepEquals, s.stubAPI.subnetsResponse.Subnets)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

func (s *vpcSuite) TestFindFirstPublicSubnetSuccess(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, withPublicIPOnLaunch)
	s.stubAPI.subnetsResponse.Subnets[0].MapPublicIPOnLaunch = false

	subnet, err := findFirstPublicSubnet(s.stubAPI.subnetsResponse.Subnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnet, jc.DeepEquals, &s.stubAPI.subnetsResponse.Subnets[1])
}

func (s *vpcSuite) TestFindFirstPublicSubnetNoneFound(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, anyZone, noPublicIPOnLaunch)

	subnet, err := findFirstPublicSubnet(s.stubAPI.subnetsResponse.Subnets)
	c.Assert(err, gc.ErrorMatches, "VPC contains no public subnets")
	c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
	c.Check(subnet, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCInternetGatewayNoResults(c *gc.C) {
	s.stubAPI.SetGatewaysResponse(noResults, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, gc.ErrorMatches, `VPC has no Internet Gateway attached`)
	c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
	c.Check(gateway, gc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCInternetGatewayUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, gc.ErrorMatches, `unexpected AWS response getting Internet Gateway of VPC "vpc-anything": AWS failed!`)
	c.Check(gateway, gc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCInternetGatewayCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Check(gateway, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCInternetGatewayMultipleResults(c *gc.C) {
	s.stubAPI.SetGatewaysResponse(3, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, gc.ErrorMatches, "expected 1 result from AWS, got 3")
	c.Check(gateway, gc.IsNil)

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCInternetGatewaySuccess(c *gc.C) {
	s.stubAPI.SetGatewaysResponse(1, anyState)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	gateway, err := getVPCInternetGateway(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gateway, jc.DeepEquals, &s.stubAPI.gatewaysResponse.InternetGateways[0])

	s.stubAPI.CheckSingleInternetGatewaysCall(c, anyVPC)
}

func (s *vpcSuite) TestCheckInternetGatewayIsAvailable(c *gc.C) {
	availableIGW := makeEC2InternetGateway(anyGatewayID, availableState)
	c.Check(checkInternetGatewayIsAvailable(availableIGW), jc.ErrorIsNil)

	pendingIGW := makeEC2InternetGateway(anyGatewayID, "pending")
	err := checkInternetGatewayIsAvailable(pendingIGW)
	c.Assert(err, gc.ErrorMatches, `VPC has Internet Gateway "igw-anything" in unexpected state "pending"`)
	c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
}

func (s *vpcSuite) TestGetVPCRouteTablesNoResults(c *gc.C) {
	s.stubAPI.SetRouteTablesResponse() // no results

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	tables, err := getVPCRouteTables(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, gc.ErrorMatches, `VPC has no route tables`)
	c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
	c.Check(tables, gc.IsNil)

	s.stubAPI.CheckSingleRouteTablesCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCRouteTablesUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	tables, err := getVPCRouteTables(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, gc.ErrorMatches, `unexpected AWS response getting route tables of VPC "vpc-anything": AWS failed!`)
	c.Check(tables, gc.IsNil)

	s.stubAPI.CheckSingleRouteTablesCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCRouteTablesCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("AWS failed!"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	tables, err := getVPCRouteTables(s.stubAPI, s.cloudCallCtx, anyVPC)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Check(tables, gc.IsNil)
}

func (s *vpcSuite) TestGetVPCRouteTablesSuccess(c *gc.C) {
	givenVPC := makeEC2VPC("vpc-given", anyState)
	givenVPC.CIDRBlock = "0.1.0.0/16"
	givenGateway := makeEC2InternetGateway("igw-given", availableState)

	s.stubAPI.SetRouteTablesResponse(
		makeEC2RouteTable("rtb-other", notMainRouteTable, []string{"subnet-1", "subnet-2"}, nil),
		makeEC2RouteTable("rtb-main", mainRouteTable, nil, makeEC2Routes(
			givenGateway.Id, givenVPC.CIDRBlock, activeState, 3, // 3 extra routes
		)),
	)

	tables, err := getVPCRouteTables(s.stubAPI, s.cloudCallCtx, givenVPC)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(tables, jc.DeepEquals, s.stubAPI.routeTablesResponse.Tables)

	s.stubAPI.CheckSingleRouteTablesCall(c, givenVPC)
}

func (s *vpcSuite) TestFindVPCMainRouteTableWithMainAndPerSubnetTables(c *gc.C) {
	givenTables := []ec2.RouteTable{
		*makeEC2RouteTable("rtb-main", mainRouteTable, nil, nil),
		*makeEC2RouteTable("rtb-2-subnets", notMainRouteTable, []string{"subnet-1", "subnet-2"}, nil),
	}

	mainTable, err := findVPCMainRouteTable(givenTables)
	c.Assert(err, gc.ErrorMatches, `subnet "subnet-1" not associated with VPC "vpc-anything" main route table`)
	c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
	c.Check(mainTable, gc.IsNil)
}

func (s *vpcSuite) TestFindVPCMainRouteTableWithOnlyNonAssociatedTables(c *gc.C) {
	givenTables := []ec2.RouteTable{
		*makeEC2RouteTable("rtb-1", notMainRouteTable, nil, nil),
		*makeEC2RouteTable("rtb-2", notMainRouteTable, nil, nil),
		*makeEC2RouteTable("rtb-3", notMainRouteTable, nil, nil),
	}

	mainTable, err := findVPCMainRouteTable(givenTables)
	c.Assert(err, gc.ErrorMatches, "VPC has no associated main route table")
	c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
	c.Check(mainTable, gc.IsNil)
}

func (s *vpcSuite) TestFindVPCMainRouteTableWithSingleMainTable(c *gc.C) {
	givenTables := []ec2.RouteTable{
		*makeEC2RouteTable("rtb-main", mainRouteTable, nil, nil),
	}

	mainTable, err := findVPCMainRouteTable(givenTables)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mainTable, jc.DeepEquals, &givenTables[0])
}

func (s *vpcSuite) TestFindVPCMainRouteTableWithExtraMainTables(c *gc.C) {
	givenTables := []ec2.RouteTable{
		*makeEC2RouteTable("rtb-non-associated", notMainRouteTable, nil, nil),
		*makeEC2RouteTable("rtb-main", mainRouteTable, nil, nil),
		*makeEC2RouteTable("rtb-main-extra", mainRouteTable, nil, nil),
	}

	mainTable, err := findVPCMainRouteTable(givenTables)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mainTable, jc.DeepEquals, &givenTables[1]) // first found counts
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesWithNoDefaultRoute(c *gc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	c.Check(table.Routes, gc.HasLen, 0) // no routes at all

	checkFailed := func() {
		err := checkVPCRouteTableRoutes(vpc, table, gateway)
		c.Assert(err, gc.ErrorMatches, `missing default route via gateway "igw-anything"`)
		c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
	}
	checkFailed()

	table.Routes = makeEC2Routes(gateway.Id, vpc.CIDRBlock, "blackhole", 3) // inactive routes only
	checkFailed()

	table.Routes = makeEC2Routes("", vpc.CIDRBlock, activeState, 1) // local and 1 extra route
	checkFailed()

	table.Routes = makeEC2Routes("", vpc.CIDRBlock, activeState, 0) // local route only
	checkFailed()
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesWithDefaultButNoLocalRoutes(c *gc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	table.Routes = makeEC2Routes(gateway.Id, "", activeState, 3) // default and 3 extra routes; no local route

	checkFailed := func() {
		err := checkVPCRouteTableRoutes(vpc, table, gateway)
		c.Assert(err, gc.ErrorMatches, `missing local route with destination "0.1.0.0/16"`)
		c.Check(err, jc.Satisfies, isVPCNotRecommendedError)
	}
	checkFailed()

	table.Routes = makeEC2Routes(gateway.Id, "", activeState, 0) // only default route
	checkFailed()
}

func (s *vpcSuite) TestCheckVPCRouteTableRoutesSuccess(c *gc.C) {
	vpc, table, gateway := prepareCheckVPCRouteTableRoutesArgs()
	table.Routes = makeEC2Routes(gateway.Id, vpc.CIDRBlock, activeState, 3) // default, local and 3 extra routes

	err := checkVPCRouteTableRoutes(vpc, table, gateway)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *vpcSuite) TestFindDefaultVPCIDUnexpectedAWSError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("AWS failed!"))

	vpcID, err := findDefaultVPCID(s.stubAPI, s.cloudCallCtx)
	c.Assert(err, gc.ErrorMatches, "unexpected AWS response getting default-vpc account attribute: AWS failed!")
	c.Check(vpcID, gc.Equals, "")

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestFindDefaultVPCIDCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("AWS failed!"))
	_, err := findDefaultVPCID(s.stubAPI, s.cloudCallCtx)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
}

func (s *vpcSuite) TestFindDefaultVPCIDNoAttributeOrNoValue(c *gc.C) {
	s.stubAPI.SetAttributesResponse(nil) // no attributes at all

	checkFailed := func() {
		vpcID, err := findDefaultVPCID(s.stubAPI, s.cloudCallCtx)
		c.Assert(err, gc.ErrorMatches, "default-vpc account attribute not found")
		c.Check(err, jc.Satisfies, errors.IsNotFound)
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

	vpcID, err := findDefaultVPCID(s.stubAPI, s.cloudCallCtx)
	c.Assert(err, gc.ErrorMatches, "default VPC not found")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(vpcID, gc.Equals, "")

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestFindDefaultVPCIDSuccess(c *gc.C) {
	s.stubAPI.SetAttributesResponse(map[string][]string{
		"default-vpc": {"vpc-foo", "vpc-bar"},
	})

	vpcID, err := findDefaultVPCID(s.stubAPI, s.cloudCallCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(vpcID, gc.Equals, "vpc-foo") // always the first value is used.

	s.stubAPI.CheckSingleAccountAttributesCall(c, "default-vpc")
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsError(c *gc.C) {
	s.stubAPI.SetErrors(errors.New("too cloudy"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnetIDs, err := getVPCSubnetIDsForAvailabilityZone(s.stubAPI, s.cloudCallCtx, anyVPC.Id, anyZone, nil)
	c.Assert(err, gc.ErrorMatches, `cannot get VPC "vpc-anything" subnets: unexpected AWS .*: too cloudy`)
	c.Check(subnetIDs, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsCredentialError(c *gc.C) {
	s.stubAPI.SetErrors(common.NewCredentialNotValid("too cloudy"))

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnetIDs, err := getVPCSubnetIDsForAvailabilityZone(s.stubAPI, s.cloudCallCtx, anyVPC.Id, anyZone, nil)
	c.Assert(err, gc.ErrorMatches, `cannot get VPC "vpc-anything" subnets: unexpected AWS .*: too cloudy`)
	c.Check(subnetIDs, gc.IsNil)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneNoSubnetsAtAll(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(noResults, anyZone, noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnetIDs, err := getVPCSubnetIDsForAvailabilityZone(s.stubAPI, s.cloudCallCtx, anyVPC.Id, anyZone, nil)
	c.Assert(err, gc.ErrorMatches, `VPC "vpc-anything" has no subnets in AZ "any-zone": no subnets found for VPC.*`)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(subnetIDs, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneNoSubnetsInAZ(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(3, "other-zone", noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnetIDs, err := getVPCSubnetIDsForAvailabilityZone(s.stubAPI, s.cloudCallCtx, anyVPC.Id, "given-zone", nil)
	c.Assert(err, gc.ErrorMatches, `VPC "vpc-anything" has no subnets in AZ "given-zone"`)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(subnetIDs, gc.IsNil)

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneWithSubnetsToZones(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(4, "my-zone", noPublicIPOnLaunch)
	// Simulate we used --constraints spaces=foo, which contains subnet-1 and
	// subnet-3 out of the 4 subnets in AZ my-zone (see the related bug
	// http://pad.lv/1609343).
	allowedSubnetIDs := []string{"subnet-1", "subnet-3"}

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnetIDs, err := getVPCSubnetIDsForAvailabilityZone(s.stubAPI, s.cloudCallCtx, anyVPC.Id, "my-zone", allowedSubnetIDs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetIDs, jc.DeepEquals, []string{"subnet-1", "subnet-3"})

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

func (s *vpcSuite) TestGetVPCSubnetIDsForAvailabilityZoneSuccess(c *gc.C) {
	s.stubAPI.SetSubnetsResponse(2, "my-zone", noPublicIPOnLaunch)

	anyVPC := makeEC2VPC(anyVPCID, anyState)
	subnetIDs, err := getVPCSubnetIDsForAvailabilityZone(s.stubAPI, s.cloudCallCtx, anyVPC.Id, "my-zone", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Result slice of IDs is always sorted.
	c.Check(subnetIDs, jc.DeepEquals, []string{"subnet-0", "subnet-1"})

	s.stubAPI.CheckSingleSubnetsCall(c, anyVPC)
}

var fakeSubnetsToZones = map[network.Id][]string{
	"subnet-foo": {"az1", "az2"},
	"subnet-bar": {"az1"},
	"subnet-oof": {"az3"},
}

func (s *vpcSuite) TestFindSubnetIDsForAvailabilityZoneNoneFound(c *gc.C) {
	subnetIDs, err := findSubnetIDsForAvailabilityZone("unknown-zone", fakeSubnetsToZones)
	c.Assert(err, gc.ErrorMatches, `subnets in AZ "unknown-zone" not found`)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(subnetIDs, gc.IsNil)
}

func (s *vpcSuite) TestFindSubnetIDsForAvailabilityOneMatched(c *gc.C) {
	subnetIDs, err := findSubnetIDsForAvailabilityZone("az3", fakeSubnetsToZones)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetIDs, gc.DeepEquals, []string{"subnet-oof"})
}

func (s *vpcSuite) TestFindSubnetIDsForAvailabilityMultipleMatched(c *gc.C) {
	subnetIDs, err := findSubnetIDsForAvailabilityZone("az1", fakeSubnetsToZones)
	c.Assert(err, jc.ErrorIsNil)
	// Result slice of IDs is always sorted.
	c.Check(subnetIDs, gc.DeepEquals, []string{"subnet-bar", "subnet-foo"})
}

const (
	notDefaultVPC = false
	defaultVPC    = true

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
	vpcAPIClient // embedded mostly for documentation

	attributesResponse  *ec2.AccountAttributesResp
	vpcsResponse        *ec2.VPCsResp
	subnetsResponse     *ec2.SubnetsResp
	gatewaysResponse    *ec2.InternetGatewaysResp
	routeTablesResponse *ec2.RouteTablesResp
}

// AccountAttributes implements vpcAPIClient and is used to test finding the
// default VPC from the "default-vpc"" attribute.
func (s *stubVPCAPIClient) AccountAttributes(attributeNames ...string) (*ec2.AccountAttributesResp, error) {
	s.Stub.AddCall("AccountAttributes", makeArgsFromStrings(attributeNames...)...)
	return s.attributesResponse, s.Stub.NextErr()
}

// VPCs implements vpcAPIClient and is used to test getting the details of a
// VPC.
func (s *stubVPCAPIClient) VPCs(ids []string, filter *ec2.Filter) (*ec2.VPCsResp, error) {
	s.Stub.AddCall("VPCs", ids, filter)
	return s.vpcsResponse, s.Stub.NextErr()
}

// Subnets implements vpcAPIClient and is used to test getting a VPC's subnets.
func (s *stubVPCAPIClient) Subnets(ids []string, filter *ec2.Filter) (*ec2.SubnetsResp, error) {
	s.Stub.AddCall("Subnets", ids, filter)
	return s.subnetsResponse, s.Stub.NextErr()
}

// InternetGateways implements vpcAPIClient and is used to test getting the
// attached IGW of a VPC.
func (s *stubVPCAPIClient) InternetGateways(ids []string, filter *ec2.Filter) (*ec2.InternetGatewaysResp, error) {
	s.Stub.AddCall("InternetGateways", ids, filter)
	return s.gatewaysResponse, s.Stub.NextErr()
}

// RouteTables implements vpcAPIClient and is used to test getting all route
// tables of a VPC, alond with their routes.
func (s *stubVPCAPIClient) RouteTables(ids []string, filter *ec2.Filter) (*ec2.RouteTablesResp, error) {
	s.Stub.AddCall("RouteTables", ids, filter)
	return s.routeTablesResponse, s.Stub.NextErr()
}

func (s *stubVPCAPIClient) SetAttributesResponse(attributeNameToValues map[string][]string) {
	s.attributesResponse = &ec2.AccountAttributesResp{
		RequestId:  "fake-request-id",
		Attributes: make([]ec2.AccountAttribute, 0, len(attributeNameToValues)),
	}

	for name, values := range attributeNameToValues {
		attribute := ec2.AccountAttribute{
			Name:   name,
			Values: values,
		}
		s.attributesResponse.Attributes = append(s.attributesResponse.Attributes, attribute)
	}
}
func (s *stubVPCAPIClient) CheckSingleAccountAttributesCall(c *gc.C, attributeNames ...string) {
	s.Stub.CheckCallNames(c, "AccountAttributes")
	s.Stub.CheckCall(c, 0, "AccountAttributes", makeArgsFromStrings(attributeNames...)...)
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetVPCsResponse(numResults int, state string, isDefault bool) {
	s.vpcsResponse = &ec2.VPCsResp{
		RequestId: "fake-request-id",
		VPCs:      make([]ec2.VPC, numResults),
	}

	for i := range s.vpcsResponse.VPCs {
		id := fmt.Sprintf("vpc-%d", i)
		vpc := makeEC2VPC(id, state)
		vpc.IsDefault = isDefault
		s.vpcsResponse.VPCs[i] = *vpc
	}
}

func (s *stubVPCAPIClient) CheckSingleVPCsCall(c *gc.C, vpcID string) {
	var nilFilter *ec2.Filter
	s.Stub.CheckCallNames(c, "VPCs")
	s.Stub.CheckCall(c, 0, "VPCs", []string{vpcID}, nilFilter)
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetSubnetsResponse(numResults int, zone string, mapPublicIpOnLaunch bool) {
	s.subnetsResponse = &ec2.SubnetsResp{
		RequestId: "fake-request-id",
		Subnets:   make([]ec2.Subnet, numResults),
	}

	for i := range s.subnetsResponse.Subnets {
		s.subnetsResponse.Subnets[i] = ec2.Subnet{
			Id:                  fmt.Sprintf("subnet-%d", i),
			VPCId:               anyVPCID,
			State:               anyState,
			AvailZone:           zone,
			CIDRBlock:           fmt.Sprintf("0.1.%d.0/20", i),
			MapPublicIPOnLaunch: mapPublicIpOnLaunch,
		}
	}
}

func (s *stubVPCAPIClient) CheckSingleSubnetsCall(c *gc.C, vpc *ec2.VPC) {
	var nilIDs []string
	filter := ec2.NewFilter()
	filter.Add("vpc-id", vpc.Id)

	s.Stub.CheckCallNames(c, "Subnets")
	s.Stub.CheckCall(c, 0, "Subnets", nilIDs, filter)
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetGatewaysResponse(numResults int, attachmentState string) {
	s.gatewaysResponse = &ec2.InternetGatewaysResp{
		RequestId:        "fake-request-id",
		InternetGateways: make([]ec2.InternetGateway, numResults),
	}

	for i := range s.gatewaysResponse.InternetGateways {
		id := fmt.Sprintf("igw-%d", i)
		gateway := makeEC2InternetGateway(id, attachmentState)
		s.gatewaysResponse.InternetGateways[i] = *gateway
	}
}

func (s *stubVPCAPIClient) CheckSingleInternetGatewaysCall(c *gc.C, vpc *ec2.VPC) {
	var nilIDs []string
	filter := ec2.NewFilter()
	filter.Add("attachment.vpc-id", vpc.Id)

	s.Stub.CheckCallNames(c, "InternetGateways")
	s.Stub.CheckCall(c, 0, "InternetGateways", nilIDs, filter)
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) SetRouteTablesResponse(tables ...*ec2.RouteTable) {
	s.routeTablesResponse = &ec2.RouteTablesResp{
		RequestId: "fake-request-id",
		Tables:    make([]ec2.RouteTable, len(tables)),
	}

	for i := range s.routeTablesResponse.Tables {
		s.routeTablesResponse.Tables[i] = *tables[i]
	}
}

func (s *stubVPCAPIClient) CheckSingleRouteTablesCall(c *gc.C, vpc *ec2.VPC) {
	var nilIDs []string
	filter := ec2.NewFilter()
	filter.Add("vpc-id", vpc.Id)

	s.Stub.CheckCallNames(c, "RouteTables")
	s.Stub.CheckCall(c, 0, "RouteTables", nilIDs, filter)
	s.Stub.ResetCalls()
}

func (s *stubVPCAPIClient) PrepareValidateVPCResponses() {
	s.SetVPCsResponse(1, availableState, notDefaultVPC)
	s.vpcsResponse.VPCs[0].CIDRBlock = "0.1.0.0/16"
	s.SetSubnetsResponse(1, anyZone, withPublicIPOnLaunch)
	s.SetGatewaysResponse(1, availableState)
	onlyDefaultAndLocalRoutes := makeEC2Routes(
		s.gatewaysResponse.InternetGateways[0].Id,
		s.vpcsResponse.VPCs[0].CIDRBlock,
		activeState,
		0, // no extra routes
	)
	s.SetRouteTablesResponse(
		makeEC2RouteTable(anyTableID, mainRouteTable, nil, onlyDefaultAndLocalRoutes),
	)
}

func (s *stubVPCAPIClient) CallValidateVPCAndCheckCallsUpToExpectingVPCNotRecommendedError(c *gc.C, ctx context.ProviderCallContext, lastExpectedCallName string) {
	err := validateVPC(s, ctx, anyVPCID)
	c.Assert(err, jc.Satisfies, isVPCNotRecommendedError)

	allCalls := []string{"VPCs", "Subnets", "InternetGateways", "RouteTables"}
	var expectedCalls []string
	for i := range allCalls {
		expectedCalls = append(expectedCalls, allCalls[i])
		if allCalls[i] == lastExpectedCallName {
			break
		}
	}
	s.CheckCallNames(c, expectedCalls...)
}

func makeEC2VPC(vpcID, state string) *ec2.VPC {
	return &ec2.VPC{
		Id:    vpcID,
		State: state,
	}
}

func makeEC2InternetGateway(gatewayID, attachmentState string) *ec2.InternetGateway {
	return &ec2.InternetGateway{
		Id:              gatewayID,
		VPCId:           anyVPCID,
		AttachmentState: attachmentState,
	}
}

func makeEC2RouteTable(tableID string, isMain bool, associatedSubnetIDs []string, routes []ec2.Route) *ec2.RouteTable {
	table := &ec2.RouteTable{
		Id:     tableID,
		VPCId:  anyVPCID,
		Routes: routes,
	}

	if isMain {
		table.Associations = []ec2.RouteTableAssociation{{
			Id:      "rtbassoc-main",
			TableId: tableID,
			IsMain:  true,
		}}
	} else {
		table.Associations = make([]ec2.RouteTableAssociation, len(associatedSubnetIDs))
		for i := range associatedSubnetIDs {
			table.Associations[i] = ec2.RouteTableAssociation{
				Id:       fmt.Sprintf("rtbassoc-%d", i),
				TableId:  tableID,
				SubnetId: associatedSubnetIDs[i],
			}
		}
	}
	return table
}

func makeEC2Routes(defaultRouteGatewayID, localRouteCIDRBlock, state string, numExtraRoutes int) []ec2.Route {
	var routes []ec2.Route

	if defaultRouteGatewayID != "" {
		routes = append(routes, ec2.Route{
			DestinationCIDRBlock: defaultRouteCIDRBlock,
			GatewayId:            defaultRouteGatewayID,
			State:                state,
		})
	}

	if localRouteCIDRBlock != "" {
		routes = append(routes, ec2.Route{
			DestinationCIDRBlock: localRouteCIDRBlock,
			GatewayId:            localRouteGatewayID,
			State:                state,
		})
	}

	if numExtraRoutes > 0 {
		for i := 0; i < numExtraRoutes; i++ {
			routes = append(routes, ec2.Route{
				DestinationCIDRBlock: fmt.Sprintf("0.1.%d.0/24", i),
				State:                state,
			})
		}
	}

	return routes
}

func prepareCheckVPCRouteTableRoutesArgs() (*ec2.VPC, *ec2.RouteTable, *ec2.InternetGateway) {
	anyVPC := makeEC2VPC(anyVPCID, anyState)
	anyVPC.CIDRBlock = "0.1.0.0/16"
	anyTable := makeEC2RouteTable(anyTableID, notMainRouteTable, nil, nil)
	anyGateway := makeEC2InternetGateway(anyGatewayID, anyState)

	return anyVPC, anyTable, anyGateway
}

func makeEC2Error(statusCode int, code, message, requestID string) error {
	return &ec2.Error{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		RequestId:  requestID,
	}
}

func makeVPCNotFoundError(vpcID string) error {
	return makeEC2Error(
		400,
		"InvalidVpcID.NotFound",
		fmt.Sprintf("The vpc ID '%s' does not exist", vpcID),
		"fake-request-id",
	)
}

func makeArgsFromStrings(strings ...string) []interface{} {
	args := make([]interface{}, len(strings))
	for i := range strings {
		args[i] = strings[i]
	}
	return args
}
