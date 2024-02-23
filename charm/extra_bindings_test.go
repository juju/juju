// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm"
)

var _ = gc.Suite(&extraBindingsSuite{})

type extraBindingsSuite struct {
	riakMeta charm.Meta
}

func (s *extraBindingsSuite) SetUpTest(c *gc.C) {
	riakMeta, err := charm.ReadMeta(repoMeta(c, "riak"))
	c.Assert(err, jc.ErrorIsNil)
	s.riakMeta = *riakMeta
}

func (s *extraBindingsSuite) TestSchemaOkay(c *gc.C) {
	raw := map[interface{}]interface{}{
		"foo": nil,
		"bar": nil,
	}
	v, err := charm.ExtraBindingsSchema.Coerce(raw, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(v, jc.DeepEquals, map[interface{}]interface{}{
		"foo": nil,
		"bar": nil,
	})
}

func (s *extraBindingsSuite) TestValidateWithEmptyNonNilMap(c *gc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, gc.ErrorMatches, "extra bindings cannot be empty when specified")
}

func (s *extraBindingsSuite) TestValidateWithEmptyName(c *gc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{
		"": charm.ExtraBinding{Name: ""},
	}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, gc.ErrorMatches, "missing binding name")
}

func (s *extraBindingsSuite) TestValidateWithMismatchedName(c *gc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{
		"bar": charm.ExtraBinding{Name: "foo"},
	}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, gc.ErrorMatches, `mismatched extra binding name: got "foo", expected "bar"`)
}

func (s *extraBindingsSuite) TestValidateWithRelationNamesMatchingExtraBindings(c *gc.C) {
	s.riakMeta.ExtraBindings = map[string]charm.ExtraBinding{
		"admin": charm.ExtraBinding{Name: "admin"},
		"ring":  charm.ExtraBinding{Name: "ring"},
		"foo":   charm.ExtraBinding{Name: "foo"},
	}
	err := charm.ValidateMetaExtraBindings(s.riakMeta)
	c.Assert(err, gc.ErrorMatches, `relation names \(admin, ring\) cannot be used in extra bindings`)
}
