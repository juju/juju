// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
)

type ActionsSuite struct{}

var _ = gc.Suite(&ActionsSuite{})

var yamlReaderTests = []struct {
	yaml    string
	actions charm.Actions
}{
	{`actions:
  snapshot:
    decription: Take a snapshot of the database.
    params:
      outfile:
        The file to write out to.
        type: string
        default: foo.bz2
`, charm.Actions{}}, {`actions:
  snapshot:
    decription: Take a snapshot of the database.
    params:
      outfile:
        description: The file to write out to.
        type: string
        default: foo.bz2
`, charm.Actions{}}, {`actions:
  snapshot:
    description: Take a snapshot of the database.
    params:
      outfile:
        description: The file to write out to.
        type: string
        default: foo.bz2
`, charm.Actions{map[string]charm.ActionSpec{
		"snapshot": charm.ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"outfile": map[string]interface{}{
					"description": "The file to write out to.",
					"type":        "string",
					"default":     "foo.bz2"}}}}}},
}

func (s *ActionsSuite) TestNewActions(c *gc.C) {
	newActions := charm.NewActions()
	c.Assert(newActions, gc.NotNil, "Newly created Actions is not nil.")
}

//
// func (s *ActionsSuite) TestReadTypoActions(c *gc.C) {
// 	reader := bytes.NewReader([]byte(typoYamlString))
// 	as, err := ReadActionsYaml(reader)
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	t.Logf("Actions: %#v\n", as)
//
// 	if as.ActionSpecs == nil {
// 		t.Error("as is nil")
// 	}
// 	if len(as.ActionSpecs) < 1 {
// 		t.Error("as.ActionSpecs is empty")
// 	}
// }
//
// func TestReadGoodActionsBadParam(t *testing.T) {
// 	reader := bytes.NewReader([]byte(yamlString))
// 	as, err := ReadActionsYaml(reader)
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	t.Logf("Actions: %#v\n", as)
//
// 	if as.ActionSpecs == nil {
// 		t.Error("as is nil")
// 	}
// 	if len(as.ActionSpecs) < 1 {
// 		t.Error("as.ActionSpecs is empty")
// 	}
// }
//
// func TestReadGoodActions(t *testing.T) {
// 	reader := bytes.NewReader([]byte(yamlString))
// 	as, err := ReadActionsYaml(reader)
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	t.Logf("Actions: %#v\n", as)
//
// 	if as.ActionSpecs == nil {
// 		t.Error("as is nil")
// 	}
// 	if len(as.ActionSpecs) < 1 {
// 		t.Error("as.ActionSpecs is empty")
// 	}
// }
//
