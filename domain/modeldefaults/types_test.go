// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldefaults

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type typesSuite struct{}

func TestTypesSuite(t *stdtesting.T) { tc.Run(t, &typesSuite{}) }

// TestZeroDefaultsValue is here to test what the zero value of a
// DefaultAttributeValue does. Specifically that Has returns false and the apply
// strategy just returns whatever is passed to it.
//
// We want to make sure that if a zero value escapes by accident it will not
// cause damage to a models config.
func (s *typesSuite) TestZeroDefaultsValue(c *tc.C) {
	val := DefaultAttributeValue{}

	has, source := val.ValueSource("someval")
	c.Check(has, tc.IsFalse)
	c.Check(source, tc.Equals, "")

	applied, is := val.ApplyStrategy("teststring").(string)
	c.Assert(is, tc.IsTrue, tc.Commentf("expected zero value apply strategy to return what was passed to it verbatim"))
	c.Check(applied, tc.Equals, "teststring")
}

// TestValueSourceSupportForNil is testing for nil values to ValueSource() we always return
// false and no source information as per the contract of the function.
func (s *typesSuite) TestValueSourceSupportForNil(c *tc.C) {
	val := DefaultAttributeValue{
		Region: "someval",
	}

	has, source := val.ValueSource(nil)
	c.Check(has, tc.IsFalse)
	c.Check(source, tc.Equals, "")

	val = DefaultAttributeValue{}

	has, source = val.ValueSource("myval")
	c.Check(has, tc.IsFalse)
	c.Check(source, tc.Equals, "")
}

// TestValueSourceSupport is testing ValueSource for DefaultAttributeValue and that we can ask
// the question correctly. We are only checking basic comparison here as that is
// all Has supports.
func (s *typesSuite) TestValueSourceSupport(c *tc.C) {
	val := DefaultAttributeValue{
		Region: "someval",
	}

	has, source := val.ValueSource("someval")
	c.Check(has, tc.IsTrue)
	c.Check(source, tc.Equals, "region")

	i := int32(10)
	val = DefaultAttributeValue{
		Region: &i,
	}

	has, source = val.ValueSource(&i)
	c.Check(has, tc.IsTrue)
	c.Check(source, tc.Equals, "region")

	val = DefaultAttributeValue{
		Region: []any{
			"one",
			"two",
			"three",
		},
	}

	has, source = val.ValueSource([]any{
		"one", "two", "three",
	})
	c.Check(has, tc.IsTrue)
	c.Check(source, tc.Equals, "region")

	structVal := struct{ name string }{"test"}
	val = DefaultAttributeValue{
		Region: &structVal,
	}

	has, source = val.ValueSource(&struct{ name string }{"test"})
	c.Check(has, tc.IsFalse)
	c.Check(source, tc.Equals, "")
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
func (s *typesSuite) TestApplyStrategy(c *tc.C) {
	strategy := &testApplyStrategy{}
	val := DefaultAttributeValue{
		Strategy:   strategy,
		Controller: "someval",
	}

	out := val.ApplyStrategy("someval1")
	c.Check(strategy.called, tc.IsTrue)
	c.Check(out, tc.Equals, "someval1")
}

// TestPreferSetApplyStrategy is testing the contract offered by
// [PreferSetApplyStrategy] (the happy path).
func (s *typesSuite) TestPreferSetApplyStrategy(c *tc.C) {
	strategy := PreferSetApplyStrategy{}
	c.Check(strategy.Apply(nil, "test"), tc.Equals, "test")
	c.Check(strategy.Apply("default", nil), tc.Equals, "default")
	c.Check(strategy.Apply("default", "set"), tc.Equals, "set")
	c.Check(strategy.Apply(nil, nil), tc.IsNil)
}

func (s *typesSuite) TestPreferDefaultApplyStrategy(c *tc.C) {
	strategy := PreferDefaultApplyStrategy{}
	c.Check(strategy.Apply(nil, "test"), tc.Equals, "test")
	c.Check(strategy.Apply("default", nil), tc.Equals, "default")
	c.Check(strategy.Apply("default", "set"), tc.Equals, "default")
	c.Check(strategy.Apply(nil, nil), tc.IsNil)
}
