// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
)

type cloudimagemetadatas struct {
	Version              int                   `yaml:"version"`
	CloudImageMetadatas_ []*cloudimagemetadata `yaml:"cloudimagemetadata"`
}

type cloudimagemetadata struct {
	Id_         string                 `yaml:"id"`
	Receiver_   string                 `yaml:"receiver"`
	Name_       string                 `yaml:"name"`
	Parameters_ map[string]interface{} `yaml:"parameters"`
	Enqueued_   time.Time              `yaml:"enqueued"`
	// Can't use omitempty with time.Time, it just doesn't work
	// (nothing is serialised), so use a pointer in the struct.
	Started_   *time.Time             `yaml:"started,omitempty"`
	Completed_ *time.Time             `yaml:"completed,omitempty"`
	Status_    string                 `yaml:"status"`
	Message_   string                 `yaml:"message"`
	Results_   map[string]interface{} `yaml:"results"`
}

// Id implements CloudImageMetadata.
func (i *cloudimagemetadata) Id() string {
	return i.Id_
}

// Receiver implements CloudImageMetadata.
func (i *cloudimagemetadata) Receiver() string {
	return i.Receiver_
}

// Name implements CloudImageMetadata.
func (i *cloudimagemetadata) Name() string {
	return i.Name_
}

// Parameters implements CloudImageMetadata.
func (i *cloudimagemetadata) Parameters() map[string]interface{} {
	return i.Parameters_
}

// Enqueued implements CloudImageMetadata.
func (i *cloudimagemetadata) Enqueued() time.Time {
	return i.Enqueued_
}

// Started implements CloudImageMetadata.
func (i *cloudimagemetadata) Started() time.Time {
	var zero time.Time
	if i.Started_ == nil {
		return zero
	}
	return *i.Started_
}

// Completed implements CloudImageMetadata.
func (i *cloudimagemetadata) Completed() time.Time {
	var zero time.Time
	if i.Completed_ == nil {
		return zero
	}
	return *i.Completed_
}

// Status implements CloudImageMetadata.
func (i *cloudimagemetadata) Status() string {
	return i.Status_
}

// Message implements CloudImageMetadata.
func (i *cloudimagemetadata) Message() string {
	return i.Message_
}

// Results implements CloudImageMetadata.
func (i *cloudimagemetadata) Results() map[string]interface{} {
	return i.Results_
}

// CloudImageMetadataArgs is an argument struct used to create a
// new internal cloudimagemetadata type that supports the CloudImageMetadata interface.
type CloudImageMetadataArgs struct {
	Id         string
	Receiver   string
	Name       string
	Parameters map[string]interface{}
	Enqueued   time.Time
	Started    time.Time
	Completed  time.Time
	Status     string
	Message    string
	Results    map[string]interface{}
}

func newCloudImageMetadata(args CloudImageMetadataArgs) *cloudimagemetadata {
	cloudimagemetadata := &cloudimagemetadata{
		Receiver_:   args.Receiver,
		Name_:       args.Name,
		Parameters_: args.Parameters,
		Enqueued_:   args.Enqueued,
		Status_:     args.Status,
		Message_:    args.Message,
		Id_:         args.Id,
		Results_:    args.Results,
	}
	if !args.Started.IsZero() {
		value := args.Started
		cloudimagemetadata.Started_ = &value
	}
	if !args.Completed.IsZero() {
		value := args.Completed
		cloudimagemetadata.Completed_ = &value
	}
	return cloudimagemetadata
}

func importCloudImageMetadatas(source map[string]interface{}) ([]*cloudimagemetadata, error) {
	checker := versionedChecker("cloudimagemetadata")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cloudimagemetadata version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := cloudimagemetadataDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["cloudimagemetadata"].([]interface{})
	return importCloudImageMetadataList(sourceList, importFunc)
}

func importCloudImageMetadataList(sourceList []interface{}, importFunc cloudimagemetadataDeserializationFunc) ([]*cloudimagemetadata, error) {
	result := make([]*cloudimagemetadata, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for cloudimagemetadata %d, %T", i, value)
		}
		cloudimagemetadata, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "cloudimagemetadata %d", i)
		}
		result = append(result, cloudimagemetadata)
	}
	return result, nil
}

type cloudimagemetadataDeserializationFunc func(map[string]interface{}) (*cloudimagemetadata, error)

var cloudimagemetadataDeserializationFuncs = map[int]cloudimagemetadataDeserializationFunc{
	1: importCloudImageMetadataV1,
}

func importCloudImageMetadataV1(source map[string]interface{}) (*cloudimagemetadata, error) {
	fields := schema.Fields{
		"receiver":   schema.String(),
		"name":       schema.String(),
		"parameters": schema.StringMap(schema.Any()),
		"enqueued":   schema.Time(),
		"started":    schema.Time(),
		"completed":  schema.Time(),
		"status":     schema.String(),
		"message":    schema.String(),
		"results":    schema.StringMap(schema.Any()),
		"id":         schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"started":   time.Time{},
		"completed": time.Time{},
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cloudimagemetadata v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	cloudimagemetadata := &cloudimagemetadata{
		Id_:         valid["id"].(string),
		Receiver_:   valid["receiver"].(string),
		Name_:       valid["name"].(string),
		Status_:     valid["status"].(string),
		Message_:    valid["message"].(string),
		Parameters_: valid["parameters"].(map[string]interface{}),
		Enqueued_:   valid["enqueued"].(time.Time),
		Results_:    valid["results"].(map[string]interface{}),
	}

	started := valid["started"].(time.Time)
	if !started.IsZero() {
		cloudimagemetadata.Started_ = &started
	}
	completed := valid["completed"].(time.Time)
	if !started.IsZero() {
		cloudimagemetadata.Completed_ = &completed
	}
	return cloudimagemetadata, nil
}
