// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type BindingsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&BindingsSuite{})

func (s *BindingsSuite) TestEndpointBindingsForCharmOpWithNilArgs(c *gc.C) {
	op, err := state.EndpointBindingsForCharmOp(nil, "", nil, nil)
	c.Assert(err, gc.ErrorMatches, "nil state")
	c.Assert(op, jc.DeepEquals, txn.Op{})

	op, err = state.EndpointBindingsForCharmOp(s.State, "", nil, nil)
	c.Assert(err, gc.ErrorMatches, "nil charm metadata")
	c.Assert(op, jc.DeepEquals, txn.Op{})
}

func (s *BindingsSuite) TestEndpointBindingsForCharmOpWithEmptyNewMap(c *gc.C) {
	// Ensure nil and empty new bindings map are handled the same way as if the
	// defaults were used.
	_, meta := s.addTestingCharmAndMeta(c, "")
	defaults := s.bindingsWithDefaults(c, meta, nil, nil)
	op, err := state.EndpointBindingsForCharmOp(s.State, "", nil, meta)
	c.Assert(err, jc.ErrorIsNil)
	state.AssertEndpointBindingsOp(c, op, "", defaults, nil, 0, false)

	op, err = state.EndpointBindingsForCharmOp(s.State, "", map[string]string{}, meta)
	c.Assert(err, jc.ErrorIsNil)
	state.AssertEndpointBindingsOp(c, op, "", defaults, nil, 0, false)

	op, err = state.EndpointBindingsForCharmOp(s.State, "", defaults, meta)
	c.Assert(err, jc.ErrorIsNil)
	state.AssertEndpointBindingsOp(c, op, "", defaults, nil, 0, false)
}

func (s *BindingsSuite) TestEndpointBindingsForCharmOpValidatesAgainstCharm(c *gc.C) {
	// Ensure given bindings are validated against the charm:
	// 1. if an endpoint is bound to unknown space.
	_, meta := s.addTestingCharmAndMeta(c, "")
	modifiedDefaults := s.bindingsWithDefaults(c, meta, map[string]string{"me": "missing"}, nil)
	op, err := state.EndpointBindingsForCharmOp(s.State, "", modifiedDefaults, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "me" bound to unknown space "missing" not valid`)
	c.Assert(op, jc.DeepEquals, txn.Op{})

	// 2. if an endpoint is bound to an empty space.
	modifiedDefaults = s.bindingsWithDefaults(c, meta, map[string]string{"self": ""}, nil)
	op, err = state.EndpointBindingsForCharmOp(s.State, "", modifiedDefaults, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "self" not bound to a space not valid`)
	c.Assert(op, jc.DeepEquals, txn.Op{})

	// 3. if an unknown endpoint bound to a known space is given.
	modifiedDefaults = s.bindingsWithDefaults(c, meta,
		map[string]string{"new": network.DefaultSpace}, nil,
	)
	op, err = state.EndpointBindingsForCharmOp(s.State, "", modifiedDefaults, meta)
	c.Assert(err, gc.ErrorMatches, `unknown endpoint "new" bound to space "default" not valid`)
	c.Assert(op, jc.DeepEquals, txn.Op{})

	// 4. if an both the endpoint and space given are unknown.
	modifiedDefaults = s.bindingsWithDefaults(c, meta, map[string]string{"new": "bar"}, nil)
	op, err = state.EndpointBindingsForCharmOp(s.State, "", modifiedDefaults, meta)
	c.Assert(err, gc.ErrorMatches, `unknown endpoint "new" bound to unknown space "bar" not valid`)
	c.Assert(op, jc.DeepEquals, txn.Op{})
}

func (s *BindingsSuite) TestEndpointBindingsForCharmOpMergesDefaultsWithExisting(c *gc.C) {
	s.addTestingSpaces(c)
	oldCharm, oldMeta := s.addTestingCharmAndMeta(c, "")
	service := s.AddTestingServiceWithBindings(c, "blog", oldCharm, nil)
	key := state.ServiceGlobalKey(service.Name())
	txnRevno, err := state.TxnRevno(s.State, state.EndpointBindingsC, key)
	c.Assert(err, jc.ErrorIsNil)

	givenMap := map[string]string{
		"bar1": "client",             // update existing (from "default").
		"me":   "apps",               // update existing (from "default").
		"foo2": network.DefaultSpace, // no-op, as the value is the same.
	}
	op, err := state.EndpointBindingsForCharmOp(s.State, key, givenMap, oldMeta)
	c.Assert(err, jc.ErrorIsNil)
	updates := bson.D{{"$set", bson.M{
		"bindings.bar1": "client",
		"bindings.me":   "apps",
	}}}
	state.AssertEndpointBindingsOp(c, op, key, nil, updates, txnRevno, false)

	// With a new charm metadata, ensure old charm endpoint which no longer
	// exist are unset, while new unspecified ones use the default space.
	_, newMeta := s.addTestingCharmAndMeta(c, dummyCharmWithOneOfEachRelationType)
	givenMap = map[string]string{
		"bar1": "apps",               // update existing (from "client")
		"foo3": "client",             // new endpoint bound explicitly.
		"self": network.DefaultSpace, // no-op, as the value is the same.
	}
	op, err = state.EndpointBindingsForCharmOp(s.State, key, givenMap, newMeta)
	c.Assert(err, jc.ErrorIsNil)
	updates = bson.D{
		{"$set", bson.M{
			"bindings.foo3": "client",
			"bindings.bar1": "apps",
		}},
		{"$unset", bson.M{ // no longer present old endpoints are all unset.
			"bindings.foo1": 1,
			"bindings.foo2": 1,
			"bindings.bar2": 1,
			"bindings.me":   1,
		}},
	}
	state.AssertEndpointBindingsOp(c, op, key, nil, updates, txnRevno, false)
}

func (s *BindingsSuite) TestReplaceEndpointBindingsOpOnInsert(c *gc.C) {
	op, err := state.ReplaceEndpointBindingsOp(nil, "", nil)
	c.Assert(err, gc.ErrorMatches, "nil state")
	c.Assert(op, jc.DeepEquals, txn.Op{})

	newBindings := map[string]string{
		"foo":  "bar",
		"this": "that",
	}
	op, err = state.ReplaceEndpointBindingsOp(s.State, "missing", newBindings)
	c.Assert(err, jc.ErrorIsNil)
	state.AssertEndpointBindingsOp(c, op, "missing", newBindings, nil, 0, false)

	// Modify newBindings to ensure replaceEndpointBindingsOp makes a copy.
	newBindings["bar"] = "baz"
	delete(newBindings, "foo")
	state.AssertEndpointBindingsOp(c, op, "missing", map[string]string{
		"foo":  "bar",
		"this": "that",
	}, nil, 0, false)
}

func (s *BindingsSuite) TestReplaceEndpointBindingsOpMergesNewAndExistingOnUpdate(c *gc.C) {
	ch, meta := s.addTestingCharmAndMeta(c, "")
	existingBindings := s.bindingsWithDefaults(c, meta, nil, nil)
	service := s.AddTestingServiceWithBindings(c, "blog", ch, existingBindings)
	key := state.ServiceGlobalKey(service.Name())
	s.addTestingSpaces(c)
	newBindings := s.bindingsWithDefaults(c, meta, map[string]string{
		"foo1": "client",
		"bar2": "apps",
	}, []string{"self", "me"})
	txnRevno, err := state.TxnRevno(s.State, state.EndpointBindingsC, key)
	c.Assert(err, jc.ErrorIsNil)

	op, err := state.ReplaceEndpointBindingsOp(s.State, key, newBindings)
	c.Assert(err, jc.ErrorIsNil)
	updates := bson.D{
		{"$set", bson.M{
			"bindings.foo1": "client",
			"bindings.bar2": "apps",
		}},
		{"$unset", bson.M{
			"bindings.self": 1,
			"bindings.me":   1,
		}},
	}
	state.AssertEndpointBindingsOp(c, op, key, nil, updates, txnRevno, false)

	// Modify newBindings to ensure replaceEndpointBindingsOp makes a copy.
	newBindings["bar"] = "baz"
	delete(newBindings, "foo")
	state.AssertEndpointBindingsOp(c, op, key, nil, updates, txnRevno, false)
}

func (s *BindingsSuite) TestReplaceEndpointBindingsOpEscapesKeysOnUpdate(c *gc.C) {
	// NOTE: There is no way keys like the ones below ending up in state
	// naturally, as bindings are validated against the charm metadata and
	// dollar or dot characters are not valid for endpoint names, but we need to
	// ensure updates will still work with such keys.
	key := "myid"
	bindings := state.BindingsMap{
		"empty":       "",
		"simple-key":  "foo",
		"dollar$key":  "bar",
		"dot.key":     "baz",
		"another.key": "to be removed",
		"drop$key":    "this as well",
		"key":         "value with $ or . is OK",
	}
	endpointBindings, closer := state.GetCollection(s.State, state.EndpointBindingsC)
	defer closer()
	writeableC := endpointBindings.Writeable()

	err := writeableC.Insert(state.MakeEndpointBindingsDoc(key, "", bindings))
	c.Assert(err, jc.ErrorIsNil)
	txnRevno, err := state.TxnRevno(s.State, state.EndpointBindingsC, key)
	c.Assert(err, jc.ErrorIsNil)

	newBindings := map[string]string{
		"simple-key": "new foo",
		"dollar$key": "new bar",
		"dot.key":    "new baz",
		"key":        "value $till ok.",
	}
	op, err := state.ReplaceEndpointBindingsOp(s.State, key, newBindings)
	c.Assert(err, jc.ErrorIsNil)
	updates := bson.D{
		{"$set", bson.M{
			"bindings.simple-key":                             "new foo",
			"bindings.dollar" + state.FullWidthDollar + "key": "new bar",
			"bindings.dot" + state.FullWidthDot + "key":       "new baz",
			"bindings.key":                                    "value $till ok.",
		}},
		{"$unset", bson.M{
			"bindings.empty":                                1,
			"bindings.another" + state.FullWidthDot + "key": 1,
			"bindings.drop" + state.FullWidthDollar + "key": 1,
		}},
	}
	state.AssertEndpointBindingsOp(c, op, key, nil, updates, txnRevno, false)

	err = writeableC.UpdateId(key, op.Update)
	c.Assert(err, jc.ErrorIsNil)
	var doc state.EndpointBindingsDoc
	err = writeableC.FindId(key).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]string(doc.Bindings), jc.DeepEquals, newBindings)
}

func (s *BindingsSuite) TestRemoveEndpointBindingsOp(c *gc.C) {
	op := state.RemoveEndpointBindingsOp("foo")
	state.AssertEndpointBindingsOp(c, op, "foo", nil, nil, 0, true)
}

func (s *BindingsSuite) TestReadEndpointBindings(c *gc.C) {
	bindings, txnRevno, err := state.ReadEndpointBindings(nil, "")
	c.Assert(err, gc.ErrorMatches, "nil state")
	c.Assert(txnRevno, gc.Equals, int64(0))
	c.Assert(bindings, gc.IsNil)

	bindings, txnRevno, err = state.ReadEndpointBindings(s.State, "")
	c.Assert(err, gc.ErrorMatches, `endpoint bindings for "" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(txnRevno, gc.Equals, int64(0))
	c.Assert(bindings, gc.IsNil)

	bindings, txnRevno, err = state.ReadEndpointBindings(s.State, "foo")
	c.Assert(err, gc.ErrorMatches, `endpoint bindings for "foo" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(txnRevno, gc.Equals, int64(0))
	c.Assert(bindings, gc.IsNil)

	s.addTestingSpaces(c)
	setBindings := map[string]string{
		"foo1": "client",
		"bar2": "apps",
		"me":   network.DefaultSpace,
	}
	ch, _ := s.addTestingCharmAndMeta(c, "")
	service := s.AddTestingServiceWithBindings(c, "blog", ch, setBindings)

	key := state.ServiceGlobalKey(service.Name())
	bindings, txnRevno, err = state.ReadEndpointBindings(s.State, key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(int(txnRevno), jc.GreaterThan, 0)
	c.Assert(bindings, jc.DeepEquals, map[string]string{
		"foo1": "client",
		"foo2": network.DefaultSpace,
		"bar1": network.DefaultSpace,
		"bar2": "apps",
		"self": network.DefaultSpace,
		"me":   network.DefaultSpace,
	})

	err = service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	bindings, txnRevno, err = state.ReadEndpointBindings(s.State, key)
	c.Assert(err, gc.ErrorMatches, `endpoint bindings for "s#blog" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(txnRevno, gc.Equals, int64(0))
	c.Assert(bindings, gc.IsNil)
}

func (s *BindingsSuite) TestValidateEndpointBindingsForCharm(c *gc.C) {
	err := state.ValidateEndpointBindingsForCharm(nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "nil state")
	err = state.ValidateEndpointBindingsForCharm(s.State, nil, nil)
	c.Assert(err, gc.ErrorMatches, "nil bindings not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	err = state.ValidateEndpointBindingsForCharm(s.State, map[string]string{}, nil)
	c.Assert(err, gc.ErrorMatches, "nil charm metadata not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	s.addTestingSpaces(c)

	_, meta := s.addTestingCharmAndMeta(c, "")
	bindingsWithInvalidSpace := s.bindingsWithDefaults(c, meta,
		map[string]string{"foo1": "invalid"}, nil,
	)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithInvalidSpace, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "foo1" bound to unknown space "invalid" not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithMissingEndpoint := s.bindingsWithDefaults(c, meta, nil, []string{"foo2"})
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithMissingEndpoint, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "foo2" not bound to a space not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithEmptySpace := s.bindingsWithDefaults(c, meta, map[string]string{"me": ""}, nil)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithEmptySpace, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "me" not bound to a space not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithUnknownEndpointAndSpace := s.bindingsWithDefaults(c, meta,
		map[string]string{"new": "thing"}, nil,
	)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithUnknownEndpointAndSpace, meta)
	c.Assert(err, gc.ErrorMatches, `unknown endpoint "new" bound to unknown space "thing" not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithUnknownEndpointAndEmptySpace := s.bindingsWithDefaults(c, meta,
		map[string]string{"new": ""}, nil,
	)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithUnknownEndpointAndEmptySpace, meta)
	c.Assert(err, gc.ErrorMatches, `unknown endpoint "new" not bound to a space not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithUnknownEndpointKnownSpace := s.bindingsWithDefaults(c, meta,
		map[string]string{"new": "client"}, nil,
	)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithUnknownEndpointKnownSpace, meta)
	c.Assert(err, gc.ErrorMatches, `unknown endpoint "new" bound to space "client" not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithOnlyDefaults := s.bindingsWithDefaults(c, meta, nil, nil)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithOnlyDefaults, meta)
	c.Assert(err, jc.ErrorIsNil)

	bindingsWithExplicitSpaces := s.bindingsWithDefaults(c, meta,
		map[string]string{"bar2": "client", "self": "apps"}, nil,
	)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithExplicitSpaces, meta)
	c.Assert(err, jc.ErrorIsNil)

	// Add the default space and retry the last case - should make no
	// difference.
	_, err = s.State.AddSpace(network.DefaultSpace, nil, true)
	c.Assert(err, jc.ErrorIsNil)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithExplicitSpaces, meta)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BindingsSuite) TestDefaultEndpointBindingsForCharm(c *gc.C) {
	bindings, err := state.DefaultEndpointBindingsForCharm(nil)
	c.Assert(err, gc.ErrorMatches, "nil charm metadata")
	c.Assert(bindings, gc.IsNil)

	_, meta := s.addTestingCharmAndMeta(c, "")
	bindings, err = state.DefaultEndpointBindingsForCharm(meta)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings, gc.HasLen, len(meta.Provides)+len(meta.Requires)+len(meta.Peers))
	c.Assert(bindings, jc.DeepEquals, map[string]string{
		"foo1": network.DefaultSpace,
		"foo2": network.DefaultSpace,
		"bar1": network.DefaultSpace,
		"bar2": network.DefaultSpace,
		"self": network.DefaultSpace,
		"me":   network.DefaultSpace,
	})
}

func (s *BindingsSuite) TestCombinedCharmRelations(c *gc.C) {
	withNilMeta := func() {
		state.CombinedCharmRelations(nil)
	}
	c.Assert(withNilMeta, gc.PanicMatches, "nil charm metadata")
	_, meta := s.addTestingCharmAndMeta(c, "")
	allRelations := state.CombinedCharmRelations(meta)
	c.Assert(allRelations, gc.HasLen, len(meta.Provides)+len(meta.Requires)+len(meta.Peers))
	c.Assert(allRelations, jc.DeepEquals, map[string]charm.Relation{
		"foo1": meta.Provides["foo1"],
		"foo2": meta.Provides["foo2"],
		"bar1": meta.Requires["bar1"],
		"bar2": meta.Requires["bar2"],
		"self": meta.Peers["self"],
		"me":   meta.Peers["me"],
	})
}

func (s *BindingsSuite) TestBindingsMapSetGetBSON(c *gc.C) {
	bindings := state.BindingsMap{
		"empty":      "",
		"simple-key": "foo",
		"dollar$key": "bar",
		"dot.key":    "baz",
		"key":        "value with $ or . is OK",
	}
	doc := state.MakeEndpointBindingsDoc("mydocid", "uuid", bindings)
	marshalled, err := bson.Marshal(doc)
	c.Assert(err, jc.ErrorIsNil)
	asString := string(marshalled)
	c.Assert(asString, jc.Contains, state.FullWidthDot)
	c.Assert(asString, gc.Not(jc.Contains), "dot.")
	c.Assert(asString, jc.Contains, state.FullWidthDollar)
	c.Assert(asString, gc.Not(jc.Contains), "dollar$")

	var outDoc state.EndpointBindingsDoc
	err = bson.Unmarshal(marshalled, &outDoc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outDoc, jc.DeepEquals, doc)

	doc2 := state.MakeEndpointBindingsDoc("mydocid", "uuid", nil)
	marshalled, err = bson.Marshal(doc2)
	c.Assert(err, jc.ErrorIsNil)
	var outDoc2 state.EndpointBindingsDoc
	err = bson.Unmarshal(marshalled, &outDoc2)
	c.Assert(err, jc.ErrorIsNil)
	// Even though the input bindings were nil, they were marshalled as an
	// empty, non-nil map.
	c.Assert(outDoc2.Bindings, gc.NotNil)
	c.Assert(outDoc2.Bindings, gc.HasLen, 0)
	outDoc2.Bindings = doc2.Bindings
	c.Assert(outDoc2, jc.DeepEquals, doc2)
}

const (
	dummyCharmWithOneOfEachRelationType = `
name: dummy
summary: "That's a dummy charm with one relation of each type."
description: "This is a longer description."
provides:
  foo3:
    interface: phony
requires:
  bar1:
    interface: fake
peers:
  self:
    interface: dummy
`
	dummyCharmWithTwoOfEachRelationType = `
name: dummy
summary: "That's a dummy charm with 2 relations for each relation type."
description: "This is a longer description."
provides:
  foo1:
    interface: phony
  foo2:
    interface: secret
requires:
  bar1:
    interface: fake
  bar2: real
peers:
  self:
    interface: dummy
  me: peer
`
)

func (s *BindingsSuite) addTestingCharmAndMeta(c *gc.C, metadata string) (*state.Charm, *charm.Meta) {
	revno := 0
	if metadata == "" {
		metadata = dummyCharmWithTwoOfEachRelationType
	} else {
		revno++
	}
	testCharm := s.AddMetaCharm(c, "dummy", metadata, revno)
	return testCharm, testCharm.Meta()
}

func (s *BindingsSuite) addTestingSpaces(c *gc.C) {
	// Add some spaces to use in bindings, but notably NOT the default space, as
	// it should be always allowed.
	_, err := s.State.AddSpace("client", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("apps", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Space(network.DefaultSpace)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *BindingsSuite) bindingsWithDefaults(
	c *gc.C,
	charmMeta *charm.Meta,
	updates map[string]string,
	deletes []string,
) map[string]string {
	mergedBindings := make(map[string]string)
	defaultBindings, err := state.DefaultEndpointBindingsForCharm(charmMeta)
	c.Assert(err, jc.ErrorIsNil)
	for key, defaultValue := range defaultBindings {
		mergedBindings[key] = defaultValue
	}
	for key, updatedValue := range updates {
		mergedBindings[key] = updatedValue
	}
	for _, key := range deletes {
		if _, found := mergedBindings[key]; found {
			delete(mergedBindings, key)
		}
	}
	return mergedBindings
}
