// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

// TODO(ericsnow) Change "tags" to "labels"?

// FormattedPayload holds the formatted representation of a Payload.
type FormattedPayload struct {
	// These fields are exported for the sake of serialization.
	Unit    string   `json:"unit" yaml:"unit"`
	Machine string   `json:"machine" yaml:"machine"`
	ID      string   `json:"id" yaml:"id"`
	Type    string   `json:"type" yaml:"type"`
	Class   string   `json:"payload-class" yaml:"payload-class"`
	Labels  []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Status  string   `json:"status" yaml:"status"`
}
