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
	"time"
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
	envs, err := environs.ReadEnvironsBytes([]byte(amazonConfig))
	if err != nil {
		panic(fmt.Errorf("cannot parse amazon tests config data: %v", err))
	}
	for _, name := range envs.Names() {
		Suite(&LiveTests{
			jujutest.LiveTests{
				Environs: envs,
				Name:     name,
				ConsistencyDelay: 5 * time.Second,
			},
		})
	}
}

// LiveTests contains tests that can be run against the Amazon servers.
// Each test runs using the same ec2 connection.
type LiveTests struct {
	jujutest.LiveTests
}

func (t *LiveTests) TestInstanceDNSName(c *C) {
	inst, err := t.Env.StartInstance(30, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst})
	dns, err := inst.WaitDNSName()
	c.Check(err, IsNil)
	c.Check(dns, Not(Equals), "")

	insts, err := t.Env.Instances([]string{inst.Id()})
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 1)

	ec2inst := ec2.InstanceEC2(insts[0])
	c.Check(ec2inst.DNSName, Equals, dns)
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

	// Add two permissions: one is required and should be left alone;
	// the other is not and should be deleted.
	// N.B. this is unfortunately sensitive to the actual set of permissions used.
	_, err := ec2conn.AuthorizeSecurityGroup(oldJujuGroup,
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
	c.Assert(err, IsNil)

	inst0, err := t.Env.StartInstance(98, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst0})

	// Create a same-named group for the second instance
	// before starting it, to check that it's reused correctly.
	oldMachineGroup := createGroup(c, ec2conn, groups[2].Name, "old machine group")

	inst1, err := t.Env.StartInstance(99, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst1})

	groupsResp, err := ec2conn.SecurityGroups(groups, nil)
	c.Assert(err, IsNil)
	c.Assert(groupsResp.Groups, HasLen, len(groups))

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
	c.Check(groups[0].Id, Equals, oldJujuGroup.Id)

	// Check that it authorizes the correct ports and there
	// are no extra permissions (in particular we are checking
	// that the unneeded permission that we added earlier
	// has been deleted).
	perms := info[0].IPPerms
	c.Assert(perms, HasLen, 2)
	checkPortAllowed(c, perms, 22)
	checkPortAllowed(c, perms, 2181)

	// The old machine group should have been reused also.
	c.Check(groups[2].Id, Equals, oldMachineGroup.Id)

	// Check that each instance is part of the correct groups.
	resp, err := ec2conn.Instances([]string{inst0.Id(), inst1.Id()}, nil)
	c.Assert(err, IsNil)
	c.Assert(resp.Reservations, HasLen, 2)
	for _, r := range resp.Reservations {
		c.Assert(r.Instances, HasLen, 1)
		// each instance must be part of the general juju group.
		msg := Commentf("reservation %#v", r)
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
			c.Check(perm.SourceIPs, DeepEquals, []string{"0.0.0.0/0"})
			c.Check(perm.SourceGroups, HasLen, 0)
			return
		}
	}
	c.Errorf("ip port permission not found for %d in %#v", port, perms)
}

func (t *LiveTests) TestStopInstances(c *C) {
	// It would be nice if this test was in jujutest, but
	// there's no way for jujutest to fabricate a valid-looking
	// instance id.
	inst0, err := t.Env.StartInstance(40, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)

	inst1 := ec2.FabricateInstance(inst0, "i-aaaaaaaa")

	inst2, err := t.Env.StartInstance(41, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)

	err = t.Env.StopInstances([]environs.Instance{inst0, inst1, inst2})
	c.Check(err, IsNil)

	var insts []environs.Instance

	gone := false
	for a := ec2.ShortAttempt.Start(); a.Next(); {
		insts, err = t.Env.Instances([]string{inst0.Id(), inst2.Id()})
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
func createGroup(c *C, ec2conn *amzec2.EC2, name, descr string) amzec2.SecurityGroup {
	resp, err := ec2conn.CreateSecurityGroup(name, descr)
	if err == nil {
		return resp.SecurityGroup
	}
	if err.(*amzec2.Error).Code != "InvalidGroup.Duplicate" {
		c.Fatalf("cannot make group %q: %v", name, err)
	}

	// Found duplicate group, so revoke its permissions and return it.
	gresp, err := ec2conn.SecurityGroups(amzec2.SecurityGroupNames(name), nil)
	c.Assert(err, IsNil)

	gi := gresp.Groups[0]
	_, err = ec2conn.RevokeSecurityGroup(gi.SecurityGroup, gi.IPPerms)
	c.Assert(err, IsNil)
	return gi.SecurityGroup
}

func hasSecurityGroup(r amzec2.Reservation, g amzec2.SecurityGroup) bool {
	for _, rg := range r.SecurityGroups {
		if rg.Id == g.Id {
			return true
		}
	}
	return false
}
