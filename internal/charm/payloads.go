// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"

	"github.com/juju/names/v5"
	"github.com/juju/schema"
)

var payloadClassSchema = schema.FieldMap(
	schema.Fields{
		"type": schema.String(),
	},
	schema.Defaults{},
)

// PayloadClass holds the information about a payload class, as stored
// in a charm's metadata.
type PayloadClass struct {
	// Name identifies the payload class.
	Name string

	// Type identifies the type of payload (e.g. kvm, docker).
	Type string
}

func parsePayloadClasses(data interface{}) map[string]PayloadClass {
	if data == nil {
		return nil
	}

	result := make(map[string]PayloadClass)
	for name, val := range data.(map[string]interface{}) {
		result[name] = parsePayloadClass(name, val)
	}

	return result
}

func parsePayloadClass(name string, data interface{}) PayloadClass {
	payloadClass := PayloadClass{
		Name: name,
	}
	if data == nil {
		return payloadClass
	}
	pcMap := data.(map[string]interface{})

	if val := pcMap["type"]; val != nil {
		payloadClass.Type = val.(string)
	}

	return payloadClass
}

// Validate checks the payload class to ensure its data is valid.
func (pc PayloadClass) Validate() error {
	if pc.Name == "" {
		return fmt.Errorf("payload class missing name")
	}
	if !names.IsValidPayload(pc.Name) {
		return fmt.Errorf("invalid payload class %q", pc.Name)
	}

	if pc.Type == "" {
		return fmt.Errorf("payload class missing type")
	}

	return nil
}
