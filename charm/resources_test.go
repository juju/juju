// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm"
)

var _ = gc.Suite(&resourceSuite{})

type resourceSuite struct{}

func (s *resourceSuite) TestSchemaOkay(c *gc.C) {
	raw := map[interface{}]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	})
}

func (s *resourceSuite) TestSchemaMissingType(c *gc.C) {
	raw := map[interface{}]interface{}{
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	})
}

func (s *resourceSuite) TestSchemaUnknownType(c *gc.C) {
	raw := map[interface{}]interface{}{
		"type":        "repo",
		"filename":    "juju",
		"description": "One line that is useful when operators need to push it.",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"type":        "repo",
		"filename":    "juju",
		"description": "One line that is useful when operators need to push it.",
	})
}

func (s *resourceSuite) TestSchemaMissingComment(c *gc.C) {
	raw := map[interface{}]interface{}{
		"type":     "file",
		"filename": "filename.tgz",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "",
	})
}
