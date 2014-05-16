// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
)

type ActionsSuite struct{}

var _ = gc.Suite(&ActionsSuite{})

var yamlReaderTests = []struct {
	yaml         string
	actions      *charm.Actions
	errorMessage string
}{
	{`actionspecs:
  snapshot:
    decription: Take a snapshot of the database.
    params:
      outfile:
        The file to write out to.
        type: string
        default: foo.bz2
`, nil, "YAML error: line 6: mapping values are not allowed in this context"}, {`actionspecs:
  snapshot:
  de****c: : : : :ription: Take a snapshot of the database.
    params:
      outfile:
        description: The file to write out to.
        type: string
        default: foo.bz2
`, nil, "actions.yaml failed to unmarshal -- key mismatch"}, {`actionspecs:
  snapshot:
    description: Take a snapshot of the database.
    params:
      outfile:
        description: The file to write out to.
        type: string
        default: foo.bz2
`, &charm.Actions{map[string]charm.ActionSpec{
		"snapshot": charm.ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"outfile": map[interface{}]interface{}{
					"description": "The file to write out to.",
					"type":        "string",
					"default":     "foo.bz2"}}}}},
		""},
}

func (s *ActionsSuite) TestNewActions(c *gc.C) {
	newActions := charm.NewActions()
	c.Logf("NewActions comes back empty and not nil.")
	c.Assert(newActions, gc.NotNil)
}

func (s *ActionsSuite) TestReadActionsYaml(c *gc.C) {
	for i, t := range yamlReaderTests {
		c.Logf("ReadActionsYaml test %d", i)
		reader := bytes.NewReader([]byte(t.yaml))
		a, err := charm.ReadActionsYaml(reader)
		if t.actions != nil {
			c.Assert(a, gc.DeepEquals, t.actions)
		} else {
			c.Logf("a was %v", a)
			c.Assert(err, gc.NotNil)
		}
	}
}
