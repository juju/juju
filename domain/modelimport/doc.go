// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelimport wires the model export-version transformer together.
//
// The transformer framework lives in the transformer subpackage; individual
// version-step transformations live under transformer/transforms/. This package
// owns the registered list, in the generated registered.go, and the
// NewTransformer entry point so that the framework itself does not need to
// depend on the transform packages.
package modelimport
