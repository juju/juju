// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"strings"
	"time"
)

// FormattedCharmResource holds the formatted representation of a resource's info.
type FormattedCharmResource struct {
	// These fields are exported for the sake of serialization.
	Name        string `json:"name" yaml:"name"`
	Type        Type   `json:"type" yaml:"type"`
	Path        string `json:"path" yaml:"path"`
	Comment     string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Revision    int    `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint string `json:"fingerprint" yaml:"fingerprint"`
	Origin      Origin `json:"origin" yaml:"origin"`
}

// FormattedSvcResource holds the formatted representation of a resource's info.
type FormattedSvcResource struct {
	// These fields are exported for the sake of serialization.
	Name        string    `json:"name" yaml:"name"`
	Type        Type      `json:"type" yaml:"type"`
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
	OriginUnknown Origin = iota // unknown origin
	OriginUpload                // uploaded to controller
	OriginStore                 // retrieved from charm store
)

// MarshalJSON implements json.Marshaler.
func (o Origin) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", o.lower())), nil
}

// MarshalYAML implements yaml.Marshaler.
func (o Origin) MarshalYAML() (interface{}, error) {
	return o.lower(), nil
}

// lower produces a lowercase string representation that strips off the type name.
func (o Origin) lower() string {
	// 6 being the length of "Origin".
	return strings.ToLower(o.String())[6:]
}

//go:generate stringer -type=Type

// Type defines what kind of data a resource represents.
type Type int

const (
	TypeUnknown Type = iota // unknown resource type
	TypeFile                // file resource
)

// MarshalJSON implements json.Marshaler.
func (t Type) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", t.lower())), nil
}

// MarshalYAML implements yaml.Marshaler.
func (t Type) MarshalYAML() (interface{}, error) {
	return t.lower(), nil
}

// lower produces a lowercase string representation that strips off the type name.
func (t Type) lower() string {
	// 4 being the length of "Type".
	return strings.ToLower(t.String())[4:]
}
