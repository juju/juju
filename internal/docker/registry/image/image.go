// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package image

import (
	"encoding/json"

	"github.com/juju/errors"

	"github.com/juju/juju/core/semversion"
)

// ImageInfo defines image versions information.
type ImageInfo struct {
	version semversion.Number
}

// AgentVersion returns the image version.
func (info ImageInfo) AgentVersion() semversion.Number {
	return info.version
}

// String returns string format of the image version.
func (info ImageInfo) String() string {
	return info.version.String()
}

// MarshalJSON implements json.Marshaler.
func (info ImageInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(info.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (info *ImageInfo) UnmarshalJSON(data []byte) (err error) {
	var s string
	if err = json.Unmarshal(data, &s); err != nil {
		return err
	}
	if info.version, err = semversion.Parse(s); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// MarshalYAML implements yaml.v2.Marshaller interface.
func (info ImageInfo) MarshalYAML() (interface{}, error) {
	return info.String(), nil
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (info *ImageInfo) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	err := unmarshal(&s)
	if err != nil {
		return errors.Trace(err)
	}
	info.version, err = semversion.Parse(s)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// NewImageInfo creates an imageInfo.
func NewImageInfo(ver semversion.Number) ImageInfo {
	return ImageInfo{version: ver}
}
