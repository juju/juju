// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type cloudimagemetadataset struct {
	Version             int                   `yaml:"version"`
	CloudImageMetadata_ []*cloudimagemetadata `yaml:"cloudimagemetadata"`
}

type cloudimagemetadata struct {
	Stream_          string  `yaml:"stream"`
	Region_          string  `yaml:"region"`
	Version_         string  `yaml:"version"`
	Series_          string  `yaml:"series"`
	Arch_            string  `yaml:"arch"`
	VirtType_        string  `yaml:"virttype"`
	RootStorageType_ string  `yaml:"rootstoragetype"`
	RootStorageSize_ *uint64 `yaml:"rootstoragesize,omitempty"`
	DateCreated_     int64   `yaml:"datecreated"`
	Source_          string  `yaml:"source"`
	Priority_        int     `yaml:"priority"`
	ImageId_         string  `yaml:"imageid"`
}

// Stream implements CloudImageMetadata.
func (i *cloudimagemetadata) Stream() string {
	return i.Stream_
}

// Region implements CloudImageMetadata.
func (i *cloudimagemetadata) Region() string {
	return i.Region_
}

// Version implements CloudImageMetadata.
func (i *cloudimagemetadata) Version() string {
	return i.Version_
}

// Series implements CloudImageMetadata.
func (i *cloudimagemetadata) Series() string {
	return i.Series_
}

// Arch implements CloudImageMetadata.
func (i *cloudimagemetadata) Arch() string {
	return i.Arch_
}

// VirtType implements CloudImageMetadata.
func (i *cloudimagemetadata) VirtType() string {
	return i.VirtType_
}

// RootStorageType implements CloudImageMetadata.
func (i *cloudimagemetadata) RootStorageType() string {
	return i.RootStorageType_
}

// RootStorageSize implements CloudImageMetadata.
func (i *cloudimagemetadata) RootStorageSize() *uint64 {
	return i.RootStorageSize_
}

// DateCreated implements CloudImageMetadata.
func (i *cloudimagemetadata) DateCreated() int64 {
	return i.DateCreated_
}

// Source implements CloudImageMetadata.
func (i *cloudimagemetadata) Source() string {
	return i.Source_
}

// Priority implements CloudImageMetadata.
func (i *cloudimagemetadata) Priority() int {
	return i.Priority_
}

//ImageId implements CloudImageMetadata.
func (i *cloudimagemetadata) ImageId() string {
	return i.ImageId_
}

// CloudImageMetadataArgs is an argument struct used to create a
// new internal cloudimagemetadata type that supports the CloudImageMetadata interface.
type CloudImageMetadataArgs struct {
	Stream          string
	Region          string
	Version         string
	Series          string
	Arch            string
	VirtType        string
	RootStorageType string
	RootStorageSize *uint64
	DateCreated     int64
	Source          string
	Priority        int
	ImageId         string
}

func newCloudImageMetadata(args CloudImageMetadataArgs) *cloudimagemetadata {
	cloudimagemetadata := &cloudimagemetadata{
		Stream_:          args.Stream,
		Region_:          args.Region,
		Version_:         args.Version,
		Series_:          args.Series,
		Arch_:            args.Arch,
		VirtType_:        args.VirtType,
		RootStorageType_: args.RootStorageType,
		RootStorageSize_: args.RootStorageSize,
		DateCreated_:     args.DateCreated,
		Source_:          args.Source,
		Priority_:        args.Priority,
		ImageId_:         args.ImageId,
	}
	return cloudimagemetadata
}

func importCloudImageMetadata(source map[string]interface{}) ([]*cloudimagemetadata, error) {
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
		"stream":          schema.String(),
		"region":          schema.String(),
		"version":         schema.String(),
		"series":          schema.String(),
		"arch":            schema.String(),
		"virttype":        schema.String(),
		"rootstoragetype": schema.String(),
		"rootstoragesize": schema.OneOf(schema.Uint(), schema.Nil("rootstoragesize")),
		"datecreated":     schema.Int(),
		"source":          schema.String(),
		"priority":        schema.Int(),
		"imageid":         schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"rootstoragesize": nil,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cloudimagemetadata v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	rootstoragesize, ok := valid["rootstoragesize"].(uint64)
	var pointerSize *uint64
	if ok {
		pointerSize = &rootstoragesize
	}

	cloudimagemetadata := &cloudimagemetadata{
		Stream_:          valid["stream"].(string),
		Region_:          valid["region"].(string),
		Version_:         valid["version"].(string),
		Series_:          valid["series"].(string),
		Arch_:            valid["arch"].(string),
		VirtType_:        valid["virttype"].(string),
		RootStorageType_: valid["rootstoragetype"].(string),
		RootStorageSize_: pointerSize,
		DateCreated_:     valid["datecreated"].(int64),
		Source_:          valid["source"].(string),
		Priority_:        int(valid["priority"].(int64)),
		ImageId_:         valid["imageid"].(string),
	}

	return cloudimagemetadata, nil
}
