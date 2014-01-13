package replicaset

import (
	"fmt"
	"testing"
	"time"

	gc "launchpad.net/gocheck"

	"labix.org/v2/mgo"

	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

var (
	name = "juju"
	root *coretesting.MgoInstance
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func newServer() (*coretesting.MgoInstance, error) {
	inst := &coretesting.MgoInstance{Params: []string{"--replSet", name}}

	err := inst.Start(true)
	if err != nil {
		return nil, fmt.Errorf("Error starting mongo server: %s", err.Error())
	}

	// by dialing right now, we'll wait until it's running
	strategy := utils.AttemptStrategy{Total: time.Second * 5, Delay: time.Millisecond * 100}
	attempt := strategy.Start()
	for attempt.Next() {
		var session *mgo.Session
		session, err = inst.DialDirect()
		if err != nil {
			err = fmt.Errorf("Error dialing mongo server %q: %s", inst.Addr(), err.Error())
		} else {
			session.SetMode(mgo.Monotonic, true)
			err = session.Ping()
			if err != nil {
				err = fmt.Errorf("Error pinging mongo server %q: %s", inst.Addr(), err.Error())
			}
			session.Close()
		}
		if err == nil || !attempt.HasNext() {
			break
		}
	}
	return inst, err
}

type MongoSuite struct{}

var _ = gc.Suite(&MongoSuite{})

func (s *MongoSuite) SetUpSuite(c *gc.C) {
	var err error
	// do all this stuff here, since we don't want to have to redo it for each test
	root, err = newServer()
	if err != nil {
		c.Fatalf("Got error from Start of root server: %s", err.Error())
	}
	// note, this is an actual test around Initiate, but again, I don't want to
	// have to redo it, so I just do it once.
	dialAndTestInitiate(c)
}

func (s *MongoSuite) TearDownTest(c *gc.C) {
	// remove all secondaries from the replicaset on test teardown
	session, err := root.DialDirect()
	if err != nil {
		c.Logf("Failed to dial root during test cleanup: %v", err)
		return
	}
	defer session.Close()
	mems, err := CurrentMembers(session)
	if err != nil {
		c.Logf("Failed to get list of memners during test cleanup: %v", err)
		return
	}

	addrs := []string{}
	for _, m := range mems {
		if root.Addr() != m.Address {
			addrs = append(addrs, m.Address)
		}
	}
	if err = Remove(session, addrs...); err != nil {
		c.Logf("Error removing secondaries: %v", err)
	}
}

func dialAndTestInitiate(c *gc.C) {
	session := root.MustDialDirect()
	defer session.Close()

	err := Initiate(session, root.Addr(), name)
	c.Assert(err, gc.IsNil)

	// Ids start at 1 for us, so we can differentiate between set and unset
	expectedMembers := []Member{Member{Id: 1, Address: root.Addr()}}

	// need to set mode to strong so that we wait for the write to succeed
	// before reading and thus ensure that we're getting consistent reads.
	session.SetMode(mgo.Strong, false)

	mems, err := CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)

	// now add some data so we get a more real-life test
	loadData(session, c)
}

func loadData(session *mgo.Session, c *gc.C) {
	type foo struct {
		Name    string
		Address string
		Count   int
	}

	for col := 0; col < 10; col++ {
		foos := make([]foo, 10000)
		for n := range foos {
			foos[n] = foo{
				Name:    fmt.Sprintf("name_%d_%d", col, n),
				Address: fmt.Sprintf("address_%d_%d", col, n),
				Count:   n * (col + 1),
			}
		}

		err := session.DB("testing").C(fmt.Sprintf("data%d", col)).Insert(foos)
		c.Assert(err, gc.IsNil)
	}
}

func (s *MongoSuite) TearDownSuite(c *gc.C) {
	root.Destroy()
}

func (s *MongoSuite) TestAddRemoveSet(c *gc.C) {
	session := root.MustDial()
	defer session.Close()

	members := make([]Member, 0, 5)

	// Add should be idempotent, so re-adding root here shouldn't result in
	// two copies of root in the replica set
	members = append(members, Member{Address: root.Addr()})

	instances := make([]*coretesting.MgoInstance, 0, 5)
	instances = append(instances, root)

	for x := 0; x < 4; x++ {
		inst, err := newServer()
		c.Assert(err, gc.IsNil)
		instances = append(instances, inst)
		defer inst.Destroy()
		defer Remove(session, inst.Addr())

		key := fmt.Sprintf("key%d", x)
		val := fmt.Sprintf("val%d", x)

		tags := map[string]string{key: val}

		members = append(members, Member{Address: inst.Addr(), Tags: tags})
	}

	var err error

	strategy := utils.AttemptStrategy{Total: time.Second * 30, Delay: time.Millisecond * 100}
	attempt := strategy.Start()
	for attempt.Next() {
		err = Add(session, members...)
		if err == nil || !attempt.HasNext() {
			break
		}
	}
	c.Assert(err, gc.IsNil)

	expectedMembers := make([]Member, len(members))
	for x, m := range members {
		// Ids should start at 1 (for the root) and go up
		m.Id = x + 1
		expectedMembers[x] = m
	}

	var cfg *Config
	attempt = strategy.Start()
	for attempt.Next() {
		cfg, err = CurrentConfig(session)
		if err == nil || !attempt.HasNext() {
			break
		}
	}

	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name, gc.Equals, name)

	// 2 since we already changed it once
	c.Assert(cfg.Version, gc.Equals, 2)

	mems := cfg.Members

	c.Assert(mems, gc.DeepEquals, expectedMembers)

	// Now remove the last two Members
	attempt = strategy.Start()
	for attempt.Next() {
		err = Remove(session, members[3].Address, members[4].Address)
		if err == nil || !attempt.HasNext() {
			break
		}
	}
	c.Assert(err, gc.IsNil)

	expectedMembers = expectedMembers[0:3]

	mems, err = CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)

	// now let's mix it up and set the new members to a mix of the previous
	// plus the new arbiter
	mems = []Member{members[3], mems[2], mems[0], members[4]}

	attempt = strategy.Start()
	for attempt.Next() {
		err = Set(session, mems)
		if err == nil || !attempt.HasNext() {
			break
		}
	}

	c.Assert(err, gc.IsNil)

	attempt = strategy.Start()
	for attempt.Next() {
		// can dial whichever replica address here, mongo will figure it out
		session = instances[0].MustDialDirect()
		err = session.Ping()
		if err == nil || !attempt.HasNext() {
			break
		}
	}
	c.Assert(err, gc.IsNil)

	expectedMembers = []Member{members[3], expectedMembers[2], expectedMembers[0], members[4]}

	// any new members will get an id of max(other_ids...)+1
	expectedMembers[0].Id = 4
	expectedMembers[3].Id = 5

	attempt = strategy.Start()
	for attempt.Next() {
		mems, err = CurrentMembers(session)
		if err == nil || !attempt.HasNext() {
			break
		}
	}
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)
}

func (s *MongoSuite) TestIsMaster(c *gc.C) {
	session := root.MustDial()
	defer session.Close()

	expected := IsMasterResults{
		// The following fields hold information about the specific mongodb node.
		IsMaster:  true,
		Secondary: false,
		Arbiter:   false,
		Address:   root.Addr(),
		LocalTime: time.Time{},

		// The following fields hold information about the replica set.
		ReplicaSetName: name,
		Addresses:      []string{root.Addr()},
		Arbiters:       nil,
		PrimaryAddress: root.Addr(),
	}

	res, err := IsMaster(session)
	c.Assert(err, gc.IsNil)
	c.Check(closeEnough(res.LocalTime, time.Now()), gc.Equals, true)
	res.LocalTime = time.Time{}
	c.Check(*res, gc.DeepEquals, expected)
}

func (s *MongoSuite) TestCurrentStatus(c *gc.C) {
	session := root.MustDial()
	defer session.Close()

	inst1, err := newServer()
	c.Assert(err, gc.IsNil)
	defer inst1.Destroy()
	defer Remove(session, inst1.Addr())

	inst2, err := newServer()
	c.Assert(err, gc.IsNil)
	defer inst2.Destroy()
	defer Remove(session, inst2.Addr())

	strategy := utils.AttemptStrategy{Total: time.Second * 30, Delay: time.Millisecond * 100}
	attempt := strategy.Start()
	for attempt.Next() {
		err = Add(session, Member{Address: inst1.Addr()}, Member{Address: inst2.Addr()})
		if err == nil || !attempt.HasNext() {
			break
		}
	}
	c.Assert(err, gc.IsNil)

	expected := &Status{
		Name: name,
		Members: []MemberStatus{{
			Id:      1,
			Address: root.Addr(),
			Self:    true,
			ErrMsg:  "",
			Healthy: true,
			State:   PrimaryState,
		}, {
			Id:      2,
			Address: inst1.Addr(),
			Self:    false,
			ErrMsg:  "",
			Healthy: true,
			State:   SecondaryState,
		}, {
			Id:      3,
			Address: inst2.Addr(),
			Self:    false,
			ErrMsg:  "",
			Healthy: true,
			State:   SecondaryState,
		}},
	}

	strategy.Total = time.Second * 60
	attempt = strategy.Start()
	var res *Status
	for attempt.Next() {
		var err error
		res, err = CurrentStatus(session)

		if err != nil && !attempt.HasNext() {
			c.Errorf("Couldn't get status before timeout, got err: %v", err)
			return
		}

		if res.Members[0].State == PrimaryState &&
			res.Members[1].State == SecondaryState &&
			res.Members[2].State == SecondaryState {
			break
		}
		if !attempt.HasNext() {
			c.Errorf("Servers did not get into final state before timeout.  Status: %#v", res)
			return
		}
	}

	for x, _ := range res.Members {
		// non-empty uptime and ping
		c.Check(res.Members[x].Uptime, gc.Not(gc.Equals), 0)

		// ping is always going to be zero since we're on localhost
		// so we can't really test it right now

		// now overwrite Uptime so it won't throw off DeepEquals
		res.Members[x].Uptime = 0
	}
	c.Check(res, gc.DeepEquals, expected)
}

func closeEnough(expected, obtained time.Time) bool {
	t := obtained.Sub(expected)
	return (-500*time.Millisecond) < t && t < (500*time.Millisecond)
}
