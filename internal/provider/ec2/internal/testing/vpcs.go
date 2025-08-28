// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
)

// AddVpc inserts the given Vpc in the test server, as if it was
// created using the simulated AWS API. The Id field of v is ignored
// and replaced by the next vpcId counter value, prefixed by "vpc-".
// When IsDefault is true, the Vpc becomes the default Vpc for the
// simulated region.
func (srv *Server) AddVpc(v types.Vpc) types.Vpc {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	added := &vpc{v}
	added.VpcId = aws.String(fmt.Sprintf("vpc-%d", srv.vpcId.next()))
	srv.vpcs[aws.ToString(added.VpcId)] = added
	return added.Vpc
}

// UpdateVpc updates the Vpc info stored in the test server, matching
// the Id field of v, replacing all the other values with v's field
// values. It's an error to try to update a Vpc with unknown or empty
// Id.
func (srv *Server) UpdateVpc(v types.Vpc) error {
	vpcId := aws.ToString(v.VpcId)
	if vpcId == "" {
		return fmt.Errorf("missing Vpc id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	vpc, found := srv.vpcs[vpcId]
	if !found {
		return fmt.Errorf("Vpc %q not found", vpcId)
	}
	vpc.CidrBlock = v.CidrBlock
	vpc.DhcpOptionsId = v.DhcpOptionsId
	vpc.InstanceTenancy = v.InstanceTenancy
	vpc.Tags = append([]types.Tag{}, v.Tags...)
	vpc.IsDefault = v.IsDefault
	vpc.State = v.State
	srv.vpcs[vpcId] = vpc
	return nil
}

// RemoveVpc removes an existing Vpc with the given vpcId from the
// test server. It's an error to try to remove an unknown or empty
// vpcId.
//
// NOTE: Removing a Vpc will remove all of its subnets.
func (srv *Server) RemoveVpc(vpcId string) error {
	if vpcId == "" {
		return fmt.Errorf("missing Vpc id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, found := srv.vpcs[vpcId]; found {
		delete(srv.vpcs, vpcId)
		remainingSubnets := make(map[string]*subnet)
		for _, sub := range srv.subnets {
			if aws.ToString(sub.VpcId) != vpcId {
				remainingSubnets[aws.ToString(sub.SubnetId)] = sub
			}
		}
		srv.subnets = remainingSubnets
		return nil
	}
	return fmt.Errorf("Vpc %q not found", vpcId)
}

// DescribeVpcs implements ec2.Client.
func (srv *Server) DescribeVpcs(ctx context.Context, in *ec2.DescribeVpcsInput, opts ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var f ec2filter
	idSet := set.NewStrings()
	if in != nil {
		f = in.Filters
		idSet = set.NewStrings(in.VpcIds...)
	}

	resp := &ec2.DescribeVpcsOutput{}
	for _, v := range srv.vpcs {
		ok, err := f.ok(v)
		if ok && (idSet.Size() == 0 || idSet.Contains(aws.ToString(v.VpcId))) {
			resp.Vpcs = append(resp.Vpcs, v.Vpc)
		} else if err != nil {
			return nil, apiError("InvalidParameterValue", "describe Vpcs: %v", err)
		}
	}
	return resp, nil
}

// AddDefaultVpcAndSubnets makes it easy to simulate a default Vpc is
// present in the test server. Calling this method is more or less
// an equivalent of calling the following methods as described:
//
// 1. AddVpc(), using 10.10.0.0/16 as CIDR and sane defaults.
// 2. AddInternetGateway(), attached to the default Vpc.
// 3. AddRouteTable(), attached to the default Vpc, with sane defaults
// and using the IGW above as default route.
// 4. AddSubnet(), once per defined AZ, with 10.10.X.0/24 CIDR (X
// is a zero-based index). Each subnet has both DefaultForAZ and
// MapPublicIPOnLaunch attributes set.
// 5. SetAccountAttributes(), with "supported-platforms" set to "EC2",
// "Vpc"; and "default-vpc" set to the added default Vpc.
//
// NOTE: If no AZs are set on the test server, this method fails.
func (srv *Server) AddDefaultVpcAndSubnets() (defaultVpc types.Vpc, err error) {
	zeroVpc := types.Vpc{}
	var igw types.InternetGateway
	var rtbMain types.RouteTable

	defer func() {
		// Cleanup all partially added items on error.
		defaultVpcId := aws.ToString(defaultVpc.VpcId)
		if err != nil && defaultVpcId != "" {
			_ = srv.RemoveVpc(defaultVpcId)
			if rtbId := aws.ToString(rtbMain.RouteTableId); rtbId != "" {
				_ = srv.RemoveRouteTable(rtbId)
			}
			if igwId := aws.ToString(igw.InternetGatewayId); igwId != "" {
				_ = srv.RemoveInternetGateway(igwId)
			}
			_ = srv.SetAccountAttributes(map[string][]types.AccountAttributeValue{})
			defaultVpc.VpcId = aws.String("") // it's gone anyway.
		}
	}()

	if len(srv.zones) == 0 {
		return zeroVpc, fmt.Errorf("no AZs defined")
	}
	defaultVpc = srv.AddVpc(types.Vpc{
		State:           "available",
		CidrBlock:       aws.String("10.10.0.0/16"),
		DhcpOptionsId:   aws.String(fmt.Sprintf("dopt-%d", srv.dhcpOptsId.next())),
		InstanceTenancy: "default",
		IsDefault:       aws.Bool(true),
	})

	defaultVpcId := aws.ToString(defaultVpc.VpcId)
	zeroVpc.VpcId = aws.String(defaultVpcId) // zeroed again in the deferred
	igw, err = srv.AddInternetGateway(types.InternetGateway{
		Attachments: []types.InternetGatewayAttachment{{
			VpcId: aws.String(defaultVpcId),
			State: "available",
		}},
	})
	if err != nil {
		return zeroVpc, err
	}
	rtbMain, err = srv.AddRouteTable(types.RouteTable{
		VpcId: defaultVpc.VpcId,
		Associations: []types.RouteTableAssociation{{
			Main: aws.Bool(true),
		}},
		Routes: []types.Route{{
			DestinationCidrBlock: defaultVpc.CidrBlock, // default Vpc internal traffic
			GatewayId:            aws.String("local"),
			State:                "active",
		}, {
			DestinationCidrBlock: aws.String("0.0.0.0/0"), // default Vpc default egress route.
			GatewayId:            igw.InternetGatewayId,
			State:                "active",
		}},
	})
	if err != nil {
		return zeroVpc, err
	}
	subnetIndex := 0
	for zone := range srv.zones {
		cidrBlock := fmt.Sprintf("10.10.%d.0/24", subnetIndex)
		availIPs, _ := srv.calcSubnetAvailIPs(cidrBlock)
		_, err = srv.AddSubnet(types.Subnet{
			VpcId:                   aws.String(defaultVpcId),
			State:                   "available",
			CidrBlock:               aws.String(cidrBlock),
			AvailabilityZone:        aws.String(zone),
			AvailableIpAddressCount: aws.Int32(int32(availIPs)),
			DefaultForAz:            aws.Bool(true),
		})
		if err != nil {
			return zeroVpc, err
		}
		subnetIndex++
	}
	err = srv.SetAccountAttributes(map[string][]types.AccountAttributeValue{
		"supported-platforms": {
			types.AccountAttributeValue{AttributeValue: aws.String("EC2")},
			types.AccountAttributeValue{AttributeValue: aws.String("Vpc")},
		},
		"default-vpc": {
			types.AccountAttributeValue{AttributeValue: aws.String(defaultVpcId)},
		},
	})
	if err != nil {
		return zeroVpc, err
	}
	return defaultVpc, nil
}

type vpc struct {
	types.Vpc
}

func (v *vpc) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "cidr":
		return aws.ToString(v.CidrBlock) == value, nil
	case "state":
		return string(v.State) == value, nil
	case "vpc-id":
		return aws.ToString(v.VpcId) == value, nil
	case "isDefault":
		return aws.ToBool(v.IsDefault) == (value == "true"), nil
	case "tag", "tag-key", "tag-value", "dhcp-options-id":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}
