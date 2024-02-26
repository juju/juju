// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
)

// DescribeRouteTables implements ec2.Client.
func (srv *Server) DescribeRouteTables(ctx context.Context, in *ec2.DescribeRouteTablesInput, opts ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var f ec2filter
	idSet := set.NewStrings()
	if in != nil {
		f = in.Filters
		idSet = set.NewStrings(in.RouteTableIds...)
	}

	resp := &ec2.DescribeRouteTablesOutput{}
	for _, t := range srv.routeTables {
		ok, err := f.ok(t)
		if ok && (idSet.Size() == 0 || idSet.Contains(aws.ToString(t.RouteTableId))) {
			resp.RouteTables = append(resp.RouteTables, t.RouteTable)
		} else if err != nil {
			return nil, apiError("InvalidParameterValue", "describe route tables: %v", err)
		}
	}
	return resp, nil
}

// AddRouteTable inserts the given route table in the test server, as
// if it was created using the simulated AWS API. The Id field of t is
// ignored and replaced by the next rtbId counter value, prefixed by
// "rtb-". When IsMain is true, the table becomes the main route table
// for its VPC.
//
// Any empty TableId field of an item in the Associations list will be
// set to the added table's Id automatically.
func (srv *Server) AddRouteTable(t types.RouteTable) (types.RouteTable, error) {
	if aws.ToString(t.VpcId) == "" {
		return types.RouteTable{}, fmt.Errorf("missing VPC id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	vpcId := aws.ToString(t.VpcId)
	if _, found := srv.vpcs[vpcId]; !found {
		return types.RouteTable{}, fmt.Errorf("VPC %q not found", vpcId)
	}
	added := &routeTable{t}
	added.RouteTableId = aws.String(fmt.Sprintf("rtb-%d", srv.rtbId.next()))
	for i, assoc := range added.Associations {
		assoc.RouteTableAssociationId = aws.String(fmt.Sprintf("rtbassoc-%d", srv.rtbassocId.next()))
		if aws.ToString(assoc.RouteTableId) == "" {
			assoc.RouteTableId = added.RouteTableId
		}
		added.Associations[i] = assoc
	}
	srv.routeTables[aws.ToString(added.RouteTableId)] = added
	return added.RouteTable, nil
}

// RemoveRouteTable removes an route table with the given rtbId from
// the test server. It's an error to try to remove an unknown or empty
// rtbId.
func (srv *Server) RemoveRouteTable(rtbId string) error {
	if rtbId == "" {
		return fmt.Errorf("missing route table id")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, found := srv.routeTables[rtbId]; found {
		delete(srv.routeTables, rtbId)
		return nil
	}
	return fmt.Errorf("route table %q not found", rtbId)
}

type routeTable struct {
	types.RouteTable
}

func (t *routeTable) matchAttr(attr, value string) (ok bool, err error) {
	filterByAssociation := func(check func(assoc types.RouteTableAssociation) bool) (bool, error) {
		for _, assoc := range t.Associations {
			if check(assoc) {
				return true, nil
			}
		}
		return false, nil
	}
	filterByRoute := func(check func(route types.Route) bool) (bool, error) {
		for _, route := range t.Routes {
			if check(route) {
				return true, nil
			}
		}
		return false, nil
	}

	switch attr {
	case "route-table-id":
		return aws.ToString(t.RouteTableId) == value, nil
	case "vpc-id":
		return aws.ToString(t.VpcId) == value, nil
	case "route.destination-cidr-block":
		return filterByRoute(func(r types.Route) bool {
			return aws.ToString(r.DestinationCidrBlock) == value
		})
	case "route.gateway-id":
		return filterByRoute(func(r types.Route) bool {
			return aws.ToString(r.GatewayId) == value
		})
	case "route.instance-id":
		return filterByRoute(func(r types.Route) bool {
			return aws.ToString(r.InstanceId) == value
		})
	case "route.state":
		return filterByRoute(func(r types.Route) bool {
			return string(r.State) == value
		})
	case "association.main":
		val, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("bad flag %q: %s", attr, value)
		}
		return filterByAssociation(func(a types.RouteTableAssociation) bool {
			return aws.ToBool(a.Main) == val
		})
	case "association.subnet-id":
		return filterByAssociation(func(a types.RouteTableAssociation) bool {
			return aws.ToString(a.SubnetId) == value
		})
	case "association.route-table-id":
		return filterByAssociation(func(a types.RouteTableAssociation) bool {
			return aws.ToString(a.RouteTableId) == value
		})
	case "association.route-table-association-id":
		return filterByAssociation(func(a types.RouteTableAssociation) bool {
			return aws.ToString(a.RouteTableAssociationId) == value
		})
	case "tag", "tag-key", "tag-value", "route.origin",
		"route.destination-prefix-list-id", "route.vpc-peering-connection-id":
		return false, fmt.Errorf("%q filter is not implemented", attr)
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}
