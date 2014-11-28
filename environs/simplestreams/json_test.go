package simplestreams_test

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/simplestreams"
)

type jsonSuite struct{}

func (s *jsonSuite) TestItemCollectionMarshalling(c *gc.C) {
	// Ensure that unmarshalling a simplestreams.ItemCollection
	// directly (not through ParseCloudMetadata) doesn't
	// cause any surprises.
	var m simplestreams.ItemCollection
	m.Items = make(map[string]interface{})
	err := json.Unmarshal([]byte(`{
        "items": {
            "a": "b",
            "c": 123 
        }
    }`), &m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Items, gc.DeepEquals, map[string]interface{}{
		"a": "b",
		"c": float64(123),
	})
	// Ensure marshalling works as expected, too.
	b, err := json.Marshal(&m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Equals, `{"items":{"a":"b","c":123}}`)
}
