// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type StoragePoolSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&StoragePoolSerializationSuite{})

func (s *StoragePoolSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "storagepools"
	s.sliceName = "pools"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importStoragePools(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["pools"] = []interface{}{}
	}
}

func testStoragePool() *storagepool {
	v := newStoragePool(testStoragePoolArgs())
	return v
}

func testStoragePoolArgs() StoragePoolArgs {
	return StoragePoolArgs{
		Name:     "test",
		Provider: "magic",
		Attributes: map[string]interface{}{
			"method": "madness",
		},
	}
}

func (s *StoragePoolSerializationSuite) TestNewStoragePool(c *gc.C) {
	storagepool := testStoragePool()

	c.Check(storagepool.Name(), gc.Equals, "test")
	c.Check(storagepool.Provider(), gc.Equals, "magic")
	c.Check(storagepool.Attributes(), jc.DeepEquals, map[string]interface{}{
		"method": "madness",
	})
}

func (s *StoragePoolSerializationSuite) exportImport(c *gc.C, storagepool_ *storagepool) *storagepool {
	initial := storagepools{
		Version: 1,
		Pools_:  []*storagepool{storagepool_},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	storagepools, err := importStoragePools(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storagepools, gc.HasLen, 1)
	return storagepools[0]
}

func (s *StoragePoolSerializationSuite) TestParsingSerializedData(c *gc.C) {
	original := testStoragePool()
	storagepool := s.exportImport(c, original)
	c.Assert(storagepool, jc.DeepEquals, original)
}
