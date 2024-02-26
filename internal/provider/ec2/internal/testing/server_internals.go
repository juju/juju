// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	ownerId = "9876"
	// defaultAvailZone is the availability zone to use by default.
	defaultAvailZone = "us-east-1c"
)

func (srv *Server) addDefaultZonesAndGroups() {
	// Add default security group.
	g := &securityGroup{
		name:        "default",
		description: "default group",
		id:          fmt.Sprintf("sg-%d", srv.groupId.next()),
	}
	g.perms = map[permKey]bool{
		{
			protocol: "icmp",
			fromPort: -1,
			toPort:   -1,
			group:    g,
		}: true,
		{
			protocol: "tcp",
			fromPort: 0,
			toPort:   65535,
			group:    g,
		}: true,
		{
			protocol: "udp",
			fromPort: 0,
			toPort:   65535,
			group:    g,
		}: true,
	}
	srv.groups[g.id] = g

	// Add a default availability zone.
	var z availabilityZone
	z.ZoneName = aws.String(defaultAvailZone)
	z.RegionName = aws.String("us-east-1")
	z.State = "available"
	srv.zones[defaultAvailZone] = z
}
