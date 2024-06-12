// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import "context"

// Resolver describes a service that can fetch public key information for a
// supplied subject.
type Resolver interface {
	// PublicKeysForSubject is responsible for taking a subject that exists
	// within this resolver's domain and returning all of the public ssh keys
	// for that subject.
	// The following errors can be expected:
	// - [SubjectNotFound] when the subject being asked for does not exist in
	// the resolvers domain.
	PublicKeysForSubject(context.Context, string) ([]string, error)
}
