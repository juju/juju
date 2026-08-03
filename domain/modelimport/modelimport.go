// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/modelimport/transformer"
)

// NewTransformer returns a transformer that walks payloads up to the current
// target schema format version ([export.ExportVersions]'s last entry).
// It returns an error if the registered chain is malformed (missing
// step, duplicate step, or empty version list).
func NewTransformer() (*transformer.Transformer, error) {
	return transformer.NewTransformer(registered, export.ExportVersions)
}
