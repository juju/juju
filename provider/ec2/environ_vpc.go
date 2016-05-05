// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/network"
)

const (
	activeState           = "active"
	availableState        = "available"
	localRouteGatewayID   = "local"
	defaultRouteCIDRBlock = "0.0.0.0/0"
	defaultVPCIDNone      = "none"
)

// vpcAPIClient defines a subset of the goamz API calls needed to validate a VPC.
type vpcAPIClient interface {
	AccountAttributes(attributeNames ...string) (*ec2.AccountAttributesResp, error)
	VPCs(ids []string, filter *ec2.Filter) (*ec2.VPCsResp, error)
	Subnets(ids []string, filter *ec2.Filter) (*ec2.SubnetsResp, error)
	InternetGateways(ids []string, filter *ec2.Filter) (*ec2.InternetGatewaysResp, error)
	RouteTables(ids []string, filter *ec2.Filter) (*ec2.RouteTablesResp, error)
}

// validateVPC requires both arguments to be set and validates that vpcID refers
// to an existing AWS VPC (default or non-default) for the current region.
// Returns an error satifying errors.IsNotFound() when the VPC with the given
// vpcID cannot be found, or when the VPC exists but contains no subnets.
// Returns an error satisfying errors.IsNotValid() in the following cases:
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
// With the force-vpc-id config setting set to true, the provider can ignore a
// NotValidError. NotFoundError cannot be ignored, while unexpected API
// responses and errors could be retried.
func validateVPC(apiClient vpcAPIClient, vpcID string) error {
	if vpcID == "" || apiClient == nil {
		return errors.Errorf("invalid arguments: empty VPC ID or nil client")
	}

	vpc, err := getVPCByID(apiClient, vpcID)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkVPCIsAvailable(vpc); err != nil {
		return errors.Trace(err)
	}

	subnets, err := getVPCSubnets(apiClient, vpc)
	if err != nil {
		return errors.Trace(err)
	}

	publicSubnet, err := findFirstPublicSubnet(subnets)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof(
		"found subnet %q (%s) in AZ %q, suitable for a Juju controller instance",
		publicSubnet.Id, publicSubnet.CIDRBlock, publicSubnet.AvailZone,
	)

	gateway, err := getVPCInternetGateway(apiClient, vpc)
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkInternetGatewayIsAvailable(gateway); err != nil {
		return errors.Trace(err)
	}

	routeTables, err := getVPCRouteTables(apiClient, vpc)
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

func getVPCByID(apiClient vpcAPIClient, vpcID string) (*ec2.VPC, error) {
	response, err := apiClient.VPCs([]string{vpcID}, nil)
	if isVPCNotFoundError(err) {
		return nil, errors.NewNotFound(err, "")
	} else if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting VPC %q", vpcID)
	}

	if numResults := len(response.VPCs); numResults == 0 {
		return nil, errors.NotFoundf("VPC %q", vpcID)
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
		return errors.NotValidf("VPC with unexpected state %q", vpc.State)
	}

	if vpc.IsDefault {
		logger.Infof("VPC %q is the default VPC for the region", vpc.Id)
	}

	return nil
}

func getVPCSubnets(apiClient vpcAPIClient, vpc *ec2.VPC) ([]ec2.Subnet, error) {
	filter := ec2.NewFilter()
	filter.Add("vpc-id", vpc.Id)
	response, err := apiClient.Subnets(nil, filter)
	if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting subnets of VPC %q", vpc.Id)
	}

	if len(response.Subnets) == 0 {
		message := fmt.Sprintf("no subnets found for VPC %q", vpc.Id)
		return nil, errors.NewNotFound(nil, message)
	}

	return response.Subnets, nil
}

func findFirstPublicSubnet(subnets []ec2.Subnet) (*ec2.Subnet, error) {
	for _, subnet := range subnets {
		if subnet.MapPublicIPOnLaunch {
			return &subnet, nil
		}

	}
	return nil, errors.NotValidf("VPC without any public subnets")
}

func getVPCInternetGateway(apiClient vpcAPIClient, vpc *ec2.VPC) (*ec2.InternetGateway, error) {
	filter := ec2.NewFilter()
	filter.Add("attachment.vpc-id", vpc.Id)
	response, err := apiClient.InternetGateways(nil, filter)
	if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting Internet Gateway of VPC %q", vpc.Id)
	}

	if numResults := len(response.InternetGateways); numResults == 0 {
		return nil, errors.NotValidf("VPC without Internet Gateway")
	} else if numResults > 1 {
		logger.Debugf("InternetGateways() returned %#v", response)
		return nil, errors.Errorf("expected 1 result from AWS, got %d", numResults)
	}

	gateway := response.InternetGateways[0]
	return &gateway, nil
}

func checkInternetGatewayIsAvailable(gateway *ec2.InternetGateway) error {
	if state := gateway.AttachmentState; state != availableState {
		return errors.NotValidf("VPC with Internet Gateway %q in unexpected state %q", gateway.Id, state)
	}

	return nil
}

func getVPCRouteTables(apiClient vpcAPIClient, vpc *ec2.VPC) ([]ec2.RouteTable, error) {
	filter := ec2.NewFilter()
	filter.Add("vpc-id", vpc.Id)
	response, err := apiClient.RouteTables(nil, filter)
	if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting route tables of VPC %q", vpc.Id)
	}

	if len(response.Tables) == 0 {
		return nil, errors.NotValidf("VPC without any route tables")
	}
	logger.Tracef("RouteTables() returned %#v", response)

	return response.Tables, nil
}

func findVPCMainRouteTable(routeTables []ec2.RouteTable) (*ec2.RouteTable, error) {
	var mainTable *ec2.RouteTable
	for i, table := range routeTables {
		if len(table.Associations) < 1 {
			logger.Tracef("ignoring VPC %q route table %q with no associations", table.VPCId, table.Id)
			continue
		}

		for _, association := range table.Associations {
			if subnetID := association.SubnetId; subnetID != "" {
				message := fmt.Sprintf("subnet %q not associated with VPC %q main route table", subnetID, table.VPCId)
				return nil, errors.NewNotValid(nil, message)
			}

			if association.IsMain && mainTable == nil {
				logger.Tracef("main route table of VPC %q has ID %q", table.VPCId, table.Id)
				mainTable = &routeTables[i]
			}
		}
	}

	if mainTable == nil {
		return nil, errors.NotValidf("VPC without associated main route table")
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
		return errors.NotValidf("missing default route via gateway %q", gateway.Id)
	}
	return errors.NotValidf("missing local route with destination %q", vpc.CIDRBlock)
}

func findDefaultVPCID(apiClient vpcAPIClient) (string, error) {
	response, err := apiClient.AccountAttributes("default-vpc")
	if err != nil {
		return "", errors.Annotate(err, "unexpected AWS response getting default-vpc account attribute")
	}

	if len(response.Attributes) == 0 ||
		len(response.Attributes[0].Values) == 0 ||
		response.Attributes[0].Name != "default-vpc" {
		// No value for the requested "default-vpc" attribute, all bets are off.
		return "", errors.NotFoundf("default-vpc account attribute")
	}

	firstAttributeValue := response.Attributes[0].Values[0]
	if firstAttributeValue == defaultVPCIDNone {
		return "", errors.NotFoundf("default VPC")
	}

	return firstAttributeValue, nil
}

func getVPCSubnetIDsForAvailabilityZone(apiClient vpcAPIClient, vpcID, zoneName string) ([]string, error) {
	vpc := &ec2.VPC{Id: vpcID}
	subnets, err := getVPCSubnets(apiClient, vpc)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "cannot get VPC %q subnets", vpcID)
	} else if errors.IsNotFound(err) {
		message := fmt.Sprintf("VPC %q has no subnets in AZ %q", vpcID, zoneName)
		return nil, errors.NewNotFound(err, message)
	}

	matchingSubnetIDs := set.NewStrings()
	for _, subnet := range subnets {
		if subnet.AvailZone == zoneName {
			matchingSubnetIDs.Add(subnet.Id)
		}
	}

	if matchingSubnetIDs.IsEmpty() {
		message := fmt.Sprintf("VPC %q has no subnets in AZ %q", vpcID, zoneName)
		return nil, errors.NewNotFound(nil, message)
	}

	return matchingSubnetIDs.SortedValues(), nil
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
