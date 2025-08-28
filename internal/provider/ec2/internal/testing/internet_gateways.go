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

// DescribeInternetGateways implements ec2.Client.
func (srv *Server) DescribeInternetGateways(ctx context.Context, in *ec2.DescribeInternetGatewaysInput, opts ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var f ec2filter
	idSet := set.NewStrings()
	if in != nil {
		f = in.Filters
		idSet = set.NewStrings(in.InternetGatewayIds...)
	}

	resp := &ec2.DescribeInternetGatewaysOutput{}
	for _, i := range srv.internetGateways {
		ok, err := f.ok(i)
		if ok && (idSet.Size() == 0 || idSet.Contains(aws.ToString(i.InternetGatewayId))) {
			resp.InternetGateways = append(resp.InternetGateways, i.InternetGateway)
		} else if err != nil {
			return nil, apiError("InvalidParameterValue", "describe internet gateways: %v", err)
		}
	}
	return resp, nil
}

// AddInternetGateway inserts the given internet gateway in the test
// server, as if it was created using the simulated AWS API. The Id
// field of igw is ignored and replaced by the next igwId counter
// value, prefixed by "igw-". When set, the VPCId field must refer to
// a VPC the test server knows about. If VPCId is empty the IGW is
// considered not attached.
func (srv *Server) AddInternetGateway(igw types.InternetGateway) (types.InternetGateway, error) {
	zeroGateway := types.InternetGateway{}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	var vpcId string
	if len(igw.Attachments) > 0 {
		vpcId = aws.ToString(igw.Attachments[0].VpcId)
	}
	if vpcId != "" {
		if _, found := srv.vpcs[vpcId]; !found {
			return zeroGateway, fmt.Errorf("VPC %q not found", vpcId)
		}
	}
	added := &internetGateway{igw}
	added.InternetGatewayId = aws.String(fmt.Sprintf("igw-%d", srv.igwId.next()))
	srv.internetGateways[aws.ToString(added.InternetGatewayId)] = added
	return added.InternetGateway, nil
}

// RemoveInternetGateway removes the internet gateway with the given
// igwId, stored in the test server. It's an error to try to remove an
// unknown or empty igwId.
func (srv *Server) RemoveInternetGateway(igwId string) error {
	if igwId == "" {
		return fmt.Errorf("missing internet gateway id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, found := srv.internetGateways[igwId]; found {
		delete(srv.internetGateways, igwId)
		return nil
	}
	return fmt.Errorf("internet gateway %q not found", igwId)
}

type internetGateway struct {
	types.InternetGateway
}

func (i *internetGateway) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "internet-gateway-id":
		return aws.ToString(i.InternetGatewayId) == value, nil
	case "attachment.state":
		if len(i.Attachments) == 0 {
			return false, nil
		}
		return string(i.Attachments[0].State) == value, nil
	case "attachment.vpc-id":
		if len(i.Attachments) == 0 {
			return false, nil
		}
		return aws.ToString(i.Attachments[0].VpcId) == value, nil
	case "tag", "tag-key", "tag-value":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}
