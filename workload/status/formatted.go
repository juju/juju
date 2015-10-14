// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"strings"
)

var allFields = strings.Split("unit machine id type payload-class tags status", " ")

// FormattedPayload holds the formatted representation of a Payload.
type FormattedPayload struct {
	// These fields are exported for the sake of serialization.
	Unit    string   `json:"unit" yaml:"unit"`
	Machine string   `json:"machine" yaml:"machine"`
	ID      string   `json:"id" yaml:"id"`
	Type    string   `json:"type" yaml:"type"`
	Class   string   `json:"payload-class" yaml:"payload-class"`
	Tags    []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Status  string   `json:"status" yaml:"status"`
}

func (fp FormattedPayload) lookUp(field string) string {
	switch field {
	case "unit":
		return fp.Unit
	case "machine":
		return fp.Machine
	case "id":
		return fp.ID
	case "type":
		return fp.Type
	case "payload-class":
		if fp.Class == "" {
			return "-"
		}
		return fp.Class
	case "tags":
		return strings.Join(fp.Tags, " ")
	case "status":
		return fp.Status
	default:
		return ""
	}
}

func (fp FormattedPayload) strings(fields ...string) []string {
	if len(fields) == 0 {
		fields = allFields
	}

	var result []string
	for _, field := range fields {
		result = append(result, fp.lookUp(field))
	}
	return result
}
