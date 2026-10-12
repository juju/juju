// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/internal/errors"
)

// row is one exported database row: column name to value.
type row map[string]any

// dump is a decoded database dump: table name to rows.
type dump struct {
	// Version is the export payload version that produced the dump.
	Version string

	// Tables maps table name to exported rows.
	Tables map[string][]row
}

// dumpEnvelope mirrors the export wire shape: a version and a payload
// whose keys are table names.
type dumpEnvelope struct {
	Version string           `yaml:"version"`
	Payload map[string][]row `yaml:"payload"`
}

// decodeDump parses a YAML database dump into tables. Column names in
// rows are database column names; values are YAML scalars, with
// timestamps decoded as time.Time by the YAML library.
func decodeDump(data []byte) (*dump, error) {
	var env dumpEnvelope
	if err := yaml.Unmarshal(data, &env); err != nil {
		return nil, errors.Errorf("decoding dump: %w", err)
	}
	if len(env.Payload) == 0 {
		return nil, errors.Errorf("decoding dump: empty payload")
	}
	return &dump{Version: env.Version, Tables: env.Payload}, nil
}
