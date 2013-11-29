package replicaset

import (
	"reflect"
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
	var err error
	// do all this stuff here, since we don't want to have to redo it for each test
	root, err = newServer()
	if err != nil {
		t.Fatalf("Got non-nil error from Start of root server: %q", err.Error())
	}
	defer root.Destroy()

	// note, this is an actual test around Initiate, but again, I don't want to
	// have to redo it, so I just do it once.
	func() {
		session := root.MgoDialDirect()
		defer session.Close()

		err := Initiate(session, root.Addr, name)
		if err != nil {
			t.Fatalf("Got non-nil error from Intiate %q", err.Error())
		}

		expectedMembers := []Member{Member{Address: root.Addr}}

		// need to set mode to strong so that we wait for the write to succeed
		// before reading and thus ensure that we're getting consistent reads.
		session.SetMode(mgo.Strong, false)

		mems, err := CurrentMembers(session)
		if err != nil {
			t.Fatalf("Got non-nil error from CurrentMembers %q", err.Error())
		}
		if !reflect.DeepEqual(mems, expectedMembers) {
			t.Fatalf("Expected members %v, got members %v ", expectedMembers, mems)
		}
	}()
	gc.TestingT(t)

}

func newServer() (*coretesting.MgoInstance, error) {
	inst := &coretesting.MgoInstance{}
	inst.Params = []string{"--replSet", name}
	err := inst.Start()
	return inst, err
}

type MongoSuite struct{}

var _ = gc.Suite(&MongoSuite{})

func (s *MongoSuite) TestAddRemoveSet(c *gc.C) {
	session := root.MgoDial()
	defer session.Close()

	members := make([]Member, 0, 5)
	for x := 0; x < 4; x++ {
		inst, err := newServer()
		c.Assert(err, gc.IsNil)
		defer inst.Destroy()
		defer Remove(session, inst.Addr)
		members = append(members, Member{Address: inst.Addr})
	}

	// Add should automatically skip root, so test that
	members = append(members, Member{Address: root.Addr})

	err := Add(session, members[0:5]...)
	c.Assert(err, gc.IsNil)

	expectedMembers := members[0:5]

	mems, err := CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)

	// Now remove the last two Members
	err = Remove(session, members[3].Address, members[4].Address)
	c.Assert(err, gc.IsNil)

	expectedMembers = members[0:3]

	mems, err = CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)

	// now let's mix it up and set the new members to a mix of the previous
	// plus the new arbiter
	mems = []Member{members[4], members[2], members[0], members[5]}

	// reset this guy's ID to amke sure it gets set corrcetly
	members[4].Id = 0

	err = Set(session, mems)
	c.Assert(err, gc.IsNil)

	expectedMembers = []Member{members[4], members[2], members[0], members[5]}

	// any new members will get an id of max(other_ids...)+1
	expectedMembers[0].Id = 3
	expectedMembers[3].Id = 4

	mems, err = CurrentMembers(session)
	c.Assert(err, gc.IsNil)
	c.Assert(mems, gc.DeepEquals, expectedMembers)
}

func (s *MongoSuite) TestIsMaster(c *gc.C) {
	session := root.MgoDial()
	defer session.Close()

	exp := IsMasterResults{
		// The following fields hold information about the specific mongodb node.
		IsMaster:  true,
		Secondary: false,
		Arbiter:   false,
		Address:   root.Addr,
		LocalTime: time.Time{},

		// The following fields hold information about the replica set.
		ReplicaSetName: name,
		Addresses:      []string{root.Addr},
		Arbiters:       []string{},
		PrimaryAddress: root.Addr,
	}

	res, err := IsMaster(session)
	c.Assert(err, gc.IsNil)
	c.Check(closeEnough(res.LocalTime, time.Now()), gc.Equals, true)
	res.LocalTime = time.Time{}
	c.Check(res, gc.DeepEquals, exp)
}

func closeEnough(expected, obtained time.Time) bool {
	t := obtained.Sub(expected)
	return (-500*time.Millisecond) < t && t < (500*time.Millisecond)
}
