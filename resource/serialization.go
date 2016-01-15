// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

// Serialized is a uniform serialized representation of a resource.
// Only built-in and stdlib types are used. Each of the fields
// corresponds to the same field on Resource.
type Serialized struct {
	Name    string `json:"name" yaml:"name"`
	Type    string `json:"type" yaml:"type"`
	Path    string `json:"path" yaml:"path"`
	Comment string `json:"comment,omitempty" yaml:"comment,omitempty"`

	Origin      string `json:"origin" yaml:"origin"`
	Revision    int    `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint []byte `json:"fingerprint" yaml:"fingerprint"`
	Size        int64  `json:"size" yaml:"size"`

	Username  string    `json:"username" yaml:"username"`
	Timestamp time.Time `json:"timestamp-when-added" yaml:"timestamp-when-added"`
}

// Serialize converts the given resource into a serialized
// equivalent. No validation is performed.
func Serialize(res Resource) Serialized {
	return Serialized{
		Name:    res.Name,
		Type:    res.Type.String(),
		Path:    res.Path,
		Comment: res.Comment,

		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),
		Size:        res.Size,

		Username:  res.Username,
		Timestamp: res.Timestamp,
	}
}

// Deserialize converts the serialized resource back into a Resource.
// "placeholder" resources are treated appropriately.
func (s Serialized) Deserialize() (Resource, error) {
	res, err := s.deserialize()
	if err != nil {
		return res, errors.Trace(err)
	}

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

func (sr Serialized) deserialize() (Resource, error) {
	var res Resource

	resType, err := resource.ParseType(sr.Type)
	if err != nil {
		return res, errors.Trace(err)
	}

	origin, err := resource.ParseOrigin(sr.Origin)
	if err != nil {
		return res, errors.Trace(err)
	}

	// The fingerprint is the only "placeholder" field we have to
	// treat specially.
	var fp resource.Fingerprint
	if len(sr.Fingerprint) != 0 {
		fp, err = resource.NewFingerprint(sr.Fingerprint)
		if err != nil {
			return res, errors.Trace(err)
		}
	}

	res = Resource{
		Resource: resource.Resource{
			Meta: resource.Meta{
				Name:    sr.Name,
				Type:    resType,
				Path:    sr.Path,
				Comment: sr.Comment,
			},
			Origin:      origin,
			Revision:    sr.Revision,
			Fingerprint: fp,
			Size:        sr.Size,
		},
		Username:  sr.Username,
		Timestamp: sr.Timestamp,
	}

	return res, nil
}
