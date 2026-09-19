// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"

	"github.com/juju/errors"
)

const (
	// RelationCountCorruptError indicates the relation count recorded
	// on a remote application does not match the number of relations
	// that actually reference it.
	RelationCountCorruptError = errors.ConstError("relation count for remote application is corrupt")
)

// NewRelationCountCorruptError returns an error that satisfies
// errors.Is(err, RelationCountCorruptError), reporting the recorded
// count, the number of relations that actually reference the named
// remote application, and how to clean up the application.
func NewRelationCountCorruptError(name string, recorded, actual int) error {
	// TODO - move the user facing message to the CLI.
	return errors.WithType(
		fmt.Errorf("relation count for remote application %q is corrupt: recorded %d, actual relations %d; "+
			"force remove the saas application with 'juju remove-saas %s --force' to clean up its relations",
			name, recorded, actual, name),
		RelationCountCorruptError,
	)
}
