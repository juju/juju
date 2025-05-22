// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
)

func TestExtraBindingsSuite(t *stdtesting.T) {
	tc.Run(t, &extraBindingsSuite{})
}

type extraBindingsSuite struct {
	riakMeta charm.Meta
}

func (s *extraBindingsSuite) SetUpTest(c *tc.C) {
	riakMeta, err := charm.ReadMeta(repoMeta(c, "riak"))
	c.Assert(err, tc.ErrorIsNil)
	s.riakMeta = *riakMeta
}

func (s *extraBindingsSuite) TestSchemaOkay(c *tc.C) {
	raw := map[interface{}]interface{}{
		"foo": nil,
		"bar": nil,
	}
	v, err := charm.ExtraBindingsSchema.Coerce(raw, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(v, tc.DeepEquals, map[interface{}]interface{}{
		"foo": nil,
		"bar": nil,
	})
}

func (s *extraBindingsSuite) TestValidateWithEmptyNonNilMap(c *tc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, tc.ErrorMatches, "extra bindings cannot be empty when specified")
}

func (s *extraBindingsSuite) TestValidateWithEmptyName(c *tc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{
		"": {Name: ""},
	}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, tc.ErrorMatches, "missing binding name")
}

func (s *extraBindingsSuite) TestValidateWithMismatchedName(c *tc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{
		"bar": {Name: "foo"},
	}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, tc.ErrorMatches, `mismatched extra binding name: got "foo", expected "bar"`)
}

func (s *extraBindingsSuite) TestValidateWithRelationNamesMatchingExtraBindings(c *tc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{
		"admin": {Name: "admin"},
		"ring":  {Name: "ring"},
		"foo":   {Name: "foo"},
	}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, tc.ErrorMatches, `relation names \(admin, ring\) cannot be used in extra bindings`)
}
