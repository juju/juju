// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	stdcontext "context"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/juju/juju/cloud"
	"github.com/juju/os/v2/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/arch"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/ec2"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

// uniqueName is generated afresh for every test run, so that
// we are not polluted by previous test state.
var uniqueName = randomName()

func randomName() string {
	buf := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

func registerAmazonTests() {
	// The following attributes hold the environment configuration
	// for running the amazon EC2 integration tests.
	//
	// This is missing keys for security reasons; set the following
	// environment variables to make the Amazon testing work:
	//  access-key: $AWS_ACCESS_KEY_ID
	//  secret-key: $AWS_SECRET_ACCESS_KEY
	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"name":          "sample-" + uniqueName,
		"type":          "ec2",
		"admin-secret":  "for real",
		"firewall-mode": config.FwInstance,
		"agent-version": coretesting.FakeVersionNumber.String(),
	})
	gc.Suite(&LiveTests{
		LiveTests: jujutest.LiveTests{
			TestConfig:     attrs,
			Attempt:        *ec2.ShortAttempt,
			CanOpenState:   true,
			HasProvisioner: true,
		},
	})
}

// LiveTests contains tests that can be run against the Amazon servers.
// Each test runs using the same ec2 connection.
type LiveTests struct {
	coretesting.BaseSuite
	jujutest.LiveTests

	callCtx context.ProviderCallContext
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	// Upload arches that ec2 supports; add to this
	// as ec2 coverage expands.
	t.UploadArches = []string{arch.AMD64}

	cfg, err := awsconfig.NewEnvConfig()
	if err != nil {
		c.Fatalf("detecting credentials: %v", err)
	}
	accessKeyCredential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": cfg.Credentials.AccessKeyID,
			"secret-key": cfg.Credentials.SecretAccessKey,
		},
	)
	t.Credential = accessKeyCredential
	t.CloudEndpoint = "https://ec2.us-east-1.amazonaws.com"
	t.CloudRegion = "us-east-1"
	t.BaseSuite.SetUpSuite(c)
	t.LiveTests.SetUpSuite(c)
	t.BaseSuite.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	t.BaseSuite.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	t.BaseSuite.PatchValue(&series.HostSeries, func() (string, error) { return version.DefaultSupportedLTS(), nil })
}

func (t *LiveTests) TearDownSuite(c *gc.C) {
	t.LiveTests.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *LiveTests) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)

	t.callCtx = context.NewEmptyCloudCallContext()
	t.LiveTests.BootstrapContext = bootstrapLiveContext(c, liveEnvGetter{t: t})
	t.LiveTests.ProviderCallContext = context.NewCloudCallContext(t.LiveTests.BootstrapContext.Context())
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.LiveTests.TearDownTest(c)
	t.BaseSuite.TearDownTest(c)
}

type liveEnvGetter struct {
	t *LiveTests
}

func (e liveEnvGetter) Env() environs.Environ {
	return e.t.Env
}

// TODO(niemeyer): Looks like many of those tests should be moved to jujutest.LiveTests.

func (t *LiveTests) TestInstanceAttributes(c *gc.C) {
	t.PrepareOnce(c)
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
	c.Assert(ec2inst.InstanceType, gc.Equals, "t3a.micro")
}

func (t *LiveTests) TestStartInstanceConstraints(c *gc.C) {
	t.PrepareOnce(c)
	cons := constraints.MustParse("mem=4G")
	inst, hc := testing.AssertStartInstanceWithConstraints(c, t.Env, t.callCtx, t.ControllerUUID, "30", cons)
	defer t.Env.StopInstances(t.callCtx, inst.Id())
	ec2inst := ec2.InstanceSDKEC2(inst)
	c.Assert(ec2inst.InstanceType, gc.Equals, "t3a.medium")
	c.Assert(*hc.Arch, gc.Equals, "amd64")
	c.Assert(*hc.Mem, gc.Equals, uint64(4*1024))
	c.Assert(*hc.RootDisk, gc.Equals, uint64(8*1024))
	c.Assert(*hc.CpuCores, gc.Equals, uint64(2))
}

func (t *LiveTests) TestControllerInstances(c *gc.C) {
	t.BootstrapOnce(c)
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

func (t *LiveTests) TestInstanceGroups(c *gc.C) {
	t.BootstrapOnce(c)
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

	// Add two permissions: one is required and should be left alone;
	// the other is not and should be deleted.
	// N.B. this is unfortunately sensitive to the actual set of permissions used.
	_, err = ec2conn.AuthorizeSecurityGroupIngress(t.callCtx, &awsec2.AuthorizeSecurityGroupIngressInput{
		GroupId:   oldJujuGroup.GroupId,
		GroupName: oldJujuGroup.GroupName,
		IpPermissions: []types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(22),
				ToPort:     aws.Int32(22),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
			},
			{
				IpProtocol: aws.String("ucp"),
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

func (t *LiveTests) TestInstanceGroupsWithAutocert(c *gc.C) {
	// Prepare the controller configuration.
	t.PrepareOnce(c)
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

func (t *LiveTests) TestStopInstances(c *gc.C) {
	t.PrepareOnce(c)
	// It would be nice if this test was in jujutest, but
	// there's no way for jujutest to fabricate a valid-looking
	// instance id.
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
			GroupId:   gi.GroupId,
			GroupName: gi.GroupName,
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
