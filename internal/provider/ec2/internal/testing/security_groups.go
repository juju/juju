// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// CreateSecurityGroup implements ec2.Client.
func (srv *Server) CreateSecurityGroup(ctx context.Context, in *ec2.CreateSecurityGroupInput, opts ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	srv.groupMutatingCalls.next()
	name := aws.ToString(in.GroupName)
	if name == "" {
		return nil, apiError("InvalidParameterValue", "empty security group name")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.group(types.GroupIdentifier{GroupName: in.GroupName}) != nil {
		return nil, apiError("InvalidGroup.Duplicate", "group %q already exists", name)
	}

	g := &securityGroup{
		name:        name,
		description: aws.ToString(in.Description),
		id:          fmt.Sprintf("sg-%d", srv.groupId.next()),
		perms:       make(map[permKey]bool),
		tags:        tagSpecForType(types.ResourceTypeSecurityGroup, in.TagSpecifications).Tags,
	}
	vpcId := aws.ToString(in.VpcId)
	if vpcId != "" {
		g.vpcId = vpcId
	}
	srv.groups[g.id] = g

	resp := &ec2.CreateSecurityGroupOutput{
		GroupId: aws.String(g.id),
		Tags:    g.tags,
	}
	return resp, nil
}

// DeleteSecurityGroup implements ec2.Client.
func (srv *Server) DeleteSecurityGroup(ctx context.Context, in *ec2.DeleteSecurityGroupInput, opts ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	srv.groupMutatingCalls.next()
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if in.GroupName != nil {
		return nil, apiError("InvalidParameterValue", "group should only be accessed by id")
	}
	g := srv.group(types.GroupIdentifier{
		GroupId:   in.GroupId,
		GroupName: in.GroupName,
	})
	if g == nil {
		return nil, apiError("InvalidGroup.NotFound", "group not found")
	}
	for _, r := range srv.reservations {
		for _, h := range r.groups {
			if h == g && r.hasRunningMachine() {
				return nil, apiError("DependencyViolation", "group is currently in use by a running instance")
			}
		}
	}
	for _, sg := range srv.groups {
		// If a group refers to itself, it's ok to delete it.
		if sg == g {
			continue
		}
		for k := range sg.perms {
			if k.group == g {
				return nil, apiError("DependencyViolation", "group is currently in use by group %q", sg.id)
			}
		}
	}

	delete(srv.groups, g.id)
	return &ec2.DeleteSecurityGroupOutput{}, nil
}

func (srv *Server) group(group types.GroupIdentifier) *securityGroup {
	if id := aws.ToString(group.GroupId); id != "" {
		return srv.groups[id]
	}
	for _, g := range srv.groups {
		if g.name == aws.ToString(group.GroupName) {
			return g
		}
	}
	return nil
}

// AuthorizeSecurityGroupIngress implements ec2.Client.
func (srv *Server) AuthorizeSecurityGroupIngress(ctx context.Context, in *ec2.AuthorizeSecurityGroupIngressInput, opts ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	srv.groupMutatingCalls.next()
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if in.GroupName != nil {
		return nil, apiError("InvalidParameterValue", "group should only be accessed by id")
	}
	g := srv.group(types.GroupIdentifier{
		GroupId:   in.GroupId,
		GroupName: in.GroupName,
	})
	if g == nil {
		return nil, apiError("InvalidGroup.NotFound", "group not found")
	}

	perms, err := srv.parsePerms(in.IpPermissions)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		if g.perms[p] {
			return nil, apiError("InvalidPermission.Duplicate", "Permission has already been authorized on the specified group")
		}
	}
	for _, p := range perms {
		g.perms[p] = true
	}
	return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
}

// RevokeSecurityGroupIngress implements ec2.Client.
func (srv *Server) RevokeSecurityGroupIngress(ctx context.Context, in *ec2.RevokeSecurityGroupIngressInput, opts ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	srv.groupMutatingCalls.next()
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if in.GroupName != nil {
		return nil, apiError("InvalidParameterValue", "group should only be accessed by id")
	}
	g := srv.group(types.GroupIdentifier{
		GroupId:   in.GroupId,
		GroupName: in.GroupName,
	})
	if g == nil {
		return nil, apiError("InvalidGroup.NotFound", "group not found")
	}

	// Note EC2 does not give an error if asked to revoke an authorization
	// that does not exist.
	perms, err := srv.parsePerms(in.IpPermissions)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		delete(g.perms, p)
	}
	return &ec2.RevokeSecurityGroupIngressOutput{}, nil
}

type securityGroup struct {
	id          string
	name        string
	description string
	vpcId       string

	perms map[permKey]bool
	tags  []types.Tag
}

// permKey represents permission for a given security group.
// Equality of permKeys is used in the implementation of permission
// sets, relying on the uniqueness of securityGroup instances.
type permKey struct {
	protocol string
	fromPort int32
	toPort   int32
	group    *securityGroup
	ipAddr   string
}

func (g *securityGroup) ec2SecurityGroup() types.SecurityGroup {
	return types.SecurityGroup{
		GroupName: aws.String(g.name),
		GroupId:   aws.String(g.id),
	}
}

func (g *securityGroup) matchAttr(attr, value string) (ok bool, err error) {
	switch attr {
	case "description":
		return g.description == value, nil
	case "group-id":
		return g.id == value, nil
	case "group-name":
		return g.name == value, nil
	case "ip-permission.cidr":
		return g.hasPerm(func(k permKey) bool { return k.ipAddr == value }), nil
	case "ip-permission.group-name":
		return g.hasPerm(func(k permKey) bool {
			return k.group != nil && k.group.name == value
		}), nil
	case "ip-permission.group-id":
		return g.hasPerm(func(k permKey) bool {
			return k.group != nil && k.group.id == value
		}), nil
	case "ip-permission.from-port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return g.hasPerm(func(k permKey) bool { return k.fromPort == int32(port) }), nil
	case "ip-permission.to-port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return g.hasPerm(func(k permKey) bool { return k.toPort == int32(port) }), nil
	case "ip-permission.protocol":
		return g.hasPerm(func(k permKey) bool { return k.protocol == value }), nil
	case "owner-id":
		return value == ownerId, nil
	case "vpc-id":
		return g.vpcId == value, nil
	}
	if strings.HasPrefix(attr, "tag:") {
		key := attr[len("tag:"):]
		return matchTag(g.tags, key, value), nil
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (g *securityGroup) hasPerm(test func(k permKey) bool) bool {
	for k := range g.perms {
		if test(k) {
			return true
		}
	}
	return false
}

// ec2Perms returns the list of EC2 permissions granted
// to g. It groups permissions by port range and protocol.
func (g *securityGroup) ec2Perms() (perms []types.IpPermission) {
	// The grouping is held in result. We use permKey for convenience,
	// (ensuring that the ipAddr of each key is zero). For each
	// protocol/port range combination, we build up the permission set
	// in the associated value.
	result := make(map[permKey]*types.IpPermission)
	for k := range g.perms {
		groupKey := k
		groupKey.ipAddr = ""

		ec2p := result[groupKey]
		if ec2p == nil {
			ec2p = &types.IpPermission{
				IpProtocol: aws.String(k.protocol),
				FromPort:   aws.Int32(k.fromPort),
				ToPort:     aws.Int32(k.toPort),
			}
		}
		if k.group != nil {
			ec2p.UserIdGroupPairs = append(ec2p.UserIdGroupPairs,
				types.UserIdGroupPair{
					GroupId: aws.String(k.group.id),
					UserId:  aws.String(ownerId),
				})
		} else if k.ipAddr != "" {
			ec2p.IpRanges = append(ec2p.IpRanges, types.IpRange{CidrIp: aws.String(k.ipAddr)})
		}
		result[groupKey] = ec2p
	}
	for _, ec2p := range result {
		perms = append(perms, *ec2p)
	}
	return
}

func (srv *Server) DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var groups []*securityGroup
	if in != nil && (len(in.GroupIds) > 0 || len(in.GroupNames) > 0) {
		var err error
		groups, err = srv.groupsByIdOrName(in.GroupIds, in.GroupNames)
		if err != nil {
			return nil, err
		}
	} else {
		for _, g := range srv.groups {
			groups = append(groups, g)
		}
	}

	var f ec2filter
	if in != nil {
		f = in.Filters
	}

	resp := &ec2.DescribeSecurityGroupsOutput{}
	for _, group := range groups {
		ok, err := f.ok(group)
		if ok {
			resp.SecurityGroups = append(resp.SecurityGroups, types.SecurityGroup{
				OwnerId:       aws.String(ownerId),
				GroupId:       aws.String(group.id),
				GroupName:     aws.String(group.name),
				Description:   aws.String(group.description),
				IpPermissions: group.ec2Perms(),
			})
		} else if err != nil {
			return nil, apiError("InvalidParameterValue", "describe security groups: %v", err)
		}
	}
	return resp, nil
}

func (srv *Server) groupsByIdOrName(ids []string, names []string) ([]*securityGroup, error) {
	var groups []*securityGroup
	for _, id := range ids {
		g := types.GroupIdentifier{
			GroupId: aws.String(id),
		}
		sg := srv.group(g)
		if sg == nil {
			return nil, apiError("InvalidGroup.NotFound", "no such group with id %v", id)
		}
		groups = append(groups, sg)
	}
	for _, name := range names {
		g := types.GroupIdentifier{
			GroupName: aws.String(name),
		}
		sg := srv.group(g)
		if sg == nil {
			return nil, apiError("InvalidGroup.NotFound", "no such group with name %v", name)
		}
		groups = append(groups, sg)
	}
	return groups, nil
}

func (srv *Server) parsePerms(in []types.IpPermission) ([]permKey, error) {
	perms := make(map[int]types.IpPermission)

	type subgroupKey struct {
		id1, id2 int
	}
	// Each IPPermission can have many source security groups.
	sourceGroups := make(map[subgroupKey]types.UserIdGroupPair)

	// For each value in the input permissions we store its associated
	// information in the above maps.
	for id1, inPerm := range in {
		ec2p := perms[id1]
		ec2p.IpProtocol = inPerm.IpProtocol
		ec2p.FromPort = inPerm.FromPort
		ec2p.ToPort = inPerm.ToPort
		for id2, userGroup := range inPerm.UserIdGroupPairs {
			k := subgroupKey{id1: id1, id2: id2}
			g := sourceGroups[k]
			g.UserId = userGroup.UserId
			g.GroupId = userGroup.GroupId
			g.GroupName = userGroup.GroupName
			sourceGroups[k] = g
		}
		for _, r := range inPerm.IpRanges {
			ec2p.IpRanges = append(ec2p.IpRanges, types.IpRange{CidrIp: r.CidrIp})
		}
		perms[id1] = ec2p
	}
	// Associate each set of source groups with its IPPerm.
	for k, g := range sourceGroups {
		p := perms[k.id1]
		p.UserIdGroupPairs = append(p.UserIdGroupPairs, g)
		perms[k.id1] = p
	}

	// Now that we have built up the IPPerms we need, we check for
	// parameter errors and build up a permKey for each permission,
	// looking up security groups from srv as we do so.
	var result []permKey
	for _, p := range perms {
		if !isICMPRule(p) && aws.ToInt32(p.FromPort) > aws.ToInt32(p.ToPort) {
			return nil, apiError("InvalidParameterValue", "invalid port range")
		}
		k := permKey{
			protocol: aws.ToString(p.IpProtocol),
			fromPort: aws.ToInt32(p.FromPort),
			toPort:   aws.ToInt32(p.ToPort),
		}
		for _, g := range p.UserIdGroupPairs {
			if aws.ToString(g.UserId) != "" && aws.ToString(g.UserId) != ownerId {
				return nil, apiError("InvalidGroup.NotFound", "group %q not found", aws.ToString(g.GroupName))
			}
			k.group = srv.group(types.GroupIdentifier{
				GroupId:   g.GroupId,
				GroupName: g.GroupName,
			})
			if k.group == nil {
				return nil, apiError("InvalidGroup.NotFound", "group %v not found", aws.ToString(g.GroupName))
			}
			result = append(result, k)
		}
		k.group = nil
		for _, ip := range p.IpRanges {
			k.ipAddr = aws.ToString(ip.CidrIp)
			result = append(result, k)
		}
	}
	return result, nil
}

func isICMPRule(permission types.IpPermission) bool {
	return permission.IpProtocol != nil &&
		(*permission.IpProtocol == "icmp" ||
			*permission.IpProtocol == "icmpv6" ||
			*permission.IpProtocol == "1" || // icmp protocol number
			*permission.IpProtocol == "58") // icmpv6 protocol number
}
