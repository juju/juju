// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldefaults

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type typesSuite struct{}

var _ = gc.Suite(&typesSuite{})

// TestZeroDefaultsValue is here to test what the zero value of a
// DefaultAttributeValue does. Specifically that Has returns false and the apply
// strategy just returns whatever is passed to it.
//
// We want to make sure that if a zero value escapes by accident it will not
// cause damage to a models config.
func (s *typesSuite) TestZeroDefaultsValue(c *gc.C) {
	val := DefaultAttributeValue{}

	has, source := val.Has("someval")
	c.Check(has, jc.IsFalse)
	c.Check(source, gc.Equals, "")

	applied, is := val.ApplyStrategy("teststring").(string)
	c.Assert(is, jc.IsTrue, gc.Commentf("expected zero value apply strategy to return what was passed to it verbatim"))
	c.Check(applied, gc.Equals, "teststring")
}

// TestHasSupportForNil is testing for nil values to Has() we always return
// false and no source information as per the contract of the function.
func (s *typesSuite) TestHasSupportForNil(c *gc.C) {
	val := DefaultAttributeValue{
		Source: "testsource",
		Value:  "someval",
	}

	has, source := val.Has(nil)
	c.Check(has, jc.IsFalse)
	c.Check(source, gc.Equals, "")

	val = DefaultAttributeValue{
		Source: "testsource",
		Value:  nil,
	}

	has, source = val.Has("myval")
	c.Check(has, jc.IsFalse)
	c.Check(source, gc.Equals, "")
}

// TestHasSupport is testing Has for DefaultAttributeValue and that we can ask
// the question correctly. We are only checking basic comparison here as that is
// all Has supports.
func (s *typesSuite) TestHasSupport(c *gc.C) {
	val := DefaultAttributeValue{
		Source: "testsource",
		Value:  "someval",
	}

	has, source := val.Has("someval")
	c.Check(has, jc.IsTrue)
	c.Check(source, gc.Equals, "testsource")

	i := int32(10)
	val = DefaultAttributeValue{
		Source: "testsource",
		Value:  &i,
	}

	has, source = val.Has(&i)
	c.Check(has, jc.IsTrue)
	c.Check(source, gc.Equals, "testsource")

	val = DefaultAttributeValue{
		Source: "testsource",
		Value: []any{
			"one",
			"two",
			"three",
		},
	}

	has, source = val.Has([]any{
		"one", "two", "three",
	})
	c.Check(has, jc.IsTrue)
	c.Check(source, gc.Equals, "testsource")

	structVal := struct{ name string }{"test"}
	val = DefaultAttributeValue{
		Source: "testsource",
		Value:  &structVal,
	}

	has, source = val.Has(&struct{ name string }{"test"})
	c.Check(has, jc.IsFalse)
	c.Check(source, gc.Equals, "")
}

// testApplyStrategy is a test implementation of ApplyStrategy that is here to
// just indicate that the Apply method of the strategy has been called.
type testApplyStrategy struct {
	// called indicates that the Apply func of this struct has been called.
	called bool
}

// Apply implements the ApplyStrategy interface.
func (t *testApplyStrategy) Apply(d, s any) any {
	t.called = true
	return s
}

// TestApplyStrategy is checking to make sure that if we set an apply strategy
// on the [DefaultAttributeValue.Strategy] that the strategy gets called by
// [DefaultAttributeValue.ApplyStrategy]. This test isn't concerned about
// testing the logic of strategies just that the strategy is asked to make a
// decision.
func (s *typesSuite) TestApplyStrategy(c *gc.C) {
	strategy := &testApplyStrategy{}
	val := DefaultAttributeValue{
		Source:   "source",
		Strategy: strategy,
		Value:    "someval",
	}

	out := val.ApplyStrategy("someval1")
	c.Check(strategy.called, jc.IsTrue)
	c.Check(out, gc.Equals, "someval1")
}

// TestPreferSetApplyStrategy is testing the contract offered by
// [PreferSetApplyStrategy] (the happy path).
func (s *typesSuite) TestPreferSetApplyStrategy(c *gc.C) {
	strategy := PreferSetApplyStrategy{}
	c.Check(strategy.Apply(nil, "test"), gc.Equals, "test")
	c.Check(strategy.Apply("default", nil), gc.Equals, "default")
	c.Check(strategy.Apply("default", "set"), gc.Equals, "set")
	c.Check(strategy.Apply(nil, nil), gc.IsNil)
}

func (s *typesSuite) TestPreferDefaultApplyStrategy(c *gc.C) {
	strategy := PreferDefaultApplyStrategy{}
	c.Check(strategy.Apply(nil, "test"), gc.Equals, "test")
	c.Check(strategy.Apply("default", nil), gc.Equals, "default")
	c.Check(strategy.Apply("default", "set"), gc.Equals, "default")
	c.Check(strategy.Apply(nil, nil), gc.IsNil)
}
