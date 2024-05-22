// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/v4/arch"
	"gopkg.in/yaml.v2"
)

// Manifest represents the recording of the building of the charm or bundle.
// The manifest file should represent the metadata.yaml, but a lot more
// information.
type Manifest struct {
	Bases []Base `yaml:"bases"`
}

// Validate checks the manifest to ensure there are no empty names, nor channels,
// and that architectures are supported.
func (m *Manifest) Validate() error {
	for _, b := range m.Bases {
		if err := b.Validate(); err != nil {
			return errors.Annotate(err, "validating manifest")
		}
	}
	return nil
}

func (m *Manifest) UnmarshalYAML(f func(interface{}) error) error {
	raw := make(map[interface{}]interface{})
	err := f(&raw)
	if err != nil {
		return err
	}

	v, err := schema.List(baseSchema).Coerce(raw["bases"], nil)
	if err != nil {
		return errors.Annotatef(err, "coerce")
	}

	newV, ok := v.([]interface{})
	if !ok {
		return errors.Annotatef(err, "converting")
	}
	bases, err := parseBases(newV)
	if err != nil {
		return err
	}

	*m = Manifest{Bases: bases}
	return nil
}

func parseBases(input interface{}) ([]Base, error) {
	var err error
	if input == nil {
		return nil, nil
	}
	var res []Base
	for _, v := range input.([]interface{}) {
		var base Base
		baseMap := v.(map[string]interface{})
		if value, ok := baseMap["name"]; ok {
			base.Name = value.(string)
		}
		if value, ok := baseMap["channel"]; ok {
			base.Channel, err = ParseChannelNormalize(value.(string))
			if err != nil {
				return nil, errors.Annotatef(err, "parsing channel %q", value.(string))
			}
		}
		base.Architectures = parseArchitectureList(baseMap["architectures"])
		err = base.Validate()
		if err != nil {
			return nil, errors.Trace(err)
		}
		res = append(res, base)
	}
	return res, nil
}

// ReadManifest reads in a Manifest from a charm's manifest.yaml. Some of
// validation is done when unmarshalling the manifest, including
// verification that the base.Name is a supported operating system.  Full
// validation done by calling Validate().
func ReadManifest(r io.Reader) (*Manifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var manifest *Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, errors.Annotatef(err, "manifest")
	}
	if manifest == nil {
		return nil, errors.Annotatef(err, "invalid base in manifest")
	}
	return manifest, nil
}

var baseSchema = schema.FieldMap(
	schema.Fields{
		"name":          schema.String(),
		"channel":       schema.String(),
		"architectures": schema.List(schema.String()),
	}, schema.Defaults{
		"name":          schema.Omit,
		"channel":       schema.Omit,
		"architectures": schema.Omit,
	})

func parseArchitectureList(list interface{}) []string {
	if list == nil {
		return nil
	}
	slice := list.([]interface{})
	result := make([]string, 0, len(slice))
	for _, elem := range slice {
		result = append(result, arch.NormaliseArch(elem.(string)))
	}
	return result
}
