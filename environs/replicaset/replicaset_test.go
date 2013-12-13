package replicaset

import (
	"fmt"
	"testing"
	"time"

	gc "launchpad.net/gocheck"

	"labix.org/v2/mgo"

	coretesting "launchpad.net/juju-core/testing"
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
	err := inst.Start()
	if err != nil {
		return nil, err
	}

	// by dialing right now, we'll wait until it's running
	deadline := time.Now().Add(time.Second * 5)
	for {
		session := inst.DialDirect()
		session.SetMode(mgo.Monotonic, true)
		err := session.Ping()
		session.Close()
		if err == nil || time.Now().After(deadline) {
			return inst, err
		}
	}
}

type MongoSuite struct{}

var _ = gc.Suite(&MongoSuite{})

func (s *MongoSuite) SetUpSuite(c *gc.C) {
	var err error
	// do all this stuff here, since we don't want to have to redo it for each test
	root, err = newServer()
	if err != nil {
		c.Fatalf("Got non-nil error from Start of root server: %q", err.Error())
	}
	// note, this is an actual test around Initiate, but again, I don't want to
	// have to redo it, so I just do it once.
	dialAndTestInitiate(c)
}

func dialAndTestInitiate(c *gc.C) {
	session := root.DialDirect()
	defer session.Close()

	err := Initiate(session, root.Addr(), name)
	c.Assert(err, gc.IsNil)

	expectedMembers := []Member{Member{Address: root.Addr()}}

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
	session := root.Dial()
	defer session.Close()

	expectedStatus := []Status{
		{
			Address: root.Addr(),
			Self:    true,
			ErrMsg:  "",
			Healthy: true,
			State:   StartupState,
		},
	}

	status, err := CurrentStatus(session)
	expectedStatus[0].Uptime = status[0].Uptime
	expectedStatus[0].Ping = status[0].Ping
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.DeepEquals, expectedStatus)

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
		members = append(members, Member{Address: inst.Addr(), Id: x + 1})
	}

	err = Add(session, members...)
	c.Assert(err, gc.IsNil)

	mems, err := CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, members)

	// Now remove the last two Members
	err = Remove(session, members[3].Address, members[4].Address)
	c.Assert(err, gc.IsNil)

	expectedMembers := members[0:3]

	mems, err = CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)

	// now let's mix it up and set the new members to a mix of the previous
	// plus the new arbiter
	mems = []Member{members[3], members[2], members[0], members[4]}

	// reset this guy's ID to make sure it gets set corrcetly
	mems[3].Id = 0

	err = Set(session, mems)
	c.Assert(err, gc.IsNil)

	deadline := time.Now().Add(time.Second * 60)

	for {
		// can dial whichever replica address here, mongo will figure it out
		session = instances[0].DialDirect()
		err := session.Ping()
		if err == nil || time.Now().After(deadline) {
			break
		}
	}
	c.Assert(err, gc.IsNil)

	expectedMembers = []Member{members[3], members[2], members[0], members[4]}

	// any new members will get an id of max(other_ids...)+1
	expectedMembers[0].Id = 3
	expectedMembers[3].Id = 4

	mems, err = CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)
}

func (s *MongoSuite) TestIsMaster(c *gc.C) {
	session := root.Dial()
	defer session.Close()

	exp := IsMasterResults{
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
	c.Check(*res, gc.DeepEquals, exp)
}

func (s *MongoSuite) TestCurrentStatus(c *gc.C) {
	session := root.Dial()
	defer session.Close()

	exp := IsMasterResults{
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
	c.Check(*res, gc.DeepEquals, exp)
}

func closeEnough(expected, obtained time.Time) bool {
	t := obtained.Sub(expected)
	return (-500*time.Millisecond) < t && t < (500*time.Millisecond)
}
