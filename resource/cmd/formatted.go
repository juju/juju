// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// FormattedCharmResource holds the formatted representation of a resource's info.
type FormattedCharmResource struct {
	// These fields are exported for the sake of serialization.
	Name        string   `json:"name" yaml:"name"`
	Type        DataType `json:"type" yaml:"type"`
	Path        string   `json:"path" yaml:"path"`
	Comment     string   `json:"comment,omitempty" yaml:"comment,omitempty"`
	Revision    int      `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint string   `json:"fingerprint" yaml:"fingerprint"`
	Origin      Origin   `json:"origin" yaml:"origin"`
}

// FormattedSvcResource holds the formatted representation of a resource's info.
type FormattedSvcResource struct {
	// These fields are exported for the sake of serialization.
	Name        string    `json:"name" yaml:"name"`
	Type        DataType  `json:"type" yaml:"type"`
	Path        string    `json:"path" yaml:"path"`
	Comment     string    `json:"comment,omitempty" yaml:"comment,omitempty"`
	Revision    int       `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint string    `json:"fingerprint" yaml:"fingerprint"`
	Origin      Origin    `json:"origin" yaml:"origin"`
	Used        bool      `json:"used" yaml:"used"`
	Timestamp   time.Time `json:"timestamp" yaml:"timestamp"`
	Username    string    `json:"username" yaml:"username"`
}

//go:generate stringer -type=Origin

// Origin defines where a resource came from.
type Origin int

const (
	// OriginUnknown indicates we don't know where the resource came from.
	OriginUnknown Origin = iota
	// OriginUpload indicates the resource was uploaded to the controller.
	OriginUpload
	// OriginStore indicates the resource was retrieved from the charm store.
	OriginStore
)

// MarshalJSON implements json.Marshaler.
func (o Origin) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", o.lower())), nil
}

// MarshalYAML implements yaml.Marshaler.
func (o Origin) MarshalYAML() (interface{}, error) {
	return o.lower(), nil
}

// originNameLen indicates the length of the name of the type.
var originNameLen = len(reflect.TypeOf(Origin(0)).Name())

// lower produces a lowercase string representation that strips off the type name.
func (o Origin) lower() string {
	return strings.ToLower(o.String())[originNameLen:]
}

//go:generate stringer -type=DataType

// DataType defines what kind of data a resource represents.
type DataType int

const (
	// DataTypeUnknown indicates we don't know what kind of data resources
	// represents.
	DataTypeUnknown DataType = iota
	// DataTypeFile indicates the resource is a file.
	DataTypeFile
)

// MarshalJSON implements json.Marshaler.
func (t DataType) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", t.lower())), nil
}

// MarshalYAML implements yaml.Marshaler.
func (t DataType) MarshalYAML() (interface{}, error) {
	return t.lower(), nil
}

// dataTypeNameLen indicates the length of the name of the type.
var dataTypeNameLen = len(reflect.TypeOf(DataType(0)).Name())

// lower produces a lowercase string representation that strips off the type name.
func (t DataType) lower() string {
	return strings.ToLower(t.String())[dataTypeNameLen:]
}
