// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"bytes"
	"encoding/json"
	"io"
)

// CloneParams defines attributes for drive cloning operation
type CloneParams struct {
	Affinities []string
	Media      string
	Name       string
}

func (c *CloneParams) makeJSONReader() (io.Reader, error) {
	if c == nil {
		return nil, nil
	}

	var m = make(map[string]interface{})
	if len(c.Affinities) > 0 {
		m["affinities"] = c.Affinities
	}
	if c.Media != "" {
		m["media"] = c.Media
	}
	if c.Name != "" {
		m["name"] = c.Name
	}

	bb, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(bb), nil
}
