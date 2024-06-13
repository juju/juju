// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"context"
	"fmt"
	"net/url"

	"github.com/juju/errors"

	importererrors "github.com/juju/juju/internal/ssh/importer/errors"
)

// Importer is responsible for providing a pattern for importing a subjects
// public keys from multiple sources.
type Importer struct {
	// resolvers is a map of uri schema's to Resolvers.
	resolvers map[string]Resolver
}

// FetchPublicKeysForSubject takes a uri subject to fetch public keys for. The
// schema of the URI defines the resolver to use and the opqaue data defines the
// subject to fetch public key data for.
// Example URI's:
// - gh:tlm
// - lp:~wallyworld
//
// The following errors can be expected:
// - [errors.NotValid] when the opaque data for the URI is empty.
// - [importererrors.NoResolver] when no resolver is defined for the schema of
// the URI.
// - [importererrors.SubjectNotFound] when no subject exists for the [Resolver].
func (i *Importer) FetchPublicKeysForSubject(
	ctx context.Context,
	subject *url.URL,
) ([]string, error) {
	resolver, has := i.resolvers[subject.Scheme]
	if !has {
		return nil, fmt.Errorf(
			"%w for subject scheme %q",
			importererrors.NoResolver,
			subject.Scheme,
		)
	}

	if subject.Opaque == "" {
		return nil, fmt.Errorf("uri %w, subject cannot be empty", errors.NotValid)
	}

	authorisedKeys, err := resolver.PublicKeysForSubject(ctx, subject.Opaque)
	if err != nil {
		return nil, fmt.Errorf(
			"fetching authorised keys for subject %q using resolver %q: %w",
			subject.Opaque, subject.Scheme, err,
		)
	}

	return authorisedKeys, nil
}

// resolvers constructs a factory of resolvers for the various schema's we
// understand.
func resolvers(client Client) map[string]Resolver {
	client = userAgentSetter(client)
	return map[string]Resolver{
		"gh": &GithubResolver{client},
		"lp": &LaunchpadResolver{client},
	}
}

// NewImporter constructs a new [Importer] for importing a subject's public key.
func NewImporter(client Client) *Importer {
	return &Importer{
		resolvers: resolvers(client),
	}
}
