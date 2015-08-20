// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	goyaml "gopkg.in/yaml.v1"
)

func parseDefinition(name string, data []byte) (*charm.Workload, error) {
	raw := make(map[interface{}]interface{})
	if err := goyaml.Unmarshal(data, raw); err != nil {
		return nil, errors.Trace(err)
	}
	definition, err := charm.ParseWorkload(name, raw)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if definition.Name == "" {
		definition.Name = name
	} else if definition.Name != name {
		return nil, errors.Errorf("workload name mismatch; %q != %q", definition.Name, name)
	}
	return definition, nil
}

// parseUpdate builds a charm.WorkloadFieldValue from an update string.
func parseUpdate(update string) (charm.WorkloadFieldValue, error) {
	var pfv charm.WorkloadFieldValue

	parts := strings.SplitN(update, ":", 2)
	if len(parts) == 1 {
		return pfv, errors.Errorf("missing value")
	}
	pfv.Field, pfv.Value = parts[0], parts[1]

	if pfv.Field == "" {
		return pfv, errors.Errorf("missing field")
	}
	if pfv.Value == "" {
		return pfv, errors.Errorf("missing value")
	}

	fieldParts := strings.SplitN(pfv.Field, "/", 2)
	if len(fieldParts) == 2 {
		pfv.Field = fieldParts[0]
		pfv.Subfield = fieldParts[1]
	}

	return pfv, nil
}

// parseUpdates parses the updates list in to a charm.WorkloadFieldValue list.
func parseUpdates(updates []string) ([]charm.WorkloadFieldValue, error) {
	var results []charm.WorkloadFieldValue
	for _, update := range updates {
		pfv, err := parseUpdate(update)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results = append(results, pfv)
	}
	return results, nil
}
