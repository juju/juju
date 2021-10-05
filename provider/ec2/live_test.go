// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	stdcontext "context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/ec2"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

func (t *localServerSuite) TestInstanceAttributes(c *gc.C) {
	t.Prepare(c)
	inst, hc := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "30")
	defer t.Env.StopInstances(t.callCtx, inst.Id())
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, gc.NotNil)
	c.Assert(hc.Mem, gc.NotNil)
	c.Assert(hc.RootDisk, gc.NotNil)
	c.Assert(hc.CpuCores, gc.NotNil)
	c.Assert(hc.CpuPower, gc.NotNil)
	addresses, err := testing.WaitInstanceAddresses(t.Env, t.callCtx, inst.Id())
	// TODO(niemeyer): This assert sometimes fails with "no instances found"
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	insts, err := t.Env.Instances(t.callCtx, []instance.Id{inst.Id()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(insts), gc.Equals, 1)

	ec2inst := ec2.InstanceSDKEC2(insts[0])
	c.Assert(*ec2inst.PublicIpAddress, gc.Equals, addresses[0].Value)
	c.Assert(ec2inst.InstanceType, gc.Equals, types.InstanceType("t3a.micro"))
}

func (t *localServerSuite) TestStartInstanceConstraints(c *gc.C) {
	t.Prepare(c)
	cons := constraints.MustParse("mem=4G")
	inst, hc := testing.AssertStartInstanceWithConstraints(c, t.Env, t.callCtx, t.ControllerUUID, "30", cons)
	defer t.Env.StopInstances(t.callCtx, inst.Id())
	ec2inst := ec2.InstanceSDKEC2(inst)
	c.Assert(ec2inst.InstanceType, gc.Equals, types.InstanceType("t3a.medium"))
	c.Assert(*hc.Arch, gc.Equals, "amd64")
	c.Assert(*hc.Mem, gc.Equals, uint64(4*1024))
	c.Assert(*hc.RootDisk, gc.Equals, uint64(8*1024))
	c.Assert(*hc.CpuCores, gc.Equals, uint64(2))
}

func (t *localServerSuite) TestControllerInstances(c *gc.C) {
	t.prepareAndBootstrap(c)
	allInsts, err := t.Env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInsts, gc.HasLen, 1) // bootstrap instance
	bootstrapInstId := allInsts[0].Id()

	inst0, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "98")
	defer t.Env.StopInstances(t.callCtx, inst0.Id())

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "99")
	defer t.Env.StopInstances(t.callCtx, inst1.Id())

	insts, err := t.Env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.DeepEquals, []instance.Id{bootstrapInstId})
}

func (t *localServerSuite) TODO_needs_fixing_TestInstanceGroups(c *gc.C) {
	t.prepareAndBootstrap(c)
	allInsts, err := t.Env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInsts, gc.HasLen, 1) // bootstrap instance
	bootstrapInstId := allInsts[0].Id()

	ec2conn := ec2.EnvironEC2Client(t.Env)

	groups := []*types.SecurityGroupIdentifier{
		{GroupName: aws.String(ec2.JujuGroupName(t.Env))},
		{GroupName: aws.String(ec2.MachineGroupName(t.Env, "98"))},
		{GroupName: aws.String(ec2.MachineGroupName(t.Env, "99"))},
	}
	info := make([]types.SecurityGroup, len(groups))

	// Create a group with the same name as the juju group
	// but with different permissions, to check that it's deleted
	// and recreated correctly.
	oldJujuGroup := createGroup(c, ec2conn, t.callCtx, aws.ToString(groups[0].GroupName), "old juju group")

	// Add a permission.
	// N.B. this is unfortunately sensitive to the actual set of permissions used.
	_, err = ec2conn.AuthorizeSecurityGroupIngress(t.callCtx, &awsec2.AuthorizeSecurityGroupIngressInput{
		GroupId: oldJujuGroup.GroupId,
		IpPermissions: []types.IpPermission{
			{
				IpProtocol: aws.String("udp"),
				FromPort:   aws.Int32(4321),
				ToPort:     aws.Int32(4322),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("3.4.5.6/32")}},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	inst0, _ := testing.AssertStartControllerInstance(c, t.Env, t.callCtx, t.ControllerUUID, "98")
	defer t.Env.StopInstances(t.callCtx, inst0.Id())

	// Create a same-named group for the second instance
	// before starting it, to check that it's reused correctly.
	oldMachineGroup := createGroup(c, ec2conn, t.callCtx, aws.ToString(groups[2].GroupName), "old machine group")

	inst1, _ := testing.AssertStartControllerInstance(c, t.Env, t.callCtx, t.ControllerUUID, "99")
	defer t.Env.StopInstances(t.callCtx, inst1.Id())

	groupNames := make([]string, len(groups))
	for i, g := range groups {
		g := g
		groupNames[i] = aws.ToString(g.GroupName)
	}
	groupsResp, err := ec2conn.DescribeSecurityGroups(t.callCtx, &awsec2.DescribeSecurityGroupsInput{
		GroupNames: groupNames,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groupsResp.SecurityGroups, gc.HasLen, len(groups))

	// For each group, check that it exists and record its id.
	for i, group := range groups {
		found := false
		for _, g := range groupsResp.SecurityGroups {
			if aws.ToString(g.GroupName) == aws.ToString(group.GroupName) {
				groups[i].GroupId = g.GroupId
				info[i] = g
				found = true
				break
			}
		}
		if !found {
			c.Fatalf("group %q not found", aws.ToString(group.GroupName))
		}
	}

	// The old juju group should have been reused.
	c.Check(aws.ToString(groups[0].GroupId), gc.Equals, aws.ToString(oldJujuGroup.GroupId))

	// Check that it authorizes the correct ports and there
	// are no extra permissions (in particular we are checking
	// that the unneeded permission that we added earlier
	// has been deleted).
	perms := info[0].IpPermissions
	c.Assert(perms, gc.HasLen, 5)
	checkPortAllowed(c, perms, 22) // SSH
	checkPortAllowed(c, perms, int32(coretesting.FakeControllerConfig().APIPort()))
	checkSecurityGroupAllowed(c, perms, groups[0])

	// The old machine group should have been reused also.
	c.Check(aws.ToString(groups[2].GroupId), gc.Equals, aws.ToString(oldMachineGroup.GroupId))

	// Check that each instance is part of the correct groups.
	resp, err := ec2conn.DescribeInstances(t.callCtx, &awsec2.DescribeInstancesInput{
		InstanceIds: []string{string(inst0.Id()), string(inst1.Id())},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Reservations, gc.HasLen, 2)
	for _, r := range resp.Reservations {
		c.Assert(r.Instances, gc.HasLen, 1)
		// each instance must be part of the general juju group.
		inst := r.Instances[0]
		msg := gc.Commentf("instance %#v", inst)
		c.Assert(hasSecurityGroup(inst, groups[0]), gc.Equals, true, msg)
		switch instance.Id(aws.ToString(inst.InstanceId)) {
		case inst0.Id():
			c.Assert(hasSecurityGroup(inst, groups[1]), gc.Equals, true, msg)
			c.Assert(hasSecurityGroup(inst, groups[2]), gc.Equals, false, msg)
		case inst1.Id():
			c.Assert(hasSecurityGroup(inst, groups[2]), gc.Equals, true, msg)
			c.Assert(hasSecurityGroup(inst, groups[1]), gc.Equals, false, msg)
		default:
			c.Errorf("unknown instance found: %v", inst)
		}
	}

	// Check that listing those instances finds them using the groups
	instIds := []instance.Id{inst0.Id(), inst1.Id()}
	idsFromInsts := func(insts []instances.Instance) (ids []instance.Id) {
		for _, inst := range insts {
			ids = append(ids, inst.Id())
		}
		return ids
	}
	insts, err := t.Env.Instances(t.callCtx, instIds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instIds, jc.SameContents, idsFromInsts(insts))
	allInsts, err = t.Env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	// ignore the bootstrap instance
	for i, inst := range allInsts {
		if inst.Id() == bootstrapInstId {
			if i+1 < len(allInsts) {
				copy(allInsts[i:], allInsts[i+1:])
			}
			allInsts = allInsts[:len(allInsts)-1]
			break
		}
	}
	c.Assert(instIds, jc.SameContents, idsFromInsts(allInsts))
}

func (t *localServerSuite) TestInstanceGroupsWithAutocert(c *gc.C) {
	// Prepare the controller configuration.
	t.Prepare(c)
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
	}
	err := testing.FillInStartInstanceParams(t.Env, "42", true, &params)
	c.Assert(err, jc.ErrorIsNil)
	config := params.InstanceConfig.Controller.Config
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"

	// Bootstrap the controller.
	result, err := t.Env.StartInstance(t.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	inst := result.Instance
	defer t.Env.StopInstances(t.callCtx, inst.Id())

	// Get security permissions.
	group := ec2.JujuGroupName(t.Env)
	ec2conn := ec2.EnvironEC2Client(t.Env)
	groupsResp, err := ec2conn.DescribeSecurityGroups(t.callCtx, &awsec2.DescribeSecurityGroupsInput{
		GroupNames: []string{group},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groupsResp.SecurityGroups, gc.HasLen, 1)
	perms := groupsResp.SecurityGroups[0].IpPermissions

	// Check that the expected ports are accessible.
	checkPortAllowed(c, perms, 22)
	checkPortAllowed(c, perms, 80)
	checkPortAllowed(c, perms, 443)
}

func checkPortAllowed(c *gc.C, perms []types.IpPermission, port int32) {
	for _, perm := range perms {
		if aws.ToInt32(perm.FromPort) == port {
			c.Check(aws.ToString(perm.IpProtocol), gc.Equals, "tcp")
			c.Check(aws.ToInt32(perm.ToPort), gc.Equals, port)
			c.Check(perm.IpRanges, gc.HasLen, 1)
			c.Check(aws.ToString(perm.IpRanges[0].CidrIp), gc.DeepEquals, "0.0.0.0/0")
			c.Check(perm.UserIdGroupPairs, gc.HasLen, 0)
			return
		}
	}
	c.Errorf("ip port permission not found for %d in %#v", port, perms)
}

func checkSecurityGroupAllowed(c *gc.C, perms []types.IpPermission, g *types.SecurityGroupIdentifier) {
	protos := map[string]struct {
		fromPort int
		toPort   int
	}{
		"tcp":  {0, 65535},
		"udp":  {0, 65535},
		"icmp": {-1, -1},
	}
	for _, perm := range perms {
		if len(perm.UserIdGroupPairs) > 0 {
			c.Check(perm.UserIdGroupPairs, gc.HasLen, 1)
			c.Check(aws.ToString(perm.UserIdGroupPairs[0].GroupId), gc.Equals, aws.ToString(g.GroupId))
			p := aws.ToString(perm.IpProtocol)
			ports, ok := protos[p]
			if !ok {
				c.Errorf("unexpected protocol in security group: %q", p)
				continue
			}
			delete(protos, p)
			c.Check(aws.ToInt32(perm.FromPort), gc.Equals, ports.fromPort)
			c.Check(aws.ToInt32(perm.ToPort), gc.Equals, ports.toPort)
		}
	}
	if len(protos) > 0 {
		c.Errorf("%d security group permission not found for %s in %s", len(protos), pretty.Sprint(g), pretty.Sprint(perms))
	}
}

func (t *localServerSuite) TestStopInstances(c *gc.C) {
	t.Prepare(c)
	inst0, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "40")
	inst1 := ec2.FabricateInstance(inst0, "i-aaaaaaaa")
	inst2, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "41")

	err := t.Env.StopInstances(t.callCtx, inst0.Id(), inst1.Id(), inst2.Id())
	c.Check(err, jc.ErrorIsNil)

	var insts []instances.Instance

	// We need the retry logic here because we are waiting
	// for Instances to return an error, and it will not retry
	// if it succeeds.
	gone := false
	for a := ec2.ShortAttempt.Start(); a.Next(); {
		insts, err = t.Env.Instances(t.callCtx, []instance.Id{inst0.Id(), inst2.Id()})
		if err == environs.ErrPartialInstances {
			// instances not gone yet.
			continue
		}
		if err == environs.ErrNoInstances {
			gone = true
			break
		}
		c.Fatalf("error getting instances: %v", err)
	}
	if !gone {
		c.Errorf("after termination, instances remaining: %v", insts)
	}
}

func (t *localServerSuite) TestPrechecker(c *gc.C) {
	// All implementations of InstancePrechecker should
	// return nil for empty constraints (excluding the
	// manual provider).
	t.Prepare(c)
	err := t.Env.PrecheckInstance(t.ProviderCallContext,
		environs.PrecheckInstanceParams{
			Series: "precise",
		})
	c.Assert(err, jc.ErrorIsNil)
}

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *localServerSuite) TODO_needs_fixing_TestStartStop(c *gc.C) {
	t.prepareAndBootstrap(c)

	inst, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "0")
	c.Assert(inst, gc.NotNil)
	id0 := inst.Id()

	insts, err := t.Env.Instances(t.ProviderCallContext, []instance.Id{id0, id0})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id0)

	// Asserting on the return of AllInstances makes the test fragile,
	// as even comparing the before and after start values can be thrown
	// off if other instances have been created or destroyed in the same
	// time frame. Instead, just check the instance we created exists.
	insts, err = t.Env.AllInstances(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	found := false
	for _, inst := range insts {
		if inst.Id() == id0 {
			c.Assert(found, gc.Equals, false, gc.Commentf("%v", insts))
			found = true
		}
	}
	c.Assert(found, gc.Equals, true, gc.Commentf("expected %v in %v", inst, insts))

	addresses, err := testing.WaitInstanceAddresses(t.Env, t.ProviderCallContext, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	insts, err = t.Env.Instances(t.ProviderCallContext, []instance.Id{id0, ""})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts, gc.HasLen, 2)
	c.Check(insts[0].Id(), gc.Equals, id0)
	c.Check(insts[1], gc.IsNil)

	err = t.Env.StopInstances(t.ProviderCallContext, inst.Id())
	c.Assert(err, jc.ErrorIsNil)

	// The machine may not be marked as shutting down
	// immediately. Repeat a few times to ensure we get the error.
	attempt := utils.AttemptStrategy{
		Total: 15 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	for a := attempt.Start(); a.Next(); {
		insts, err = t.Env.Instances(t.ProviderCallContext, []instance.Id{id0})
		if err != nil {
			break
		}
	}
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(insts, gc.HasLen, 0)
}

func (t *localServerSuite) TestPorts(c *gc.C) {
	t.prepareAndBootstrap(c)

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	c.Assert(inst1, gc.NotNil)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "2")
	c.Assert(inst2, gc.NotNil)
	fwInst2, ok := inst2.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst2.Id()) }()

	// Open some ports and check they're there.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Check there's no crosstalk to another machine
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check that opening the same port again is ok.
	oldRules, err := fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, jc.DeepEquals, oldRules)

	// Check that opening the same port again and another port is ok.
	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	err = fwInst2.ClosePorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can close ports and that there's no crosstalk.
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check that we can close multiple ports.
	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	// Check that we can close ports that aren't there.
	err = fwInst2.ClosePorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("600-700/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check errors when acting on environment.
	fwEnv, ok := t.Env.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)
	err = fwEnv.OpenPorts(t.ProviderCallContext, firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for opening ports on model`)

	err = fwEnv.ClosePorts(t.ProviderCallContext, firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for closing ports on model`)

	_, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for retrieving ingress rules from model`)
}

func (t *localServerSuite) TODO_needs_fixing_TestGlobalPorts(c *gc.C) {
	t.prepareAndBootstrap(c)

	// Change configuration.
	oldConfig := t.Env.Config()
	defer func() {
		err := t.Env.SetConfig(oldConfig)
		c.Assert(err, jc.ErrorIsNil)
	}()

	attrs := t.Env.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	newConfig, err := t.Env.Config().Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	err = t.Env.SetConfig(newConfig)
	c.Assert(err, jc.ErrorIsNil)

	// Create instances and check open ports on both instances.
	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()

	fwEnv, ok := t.Env.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "2")
	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst2.Id()) }()

	err = fwEnv.OpenPorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check closing some ports.
	err = fwEnv.ClosePorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check that we can close ports that aren't there.
	err = fwEnv.ClosePorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("2000-2500/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)
	// Check errors when acting on instances.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for opening ports on instance`)

	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for closing ports on instance`)

	_, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for retrieving ingress rules from instance`)
}

func (t *localServerSuite) TestBootstrapMultiple(c *gc.C) {
	// bootstrap.Bootstrap no longer raises errors if the environment is
	// already up, this has been moved into the bootstrap command.
	t.prepareAndBootstrap(c)

	c.Logf("destroy env")
	err := environs.Destroy(t.Env.Config().Name(), t.Env, t.ProviderCallContext, t.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)
	err = t.Env.Destroy(t.ProviderCallContext) // Again, should work fine and do nothing.
	c.Assert(err, jc.ErrorIsNil)

	// check that we can bootstrap after destroy
	t.prepareAndBootstrap(c)
}

/*
TODO: this should probably be deleted as it only runs on live provider
func (t *localServerSuite) TestBootstrapAndDeploy(c *gc.C) {
	if !t.CanOpenState || !t.HasProvisioner {
		c.Skip(fmt.Sprintf("skipping provisioner test, CanOpenState: %v, HasProvisioner: %v", t.CanOpenState, t.HasProvisioner))
	}
	t.prepareAndBootstrap(c)

	// TODO(niemeyer): Stop growing this kitchen sink test and split it into proper parts.

	c.Logf("opening state")
	st := t.Env.(testing.GetStater).GetStateInAPIServer()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	owner := model.Owner()

	c.Logf("opening API connection")
	controllerCfg, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	caCert, _ := controllerCfg.CACert()
	apiInfo, err := environs.APIInfo(t.ProviderCallContext, model.Tag().Id(), model.Tag().Id(), caCert, controllerCfg.APIPort(), t.Env)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = owner
	apiInfo.Password = "admin-secret"
	apiState, err := api.Open(apiInfo, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	// Check that the agent version has made it through the
	// bootstrap process (it's optional in the config.Config)
	cfg, err := model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := cfg.AgentVersion()
	c.Check(ok, jc.IsTrue)
	c.Check(agentVersion, gc.Equals, jujuversion.Current)

	// Check that the constraints have been set in the environment.
	cons, err := st.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, "mem=2048M")

	// Wait for machine agent to come up on the bootstrap
	// machine and find the deployed series from that.
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)

	instId0, err := m0.InstanceId()
	c.Assert(err, jc.ErrorIsNil)

	// Check that the API connection is working.
	status, err := apiState.Client().Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines["0"].InstanceId, gc.Equals, string(instId0))

	mw0 := newMachineToolWaiter(m0)
	defer func() { _ = mw0.Stop() }()

	// Controllers always run on Ubuntu.
	expectedVersion := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: "ubuntu",
	}

	mtools0 := waitAgentTools(c, mw0, expectedVersion)

	// Create a new application and deploy a unit of it.
	c.Logf("deploying application")
	ch := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	sch, err := testing.PutCharm(st, charm.MustParseURL("local:dummy"), ch)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := st.AddApplication(state.AddApplicationArgs{Name: "dummy", Charm: sch})
	c.Assert(err, jc.ErrorIsNil)
	unit, err := svc.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = st.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the unit's machine and associated agent to come up
	// and announce itself.
	mid1, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m1, err := st.Machine(mid1)
	c.Assert(err, jc.ErrorIsNil)
	mw1 := newMachineToolWaiter(m1)
	defer func() { _ = mw1.Stop() }()
	waitAgentTools(c, mw1, mtools0.Version)

	err = m1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	instId1, err := m1.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	uw := newUnitToolWaiter(unit)
	defer func() { _ = uw.Stop() }()
	utools := waitAgentTools(c, uw, expectedVersion)

	// Check that we can upgrade the environment.
	newVersion := utools.Version
	newVersion.Patch++
	t.checkUpgrade(c, st, newVersion, mw0, mw1, uw)

	// BUG(niemeyer): Logic below is very much wrong. Must be:
	//
	// 1. EnsureDying on the unit and EnsureDying on the machine
	// 2. Unit dies by itself
	// 3. Machine removes dead unit
	// 4. Machine dies by itself
	// 5. Provisioner removes dead machine
	//

	// Now remove the unit and its assigned machine and
	// check that the PA removes it.
	c.Logf("removing unit")
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Wait until unit is dead
	uwatch := unit.Watch()
	defer func() { _ = uwatch.Stop() }()
	for unit.Life() != state.Dead {
		c.Logf("waiting for unit change")
		<-uwatch.Changes()
		err := unit.Refresh()
		c.Logf("refreshed; err %v", err)
		if jujuerrors.IsNotFound(err) {
			c.Logf("unit has been removed")
			break
		}
		c.Assert(err, jc.ErrorIsNil)
	}
	for {
		c.Logf("destroying machine")
		err := m1.Destroy()
		if err == nil {
			break
		}
		c.Assert(err, jc.Satisfies, stateerrors.IsHasAssignedUnitsError)
		time.Sleep(5 * time.Second)
		err = m1.Refresh()
		if jujuerrors.IsNotFound(err) {
			break
		}
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Logf("waiting for instance to be removed")
	t.assertStopInstance(c, instId1)
}
*/

// Check that we get a consistent error when asking for an instance without
// a valid machine config.
func (t *localServerSuite) TestStartInstanceWithEmptyNonceFails(c *gc.C) {
	machineId := "4"
	apiInfo := testing.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(coretesting.ControllerTag, machineId, "", "released", "trusty", apiInfo)
	c.Assert(err, jc.ErrorIsNil)

	t.Prepare(c)

	storageDir := c.MkDir()
	toolsStorage, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	possibleTools := coretools.List(envtesting.AssertUploadFakeToolsVersions(
		c, toolsStorage, "released", "released", version.MustParseBinary("5.4.5-ubuntu-amd64"),
	))
	params := environs.StartInstanceParams{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
		StatusCallback: fakeCallback,
	}
	err = testing.SetImageMetadata(
		t.Env,
		simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory()),
		[]string{"trusty"},
		possibleTools.Arches(),
		&params.ImageMetadata,
	)
	c.Check(err, jc.ErrorIsNil)
	result, err := t.Env.StartInstance(t.ProviderCallContext, params)
	if result != nil && result.Instance != nil {
		err := t.Env.StopInstances(t.ProviderCallContext, result.Instance.Id())
		c.Check(err, jc.ErrorIsNil)
	}
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*missing machine nonce")
}

/*
TODO: this should probably be deleted; it only runs against the live provider
func (t *localServerSuite) TestBootstrapWithDefaultSeries(c *gc.C) {
	if !t.HasProvisioner {
		c.Skip("HasProvisioner is false; cannot test deployment")
	}

	current := coretesting.CurrentVersion(c)
	other := current
	other.Release = "quantal"

	dummyCfg := dummy.SampleConfig().Merge(coretesting.Attrs{
		"controller": false,
		"name":       "dummy storage",
	})
	args := t.prepareForBootstrapParams(c)
	args.ModelConfig = dummyCfg
	e, err := bootstrap.PrepareController(false, t.BootstrapContext,
		jujuclient.NewMemStore(),
		args,
	)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = e.(environs.Environ).Destroy(t.ProviderCallContext) }()

	t.Destroy(c)

	attrs := t.TestConfig.Merge(coretesting.Attrs{
		"name":           "livetests",
		"default-series": "quantal",
	})
	args.ModelConfig = attrs
	env, err := bootstrap.PrepareController(false, t.BootstrapContext,
		t.ControllerStore,
		args)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = environs.Destroy("livetests", env, t.ProviderCallContext, t.ControllerStore) }()

	err = bootstrap.Bootstrap(t.BootstrapContext, env, t.ProviderCallContext, t.bootstrapParams())
	c.Assert(err, jc.ErrorIsNil)

	st := t.Env.(testing.GetStater).GetStateInAPIServer()
	// Wait for machine agent to come up on the bootstrap
	// machine and ensure it deployed the proper series.
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	mw0 := newMachineToolWaiter(m0)
	defer func() { _ = mw0.Stop() }()

	waitAgentTools(c, mw0, other)
}
*/

func (t *localServerSuite) TestIngressRulesWithPartiallyMatchingCIDRs(c *gc.C) {
	t.prepareAndBootstrap(c)

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	c.Assert(inst1, gc.NotNil)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	// Open ports with different CIDRs. Check that rules with same port range
	// get merged.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp")), // open to 0.0.0.0/0
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Open same port with different CIDRs and check that the CIDR gets
	// appended to the existing rule's CIDR list.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24", "192.168.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Close port on a subset of the CIDRs and ensure that that CIDR gets
	// removed from the ingress rules
	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Remove all CIDRs from the rule and check that rules without CIDRs
	// get dropped.
	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
}

// createGroup creates a new EC2 group and returns it. If it already exists,
// it revokes all its permissions and returns the existing group.
func createGroup(c *gc.C, ec2conn ec2.Client, ctx stdcontext.Context, name string, descr string) types.SecurityGroupIdentifier {
	resp, err := ec2conn.CreateSecurityGroup(ctx, &awsec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String(descr),
	})
	if err == nil {
		return types.SecurityGroupIdentifier{
			GroupId:   resp.GroupId,
			GroupName: aws.String(name),
		}
	}
	if err.(smithy.APIError).ErrorCode() != "InvalidGroup.Duplicate" {
		c.Fatalf("cannot make group %q: %v", name, err)
	}

	// Found duplicate group, so revoke its permissions and return it.
	gresp, err := ec2conn.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{
		GroupNames: []string{name},
	})
	c.Assert(err, jc.ErrorIsNil)

	gi := gresp.SecurityGroups[0]
	if len(gi.IpPermissions) > 0 {
		_, err = ec2conn.RevokeSecurityGroupIngress(ctx, &awsec2.RevokeSecurityGroupIngressInput{
			GroupId: gi.GroupId,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	return types.SecurityGroupIdentifier{
		GroupId:   gi.GroupId,
		GroupName: gi.GroupName,
	}
}

func hasSecurityGroup(inst types.Instance, group *types.SecurityGroupIdentifier) bool {
	for _, instGroup := range inst.SecurityGroups {
		if aws.ToString(instGroup.GroupId) == aws.ToString(group.GroupId) {
			return true
		}
	}
	return false
}
