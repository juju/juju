// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type CloudImageMetadataSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&CloudImageMetadataSerializationSuite{})

func (s *CloudImageMetadataSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "cloudimagemetadata"
	s.sliceName = "cloudimagemetadata"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importCloudImageMetadatas(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["cloudimagemetadata"] = []interface{}{}
	}
}

func (s *CloudImageMetadataSerializationSuite) TestNewCloudImageMetadata(c *gc.C) {
	args := CloudImageMetadataArgs{
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
	cloudimagemetadata := newCloudImageMetadata(args)
	c.Check(cloudimagemetadata.Id(), gc.Equals, args.Id)
	c.Check(cloudimagemetadata.Receiver(), gc.Equals, args.Receiver)
	c.Check(cloudimagemetadata.Name(), gc.Equals, args.Name)
	c.Check(cloudimagemetadata.Parameters(), jc.DeepEquals, args.Parameters)
	c.Check(cloudimagemetadata.Enqueued(), gc.Equals, args.Enqueued)
	c.Check(cloudimagemetadata.Started(), gc.Equals, args.Started)
	c.Check(cloudimagemetadata.Completed(), gc.Equals, args.Completed)
	c.Check(cloudimagemetadata.Status(), gc.Equals, args.Status)
	c.Check(cloudimagemetadata.Message(), gc.Equals, args.Message)
	c.Check(cloudimagemetadata.Results(), jc.DeepEquals, args.Results)
}

func (s *CloudImageMetadataSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := cloudimagemetadata{
		Version: 1,
		CloudImageMetadatas_: []*cloudimagemetadata{
			newCloudImageMetadata(CloudImageMetadataArgs{
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
			}),
			newCloudImageMetadata(CloudImageMetadataArgs{
				Name:       "bing",
				Enqueued:   time.Now(),
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

	cloudimagemetadata, err := importCloudImageMetadatas(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cloudimagemetadata, jc.DeepEquals, initial.CloudImageMetadatas_)
}
