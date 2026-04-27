// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelimport wires the model export-version transformer together.
// The transformer framework lives in the transformer subpackage; individual
// version-step transformations live under transformer/transforms/. This package
// owns the registered list (in the generated registered.go) and the
// NewTransformer entry point so that the framework itself does not need to
// depend on the transform packages (which would cause an import cycle).
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
