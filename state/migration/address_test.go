// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/testing"
)

type AddressSerializationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&AddressSerializationSuite{})

func (*AddressSerializationSuite) TestNil(c *gc.C) {
	_, err := importAddresss(nil)
	c.Check(err, gc.ErrorMatches, "addresss version schema check failed: .*")
}

func (*AddressSerializationSuite) TestMissingVersion(c *gc.C) {
	_, err := importAddresss(map[string]interface{}{
		"value": "",
		"type":  "",
	})
	c.Check(err.Error(), gc.Equals, "addresss version schema check failed: version: expected int, got nothing")
}

func (*AddressSerializationSuite) TestMissingAddresss(c *gc.C) {
	_, err := importAddresss(map[string]interface{}{
		"version": 1,
	})
	c.Check(err.Error(), gc.Equals, "addresss version schema check failed: addresss: expected list, got nothing")
}

func (*AddressSerializationSuite) TestNonIntVersion(c *gc.C) {
	_, err := importAddresss(map[string]interface{}{
		"version": "hello",
		"value":   "",
		"type":    "",
	})
	c.Check(err.Error(), gc.Equals, `addresss version schema check failed: version: expected int, got string("hello")`)
}

func (*AddressSerializationSuite) TestUnknownVersion(c *gc.C) {
	_, err := importAddresss(map[string]interface{}{
		"version": 42,
		"value":   "",
		"type":    "",
	})
	c.Check(err.Error(), gc.Equals, `version 42 not valid`)
}

func (*AddressSerializationSuite) TestParsing(c *gc.C) {
	addr, err := importMachines(map[string]interface{}{
		"version":      1,
		"value":        "no",
		"type":         "content",
		"network-name": "validation",
		"scope":        "done",
		"origin":       "here",
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := *address{
		Version:      1,
		Value_:       "no",
		Type_:        "content",
		NetworkName_: "validation",
		Scope_:       "done",
		Origin_:      "here",
	}
	c.Assert(addr, jc.DeepEquals, expected)
}

func (*AddressSerializationSuite) TestOptionalValues(c *gc.C) {
	addr, err := importMachines(map[string]interface{}{
		"version": 1,
		"value":   "no",
		"type":    "content",
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := *address{
		Version: 1,
		Value_:  "no",
		Type_:   "content",
	}
	c.Assert(addr, jc.DeepEquals, expected)
}

func (*AddressSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := addresss{
		Version: 1,
		Addresss_: []*address{
			&address{
				Id_: "0",
			},
			&address{
				Id_: "1",
				Containers_: []*address{
					&address{
						Id_: "1/lxc/0",
					},
					&address{
						Id_: "1/lxc/1",
					},
				},
			},
			&address{
				Id_: "2",
				Containers_: []*address{
					&address{
						Id_: "2/kvm/0",
						Containers_: []*address{
							&address{
								Id_: "2/kvm/0/lxc/0",
							},
							&address{
								Id_: "2/kvm/0/lxc/1",
							},
						},
					},
				},
			},
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	addresss, err := importAddresss(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addresss, jc.DeepEquals, initial.Addresss_)
}
