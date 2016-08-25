// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// Payload represents a charm payload for a unit.
type Payload interface {
	Name() string
	Type() string
	RawID() string
	State() string
	Labels() []string
}

type payloads struct {
	Version   int        `yaml:"version"`
	Payloads_ []*payload `yaml:"payloads"`
}

type payload struct {
	Name_   string   `yaml:"name"`
	Type_   string   `yaml:"type"`
	RawID_  string   `yaml:"raw-id"`
	State_  string   `yaml:"state"`
	Labels_ []string `yaml:"labels,omitempty"`
}

// Name implements Payload.
func (p *payload) Name() string {
	return p.Name_
}

// Type implements Payload.
func (p *payload) Type() string {
	return p.Type_
}

// RawID implements Payload.
func (p *payload) RawID() string {
	return p.RawID_
}

// State implements Payload.
func (p *payload) State() string {
	return p.State_
}

// Labels implements Payload.
func (p *payload) Labels() []string {
	return p.Labels_
}

// PayloadArgs is an argument struct used to create a
// new internal payload type that supports the Payload interface.
type PayloadArgs struct {
	Name   string
	Type   string
	RawID  string
	State  string
	Labels []string
}

func newPayload(args PayloadArgs) *payload {
	return &payload{
		Name_:   args.Name,
		Type_:   args.Type,
		RawID_:  args.RawID,
		State_:  args.State,
		Labels_: args.Labels,
	}
}

func importPayloads(source map[string]interface{}) ([]*payload, error) {
	checker := versionedChecker("payloads")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "payloads version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := payloadDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["payloads"].([]interface{})
	return importPayloadList(sourceList, importFunc)
}

func importPayloadList(sourceList []interface{}, importFunc payloadDeserializationFunc) ([]*payload, error) {
	result := make([]*payload, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for payload %d, %T", i, value)
		}
		payload, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "payload %d", i)
		}
		result = append(result, payload)
	}
	return result, nil
}

type payloadDeserializationFunc func(map[string]interface{}) (*payload, error)

var payloadDeserializationFuncs = map[int]payloadDeserializationFunc{
	1: importPayloadV1,
}

func importPayloadV1(source map[string]interface{}) (*payload, error) {
	fields := schema.Fields{
		"name":   schema.String(),
		"type":   schema.String(),
		"raw-id": schema.String(),
		"state":  schema.String(),
		"labels": schema.List(schema.String()),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"labels": schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "payload v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})

	return &payload{
		Name_:   valid["name"].(string),
		Type_:   valid["type"].(string),
		RawID_:  valid["raw-id"].(string),
		State_:  valid["state"].(string),
		Labels_: convertToStringSlice(valid["labels"]),
	}, nil
}
