// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transformer

import "github.com/juju/juju/internal/errors"

const (
	// ErrUnknownSourceVersion is returned when Adapt is asked to process a
	// payload whose declared source version is not present in the
	// configured chain. A source version later than the target also
	// surfaces as this error because a later version is by construction
	// not in the transformer's forward chain.
	ErrUnknownSourceVersion = errors.ConstError("unknown source export version")

	// ErrPayloadTypeMismatch is returned when a payload's runtime Go type
	// does not match the Src type the registered transformer expects.
	ErrPayloadTypeMismatch = errors.ConstError("payload Go type does not match its declared version")

	// ErrMissingTransformer is returned by NewTransformer when an adjacent pair
	// in export.ExportVersions has no registered transformer.
	ErrMissingTransformer = errors.ConstError("no transformer registered for version pair")

	// ErrDuplicateTransformer is returned by NewTransformer when two
	// registrations cover the same (from, to) pair.
	ErrDuplicateTransformer = errors.ConstError("duplicate transformer registered for version pair")
)
