// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import "time"

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

// Origin defines where a resource came from.
type Origin string

const (
	// OriginUnknown indicates we don't know where the resource came from.
	OriginUnknown Origin = "unknown"
	// OriginUpload indicates the resource was uploaded to the controller.
	OriginUpload = "upload"
	// OriginStore indicates the resource was retrieved from the charm store.
	OriginStore = "store"
)

// DataType defines what kind of data a resource represents.
type DataType string

const (
	// DataTypeUnknown indicates we don't know what kind of data resources
	// represents.
	DataTypeUnknown DataType = "unknown"
	// DataTypeFile indicates the resource is a file.
	DataTypeFile = "file"
)
