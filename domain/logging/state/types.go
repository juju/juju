// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "database/sql"

type lokiConfig struct {
	UUID               string       `db:"uuid"`
	Endpoint           string       `db:"endpoint"`
	CACertificate      *string      `db:"ca_cert"`
	InsecureSkipVerify sql.NullBool `db:"insecure_skip_verify"`
}

// nsBoolToNil converts a pointer to bool into a nullable sql.NullBool suitable
// for database storage. A nil input maps to invalid NullBool.
func nsBoolToNil(b *bool) sql.NullBool {
	if b == nil {
		return sql.NullBool{Valid: false}
	}
	return sql.NullBool{Valid: true, Bool: *b}
}

// nsBoolToPtr converts a nullable sql.NullBool into a pointer to bool.
// An invalid NullBool maps to nil, while True/False map to boolean pointers.
func nsBoolToPtr(nb sql.NullBool) *bool {
	if !nb.Valid {
		return nil
	}
	b := nb.Bool
	return &b
}

// NamespaceForWatchLokiConfig returns the namespace identifier used for
// watching Loki config changes.
func (*State) NamespaceForWatchLokiConfig() string {
	return "logging_loki_config"
}
