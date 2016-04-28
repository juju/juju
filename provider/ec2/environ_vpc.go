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

type vpcInfo struct {
	id                              network.Id
	isDefault                       bool
	suitableSubnetIDsForControllers []network.Id
}

// getVPC returns the VPC ID to use and whether it is the default VPC for the
// region. It uses cachedVPC if set, otherwise populates it, after validation
// when user-specified vpc-id or force-vpc-id is set.
func (e *environ) getVPC() (network.Id, bool, error) {
	// Use the cached info, if available.
	if e.cachedVPC != nil {
		return e.cachedVPC.id, e.cachedVPC.isDefault, nil
	}
	ec2Client := e.ec2()

	// No cache; verify if vpc-id is specified first and validate it.
	vpcID := e.ecfg().vpcID()
	forceVPCID := e.ecfg().forceVPCID()
	region := e.ecfg().region()

	if vpcID == "" {
		// Expect a default VPC to be available, but verify anyway.
		return e.getDefaultVPC(ec2Client)
	}

	// User specified vpc-id (with or without force-vpc-id).
	var err error
	e.cachedVPC, err = validateVPC(ec2Client, region, vpcID)
	if err == nil {
		// VPC should work, cache it.
		return e.cachedVPC.id, e.cachedVPC.isDefault, nil
	}

	if errors.IsNotValid(err) {
		if forceVPCID {
			// VPC is unlikely to work, but the the user insist, so cache it and
			// warn about the issue.
			logger.Warningf("specified VPC %q does not meet minimum connectivity requirements (using anyway: force-vpc-id=true)", vpcID)
			logger.Warningf("ignoring validation error: %v", err)
			e.cachedVPC = &vpcInfo{
				id:        network.Id(vpcID),
				isDefault: false, // cannot be a default VPC if it fails validation.
			}
			return e.cachedVPC.id, e.cachedVPC.isDefault, nil
		}
		return "", false, errors.Annotatef(
			err,
			"specified VPC %q does not meet minimum connectivity requirements",
			vpcID,
		)
	} else if errors.IsNotFound(err) {
		return "", false, errors.Annotate(err, "cannot find specified vpc-id")
	}

	return "", false, errors.Annotate(err, "unexpected error verifying the specified vpc-id")
}

// getDefaultVPC discovers whether the region has a default VPC and returns its ID and
func (e *environ) getDefaultVPC(ec2Client *ec2.EC2) (network.Id, bool, error) {
	resp, err := ec2Client.AccountAttributes("default-vpc")
	if err != nil {
		return "", false, errors.Trace(err)
	}

	hasDefault := true // safe assumption for most current EC2 accounts.
	defaultVPCID := ""

	if len(resp.Attributes) == 0 || len(resp.Attributes[0].Values) == 0 {
		// No value for the requested "default-vpc" attribute, all bets are off.
		hasDefault = false
		defaultVPCID = ""
	} else {
		defaultVPCID = resp.Attributes[0].Values[0]
		if defaultVPCID == none {
			// Explicitly deleted default VPC.
			hasDefault = false
			defaultVPCID = ""
		}
	}

	e.cachedVPC = &vpcInfo{
		id:        network.Id(defaultVPCID),
		isDefault: hasDefault,
	}
	return e.cachedVPC.id, e.cachedVPC.isDefault, nil
}

// validateVPC requires all arguments to be set and validates that
// vpcID refers to an existing EC2 VPC (default or non-default) for
// the chosen region. If vpcID refers to a non-default VPC, a few
// santiy checks are done in addition to validating the ID exists:
//
// 1. The VPC has an Internet Gateway (IGW) attached.
// 2. There is at least one "public" subnet (with MapPublicIPOnLaunch set)
//    in the VPC in one of the available (with "state"="available" in EC2
//    terms) availability zone in the region. The first such subnet's ID
//    will be used for the controller instance.
// 3. Either the VPC main route table or the subnet selected above
//    must have a route to the IGW, so it can access the internet,
//    in addition to being accessible from the internet
//
// If vpcID does not exist, an error satisfying errors.IsNotFound() will be
// returned. If the VPC exists but is unusable (not meeting the minimum
// requirements above), an error satisfying errors.IsNotValid() will be
// returned.
func validateVPC(ec2Client *ec2.EC2, region, vpcID string) (*vpcInfo, error) {
	if vpcID == "" || region == "" || ec2Client == nil {
		return nil, errors.Errorf("invalid arguments: empty VPC ID, region, or nil client")
	}

	// Get the VPC by ID and check its state is "available".
	vpcsResp, err := ec2Client.VPCs([]string{vpcID}, nil)
	if err != nil && ec2ErrCode(err) == "InvalidVpcID.NotFound" {
		return nil, errors.NewNotFound(err, "")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	switch numResults := len(vpcsResp.VPCs); numResults {
	case 0:
		return nil, errors.NotFoundf("VPC")
	case 1:
		if vpcState := vpcsResp.VPCs[0].State; vpcState != "available" {
			return nil, errors.NotValidf("VPC state %q", vpcState)
		}
	default:
		logger.Debugf("DescribeVpcs returned %#v", vpcsResp)
		return nil, errors.Errorf("expected one result from DescribeVpcs, got %d", numResults)
	}

	vpc := &vpcInfo{
		id:        network.Id(vpcsResp.VPCs[0].Id),
		isDefault: vpcsResp.VPCs[0].IsDefault,
	}
	if vpc.isDefault {
		// Default VPCs already meets juju requirements, no need to check anything else.
		logger.Infof("specified VPC %q is the default VPC for region %q", vpcID, region)
		return vpc, nil
	}
	vpcID = string(vpc.id)
	logger.Tracef("non-default VPC %q exists and is potentially usable", vpcID)

	// TODO: Verify the VPC has IGW attached, so instances inside can reach the internet.

	// Get all availability zones for the region with state "available".
	zoneFilter := ec2.NewFilter()
	zoneFilter.Add("region-name", region)
	zoneFilter.Add("state", "available")
	zonesResp, err := ec2Client.AvailabilityZones(zoneFilter)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get availability zones for region %q", region)
	}
	if len(zonesResp.Zones) < 1 {
		return nil, errors.Errorf(`region %q has no availability zones with state "available"`, region)
	}

	usableZones := set.NewStrings()
	for _, zone := range zonesResp.Zones {
		// The checks below ensure we get expected results from AWS with the
		// applied filters.
		switch {
		case zone.Region != region:
			return nil, errors.Errorf(
				"expected region %q for availability zone %q, got %q",
				region, zone.Name, zone.Region,
			)
		case zone.State != "available":
			return nil, errors.Errorf(
				`expected state "available" for availability zone %q, got %q`,
				zone.Name, zone.State,
			)
		default:
			usableZones.Add(zone.Name)
		}
	}
	logger.Tracef("usable AZs in region %q: %v", region, usableZones.SortedValues())

	// Fetch the available VPC subnets in the discovered usable zones and try to
	// find at least one public subnet.
	subnetFilter := ec2.NewFilter()
	subnetFilter.Add("vpc-id", vpcID)
	subnetFilter.Add("state", "available")
	for _, zone := range usableZones.SortedValues() {
		subnetFilter.Add("availability-zone", zone)
	}
	subnetsResp, err := ec2Client.Subnets(nil, subnetFilter)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get subnets in VPC %q", vpcID)
	}

	potentiallyUsableSubnets := set.NewStrings()
	for _, subnet := range subnetsResp.Subnets {
		vpcIDandSubnetID := fmt.Sprintf("VPC %q subnet %q", vpcID, subnet.Id)
		// The first 3 checks below ensure we get expected results from AWS with
		// the applied filters.
		switch {
		case subnet.VPCId != vpcID:
			return nil, errors.Errorf("expected VPC %q for subnet %q, got %q", vpcID, subnet.Id, subnet.VPCId)
		case subnet.State != "available":
			return nil, errors.Errorf(`expected state "available" for %s, got %q`, vpcIDandSubnetID, subnet.State)
		case !usableZones.Contains(subnet.AvailZone):
			return nil, errors.Errorf(
				"expected AZ among %v for %s, got %q",
				usableZones.SortedValues(), vpcIDandSubnetID, subnet.AvailZone,
			)

		case !subnet.MapPublicIPOnLaunch:
			logger.Debugf("skipping %s without MapPublicIPOnLaunch set", vpcIDandSubnetID)
			continue
		default:
			// So far so good, we still need to verify the subnet has a route to the IGW.
			logger.Tracef("found potential usable public %s", vpcIDandSubnetID)
			potentiallyUsableSubnets.Add(subnet.Id)
		}
	}
	logger.Tracef(
		"found %d public and available subnets in VPC %q: %v",
		potentiallyUsableSubnets.Size(),
		vpcID,
		potentiallyUsableSubnets.SortedValues(),
	)

	if potentiallyUsableSubnets.IsEmpty() {
		return nil, errors.NewNotValid(
			nil,
			"no public subnets with state 'available' found to host a Juju controller instance",
		)
	}

	// All good!
	return vpc, nil
}
