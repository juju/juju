package ec2_test

import (
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	amzec2 "launchpad.net/goamz/ec2"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"strings"
)

// amazonConfig holds the environments configuration
// for running the amazon EC2 integration tests.
//
// This is missing keys for security reasons; set the following environment variables
// to make the Amazon testing work:
//  access-key: $AWS_ACCESS_KEY_ID
//  secret-key: $AWS_SECRET_ACCESS_KEY
var amazonConfig = fmt.Sprintf(`
environments:
  sample-%s:
    type: ec2
    control-bucket: 'juju-test-%s'
    public-bucket: 'juju-public-test-%s'
    admin-secret: 'for real'
`, uniqueName, uniqueName, uniqueName)

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
			LiveTests: jujutest.LiveTests{
				Environs:         envs,
				Name:             name,
				Attempt:          *ec2.ShortAttempt,
				CanOpenState:     true,
				HasProvisioner:   true,
				ServerCertAndKey: []byte("fake cert"), // TODO
			},
		})
	}
}

// LiveTests contains tests that can be run against the Amazon servers.
// Each test runs using the same ec2 connection.
type LiveTests struct {
	coretesting.LoggingSuite
	jujutest.LiveTests
}

func (t *LiveTests) SetUpSuite(c *C) {
	t.LoggingSuite.SetUpSuite(c)
	e, err := t.Environs.Open("")
	c.Assert(err, IsNil)
	// Put some fake tools in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running will find them in the public bucket.
	putFakeTools(c, e.PublicStorage().(environs.Storage))
	t.LiveTests.SetUpSuite(c)
}

func (t *LiveTests) TearDownSuite(c *C) {
	if t.Env == nil {
		// This can happen if SetUpSuite fails.
		return
	}
	err := ec2.DeleteStorageContent(t.Env.PublicStorage().(environs.Storage))
	c.Assert(err, IsNil)
	t.LiveTests.TearDownSuite(c)
	t.LoggingSuite.TearDownSuite(c)
}

func (t *LiveTests) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
}

func (t *LiveTests) TearDownTest(c *C) {
	t.LiveTests.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}

// TODO(niemeyer): Looks like many of those tests should be moved to jujutest.LiveTests.

func (t *LiveTests) TestInstanceDNSName(c *C) {
	inst, err := t.Env.StartInstance(30, testing.InvalidStateInfo(30), nil)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst})
	dns, err := inst.WaitDNSName()
	// TODO(niemeyer): This assert sometimes fails with "no instances found"
	c.Assert(err, IsNil)
	c.Assert(dns, Not(Equals), "")

	insts, err := t.Env.Instances([]string{inst.Id()})
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 1)

	ec2inst := ec2.InstanceEC2(insts[0])
	c.Assert(ec2inst.DNSName, Equals, dns)
}

func (t *LiveTests) TestInstanceGroups(c *C) {
	ec2conn := ec2.EnvironEC2(t.Env)

	groups := amzec2.SecurityGroupNames(
		ec2.JujuGroupName(t.Env),
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

	inst0, err := t.Env.StartInstance(98, testing.InvalidStateInfo(98), nil)
	c.Assert(err, IsNil)
	defer t.Env.StopInstances([]environs.Instance{inst0})

	// Create a same-named group for the second instance
	// before starting it, to check that it's reused correctly.
	oldMachineGroup := createGroup(c, ec2conn, groups[2].Name, "old machine group")

	inst1, err := t.Env.StartInstance(99, testing.InvalidStateInfo(99), nil)
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
	c.Assert(perms, HasLen, 4)
	checkPortAllowed(c, perms, 22)
	checkSecurityGroupAllowed(c, perms, groups[0])

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

func (t *LiveTests) TestDestroy(c *C) {
	s := t.Env.Storage()
	err := s.Put("foo", strings.NewReader("foo"), 3)
	c.Assert(err, IsNil)
	err = s.Put("bar", strings.NewReader("bar"), 3)
	c.Assert(err, IsNil)

	// Check that the bucket exists, so we can be sure
	// we have checked correctly that it's been destroyed.
	names, err := s.List("")
	c.Assert(err, IsNil)
	c.Assert(len(names) >= 2, Equals, true)

	t.Destroy(c)
	for a := ec2.ShortAttempt.Start(); a.Next(); {
		names, err = s.List("")
		if len(names) == 0 {
			break
		}
	}
	c.Assert(names, HasLen, 0)
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

func checkSecurityGroupAllowed(c *C, perms []amzec2.IPPerm, g amzec2.SecurityGroup) {
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
			c.Check(perm.SourceGroups, HasLen, 1)
			c.Check(perm.SourceGroups[0].Id, Equals, g.Id)
			ports, ok := protos[perm.Protocol]
			if !ok {
				c.Errorf("unexpected protocol in security group: %q", perm.Protocol)
				continue
			}
			delete(protos, perm.Protocol)
			c.Check(perm.FromPort, Equals, ports.fromPort)
			c.Check(perm.ToPort, Equals, ports.toPort)
		}
	}
	if len(protos) > 0 {
		c.Errorf("%d security group permission not found for %#v in %#v", len(protos), g, perms)
	}
}

func (t *LiveTests) TestStopInstances(c *C) {
	// It would be nice if this test was in jujutest, but
	// there's no way for jujutest to fabricate a valid-looking
	// instance id.
	inst0, err := t.Env.StartInstance(40, testing.InvalidStateInfo(40), nil)
	c.Assert(err, IsNil)

	inst1 := ec2.FabricateInstance(inst0, "i-aaaaaaaa")

	inst2, err := t.Env.StartInstance(41, testing.InvalidStateInfo(41), nil)
	c.Assert(err, IsNil)

	err = t.Env.StopInstances([]environs.Instance{inst0, inst1, inst2})
	c.Check(err, IsNil)

	var insts []environs.Instance

	// We need the retry logic here because we are waiting
	// for Instances to return an error, and it will not retry
	// if it succeeds.
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

func (t *LiveTests) TestPublicStorage(c *C) {
	s := t.Env.PublicStorage().(environs.Storage)

	contents := "test"
	err := s.Put("test-object", strings.NewReader(contents), int64(len(contents)))
	c.Assert(err, IsNil)

	r, err := s.Get("test-object")
	c.Assert(err, IsNil)
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, contents)

	// check that the public storage isn't aliased to the private storage.
	r, err = t.Env.Storage().Get("test-object")
	var notFoundError *environs.NotFoundError
	c.Assert(err, FitsTypeOf, notFoundError)
}

func (t *LiveTests) TestPutBucketOnlyOnce(c *C) {
	s3inst := ec2.EnvironS3(t.Env)
	b := s3inst.Bucket("test-once-" + uniqueName)
	s := ec2.BucketStorage(b)

	// Check that we don't do a PutBucket every time by
	// getting it to create the bucket, destroying the bucket behind
	// the scenes, and trying to put another object,
	// which should fail because it doesn't try to do
	// the PutBucket again.

	err := s.Put("test-object", strings.NewReader("test"), 4)
	c.Assert(err, IsNil)

	err = s.Remove("test-object")
	c.Assert(err, IsNil)

	err = b.DelBucket()
	c.Assert(err, IsNil)

	err = s.Put("test-object", strings.NewReader("test"), 4)
	c.Assert(err, ErrorMatches, ".*The specified bucket does not exist")
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
	if len(gi.IPPerms) > 0 {
		_, err = ec2conn.RevokeSecurityGroup(gi.SecurityGroup, gi.IPPerms)
		c.Assert(err, IsNil)
	}
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
