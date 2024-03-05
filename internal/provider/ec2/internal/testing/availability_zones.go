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

// SetAvailabilityZones replaces the availability zones the test
// server is returning.
//
// NOTE: If zones does not contain one or more existing zones those
// existing zones are removed along with any subnets that are
// associated with them!
func (srv *Server) SetAvailabilityZones(zones []types.AvailabilityZone) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	oldZones := srv.zones
	srv.zones = make(map[string]availabilityZone)
	for _, z := range zones {
		name := aws.ToString(z.ZoneName)
		srv.zones[name] = availabilityZone{z}

		_, isNew := srv.zones[name]
		_, isOld := oldZones[name]
		if isOld && !isNew {
			// Remove any subnets attached to this zone as we're
			// removing it.
			remainingSubnets := make(map[string]*subnet)
			for _, sub := range srv.subnets {
				if aws.ToString(sub.AvailabilityZone) != name {
					remainingSubnets[aws.ToString(sub.SubnetId)] = sub
				}
			}
			srv.subnets = remainingSubnets
		}
	}
}

type availabilityZone struct {
	types.AvailabilityZone
}

func (z *availabilityZone) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "message":
		for _, m := range z.Messages {
			if aws.ToString(m.Message) == value {
				return true, nil
			}
		}
		return false, nil
	case "region-name":
		return aws.ToString(z.RegionName) == value, nil
	case "state":
		switch value {
		case "available", "impaired", "unavailable":
			return string(z.State) == value, nil
		}
		return false, fmt.Errorf("invalid state %q", value)
	case "zone-name":
		return aws.ToString(z.ZoneName) == value, nil
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

// DescribeAvailabilityZones implements ec2.Client
func (srv *Server) DescribeAvailabilityZones(ctx context.Context, in *ec2.DescribeAvailabilityZonesInput, opts ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var f ec2filter
	idSet := set.NewStrings()
	if in != nil {
		f = in.Filters
		idSet = set.NewStrings(in.ZoneIds...)
	}
	resp := &ec2.DescribeAvailabilityZonesOutput{}
	for _, zone := range srv.zones {
		ok, err := f.ok(&zone)
		if ok && (idSet.Size() == 0 || idSet.Contains(aws.ToString(zone.ZoneId))) {
			resp.AvailabilityZones = append(resp.AvailabilityZones, zone.AvailabilityZone)
		} else if err != nil {
			return nil, apiError("InvalidParameterValue", "describe availability zones: %v", err)
		}
	}
	return resp, nil
}
