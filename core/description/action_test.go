// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type ActionSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&ActionSerializationSuite{})

func (s *ActionSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "actions"
	s.sliceName = "actions"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importActions(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["actions"] = []interface{}{}
	}
}

func (s *ActionSerializationSuite) TestNewAction(c *gc.C) {
	args := ActionArgs{
		Id:         "foo",
		Receiver:   "bar",
		Name:       "bam",
		Parameters: map[string]interface{}{"foo": 3, "bar": "bam"},
		Enqueued:   time.Now(),
		Started:    time.Now(),
		Completed:  time.Now(),
		Status:     "happy",
		Message:    "a message",
		Results:    map[string]interface{}{"the": 3, "thing": "bam"},
	}
	action := newAction(args)
	c.Check(action.Id(), gc.Equals, args.Id)
	c.Check(action.Receiver(), gc.Equals, args.Receiver)
	c.Check(action.Name(), gc.Equals, args.Name)
	c.Check(action.Parameters(), jc.DeepEquals, args.Parameters)
	c.Check(action.Enqueued(), gc.Equals, args.Enqueued)
	c.Check(action.Started(), gc.Equals, args.Started)
	c.Check(action.Completed(), gc.Equals, args.Completed)
	c.Check(action.Status(), gc.Equals, args.Status)
	c.Check(action.Message(), gc.Equals, args.Message)
	c.Check(action.Results(), jc.DeepEquals, args.Results)
}

func (s *ActionSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := actions{
		Version: 1,
		Actions_: []*action{
			newAction(ActionArgs{
				Id:         "foo",
				Receiver:   "bar",
				Name:       "bam",
				Parameters: map[string]interface{}{"foo": 3, "bar": "bam"},
				Enqueued:   time.Now().UTC(),
				Started:    time.Now().UTC(),
				Completed:  time.Now().UTC(),
				Status:     "happy",
				Message:    "a message",
				Results:    map[string]interface{}{"the": 3, "thing": "bam"},
			}),
			newAction(ActionArgs{
				Name:       "bing",
				Enqueued:   time.Now().UTC(),
				Parameters: map[string]interface{}{"bop": 4, "beep": "fish"},
				Results:    map[string]interface{}{"eggs": 5, "spam": "wow"},
			}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	actions, err := importActions(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(actions, jc.DeepEquals, initial.Actions_)
}
