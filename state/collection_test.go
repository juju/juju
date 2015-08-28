// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type collectionSuite struct {
	ConnSuite
}

var _ = gc.Suite(&collectionSuite{})

type collectionTestCase struct {
	label         string
	test          func() (int, error)
	expectedCount int
	expectedPanic string
	expectedError string
}

func (s *collectionSuite) TestGenericStateCollection(c *gc.C) {
	// The users collection does not require filtering by env UUID.
	coll, closer := state.GetCollection(s.State, state.UsersC)
	defer closer()

	c.Check(coll.Name(), gc.Equals, state.UsersC)

	s.Factory.MakeUser(c, &factory.UserParams{Name: "foo", DisplayName: "Ms Foo"})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "bar"})

	collSnapshot := newCollectionSnapshot(c, coll.Writeable().Underlying())

	for i, t := range []collectionTestCase{
		{
			label: "Count",
			test: func() (int, error) {
				return coll.Count()
			},
			expectedCount: 3,
		},
		{
			label: "FindId",
			test: func() (int, error) {
				return coll.FindId("foo").Count()
			},
			expectedCount: 1,
		},
		{
			label: "Find with one result",
			test: func() (int, error) {
				return coll.Find(bson.D{{"displayname", "Ms Foo"}}).Count()
			},
			expectedCount: 1,
		},
		{
			label: "Find with nil",
			test: func() (int, error) {
				return coll.Find(nil).Count()
			},
			expectedCount: 3,
		},
		{
			label: "Insert",
			test: func() (int, error) {
				err := coll.Writeable().Insert(bson.D{{"_id", "more"}})
				c.Assert(err, jc.ErrorIsNil)
				return coll.Count()
			},
			expectedCount: 4,
		},
		{
			label: "RemoveId",
			test: func() (int, error) {
				err := coll.Writeable().RemoveId("bar")
				c.Assert(err, jc.ErrorIsNil)
				return coll.Count()
			},
			expectedCount: 2,
		},
		{
			label: "Remove",
			test: func() (int, error) {
				err := coll.Writeable().Remove(bson.D{{"displayname", "Ms Foo"}})
				c.Assert(err, jc.ErrorIsNil)
				return coll.Count()
			},
			expectedCount: 2,
		},
		{
			label: "RemoveAll",
			test: func() (int, error) {
				_, err := coll.Writeable().RemoveAll(bson.D{{"createdby", s.Owner.Name()}})
				c.Assert(err, jc.ErrorIsNil)
				return coll.Count()
			},
			expectedCount: 0,
		},
		{
			label: "Update",
			test: func() (int, error) {
				err := coll.Writeable().Update(bson.D{{"_id", "bar"}},
					bson.D{{"$set", bson.D{{"displayname", "Updated Bar"}}}})
				c.Assert(err, jc.ErrorIsNil)

				return coll.Find(bson.D{{"displayname", "Updated Bar"}}).Count()
			},
			expectedCount: 1,
		},
		{
			label: "UpdateId",
			test: func() (int, error) {
				err := coll.Writeable().UpdateId("bar",
					bson.D{{"$set", bson.D{{"displayname", "Updated Bar"}}}})
				c.Assert(err, jc.ErrorIsNil)

				return coll.Find(bson.D{{"displayname", "Updated Bar"}}).Count()
			},
			expectedCount: 1,
		},
	} {
		c.Logf("test %d: %s", i, t.label)
		collSnapshot.restore(c)

		count, err := t.test()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(count, gc.Equals, t.expectedCount)
	}
}

func (s *collectionSuite) TestEnvStateCollection(c *gc.C) {
	// The machines collection requires filtering by env UUID. Set up
	// 2 environments with machines in each.
	m0 := s.Factory.MakeMachine(c, nil)
	s.Factory.MakeMachine(c, nil)
	st1 := s.Factory.MakeEnvironment(c, nil)
	defer st1.Close()
	f1 := factory.NewFactory(st1)
	otherM0 := f1.MakeMachine(c, &factory.MachineParams{Series: "trusty"})

	// Ensure that the first machine in each env have overlapping ids
	// (otherwise tests may not fail when they should)
	c.Assert(m0.Id(), gc.Equals, otherM0.Id())

	getIfaceId := func(st *state.State) bson.ObjectId {
		var doc bson.M
		coll, closer := state.GetRawCollection(st, state.NetworkInterfacesC)
		defer closer()
		err := coll.Find(bson.D{{"env-uuid", st.EnvironUUID()}}).One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		return doc["_id"].(bson.ObjectId)
	}

	// Also add a network interface to test collections with ObjectId ids
	_, err := s.State.AddNetwork(state.NetworkInfo{"net1", "net1", "0.1.2.3/24", 0})
	c.Assert(err, jc.ErrorIsNil)
	_, err = m0.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "91:de:f1:02:f6:f0",
		InterfaceName: "foo0",
		NetworkName:   "net1",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Grab the document id of the just added network interface for use in tests.
	ifaceId := getIfaceId(s.State)

	// Add a network interface to the other environment to test collections that rely on the env-uuid field.
	_, err = st1.AddNetwork(state.NetworkInfo{"net2", "net2", "0.1.2.4/24", 0})
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherM0.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    "91:de:f1:02:f6:f0",
		InterfaceName: "foo1",
		NetworkName:   "net2",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Grab the document id of the network interface just added to the other environment for use in tests.
	otherIfaceId := getIfaceId(st1)

	machines0, closer := state.GetCollection(s.State, state.MachinesC)
	defer closer()
	machines1, closer := state.GetCollection(st1, state.MachinesC)
	defer closer()
	networkInterfaces, closer := state.GetCollection(s.State, state.NetworkInterfacesC)
	defer closer()

	machinesSnapshot := newCollectionSnapshot(c, machines0.Writeable().Underlying())
	networkInterfacesSnapshot := newCollectionSnapshot(c, networkInterfaces.Writeable().Underlying())

	c.Assert(machines0.Name(), gc.Equals, state.MachinesC)
	c.Assert(networkInterfaces.Name(), gc.Equals, state.NetworkInterfacesC)

	for i, t := range []collectionTestCase{
		{
			label: "Count filters by env",
			test: func() (int, error) {
				return machines0.Count()
			},
			expectedCount: 2,
		},
		{
			label: "Find filters by env",
			test: func() (int, error) {
				return machines0.Find(bson.D{{"series", m0.Series()}}).Count()
			},
			expectedCount: 2,
		},
		{
			label: "Find adds env UUID when _id is provided",
			test: func() (int, error) {
				return machines0.Find(bson.D{{"_id", m0.Id()}}).Count()
			},
			expectedCount: 1,
		},
		{
			label: "Find tolerates env UUID prefix already being present",
			test: func() (int, error) {
				return machines0.Find(bson.D{
					{"_id", state.DocID(s.State, m0.Id())},
				}).Count()
			},
			expectedCount: 1,
		},
		{
			label: "Find with no selector still filters by env",
			test: func() (int, error) {
				return machines0.Find(nil).Count()
			},
			expectedCount: 2,
		},
		{
			label: "Find leaves _id alone if used with operator",
			test: func() (int, error) {
				return machines0.Find(bson.D{
					{"_id", bson.D{{"$regex", ":" + m0.Id() + "$"}}},
				}).Count()
			},
			expectedCount: 1, // not 2 because env-uuid filter is still added
		},
		{
			label: "Find works with collections with ObjectId ids",
			test: func() (int, error) {
				return networkInterfaces.Find(bson.D{{"interfacename", "foo0"}}).Count()
			},
			expectedCount: 1,
		},
		{
			label: "Find works with ObjectId ids",
			test: func() (int, error) {
				return networkInterfaces.Find(bson.D{{"_id", ifaceId}}).Count()
			},
			expectedCount: 1,
		},
		{
			label: "Find panics if env-uuid is included",
			test: func() (int, error) {
				machines0.Find(bson.D{{"env-uuid", "whatever"}})
				return 0, nil
			},
			expectedPanic: "env-uuid is added automatically and should not be provided",
		},
		{
			label: "Find panics if query type is unsupported",
			test: func() (int, error) {
				machines0.Find(map[string]string{"foo": "bar"})
				return 0, nil
			},
			expectedPanic: "query must be bson.D, bson.M, or nil",
		},
		{
			label: "FindId adds env UUID prefix",
			test: func() (int, error) {
				return machines0.FindId(m0.Id()).Count()
			},
			expectedCount: 1,
		},
		{
			label: "FindId tolerates env UUID prefix already being there",
			test: func() (int, error) {
				return machines0.FindId(state.DocID(s.State, m0.Id())).Count()
			},
			expectedCount: 1,
		},
		{
			label: "FindId works with ObjectId ids",
			test: func() (int, error) {
				return networkInterfaces.FindId(ifaceId).Count()
			},
			expectedCount: 1,
		},
		{
			label: "FindId adds env-uuid field",
			test: func() (int, error) {
				return networkInterfaces.FindId(otherIfaceId).Count()
			},
			// expect to find no networks, as we are searching with the id of
			// the network in the other environment.
			expectedCount: 0,
		},
		{
			label: "Insert adds env-uuid",
			test: func() (int, error) {
				err := machines0.Writeable().Insert(bson.D{
					{"_id", state.DocID(s.State, "99")},
					{"machineid", 99},
				})
				c.Assert(err, jc.ErrorIsNil)
				return machines0.Count()
			},
			expectedCount: 3,
		},
		{
			label: "Insert populates env-uuid if blank",
			test: func() (int, error) {
				err := machines0.Writeable().Insert(bson.D{
					{"_id", state.DocID(s.State, "99")},
					{"machineid", 99},
					{"env-uuid", ""},
				})
				c.Assert(err, jc.ErrorIsNil)
				return machines0.Count()
			},
			expectedCount: 3,
		},
		{
			label: "Insert prefixes _id",
			test: func() (int, error) {
				err := machines0.Writeable().Insert(bson.D{
					{"_id", "99"},
					{"machineid", 99},
				})
				c.Assert(err, jc.ErrorIsNil)
				return machines0.FindId("99").Count()
			},
			expectedCount: 1,
		},
		{
			label: "Insert tolerates prefixed _id and correct env-uuid if provided",
			test: func() (int, error) {
				err := machines0.Writeable().Insert(bson.D{
					{"_id", state.DocID(s.State, "99")},
					{"machineid", 99},
					{"env-uuid", s.State.EnvironUUID()},
				})
				c.Assert(err, jc.ErrorIsNil)
				return machines0.Count()
			},
			expectedCount: 3,
		},
		{
			label: "Insert fails if env-uuid doesn't match",
			test: func() (int, error) {
				err := machines0.Writeable().Insert(bson.D{
					{"_id", "99"},
					{"machineid", 99},
					{"env-uuid", "something-else"},
				})
				return 0, err
			},
			expectedError: "insert env-uuid is not correct: .+",
		},
		{
			label: "Remove adds env UUID prefix to _id",
			test: func() (int, error) {
				err := machines0.Writeable().Remove(bson.D{{"_id", "0"}})
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 2, // Expect machine-1 in first env and machine-0 in second env
		},
		{
			label: "Remove filters by env",
			test: func() (int, error) {
				// Attempt to remove the trusty machine in the second
				// env with the collection that's filtering for the
				// first env - nothing should get removed.
				err := machines0.Writeable().Remove(bson.D{{"series", "trusty"}})
				c.Assert(err, gc.ErrorMatches, "not found")
				return s.machines.Count()
			},
			expectedCount: 3, // Expect all machines to still be there.
		},
		{
			label: "Remove filters by env 2",
			test: func() (int, error) {
				err := machines0.Writeable().Remove(bson.D{{"machineid", "0"}})
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 2, // Expect machine 1 in first env and machine-0 in second env
		},
		{
			label: "RemoveId adds env UUID prefix",
			test: func() (int, error) {
				err := machines0.Writeable().RemoveId(m0.Id())
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 2, // Expect machine-1 in first env and machine-0 in second env
		},
		{
			label: "RemoveId tolerates env UUID prefix already being there",
			test: func() (int, error) {
				err := machines0.Writeable().RemoveId(state.DocID(s.State, m0.Id()))
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 2, // Expect machine-1 in first env and machine-0 in second env
		},
		{
			label: "RemoveId filters by env-uuid field",
			test: func() (int, error) {
				err := networkInterfaces.Writeable().RemoveId(otherIfaceId)
				c.Assert(err, gc.ErrorMatches, "not found")
				return networkInterfaces.Count()
			},
			expectedCount: 1, // ensure doc was not removed
		},
		{
			label: "RemoveAll filters by env",
			test: func() (int, error) {
				_, err := machines0.Writeable().RemoveAll(bson.D{{"series", m0.Series()}})
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 1, // Expect machine-1 in second env
		},
		{
			label: "RemoveAll adds env UUID when _id is provided",
			test: func() (int, error) {
				_, err := machines0.Writeable().RemoveAll(bson.D{{"_id", m0.Id()}})
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 2, // Expect machine-1 in first env and machine-0 in second env
		},
		{
			label: "RemoveAll tolerates env UUID prefix already being present",
			test: func() (int, error) {
				_, err := machines0.Writeable().RemoveAll(bson.D{
					{"_id", state.DocID(s.State, m0.Id())},
				})
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 2, // Expect machine-1 in first env and machine-0 in second env
		},
		{
			label: "RemoveAll with no selector still filters by env",
			test: func() (int, error) {
				_, err := machines0.Writeable().RemoveAll(nil)
				c.Assert(err, jc.ErrorIsNil)
				return s.machines.Count()
			},
			expectedCount: 1, // Expect machine-0 in second env
		},
		{
			label: "RemoveAll panics if env-uuid is included",
			test: func() (int, error) {
				machines0.Writeable().RemoveAll(bson.D{{"env-uuid", "whatever"}})
				return 0, nil
			},
			expectedPanic: "env-uuid is added automatically and should not be provided",
		},
		{
			label: "RemoveAll panics if query type is unsupported",
			test: func() (int, error) {
				machines0.Writeable().RemoveAll(map[string]string{"foo": "bar"})
				return 0, nil
			},
			expectedPanic: "query must be bson.D, bson.M, or nil",
		},
		{
			label: "Update",
			test: func() (int, error) {
				err := machines0.Writeable().Update(bson.D{{"_id", m0.Id()}},
					bson.D{{"$set", bson.D{{"update-field", "field value"}}}})
				c.Assert(err, jc.ErrorIsNil)
				return machines0.Find(bson.D{{"update-field", "field value"}}).Count()
			},
			expectedCount: 1,
		},
		{
			label: "UpdateId",
			test: func() (int, error) {
				err := machines0.Writeable().UpdateId(m0.Id(),
					bson.D{{"$set", bson.D{{"update-field", "field value"}}}})
				c.Assert(err, jc.ErrorIsNil)
				return machines0.Find(bson.D{{"update-field", "field value"}}).Count()
			},
			expectedCount: 1,
		},
	} {
		c.Logf("test %d: %s", i, t.label)
		machinesSnapshot.restore(c)
		networkInterfacesSnapshot.restore(c)

		if t.expectedPanic == "" {
			count, err := t.test()
			if t.expectedError != "" {
				c.Assert(err, gc.ErrorMatches, t.expectedError)
			} else {
				c.Assert(err, jc.ErrorIsNil)
			}
			c.Check(count, gc.Equals, t.expectedCount)
		} else {
			c.Check(func() { t.test() }, gc.PanicMatches, t.expectedPanic)
		}

		// Check that other environment is untouched after each test
		count, err := machines1.Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(count, gc.Equals, 1)
	}
}

type collectionSnapshot struct {
	coll     *mgo.Collection
	origDocs []interface{}
}

func newCollectionSnapshot(c *gc.C, coll *mgo.Collection) *collectionSnapshot {
	ss := &collectionSnapshot{coll: coll}
	err := coll.Find(nil).All(&ss.origDocs)
	c.Assert(err, jc.ErrorIsNil)
	return ss
}

func (ss *collectionSnapshot) restore(c *gc.C) {
	_, err := ss.coll.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = ss.coll.Insert(ss.origDocs...)
	c.Assert(err, jc.ErrorIsNil)
}
