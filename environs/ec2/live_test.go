package ec2_test

import (
	"crypto/rand"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	amzec2 "launchpad.net/goamz/ec2"
	"launchpad.net/juju/go/environs/ec2"
	"launchpad.net/juju/go/environs/jujutest"
)

// integrationConfig holds the environments configuration
// for running the amazon EC2 integration tests.
//
// This is missing keys for security reasons; set the following environment variables
// to make the integration testing work:
//  access-key: $AWS_ACCESS_KEY_ID
//  admin-secret: $AWS_SECRET_ACCESS_KEY
var integrationConfig = `
environments:
  sample:
    type: ec2
    control-bucket: '%s'
`

func registerIntegrationTests() {
	cfg := fmt.Sprintf(integrationConfig, bucketName)
	envs, err := environs.ReadEnvironsBytes([]byte(cfg))
	if err != nil {
		panic(fmt.Errorf("cannot parse integration tests config data: %v", err))
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

func bucketName() string {
	buf := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("juju-test-%x", buf)
}

type LiveTests struct {
	jujutest.LiveTests
}


func (t *LiveTests) TestInstanceGroups(c *C) {
	ec2conn := ec2.EnvironEC2(t.Env)

	groups := amzec2.SecurityGroupNames(
		fmt.Sprintf("juju-%s", t.Name),
		fmt.Sprintf("juju-%s-%d", t.Name, 98),
		fmt.Sprintf("juju-%s-%d", t.Name, 99),
	)
	info := make([]amzec2.SecurityGroupInfo, len(groups))

	inst0, err := t.Env.StartInstance(98, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst0})

	// create a same-named group for the second instance
	// before starting it, to check that it's deleted and
	// recreated correctly.
	oldGroup := ensureGroupExists(c, ec2conn, groups[2], "old group")

	inst1, err := t.Env.StartInstance(99, jujutest.InvalidStateInfo)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst1})

	// go behind the scenes to check the machines have
	// been put into the correct groups.

	// first check that the old group has been deleted
	groupsResp, err := ec2conn.SecurityGroups([]amzec2.SecurityGroup{oldGroup}, nil)
	c.Assert(err, IsNil)
	c.Check(len(groupsResp.Groups), Equals, 0)

	// then check that the groups have been created.
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
}
