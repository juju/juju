// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type storagepools struct {
	Version int            `yaml:"version"`
	Pools_  []*storagepool `yaml:"pools"`
}

type storagepool struct {
	Name_       string                 `yaml:"name"`
	Provider_   string                 `yaml:"provider"`
	Attributes_ map[string]interface{} `yaml:"attributes"`
}

// StoragePoolArgs is an argument struct used to add a storage pool to the
// Model.
type StoragePoolArgs struct {
	Name       string
	Provider   string
	Attributes map[string]interface{}
}

func newStoragePool(args StoragePoolArgs) *storagepool {
	return &storagepool{
		Name_:       args.Name,
		Provider_:   args.Provider,
		Attributes_: args.Attributes,
	}
}

// Name implements StoragePool.
func (s *storagepool) Name() string {
	return s.Name_
}

// Provider implements StoragePool.
func (s *storagepool) Provider() string {
	return s.Provider_
}

// Name implements StoragePool.
func (s *storagepool) Attributes() map[string]interface{} {
	return s.Attributes_
}

func importStoragePools(source map[string]interface{}) ([]*storagepool, error) {
	checker := versionedChecker("pools")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "storagepools version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := storagePoolDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["pools"].([]interface{})
	return importStoragePoolList(sourceList, importFunc)
}

func importStoragePoolList(sourceList []interface{}, importFunc storagePoolDeserializationFunc) ([]*storagepool, error) {
	result := make([]*storagepool, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for storagepool %d, %T", i, value)
		}
		pool, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "storagepool %d", i)
		}
		result = append(result, pool)
	}
	return result, nil
}

type storagePoolDeserializationFunc func(map[string]interface{}) (*storagepool, error)

var storagePoolDeserializationFuncs = map[int]storagePoolDeserializationFunc{
	1: importStoragePoolV1,
}

func importStoragePoolV1(source map[string]interface{}) (*storagepool, error) {
	fields := schema.Fields{
		"name":       schema.String(),
		"provider":   schema.String(),
		"attributes": schema.StringMap(schema.Any()),
	}

	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "storagepool v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &storagepool{
		Name_:       valid["name"].(string),
		Provider_:   valid["provider"].(string),
		Attributes_: valid["attributes"].(map[string]interface{}),
	}

	return result, nil
}
