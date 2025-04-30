// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/errors"
	"github.com/kr/pretty"

	corelogger "github.com/juju/juju/core/logger"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

const (
	// attachmentStatusAvailble is our own enum of available for attachment
	// status as the AWS sdk doesn't currently provide this enum.
	// Submitted https://github.com/aws/aws-sdk-go-v2/issues/2235 to address
	// this issue.
	attachmentStatusAvailable = types.AttachmentStatus("available")

	localRouteGatewayID       = "local"
	defaultRouteIpv4CIDRBlock = "0.0.0.0/0"
	defaultRouteIPv6CIDRBlock = "::/0"
	vpcIDNone                 = "none"

	// errorVPCNotRecommended indicates a user-specified VPC is unlikely to be
	// suitable for hosting a Juju controller instance and/or exposed workloads,
	// due to not satisfying the mininum requirements described in
	// validateVPC()'s doc comment. Users can still force Juju to use such a
	// VPC by passing 'vpc-id-force=true' setting.
	errorVPCNotRecommended = errors.ConstError("vpc not recommended")

	// errorVPCNotUsable indicates a user-specified VPC cannot be used either
	// because it is missing or because it contains no subnets.
	errorVPCNotUsable = errors.ConstError("vpc not usable")
)

var (
	vpcNotUsableForBootstrapErrorPrefix = `
Juju cannot use the given vpc-id for bootstrapping a controller
instance. Please, double check the given VPC ID is correct, and that
the VPC contains at least one subnet.

Error details`[1:]

	vpcNotUsableForModelErrorPrefix = `
Juju cannot use the given vpc-id for the model being added.
Please double check the given VPC ID is correct, and that
the VPC contains at least one subnet.

Error details`[1:]

	vpcNotRecommendedErrorPrefix = `
The given vpc-id does not meet one or more of the following minimum
Juju requirements:

1. VPC should be in "available" state and contain one or more subnets.
2. An Internet Gateway (IGW) should be attached to the VPC.
3. The main route table of the VPC should have both a default route
   to the attached IGW and a local route matching the VPC CIDR block.
4. At least one of the VPC subnets should have MapPublicIPOnLaunch
   attribute enabled (i.e. at least one subnet needs to be 'public').
5. All subnets should be implicitly associated to the VPC main route
   table, rather than explicitly to per-subnet route tables.

A default VPC already satisfies all of the requirements above. If you
still want to use the VPC, try running 'juju bootstrap' again with:

  --config vpc-id=%s --config vpc-id-force=true

to force Juju to bypass the requirements check (NOT recommended unless
you understand the implications: most importantly, not being able to
access the Juju controller, likely causing bootstrap to fail, or trying
to deploy exposed workloads on instances started in private or isolated
subnets).

Error details`[1:]

	cannotValidateVPCErrorPrefix = `
Juju could not verify whether the given vpc-id meets the minimum Juju
connectivity requirements. Please, double check the VPC ID is correct,
you have a working connection to the Internet, your AWS credentials are
sufficient to access VPC features, or simply retry bootstrapping again.

Error details`[1:]

	vpcNotRecommendedButForcedWarning = `
WARNING! The specified vpc-id does not satisfy the minimum Juju requirements,
but will be used anyway because vpc-id-force=true is also specified.

`[1:]
)

// vpcAPIClient defines a subset of the aws sdk API calls needed to validate a VPC.
type vpcAPIClient interface {
	DescribeAccountAttributes(context.Context, *ec2.DescribeAccountAttributesInput, ...func(*ec2.Options)) (*ec2.DescribeAccountAttributesOutput, error)
	DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeInternetGateways(context.Context, *ec2.DescribeInternetGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error)
	DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
}

// validateVPC requires both arguments to be set and validates that vpcID refers
// to an existing AWS VPC (default or non-default) for the current region.
// Returns an error satifying isVPCNotUsableError() when the VPC with the given
// vpcID cannot be found, or when the VPC exists but contains no subnets.
// Returns an error satisfying isVPCNotRecommendedError() in the following
// cases:
//
//  1. The VPC's state is not "available".
//  2. The VPC does not have an Internet Gateway (IGW) attached.
//  3. A main route table is not associated with the VPC.
//  4. The main route table lacks both a default route via the IGW and a local
//     route matching the VPC's CIDR block.
//  5. One or more of the VPC's subnets are not associated with the main route
//     table of the VPC.
//  6. None of the the VPC's subnets have the MapPublicIPOnLaunch attribute set.
//
// With the vpc-id-force config setting set to true, the provider can ignore a
// vpcNotRecommendedError. A vpcNotUsableError cannot be ignored, while
// unexpected API responses and errors could be retried.
//
// The above minimal requirements allow Juju to work out-of-the-box with most
// common (and officially documented by AWS) VPC setups, easy try out with AWS
// Console / VPC Wizard / CLI. Detecting VPC setups indicating intentional
// customization by experienced users, protecting beginners from bad Juju-UX due
// to broken VPC setup, while still allowing power users to override that and
// continue (but knowing what that implies).
func validateVPC(ctx context.Context, apiClient vpcAPIClient, vpcID string) error {
	if vpcID == "" || apiClient == nil {
		return errors.Errorf("invalid arguments: empty VPC ID or nil client")
	}

	vpc, err := getVPCByID(ctx, apiClient, vpcID)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkVPCIsAvailable(vpc); err != nil {
		return errors.Trace(err)
	}

	subnets, err := getVPCSubnets(ctx, apiClient, vpcID)
	if err != nil {
		return errors.Trace(err)
	}

	publicSubnet, err := findFirstPublicSubnet(subnets)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(dimitern): Rather than just logging that, use publicSubnet.Id or
	// even publicSubnet.AvailZone as default bootstrap placement directive, so
	// the controller would be reachable.
	logger.Infof(ctx,
		"found subnet %q (%s) in AZ %q, suitable for a Juju controller instance",
		aws.ToString(publicSubnet.SubnetId), aws.ToString(publicSubnet.CidrBlock), aws.ToString(publicSubnet.AvailabilityZone),
	)

	gateway, err := getVPCInternetGateway(ctx, apiClient, vpc)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkInternetGatewayIsAvailable(gateway); err != nil {
		return errors.Trace(err)
	}

	mainRouteTable, err := getVPCMainRouteTable(ctx, apiClient, vpc)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkVPCRouteTableRoutes(ctx, vpc, &mainRouteTable, gateway); err != nil {
		return errors.Annotatef(err, "VPC %q main route table %q", vpcID, aws.ToString(mainRouteTable.RouteTableId))
	}

	logger.Infof(ctx, "VPC %q is suitable for Juju controllers and expose-able workloads", vpcID)
	return nil
}

// isDualStackSubnet interrogates the aws subnet to assert if it is a dual stack
// subnet. The criteria for this is the subnet must have both ipv4 and ipv6
// address space and the subnet must auto assign ipv6 address on instance
// creation.
func isDualStackSubnet(subnet types.Subnet) bool {
	ipv6Native := subnet.Ipv6Native != nil && *subnet.Ipv6Native
	assignOnCreation := subnet.AssignIpv6AddressOnCreation != nil && *subnet.AssignIpv6AddressOnCreation
	return !ipv6Native && assignOnCreation
}

func getVPCByID(ctx context.Context, apiClient vpcAPIClient, vpcID string) (*types.Vpc, error) {
	response, err := apiClient.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		VpcIds: []string{vpcID},
	})
	if isVPCNotFoundError(err) {
		return nil, fmt.Errorf("VPC %q %w%w", vpcID, errors.NotFound, errors.Hide(errorVPCNotUsable))
	} else if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting VPC %q", vpcID)
	}

	if numResults := len(response.Vpcs); numResults == 0 {
		return nil, fmt.Errorf("VPC %q %w%w", vpcID, errors.NotFound, errors.Hide(errorVPCNotUsable))
	} else if numResults > 1 {
		logger.Debugf(ctx, "VPCs() returned %s", pretty.Sprint(response))
		return nil, errors.Errorf("expected 1 result from AWS, got %d", numResults)
	}

	vpc := response.Vpcs[0]
	return &vpc, nil
}

func isVPCNotFoundError(err error) bool {
	return err != nil && ec2ErrCode(err) == "InvalidVpcID.NotFound"
}

func checkVPCIsAvailable(vpc *types.Vpc) error {
	if vpcState := vpc.State; vpcState != types.VpcStateAvailable {
		return fmt.Errorf(
			"VPC %q has unexpected state %q%w",
			*vpc.VpcId,
			vpcState,
			errors.Hide(errorVPCNotRecommended),
		)
	}

	if aws.ToBool(vpc.IsDefault) {
		logger.Infof(context.TODO(), "VPC %q is the default VPC for the region", aws.ToString(vpc.VpcId))
	}

	return nil
}

func getVPCSubnets(ctx context.Context, apiClient vpcAPIClient, vpcID string) ([]types.Subnet, error) {
	response, err := apiClient.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{makeFilter("vpc-id", vpcID)},
	})
	if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting subnets of VPC %q", vpcID)
	}

	if len(response.Subnets) == 0 {
		return nil, fmt.Errorf("subnets for VPC %q %w%w", vpcID, errors.NotFound, errors.Hide(errorVPCNotUsable))
	}

	return response.Subnets, nil
}

// subnetsForIDs is responsible for taking a list of AWS subnet ids and
// returning the subnet information types for the ids. If the number of subnets
// returned by AWS differs to that of the ids into this function an error
// that conforms to NotFound is returned.
func subnetsForIDs(
	apiClient vpcAPIClient,
	ctx context.Context,
	subnetIds []corenetwork.Id,
) ([]types.Subnet, error) {
	if len(subnetIds) == 0 {
		return []types.Subnet{}, nil
	}

	awsSubnetIds := make([]string, 0, len(subnetIds))
	for _, netId := range subnetIds {
		awsSubnetIds = append(awsSubnetIds, string(netId))
	}

	response, err := apiClient.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: awsSubnetIds,
	})
	if err != nil {
		return nil, fmt.Errorf("unexpected AWS response getting subnets %v: %w", subnetIds, err)
	}

	if len(response.Subnets) != len(subnetIds) {
		lenSubnetIds := len(subnetIds)
		lenSubnets := len(response.Subnets)
		return response.Subnets, fmt.Errorf(
			"expected %d subnets got %d, %d subnets %w",
			lenSubnetIds,
			lenSubnets,
			lenSubnetIds-lenSubnets,
			errors.NotFound,
		)
	}

	return response.Subnets, nil
}

func findFirstPublicSubnet(subnets []types.Subnet) (*types.Subnet, error) {
	for _, subnet := range subnets {
		// TODO(dimitern): goamz's AddDefaultVPCAndSubnets() does not set
		// MapPublicIPOnLaunch only DefaultForAZ, but in reality the former is
		// always set when the latter is. Until this is fixed in goamz, we check
		// for both below to allow testing the behavior.
		mapOnLaunch := aws.ToBool(subnet.MapPublicIpOnLaunch)
		defaultForAZ := aws.ToBool(subnet.DefaultForAz)
		assignIpv6 := aws.ToBool(subnet.AssignIpv6AddressOnCreation)
		if mapOnLaunch || defaultForAZ || assignIpv6 {
			logger.Debugf(context.TODO(),
				"VPC %q subnet %q has MapPublicIPOnLaunch=%v, DefaultForAZ=%v, AssignIPv6AddressOnCreation=%v",
				aws.ToString(subnet.VpcId), aws.ToString(subnet.SubnetId),
				mapOnLaunch, defaultForAZ, assignIpv6,
			)
			return &subnet, nil
		}

	}
	return nil, fmt.Errorf("VPC contains no public subnets%w", errors.Hide(errorVPCNotRecommended))
}

func getVPCInternetGateway(ctx context.Context, apiClient vpcAPIClient, vpc *types.Vpc) (*types.InternetGateway, error) {
	vpcID := aws.ToString(vpc.VpcId)
	resp, err := apiClient.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
		Filters: []types.Filter{makeFilter("attachment.vpc-id", vpcID)},
	})
	if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting Internet Gateway of VPC %q", vpcID)
	}

	if numResults := len(resp.InternetGateways); numResults == 0 {
		return nil, fmt.Errorf("VPC has no Internet Gateway attached%w", errors.Hide(errorVPCNotRecommended))
	} else if numResults > 1 {
		logger.Debugf(ctx, "InternetGateways() returned %#v", resp)
		return nil, errors.Errorf("expected 1 result from AWS, got %d", numResults)
	}

	gateway := resp.InternetGateways[0]
	return &gateway, nil
}

func checkInternetGatewayIsAvailable(gateway *types.InternetGateway) error {
	gatewayID := aws.ToString(gateway.InternetGatewayId)
	if len(gateway.Attachments) == 0 {
		return fmt.Errorf("VPC has Internet Gateway %q with no attachments%w", gatewayID, errors.Hide(errorVPCNotRecommended))
	}
	if state := gateway.Attachments[0].State; state != attachmentStatusAvailable {
		return fmt.Errorf(
			"VPC has Internet Gateway %q in unexpected state %q%w",
			gatewayID,
			state,
			errors.Hide(errorVPCNotRecommended),
		)
	}

	return nil
}

func getVPCMainRouteTable(ctx context.Context, apiClient vpcAPIClient, vpc *types.Vpc) (types.RouteTable, error) {
	vpcID := aws.ToString(vpc.VpcId)
	resp, err := apiClient.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			makeFilter("association.main", "true"),
			makeFilter("vpc-id", vpcID),
		},
	})

	if err != nil {
		return types.RouteTable{}, fmt.Errorf(
			"fetching vpc %q main route table, unexpected AWS response: %w",
			vpcID,
			err,
		)
	}

	if len(resp.RouteTables) == 0 {
		return types.RouteTable{}, fmt.Errorf(
			"VPC %q has no main route tables%w",
			vpcID,
			errors.Hide(errorVPCNotRecommended),
		)
	}
	if len(resp.RouteTables) != 1 {
		return types.RouteTable{}, fmt.Errorf(
			"expected 1 result for VPC %q main route table, got %d%w",
			vpcID,
			len(resp.RouteTables),
			errors.Hide(errorVPCNotRecommended),
		)
	}

	return resp.RouteTables[0], nil
}

func checkVPCRouteTableRoutes(ctx context.Context, vpc *types.Vpc, routeTable *types.RouteTable, gateway *types.InternetGateway) error {
	hasDefaultRoute := false
	hasLocalRoute := false

	logger.Tracef(ctx, "checking route table %+v routes", routeTable)
	gatewayID := aws.ToString(gateway.InternetGatewayId)
	vpcCIDRBlock := aws.ToString(vpc.CidrBlock)
	for _, route := range routeTable.Routes {
		if route.State != types.RouteStateActive {
			if logger.IsLevelEnabled(corelogger.TRACE) {
				logger.Tracef(ctx, "skipping inactive route %s", pretty.Sprint(route))
			}
			continue
		}

		routeGatewayID := aws.ToString(route.GatewayId)
		routeCIDRBlock := aws.ToString(route.DestinationCidrBlock)
		routeCIDRIPv6Block := aws.ToString(route.DestinationIpv6CidrBlock)
		switch {
		case routeCIDRIPv6Block == defaultRouteIPv6CIDRBlock:
		case routeCIDRBlock == defaultRouteIpv4CIDRBlock:
			if routeGatewayID == gatewayID {
				logger.Tracef(ctx, "default route uses expected gateway %q", gatewayID)
				hasDefaultRoute = true
			}
		case routeGatewayID == localRouteGatewayID:
			logger.Tracef(ctx, "local route uses expected CIDR %q", vpcCIDRBlock)
			hasLocalRoute = true
		default:
			if logger.IsLevelEnabled(corelogger.TRACE) {
				logger.Tracef(ctx, "route %s is neither local nor default (skipping)", pretty.Sprint(route))
			}
		}
	}

	if hasDefaultRoute && hasLocalRoute {
		return nil
	}

	if !hasDefaultRoute {
		return fmt.Errorf("missing default route via gateway %q%w", gatewayID, errors.Hide(errorVPCNotRecommended))
	}
	return fmt.Errorf("missing local route with destination %q%w", vpcCIDRBlock, errors.Hide(errorVPCNotRecommended))
}

func findDefaultVPCID(ctx context.Context, apiClient vpcAPIClient) (string, error) {
	response, err := apiClient.DescribeAccountAttributes(ctx, &ec2.DescribeAccountAttributesInput{
		AttributeNames: []types.AccountAttributeName{"default-vpc"},
	})
	if err != nil {
		return "", errors.Annotate(err, "unexpected AWS response getting default-vpc account attribute")
	}

	if len(response.AccountAttributes) == 0 ||
		len(response.AccountAttributes[0].AttributeValues) == 0 ||
		aws.ToString(response.AccountAttributes[0].AttributeName) != "default-vpc" {
		// No value for the requested "default-vpc" attribute, all bets are off.
		return "", errors.NotFoundf("default-vpc account attribute")
	}

	firstAttributeValue := aws.ToString(response.AccountAttributes[0].AttributeValues[0].AttributeValue)
	if firstAttributeValue == vpcIDNone {
		return "", errors.NotFoundf("default VPC")
	}

	return firstAttributeValue, nil
}

// getVPCSubnetIDsForAvailabilityZone returns a sorted list of subnet IDs, which
// are both in the given vpcID and the given zoneName. If allowedSubnetIDs is
// not empty, the returned list will only contain IDs present there. Returns an
// error satisfying errors.IsNotFound() when no results match.
func getVPCSubnetsForAvailabilityZone(
	ctx context.Context,
	apiClient vpcAPIClient,
	vpcID, zoneName string,
	allowedSubnetIDs []corenetwork.Id,
) ([]types.Subnet, error) {
	allowedSubnets := corenetwork.MakeIDSet(allowedSubnetIDs...)
	subnets, err := getVPCSubnets(ctx, apiClient, vpcID)
	if err != nil && !errors.Is(err, errorVPCNotUsable) {
		return nil, errors.Annotatef(err, "cannot get VPC %q subnets", vpcID)
	} else if errors.Is(err, errorVPCNotUsable) {
		// We're reusing getVPCSubnets(), but not while validating a VPC
		// pre-bootstrap, so we should change vpcNotUsableError to a simple
		// NotFoundError.
		message := fmt.Sprintf("VPC %q has no subnets in AZ %q", vpcID, zoneName)
		return nil, errors.NewNotFound(err, message)
	}

	matchingSubnets := make([]types.Subnet, 0, len(subnets))
	for _, subnet := range subnets {
		subnetID := aws.ToString(subnet.SubnetId)
		if aws.ToString(subnet.AvailabilityZone) != zoneName {
			logger.Debugf(ctx, "skipping subnet %q (in VPC %q): not in the chosen AZ %q", subnetID, vpcID, zoneName)
			continue
		}
		if !allowedSubnets.IsEmpty() && !allowedSubnets.Contains(corenetwork.Id(subnetID)) {
			logger.Debugf(ctx, "skipping subnet %q (in VPC %q, AZ %q): not matching spaces constraints", subnetID, vpcID, zoneName)
			continue
		}
		matchingSubnets = append(matchingSubnets, subnet)
	}

	if len(matchingSubnets) == 0 {
		message := fmt.Sprintf("VPC %q has no subnets in AZ %q", vpcID, zoneName)
		return nil, errors.NewNotFound(nil, message)
	}

	logger.Infof(ctx, "found %d subnets in VPC %q matching AZ %q and constraints: %v", len(matchingSubnets), vpcID, zoneName, matchingSubnets)
	return matchingSubnets, nil
}

func isVPCIDSetButInvalid(vpcID string) bool {
	return isVPCIDSet(vpcID) && !strings.HasPrefix(vpcID, "vpc-")
}

func isVPCIDSet(vpcID string) bool {
	return vpcID != "" && vpcID != vpcIDNone
}

func validateBootstrapVPC(stdCtx context.Context, apiClient vpcAPIClient, region, vpcID string, forceVPCID bool, ctx environs.BootstrapContext) error {
	if vpcID == vpcIDNone {
		ctx.Infof("Using EC2-classic features or default VPC in region %q", region)
	}
	if !isVPCIDSet(vpcID) {
		return nil
	}

	err := validateVPC(stdCtx, apiClient, vpcID)
	switch {
	case errors.Is(err, errorVPCNotUsable):
		// VPC missing or has no subnets at all.
		return errors.Annotate(err, vpcNotUsableForBootstrapErrorPrefix)
	case errors.Is(err, errorVPCNotRecommended):
		// VPC does not meet minimum validation criteria.
		if !forceVPCID {
			return errors.Annotatef(err, vpcNotRecommendedErrorPrefix, vpcID)
		}
		ctx.Infof(vpcNotRecommendedButForcedWarning)
	case err != nil:
		// Anything else unexpected while validating the VPC.
		return errors.Annotate(err, cannotValidateVPCErrorPrefix)
	}

	ctx.Infof("Using VPC %q in region %q", vpcID, region)

	return nil
}

func validateModelVPC(ctx context.Context, apiClient vpcAPIClient, modelName, vpcID string) error {
	if !isVPCIDSet(vpcID) {
		return nil
	}

	err := validateVPC(ctx, apiClient, vpcID)
	switch {
	case errors.Is(err, errorVPCNotUsable):
		// VPC missing or has no subnets at all.
		return errors.Annotate(err, vpcNotUsableForModelErrorPrefix)
	case errors.Is(err, errorVPCNotRecommended):
		// VPC does not meet minimum validation criteria, but that's less
		// important for hosted models, as the controller is already accessible.
		logger.Infof(ctx,
			"Juju will use, but does not recommend using VPC %q: %v",
			vpcID, err.Error(),
		)
	case err != nil:
		// Anything else unexpected while validating the VPC.
		return errors.Annotate(err, cannotValidateVPCErrorPrefix)
	}
	logger.Infof(ctx, "Using VPC %q for model %q", vpcID, modelName)

	return nil
}
