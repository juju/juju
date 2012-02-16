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

	// Create a group with the same name as the juju group
	// but with different permissions, to check that it's deleted
	// and recreated correctly.
	oldJujuGroup := createGroup(c, ec2conn, groups[0].Name, "old juju group")

	inst0, err := t.Env.StartInstance(98, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst0})

	// Create a same-named group for the second instance
	// before starting it, to check that it's deleted and
	// recreated correctly.
	oldMachineGroup := createGroup(c, ec2conn, groups[2].Name, "old machine group")

	inst1, err := t.Env.StartInstance(99, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst1})

	groupsResp, err := ec2conn.SecurityGroups(groups, nil)
	c.Assert(err, IsNil)
	c.Assert(len(groupsResp.Groups), Equals, len(groups))

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

	// If the id of the juju group has changed, it implies that
	// the old juju group has correctly been deleted and recreated
	// because it had different permissions.
	c.Check(groups[0].Id, Not(Equals), oldJujuGroup.Id)

	// The old machine group should have been reused because
	// it had the same permissions.
	c.Check(groups[2].Id, Equals, oldMachineGroup.Id)

	// Check that the juju group authorizes SSH for anyone.
	perms := info[0].IPPerms
	c.Assert(len(perms), Equals, 2, Bug("got security groups %#v", perms))
	checkPortAllowed(c, perms, 22)
	checkPortAllowed(c, perms, 2181)

	// Check that each instance is part of the correct groups.
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
			c.Assert(hasSecurityGroup(r, groups[1]), Equals, false, msg)
		default:
			c.Errorf("unknown instance found: %v", inst)
		}
	}
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

// createGroup creates a new EC2 group and returns it. If it already exists,
// it deletes the old one first.
func createGroup(c *C, ec2conn *amzec2.EC2, groupName string, descr string) amzec2.SecurityGroup {
	resp, err := ec2conn.CreateSecurityGroup(groupName, descr)
	if err != nil {
		if ec2err, _ := err.(*amzec2.Error); ec2err == nil || ec2err.Code != "InvalidGroup.Duplicate" {
			c.Fatalf("cannot make group %q: %v", groupName, err)
		}
	}
	_, err = ec2conn.DeleteSecurityGroup(amzec2.SecurityGroup{Name: groupName})
	c.Assert(err, IsNil)

	resp, err = ec2conn.CreateSecurityGroup(groupName, descr)
	c.Assert(err, IsNil)

	return resp.SecurityGroup
}

func hasSecurityGroup(r amzec2.Reservation, g amzec2.SecurityGroup) bool {
	for _, rg := range r.SecurityGroups {
		if rg.Id == g.Id {
			return true
		}
	}
	return false
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
