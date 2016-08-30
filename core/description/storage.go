// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
)

type storages struct {
	Version   int        `yaml:"version"`
	Storages_ []*storage `yaml:"storages"`
}

type storage struct {
	ID_    string `yaml:"id"`
	Kind_  string `yaml:"kind"`
	Owner_ string `yaml:"owner"`
	Name_  string `yaml:"name"`

	Attachments_ []string `yaml:"attachments"`
}

// StorageArgs is an argument struct used to add a storage to the Model.
type StorageArgs struct {
	Tag         names.StorageTag
	Kind        string
	Owner       names.Tag
	Name        string
	Attachments []names.UnitTag
}

func newStorage(args StorageArgs) *storage {
	s := &storage{
		ID_:   args.Tag.Id(),
		Kind_: args.Kind,
		Name_: args.Name,
	}
	if args.Owner != nil {
		s.Owner_ = args.Owner.String()
	}
	for _, unit := range args.Attachments {
		s.Attachments_ = append(s.Attachments_, unit.Id())
	}
	return s
}

// Tag implements Storage.
func (s *storage) Tag() names.StorageTag {
	return names.NewStorageTag(s.ID_)
}

// Kind implements Storage.
func (s *storage) Kind() string {
	return s.Kind_
}

// Owner implements Storage.
func (s *storage) Owner() (names.Tag, error) {
	if s.Owner_ == "" {
		return nil, nil
	}
	tag, err := names.ParseTag(s.Owner_)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return tag, nil
}

// Name implements Storage.
func (s *storage) Name() string {
	return s.Name_
}

// Attachments implements Storage.
func (s *storage) Attachments() []names.UnitTag {
	var result []names.UnitTag
	for _, unit := range s.Attachments_ {
		result = append(result, names.NewUnitTag(unit))
	}
	return result
}

// Validate implements Storage.
func (s *storage) Validate() error {
	if s.ID_ == "" {
		return errors.NotValidf("storage missing id")
	}
	if s.Owner_ == "" {
		return errors.NotValidf("storage %q missing owner", s.ID_)
	}
	// Also check that the owner and attachments are valid.
	if _, err := s.Owner(); err != nil {
		return errors.Wrap(err, errors.NotValidf("storage %q invalid owner", s.ID_))
	}
	return nil
}

func importStorages(source map[string]interface{}) ([]*storage, error) {
	checker := versionedChecker("storages")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "storages version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := storageDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["storages"].([]interface{})
	return importStorageList(sourceList, importFunc)
}

func importStorageList(sourceList []interface{}, importFunc storageDeserializationFunc) ([]*storage, error) {
	result := make([]*storage, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for storage %d, %T", i, value)
		}
		storage, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "storage %d", i)
		}
		result = append(result, storage)
	}
	return result, nil
}

type storageDeserializationFunc func(map[string]interface{}) (*storage, error)

var storageDeserializationFuncs = map[int]storageDeserializationFunc{
	1: importStorageV1,
}

func importStorageV1(source map[string]interface{}) (*storage, error) {
	fields := schema.Fields{
		"id":          schema.String(),
		"kind":        schema.String(),
		"owner":       schema.String(),
		"name":        schema.String(),
		"attachments": schema.List(schema.String()),
	}

	// Normally a list would have defaults, but in this case storage
	// should always have at least one attachment.
	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "storage v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &storage{
		ID_:          valid["id"].(string),
		Kind_:        valid["kind"].(string),
		Owner_:       valid["owner"].(string),
		Name_:        valid["name"].(string),
		Attachments_: convertToStringSlice(valid["attachments"]),
	}

	return result, nil
}
