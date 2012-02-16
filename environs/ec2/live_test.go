package ec2_test

import (
	"crypto/rand"
	"fmt"
	"io"
	amzec2 "launchpad.net/goamz/ec2"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/ec2"
	"launchpad.net/juju/go/environs/jujutest"
	"strings"
)

// amazonConfig holds the environments configuration
// for running the amazon EC2 integration tests.
//
// This is missing keys for security reasons; set the following environment variables
// to make the Amazon testing work:
//  access-key: $AWS_ACCESS_KEY_ID
//  admin-secret: $AWS_SECRET_ACCESS_KEY
var amazonConfig = fmt.Sprintf(`
environments:
  sample-%s:
    type: ec2
    control-bucket: 'juju-test-%s'
`, uniqueName, uniqueName)

// uniqueName is generated afresh for every test, so that
// we are no polluted by previous test state.
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
	envs, err := environs.ReadEnvironsBytes([]byte(amazonConfig))
	if err != nil {
		panic(fmt.Errorf("cannot parse amazon tests config data: %v", err))
	}
	for _, name := range envs.Names() {
		Suite(&LiveTests{
			jujutest.LiveTests{
				Environs: envs,
				Name:     name,
			},
		})
	}
}

type LiveTests struct {
	jujutest.LiveTests
}

func (t *LiveTests) TestInstanceGroups(c *C) {
	ec2conn := ec2.EnvironEC2(t.Env)

	groups := amzec2.SecurityGroupNames(
		ec2.GroupName(t.Env),
		ec2.MachineGroupName(t.Env, 98),
		ec2.MachineGroupName(t.Env, 99),
	)
	info := make([]amzec2.SecurityGroupInfo, len(groups))

	c.Logf("start instance 98")
	inst0, err := t.Env.StartInstance(98, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)

	c.Logf("ensure old group exists")
	// create a same-named group for the second instance
	// before starting it, to check that it's deleted and
	// recreated correctly.
	oldGroup := ensureGroupExists(c, ec2conn, groups[2].Name, "old group")

	c.Logf("start instance 99")
	inst1, err := t.Env.StartInstance(99, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)

	// go behind the scenes to check the machines have
	// been put into the correct groups.

	// first check that the old group has been deleted.
	c.Logf("checking old group was deleted")
	f := amzec2.NewFilter()
	f.Add("group-name", oldGroup.Name)
	f.Add("group-id", oldGroup.Id)
	groupsResp, err := ec2conn.SecurityGroups(nil, f)
	c.Assert(err, IsNil)
	c.Check(len(groupsResp.Groups), Equals, 0)

	// then check that the groups have been created.
	c.Logf("checking new groups were deleted")
	groupsResp, err = ec2conn.SecurityGroups(groups, nil)
	c.Assert(err, IsNil)
	c.Assert(len(groupsResp.Groups), Equals, len(groups))

	// for each group, check that it exists and record its id.
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

	perms := info[0].IPPerms

	// check that the juju group authorizes SSH for anyone.
	c.Assert(len(perms), Equals, 2, Bug("got security groups %#v", perms))
	checkPortAllowed(c, perms, 22)
	checkPortAllowed(c, perms, 2181)

	// check that each instance is part of the correct groups.
	c.Logf("checking that each insance is part of the correct groups")
	resp, err := ec2conn.Instances([]string{inst0.Id(), inst1.Id()}, nil)
	c.Assert(err, IsNil)
	c.Assert(len(resp.Reservations), Equals, 2, Bug("reservations %#v", resp.Reservations))
	for _, r := range resp.Reservations {
		c.Assert(len(r.Instances), Equals, 1)
		// each instance must be part of the general juju group.
		msg := Bug("reservation %#v", r)
		c.Assert(hasSecurityGroup(r, groups[0]), Equals, true, msg)
		inst := r.Instances[0]
		switch inst.InstanceId {
		case inst0.Id():
			c.Assert(hasSecurityGroup(r, groups[1]), Equals, true, msg)
			c.Assert(hasSecurityGroup(r, groups[2]), Equals, false, msg)
		case inst1.Id():
			c.Assert(hasSecurityGroup(r, groups[2]), Equals, true, msg)

			// check that the id of the second machine's group
			// has changed - this implies that StartInstance has
			// correctly deleted and re-created the group.
			c.Assert(groups[2].Id, Not(Equals), oldGroup.Id)
			c.Assert(hasSecurityGroup(r, groups[1]), Equals, false, msg)
		default:
			c.Errorf("unknown instance found: %v", inst)
		}
	}

	// create a third instance to check that StopInstances doesn't
	// destroy any extra groups.
	_, err = t.Env.StartInstance(100, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	group2 := amzec2.SecurityGroup{Name: ec2.MachineGroupName(t.Env, 100)}

	err = t.Env.StopInstances([]environs.Instance{inst0, inst1})
	c.Assert(err, IsNil)

	// check that StopInstances has deleted the machine groups.
	f = amzec2.NewFilter()
	f.Add("group-name", groups[1].Name, groups[2].Name, 	group2.Name)

	groupsResp, err = ec2conn.SecurityGroups(nil, f)
	c.Assert(err, IsNil)
	c.Assert(len(groupsResp.Groups), Equals, 1)
	c.Check(groupsResp.Groups[0].Name, Equals, group2.Name)

	// create an extra group to check that Destroy will destroy
	// groups not attached to machines.
	group3 := ensureGroupExists(c, ec2conn, ec2.MachineGroupName(t.Env, 101), "test group 3")

	c.Log("calling destroy")
	err = t.Env.Destroy(nil)
	c.Assert(err, IsNil)

	// check that Destroy has deleted all groups.
	f = amzec2.NewFilter()
	f.Add("group-name", groups[0].Name, groups[1].Name, groups[2].Name, group2.Name, group3.Name)
	groupsResp, err = ec2conn.SecurityGroups(nil, f)
	c.Assert(err, IsNil)
	c.Assert(len(groupsResp.Groups), Equals, 0, Bug("remaining groups: %v", groupsResp.Groups))
}

func checkPortAllowed(c *C, perms []amzec2.IPPerm, port int) {
	for _, perm := range perms {
		if perm.FromPort == port {
			c.Check(perm.Protocol, Equals, "tcp")
			c.Check(perm.ToPort, Equals, port)
			c.Check(perm.SourceIPs, Equals, []string{"0.0.0.0/0"})
			c.Check(len(perm.SourceGroups), Equals, 0)
			return
		}
	}
	c.Errorf("ip port permission not found for %d in %#v", port, perms)
}

// ensureGroupExists creates a new EC2 group if it doesn't already
// exist, and returns full SecurityGroup.
func ensureGroupExists(c *C, ec2conn *amzec2.EC2, groupName string, descr string) amzec2.SecurityGroup {
	f := amzec2.NewFilter()
	f.Add("group-name", groupName)
	groups, err := ec2conn.SecurityGroups(nil, f)
	c.Assert(err, IsNil)
	if len(groups.Groups) > 0 {
		return groups.Groups[0].SecurityGroup
	}

	resp, err := ec2conn.CreateSecurityGroup(groupName, descr)
	c.Assert(err, IsNil)

	return resp.SecurityGroup
}

func (t *LiveTests) TestBootstrap(c *C) {
	t.LiveTests.TestBootstrap(c)
	// After the bootstrap test, the environment should have been
	// destroyed. Verify that the security groups have been deleted too.
	ec2conn := ec2.EnvironEC2(t.Env)
	resp, err := ec2conn.SecurityGroups(nil, nil)
	c.Assert(err, IsNil)
	prefix := ec2.GroupName(t.Env)
	for _, g := range resp.Groups {
		if strings.HasPrefix(g.Name, prefix) {
			c.Errorf("group %q has not been deleted", g.SecurityGroup)
		}
	}
}

func hasSecurityGroup(r amzec2.Reservation, g amzec2.SecurityGroup) bool {
	for _, rg := range r.SecurityGroups {
		if rg.Id == g.Id {
			return true
		}
	}
	return false
}
