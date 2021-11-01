// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package image

import (
	"encoding/json"

	"github.com/juju/errors"
	"github.com/juju/version/v2"
)

type ImageInfo struct {
	version version.Number
}

func (info ImageInfo) AgentVersion() version.Number {
	return info.version
}

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
	info.version, err = version.Parse(s)
	if err != nil {
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
	info.version, err = version.Parse(s)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// NewImageInfo creates an imageInfo.
func NewImageInfo(ver version.Number) ImageInfo {
	return ImageInfo{version: ver}
}
