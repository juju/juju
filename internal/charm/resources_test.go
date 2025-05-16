// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
)

func TestResourceSuite(t *stdtesting.T) { tc.Run(t, &resourceSuite{}) }

type resourceSuite struct{}

func (s *resourceSuite) TestSchemaOkay(c *tc.C) {
	raw := map[interface{}]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	})
}

func (s *resourceSuite) TestSchemaMissingType(c *tc.C) {
	raw := map[interface{}]interface{}{
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	})
}

func (s *resourceSuite) TestSchemaUnknownType(c *tc.C) {
	raw := map[interface{}]interface{}{
		"type":        "repo",
		"filename":    "juju",
		"description": "One line that is useful when operators need to push it.",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"type":        "repo",
		"filename":    "juju",
		"description": "One line that is useful when operators need to push it.",
	})
}

func (s *resourceSuite) TestSchemaMissingComment(c *tc.C) {
	raw := map[interface{}]interface{}{
		"type":     "file",
		"filename": "filename.tgz",
	}
	v, err := charm.ResourceSchema.Coerce(raw, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "",
	})
}
