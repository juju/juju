// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
)

// DescribeSubnets implements ec2.Client.
func (srv *Server) DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var f ec2filter
	idSet := set.NewStrings()
	if in != nil {
		f = in.Filters
		idSet = set.NewStrings(in.SubnetIds...)
	}

	resp := &ec2.DescribeSubnetsOutput{}
	for _, s := range srv.subnets {
		ok, err := f.ok(s)
		if ok && (idSet.Size() == 0 || idSet.Contains(aws.ToString(s.SubnetId))) {
			resp.Subnets = append(resp.Subnets, s.Subnet)
		} else if err != nil {
			return nil, apiError("InvalidParameterValue", "describe subnets: %v", err)
		}
	}
	return resp, nil
}

// AddSubnet inserts the given subnet in the test server, as if it was
// created using the simulated AWS API. The Id field of sub is ignored
// and replaced by the next subnetId counter value, prefixed by
// "subnet-". The VPCId field of sub must be contain an existing VPC
// id, and the AvailabilityZone field must contain an existing AZ,
// otherwise errors are returned.
func (srv *Server) AddSubnet(sub types.Subnet) (types.Subnet, error) {
	zeroSubnet := types.Subnet{}

	vpcId := aws.ToString(sub.VpcId)
	availZone := aws.ToString(sub.AvailabilityZone)
	if vpcId == "" {
		return zeroSubnet, fmt.Errorf("empty VPCId field")
	}
	if availZone == "" {
		return zeroSubnet, fmt.Errorf("empty AvailZone field")
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	if _, found := srv.vpcs[vpcId]; !found {
		return zeroSubnet, fmt.Errorf("no such VPC %q", vpcId)
	}
	if _, found := srv.zones[availZone]; !found {
		return zeroSubnet, fmt.Errorf("no such availability zone %q", availZone)
	}

	added := &subnet{sub}
	added.SubnetId = aws.String(fmt.Sprintf("subnet-%d", srv.subnetId.next()))
	srv.subnets[aws.ToString(added.SubnetId)] = added
	return added.Subnet, nil
}

type subnet struct {
	types.Subnet
}

func (s *subnet) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "cidr":
		return aws.ToString(s.CidrBlock) == value, nil
	case "availability-zone":
		return aws.ToString(s.AvailabilityZone) == value, nil
	case "state":
		return string(s.State) == value, nil
	case "subnet-id":
		return aws.ToString(s.SubnetId) == value, nil
	case "vpc-id":
		return aws.ToString(s.VpcId) == value, nil
	case "defaultForAz", "default-for-az":
		val, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("bad flag %q: %s", attr, value)
		}
		return aws.ToBool(s.DefaultForAz) == val, nil
	case "tag", "tag-key", "tag-value", "available-ip-address-count":
		return false, fmt.Errorf("%q filter not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

// getDefaultSubnet returns the first default subnet for the AZ in the
// default VPC (if available).
func (srv *Server) getDefaultSubnet() *subnet {
	// We need to get the default VPC id and one of its subnets to use.
	defaultVPCId := ""
	for _, vpc := range srv.vpcs {
		if aws.ToBool(vpc.IsDefault) {
			defaultVPCId = aws.ToString(vpc.VpcId)
			break
		}
	}
	if defaultVPCId == "" {
		// No default VPC, so nothing to do.
		return nil
	}
	for _, subnet := range srv.subnets {
		if aws.ToString(subnet.VpcId) == defaultVPCId && aws.ToBool(subnet.DefaultForAz) {
			return subnet
		}
	}
	return nil
}

func (srv *Server) calcSubnetAvailIPs(cidrBlock string) (int, error) {
	_, ipnet, err := net.ParseCIDR(cidrBlock)
	if err != nil {
		return 0, err
	}
	// calculate the available IP addresses, removing the first 4 and
	// the last, which are reserved by AWS.
	maskOnes, maskBits := ipnet.Mask.Size()
	return 1<<uint(maskBits-maskOnes) - 5, nil
}
