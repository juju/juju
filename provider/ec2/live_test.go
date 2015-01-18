// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"

	jc "github.com/juju/testing/checkers"
	amzec2 "gopkg.in/amz.v2/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/testing"
	jujutesting "github.com/juju/juju/juju/testing"
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
		"name":           "sample-" + uniqueName,
		"type":           "ec2",
		"control-bucket": "juju-test-" + uniqueName,
		"admin-secret":   "for real",
		"firewall-mode":  config.FwInstance,
		"agent-version":  version.Current.Number.String(),
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
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	// Upload arches that ec2 supports; add to this
	// as ec2 coverage expands.
	t.UploadArches = []string{arch.AMD64, arch.I386}
	t.BaseSuite.SetUpSuite(c)
	t.LiveTests.SetUpSuite(c)
}

func (t *LiveTests) TearDownSuite(c *gc.C) {
	t.LiveTests.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *LiveTests) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
	t.BaseSuite.PatchValue(&version.Current, version.Binary{
		Number: version.Current.Number,
		Series: coretesting.FakeDefaultSeries,
		Arch:   arch.AMD64,
	})
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.LiveTests.TearDownTest(c)
	t.BaseSuite.TearDownTest(c)
}

// TODO(niemeyer): Looks like many of those tests should be moved to jujutest.LiveTests.

func (t *LiveTests) TestInstanceAttributes(c *gc.C) {
	t.PrepareOnce(c)
	inst, hc := testing.AssertStartInstance(c, t.Env, "30")
	defer t.Env.StopInstances(inst.Id())
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, gc.NotNil)
	c.Assert(hc.Mem, gc.NotNil)
	c.Assert(hc.RootDisk, gc.NotNil)
	c.Assert(hc.CpuCores, gc.NotNil)
	c.Assert(hc.CpuPower, gc.NotNil)
	addresses, err := jujutesting.WaitInstanceAddresses(t.Env, inst.Id())
	// TODO(niemeyer): This assert sometimes fails with "no instances found"
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	insts, err := t.Env.Instances([]instance.Id{inst.Id()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(insts), gc.Equals, 1)

	ec2inst := ec2.InstanceEC2(insts[0])
	c.Assert(ec2inst.IPAddress, gc.Equals, addresses[0].Value)
	c.Assert(ec2inst.InstanceType, gc.Equals, "m1.small")
}

func (t *LiveTests) TestStartInstanceConstraints(c *gc.C) {
	t.PrepareOnce(c)
	cons := constraints.MustParse("mem=2G")
	inst, hc := testing.AssertStartInstanceWithConstraints(c, t.Env, "30", cons)
	defer t.Env.StopInstances(inst.Id())
	ec2inst := ec2.InstanceEC2(inst)
	c.Assert(ec2inst.InstanceType, gc.Equals, "m1.medium")
	c.Assert(*hc.Arch, gc.Equals, "amd64")
	c.Assert(*hc.Mem, gc.Equals, uint64(3840))
	c.Assert(*hc.RootDisk, gc.Equals, uint64(8192))
	c.Assert(*hc.CpuCores, gc.Equals, uint64(1))
	c.Assert(*hc.CpuPower, gc.Equals, uint64(200))
}

func (t *LiveTests) TestInstanceGroups(c *gc.C) {
	t.BootstrapOnce(c)
	allInsts, err := t.Env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInsts, gc.HasLen, 1) // bootstrap instance
	bootstrapInstId := allInsts[0].Id()

	ec2conn := ec2.EnvironEC2(t.Env)

	groups := amzec2.SecurityGroupNames(
		ec2.JujuGroupName(t.Env),
		ec2.MachineGroupName(t.Env, "98"),
		ec2.MachineGroupName(t.Env, "99"),
	)
	info := make([]amzec2.SecurityGroupInfo, len(groups))

	// Create a group with the same name as the juju group
	// but with different permissions, to check that it's deleted
	// and recreated correctly.
	oldJujuGroup := createGroup(c, ec2conn, groups[0].Name, "old juju group")

	// Add two permissions: one is required and should be left alone;
	// the other is not and should be deleted.
	// N.B. this is unfortunately sensitive to the actual set of permissions used.
	_, err = ec2conn.AuthorizeSecurityGroup(oldJujuGroup,
		[]amzec2.IPPerm{
			{
				Protocol:  "tcp",
				FromPort:  22,
				ToPort:    22,
				SourceIPs: []string{"0.0.0.0/0"},
			},
			{
				Protocol:  "udp",
				FromPort:  4321,
				ToPort:    4322,
				SourceIPs: []string{"3.4.5.6/32"},
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	inst0, _ := testing.AssertStartInstance(c, t.Env, "98")
	defer t.Env.StopInstances(inst0.Id())

	// Create a same-named group for the second instance
	// before starting it, to check that it's reused correctly.
	oldMachineGroup := createGroup(c, ec2conn, groups[2].Name, "old machine group")

	inst1, _ := testing.AssertStartInstance(c, t.Env, "99")
	defer t.Env.StopInstances(inst1.Id())

	groupsResp, err := ec2conn.SecurityGroups(groups, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groupsResp.Groups, gc.HasLen, len(groups))

	// For each group, check that it exists and record its id.
	for i, group := range groups {
		found := false
		for _, g := range groupsResp.Groups {
			if g.Name == group.Name {
				groups[i].Id = g.Id
				info[i] = g
				found = true
				break
			}
		}
		if !found {
			c.Fatalf("group %q not found", group.Name)
		}
	}

	// The old juju group should have been reused.
	c.Check(groups[0].Id, gc.Equals, oldJujuGroup.Id)

	// Check that it authorizes the correct ports and there
	// are no extra permissions (in particular we are checking
	// that the unneeded permission that we added earlier
	// has been deleted).
	perms := info[0].IPPerms
	c.Assert(perms, gc.HasLen, 5)
	checkPortAllowed(c, perms, 22) // SSH
	checkPortAllowed(c, perms, coretesting.FakeConfig()["api-port"].(int))
	checkSecurityGroupAllowed(c, perms, groups[0])

	// The old machine group should have been reused also.
	c.Check(groups[2].Id, gc.Equals, oldMachineGroup.Id)

	// Check that each instance is part of the correct groups.
	resp, err := ec2conn.Instances([]string{string(inst0.Id()), string(inst1.Id())}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Reservations, gc.HasLen, 2)
	for _, r := range resp.Reservations {
		c.Assert(r.Instances, gc.HasLen, 1)
		// each instance must be part of the general juju group.
		inst := r.Instances[0]
		msg := gc.Commentf("instance %#v", inst)
		c.Assert(hasSecurityGroup(inst, groups[0]), gc.Equals, true, msg)
		switch instance.Id(inst.InstanceId) {
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
	idsFromInsts := func(insts []instance.Instance) (ids []instance.Id) {
		for _, inst := range insts {
			ids = append(ids, inst.Id())
		}
		return ids
	}
	insts, err := t.Env.Instances(instIds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instIds, jc.SameContents, idsFromInsts(insts))
	allInsts, err = t.Env.AllInstances()
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

func (t *LiveTests) TestDestroy(c *gc.C) {
	t.PrepareOnce(c)
	s := t.Env.(environs.EnvironStorage).Storage()
	err := s.Put("foo", strings.NewReader("foo"), 3)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Put("bar", strings.NewReader("bar"), 3)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the bucket exists, so we can be sure
	// we have checked correctly that it's been destroyed.
	names, err := storage.List(s, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(names) >= 2, jc.IsTrue)

	t.Destroy(c)
	for a := ec2.ShortAttempt.Start(); a.Next(); {
		names, err = storage.List(s, "")
		if len(names) == 0 {
			break
		}
	}
	c.Assert(names, gc.HasLen, 0)
}

func checkPortAllowed(c *gc.C, perms []amzec2.IPPerm, port int) {
	for _, perm := range perms {
		if perm.FromPort == port {
			c.Check(perm.Protocol, gc.Equals, "tcp")
			c.Check(perm.ToPort, gc.Equals, port)
			c.Check(perm.SourceIPs, gc.DeepEquals, []string{"0.0.0.0/0"})
			c.Check(perm.SourceGroups, gc.HasLen, 0)
			return
		}
	}
	c.Errorf("ip port permission not found for %d in %#v", port, perms)
}

func checkSecurityGroupAllowed(c *gc.C, perms []amzec2.IPPerm, g amzec2.SecurityGroup) {
	protos := map[string]struct {
		fromPort int
		toPort   int
	}{
		"tcp":  {0, 65535},
		"udp":  {0, 65535},
		"icmp": {-1, -1},
	}
	for _, perm := range perms {
		if len(perm.SourceGroups) > 0 {
			c.Check(perm.SourceGroups, gc.HasLen, 1)
			c.Check(perm.SourceGroups[0].Id, gc.Equals, g.Id)
			ports, ok := protos[perm.Protocol]
			if !ok {
				c.Errorf("unexpected protocol in security group: %q", perm.Protocol)
				continue
			}
			delete(protos, perm.Protocol)
			c.Check(perm.FromPort, gc.Equals, ports.fromPort)
			c.Check(perm.ToPort, gc.Equals, ports.toPort)
		}
	}
	if len(protos) > 0 {
		c.Errorf("%d security group permission not found for %#v in %#v", len(protos), g, perms)
	}
}

func (t *LiveTests) TestStopInstances(c *gc.C) {
	t.PrepareOnce(c)
	// It would be nice if this test was in jujutest, but
	// there's no way for jujutest to fabricate a valid-looking
	// instance id.
	inst0, _ := testing.AssertStartInstance(c, t.Env, "40")
	inst1 := ec2.FabricateInstance(inst0, "i-aaaaaaaa")
	inst2, _ := testing.AssertStartInstance(c, t.Env, "41")

	err := t.Env.StopInstances(inst0.Id(), inst1.Id(), inst2.Id())
	c.Check(err, jc.ErrorIsNil)

	var insts []instance.Instance

	// We need the retry logic here because we are waiting
	// for Instances to return an error, and it will not retry
	// if it succeeds.
	gone := false
	for a := ec2.ShortAttempt.Start(); a.Next(); {
		insts, err = t.Env.Instances([]instance.Id{inst0.Id(), inst2.Id()})
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

func (t *LiveTests) TestPutBucketOnlyOnce(c *gc.C) {
	t.PrepareOnce(c)
	s3inst := ec2.EnvironS3(t.Env)
	b := s3inst.Bucket("test-once-" + uniqueName)
	s := ec2.BucketStorage(b)

	// Check that we don't do a PutBucket every time by
	// getting it to create the bucket, destroying the bucket behind
	// the scenes, and trying to put another object,
	// which should fail because it doesn't try to do
	// the PutBucket again.

	err := s.Put("test-object", strings.NewReader("test"), 4)
	c.Assert(err, jc.ErrorIsNil)

	err = s.Remove("test-object")
	c.Assert(err, jc.ErrorIsNil)

	err = ec2.DeleteBucket(s)
	c.Assert(err, jc.ErrorIsNil)

	err = s.Put("test-object", strings.NewReader("test"), 4)
	c.Assert(err, gc.ErrorMatches, ".*The specified bucket does not exist")
}

// createGroup creates a new EC2 group and returns it. If it already exists,
// it revokes all its permissions and returns the existing group.
func createGroup(c *gc.C, ec2conn *amzec2.EC2, name, descr string) amzec2.SecurityGroup {
	resp, err := ec2conn.CreateSecurityGroup("", name, descr)
	if err == nil {
		return resp.SecurityGroup
	}
	if err.(*amzec2.Error).Code != "InvalidGroup.Duplicate" {
		c.Fatalf("cannot make group %q: %v", name, err)
	}

	// Found duplicate group, so revoke its permissions and return it.
	gresp, err := ec2conn.SecurityGroups(amzec2.SecurityGroupNames(name), nil)
	c.Assert(err, jc.ErrorIsNil)

	gi := gresp.Groups[0]
	if len(gi.IPPerms) > 0 {
		_, err = ec2conn.RevokeSecurityGroup(gi.SecurityGroup, gi.IPPerms)
		c.Assert(err, jc.ErrorIsNil)
	}
	return gi.SecurityGroup
}

func hasSecurityGroup(inst amzec2.Instance, group amzec2.SecurityGroup) bool {
	for _, instGroup := range inst.SecurityGroups {
		if instGroup.Id == group.Id {
			return true
		}
	}
	return false
}
