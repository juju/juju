// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

const (
	activeState           = "active"
	availableState        = "available"
	localRouteGatewayID   = "local"
	defaultRouteCIDRBlock = "0.0.0.0/0"
	vpcIDNone             = "none"
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

// vpcNotUsableError indicates a user-specified VPC cannot be used either
// because it is missing or because it contains no subnets.
type vpcNotUsableError struct {
	errors.Err
}

// vpcNotUsablef returns an error which satisfies isVPCNotUsableError().
func vpcNotUsablef(optionalCause error, format string, args ...interface{}) error {
	outerErr := errors.Errorf(format, args...)
	if optionalCause != nil {
		outerErr = errors.Maskf(optionalCause, format, args...)
	}

	innerErr, _ := outerErr.(*errors.Err) // cannot fail.
	return &vpcNotUsableError{*innerErr}
}

// isVPCNotUsableError reports whether err was created with vpcNotUsablef().
func isVPCNotUsableError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*vpcNotUsableError)
	return ok
}

// vpcNotRecommendedError indicates a user-specified VPC is unlikely to be
// suitable for hosting a Juju controller instance and/or exposed workloads, due
// to not satisfying the mininum requirements described in validateVPC()'s doc
// comment. Users can still force Juju to use such a VPC by passing
// 'vpc-id-force=true' setting.
type vpcNotRecommendedError struct {
	errors.Err
}

// vpcNotRecommendedf returns an error which satisfies isVPCNotRecommendedError().
func vpcNotRecommendedf(format string, args ...interface{}) error {
	outerErr := errors.Errorf(format, args...)
	innerErr, _ := outerErr.(*errors.Err) // cannot fail.
	return &vpcNotRecommendedError{*innerErr}
}

// isVPCNotRecommendedError reports whether err was created with vpcNotRecommendedf().
func isVPCNotRecommendedError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*vpcNotRecommendedError)
	return ok
}

// vpcAPIClient defines a subset of the goamz API calls needed to validate a VPC.
type vpcAPIClient interface {
	// AccountAttributes, called with the "default-vpc" attribute. is used to
	// find the ID of the region's default VPC (if any).
	AccountAttributes(attributeNames ...string) (*ec2.AccountAttributesResp, error)

	// VPCs is used to get details for the VPC being validated, including
	// whether it exists, is available, and its CIDRBlock and IsDefault fields.
	VPCs(ids []string, filter *ec2.Filter) (*ec2.VPCsResp, error)

	// Subnets is used to get a list of all subnets of the validated VPC (if
	// any),including their Id, AvailZone, and MapPublicIPOnLaunch fields.
	Subnets(ids []string, filter *ec2.Filter) (*ec2.SubnetsResp, error)

	// InternetGateways is used to get details of the Internet Gateway (IGW)
	// attached to the validated VPC (if any), its Id to check against routes,
	// and whether it's available.
	InternetGateways(ids []string, filter *ec2.Filter) (*ec2.InternetGatewaysResp, error)

	// RouteTables is used to find the main route table of the VPC (if any),
	// whether it includes a default route to the attached IGW, a local route to
	// the VPC CIDRBlock, and any per-subnet route tables.
	RouteTables(ids []string, filter *ec2.Filter) (*ec2.RouteTablesResp, error)
}

// validateVPC requires both arguments to be set and validates that vpcID refers
// to an existing AWS VPC (default or non-default) for the current region.
// Returns an error satifying isVPCNotUsableError() when the VPC with the given
// vpcID cannot be found, or when the VPC exists but contains no subnets.
// Returns an error satisfying isVPCNotRecommendedError() in the following
// cases:
//
// 1. The VPC's state is not "available".
// 2. The VPC does not have an Internet Gateway (IGW) attached.
// 3. A main route table is not associated with the VPC.
// 4. The main route table lacks both a default route via the IGW and a local
//    route matching the VPC's CIDR block.
// 5. One or more of the VPC's subnets are not associated with the main route
//    table of the VPC.
// 6. None of the the VPC's subnets have the MapPublicIPOnLaunch attribute set.
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
func validateVPC(apiClient vpcAPIClient, ctx context.ProviderCallContext, vpcID string) error {
	if vpcID == "" || apiClient == nil {
		return errors.Errorf("invalid arguments: empty VPC ID or nil client")
	}

	vpc, err := getVPCByID(apiClient, ctx, vpcID)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkVPCIsAvailable(vpc); err != nil {
		return errors.Trace(err)
	}

	subnets, err := getVPCSubnets(apiClient, ctx, vpc)
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
	logger.Infof(
		"found subnet %q (%s) in AZ %q, suitable for a Juju controller instance",
		publicSubnet.Id, publicSubnet.CIDRBlock, publicSubnet.AvailZone,
	)

	gateway, err := getVPCInternetGateway(apiClient, ctx, vpc)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkInternetGatewayIsAvailable(gateway); err != nil {
		return errors.Trace(err)
	}

	routeTables, err := getVPCRouteTables(apiClient, ctx, vpc)
	if err != nil {
		return errors.Trace(err)
	}

	mainRouteTable, err := findVPCMainRouteTable(routeTables)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkVPCRouteTableRoutes(vpc, mainRouteTable, gateway); err != nil {
		return errors.Annotatef(err, "VPC %q main route table %q", vpcID, mainRouteTable.Id)
	}

	logger.Infof("VPC %q is suitable for Juju controllers and expose-able workloads", vpc.Id)
	return nil
}

func getVPCByID(apiClient vpcAPIClient, ctx context.ProviderCallContext, vpcID string) (*ec2.VPC, error) {
	response, err := apiClient.VPCs([]string{vpcID}, nil)
	if isVPCNotFoundError(err) {
		return nil, vpcNotUsablef(err, "")
	} else if err != nil {
		return nil, errors.Annotatef(maybeConvertCredentialError(err, ctx), "unexpected AWS response getting VPC %q", vpcID)
	}

	if numResults := len(response.VPCs); numResults == 0 {
		return nil, vpcNotUsablef(nil, "VPC %q not found", vpcID)
	} else if numResults > 1 {
		logger.Debugf("VPCs() returned %#v", response)
		return nil, errors.Errorf("expected 1 result from AWS, got %d", numResults)
	}

	vpc := response.VPCs[0]
	return &vpc, nil
}

func isVPCNotFoundError(err error) bool {
	return err != nil && ec2ErrCode(err) == "InvalidVpcID.NotFound"
}

func checkVPCIsAvailable(vpc *ec2.VPC) error {
	if vpc.State != availableState {
		return vpcNotRecommendedf("VPC has unexpected state %q", vpc.State)
	}

	if vpc.IsDefault {
		logger.Infof("VPC %q is the default VPC for the region", vpc.Id)
	}

	return nil
}

func getVPCSubnets(apiClient vpcAPIClient, ctx context.ProviderCallContext, vpc *ec2.VPC) ([]ec2.Subnet, error) {
	filter := ec2.NewFilter()
	filter.Add("vpc-id", vpc.Id)
	response, err := apiClient.Subnets(nil, filter)
	if err != nil {
		return nil, errors.Annotatef(maybeConvertCredentialError(err, ctx), "unexpected AWS response getting subnets of VPC %q", vpc.Id)
	}

	if len(response.Subnets) == 0 {
		return nil, vpcNotUsablef(nil, "no subnets found for VPC %q", vpc.Id)
	}

	return response.Subnets, nil
}

func findFirstPublicSubnet(subnets []ec2.Subnet) (*ec2.Subnet, error) {
	for _, subnet := range subnets {
		// TODO(dimitern): goamz's AddDefaultVPCAndSubnets() does not set
		// MapPublicIPOnLaunch only DefaultForAZ, but in reality the former is
		// always set when the latter is. Until this is fixed in goamz, we check
		// for both below to allow testing the behavior.
		if subnet.MapPublicIPOnLaunch || subnet.DefaultForAZ {
			logger.Debugf(
				"VPC %q subnet %q has MapPublicIPOnLaunch=%v, DefaultForAZ=%v",
				subnet.VPCId, subnet.Id, subnet.MapPublicIPOnLaunch, subnet.DefaultForAZ,
			)
			return &subnet, nil
		}

	}
	return nil, vpcNotRecommendedf("VPC contains no public subnets")
}

func getVPCInternetGateway(apiClient vpcAPIClient, ctx context.ProviderCallContext, vpc *ec2.VPC) (*ec2.InternetGateway, error) {
	filter := ec2.NewFilter()
	filter.Add("attachment.vpc-id", vpc.Id)
	response, err := apiClient.InternetGateways(nil, filter)
	if err != nil {
		return nil, errors.Annotatef(maybeConvertCredentialError(err, ctx), "unexpected AWS response getting Internet Gateway of VPC %q", vpc.Id)
	}

	if numResults := len(response.InternetGateways); numResults == 0 {
		return nil, vpcNotRecommendedf("VPC has no Internet Gateway attached")
	} else if numResults > 1 {
		logger.Debugf("InternetGateways() returned %#v", response)
		return nil, errors.Errorf("expected 1 result from AWS, got %d", numResults)
	}

	gateway := response.InternetGateways[0]
	return &gateway, nil
}

func checkInternetGatewayIsAvailable(gateway *ec2.InternetGateway) error {
	if state := gateway.AttachmentState; state != availableState {
		return vpcNotRecommendedf("VPC has Internet Gateway %q in unexpected state %q", gateway.Id, state)
	}

	return nil
}

func getVPCRouteTables(apiClient vpcAPIClient, ctx context.ProviderCallContext, vpc *ec2.VPC) ([]ec2.RouteTable, error) {
	filter := ec2.NewFilter()
	filter.Add("vpc-id", vpc.Id)
	response, err := apiClient.RouteTables(nil, filter)
	if err != nil {
		return nil, errors.Annotatef(maybeConvertCredentialError(err, ctx), "unexpected AWS response getting route tables of VPC %q", vpc.Id)
	}

	if len(response.Tables) == 0 {
		return nil, vpcNotRecommendedf("VPC has no route tables")
	}
	logger.Tracef("RouteTables() returned %#v", response)

	return response.Tables, nil
}

func getVPCCIDR(apiClient vpcAPIClient, ctx context.ProviderCallContext, vpcID string) (string, error) {
	vpc, err := getVPCByID(apiClient, ctx, vpcID)
	if err != nil {
		return "", err
	}
	return vpc.CIDRBlock, nil
}

func findVPCMainRouteTable(routeTables []ec2.RouteTable) (*ec2.RouteTable, error) {
	var mainTable *ec2.RouteTable
	for i, table := range routeTables {
		if len(table.Associations) < 1 {
			logger.Tracef("ignoring VPC %q route table %q with no associations", table.VPCId, table.Id)
			continue
		}

		for _, association := range table.Associations {
			// TODO(dimitern): Of all the requirements, this seems like the most
			// strict and likely to push users to use vpc-id-force=true. On the
			// other hand, having to deal with more than the main route table's
			// routes will likely overcomplicate the routes checks that follow.
			if subnetID := association.SubnetId; subnetID != "" {
				return nil, vpcNotRecommendedf("subnet %q not associated with VPC %q main route table", subnetID, table.VPCId)
			}

			if association.IsMain && mainTable == nil {
				logger.Tracef("main route table of VPC %q has ID %q", table.VPCId, table.Id)
				mainTable = &routeTables[i]
			}
		}
	}

	if mainTable == nil {
		return nil, vpcNotRecommendedf("VPC has no associated main route table")
	}

	return mainTable, nil
}

func checkVPCRouteTableRoutes(vpc *ec2.VPC, routeTable *ec2.RouteTable, gateway *ec2.InternetGateway) error {
	hasDefaultRoute := false
	hasLocalRoute := false

	logger.Tracef("checking route table %+v routes", routeTable)
	for _, route := range routeTable.Routes {
		if route.State != activeState {
			logger.Tracef("skipping inactive route %+v", route)
			continue
		}

		switch route.DestinationCIDRBlock {
		case defaultRouteCIDRBlock:
			if route.GatewayId == gateway.Id {
				logger.Tracef("default route uses expected gateway %q", gateway.Id)
				hasDefaultRoute = true
			}
		case vpc.CIDRBlock:
			if route.GatewayId == localRouteGatewayID {
				logger.Tracef("local route uses expected CIDR %q", vpc.CIDRBlock)
				hasLocalRoute = true
			}
		default:
			logger.Tracef("route %+v is neither local nor default (skipping)", route)
		}
	}

	if hasDefaultRoute && hasLocalRoute {
		return nil
	}

	if !hasDefaultRoute {
		return vpcNotRecommendedf("missing default route via gateway %q", gateway.Id)
	}
	return vpcNotRecommendedf("missing local route with destination %q", vpc.CIDRBlock)
}

func findDefaultVPCID(apiClient vpcAPIClient, ctx context.ProviderCallContext) (string, error) {
	response, err := apiClient.AccountAttributes("default-vpc")
	if err != nil {
		return "", errors.Annotate(maybeConvertCredentialError(err, ctx), "unexpected AWS response getting default-vpc account attribute")
	}

	if len(response.Attributes) == 0 ||
		len(response.Attributes[0].Values) == 0 ||
		response.Attributes[0].Name != "default-vpc" {
		// No value for the requested "default-vpc" attribute, all bets are off.
		return "", errors.NotFoundf("default-vpc account attribute")
	}

	firstAttributeValue := response.Attributes[0].Values[0]
	if firstAttributeValue == vpcIDNone {
		return "", errors.NotFoundf("default VPC")
	}

	return firstAttributeValue, nil
}

// getVPCSubnetIDsForAvailabilityZone returns a sorted list of subnet IDs, which
// are both in the given vpcID and the given zoneName. If allowedSubnetIDs is
// not empty, the returned list will only contain IDs present there. Returns an
// error satisfying errors.IsNotFound() when no results match.
func getVPCSubnetIDsForAvailabilityZone(
	apiClient vpcAPIClient,
	ctx context.ProviderCallContext,
	vpcID, zoneName string,
	allowedSubnetIDs []string,
) ([]string, error) {
	allowedSubnets := set.NewStrings(allowedSubnetIDs...)
	vpc := &ec2.VPC{Id: vpcID}
	subnets, err := getVPCSubnets(apiClient, ctx, vpc)
	if err != nil && !isVPCNotUsableError(err) {
		return nil, errors.Annotatef(maybeConvertCredentialError(err, ctx), "cannot get VPC %q subnets", vpcID)
	} else if isVPCNotUsableError(err) {
		// We're reusing getVPCSubnets(), but not while validating a VPC
		// pre-bootstrap, so we should change vpcNotUsableError to a simple
		// NotFoundError.
		message := fmt.Sprintf("VPC %q has no subnets in AZ %q", vpcID, zoneName)
		return nil, errors.NewNotFound(err, message)
	}

	matchingSubnetIDs := set.NewStrings()
	for _, subnet := range subnets {
		if subnet.AvailZone != zoneName {
			logger.Debugf("skipping subnet %q (in VPC %q): not in the chosen AZ %q", subnet.Id, vpcID, zoneName)
			continue
		}
		if !allowedSubnets.IsEmpty() && !allowedSubnets.Contains(subnet.Id) {
			logger.Debugf("skipping subnet %q (in VPC %q, AZ %q): not matching spaces constraints", subnet.Id, vpcID, zoneName)
			continue
		}
		matchingSubnetIDs.Add(subnet.Id)
	}

	if matchingSubnetIDs.IsEmpty() {
		message := fmt.Sprintf("VPC %q has no subnets in AZ %q", vpcID, zoneName)
		return nil, errors.NewNotFound(nil, message)
	}

	sortedIDs := matchingSubnetIDs.SortedValues()
	logger.Infof("found %d subnets in VPC %q matching AZ %q and constraints: %v", len(sortedIDs), vpcID, zoneName, sortedIDs)
	return sortedIDs, nil
}

func findSubnetIDsForAvailabilityZone(zoneName string, subnetsToZones map[network.Id][]string) ([]string, error) {
	matchingSubnetIDs := set.NewStrings()
	for subnetID, zones := range subnetsToZones {
		zonesSet := set.NewStrings(zones...)
		if zonesSet.Contains(zoneName) {
			matchingSubnetIDs.Add(string(subnetID))
		}
	}

	if matchingSubnetIDs.IsEmpty() {
		return nil, errors.NotFoundf("subnets in AZ %q", zoneName)
	}

	return matchingSubnetIDs.SortedValues(), nil
}

func isVPCIDSetButInvalid(vpcID string) bool {
	return isVPCIDSet(vpcID) && !strings.HasPrefix(vpcID, "vpc-")
}

func isVPCIDSet(vpcID string) bool {
	return vpcID != "" && vpcID != vpcIDNone
}

func validateBootstrapVPC(apiClient vpcAPIClient, cloudCtx context.ProviderCallContext, region, vpcID string, forceVPCID bool, ctx environs.BootstrapContext) error {
	if vpcID == vpcIDNone {
		ctx.Infof("Using EC2-classic features or default VPC in region %q", region)
	}
	if !isVPCIDSet(vpcID) {
		return nil
	}

	err := validateVPC(apiClient, cloudCtx, vpcID)
	switch {
	case isVPCNotUsableError(err):
		// VPC missing or has no subnets at all.
		return errors.Annotate(err, vpcNotUsableForBootstrapErrorPrefix)
	case isVPCNotRecommendedError(err):
		// VPC does not meet minimum validation criteria.
		if !forceVPCID {
			return errors.Annotatef(err, vpcNotRecommendedErrorPrefix, vpcID)
		}
		ctx.Infof(vpcNotRecommendedButForcedWarning)
	case err != nil:
		// Anything else unexpected while validating the VPC.
		return errors.Annotate(maybeConvertCredentialError(err, cloudCtx), cannotValidateVPCErrorPrefix)
	}

	ctx.Infof("Using VPC %q in region %q", vpcID, region)

	return nil
}

func validateModelVPC(apiClient vpcAPIClient, ctx context.ProviderCallContext, modelName, vpcID string) error {
	if !isVPCIDSet(vpcID) {
		return nil
	}

	err := validateVPC(apiClient, ctx, vpcID)
	switch {
	case isVPCNotUsableError(err):
		// VPC missing or has no subnets at all.
		return errors.Annotate(err, vpcNotUsableForModelErrorPrefix)
	case isVPCNotRecommendedError(err):
		// VPC does not meet minimum validation criteria, but that's less
		// important for hosted models, as the controller is already accessible.
		logger.Infof(
			"Juju will use, but does not recommend using VPC %q: %v",
			vpcID, err.Error(),
		)
	case err != nil:
		// Anything else unexpected while validating the VPC.
		return errors.Annotate(maybeConvertCredentialError(err, ctx), cannotValidateVPCErrorPrefix)
	}
	logger.Infof("Using VPC %q for model %q", vpcID, modelName)

	return nil
}
