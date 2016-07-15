// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package charmrepo implements access to charm repositories.

package charmrepo // import "gopkg.in/juju/charmrepo.v2-unstable"

import (
	"fmt"

	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
)

var logger = loggo.GetLogger("juju.charm.charmrepo")

// Interface represents a charm repository (a collection of charms).
type Interface interface {
	// Get returns the charm referenced by curl.
	Get(curl *charm.URL) (charm.Charm, error)

	// GetBundle returns the bundle referenced by curl.
	GetBundle(curl *charm.URL) (charm.Bundle, error)

	// Resolve resolves the given reference to a canonical form which refers
	// unambiguously to a specific revision of an entity. If the entity
	// is a charm that may support more than one series, canonRef.Series will
	// be empty and supportedSeries will hold the list of series supported by
	// the charm with the preferred series first.
	// If ref holds a series, then Resolve will always ensure that the returned
	// entity supports that series.
	Resolve(ref *charm.URL) (canonRef *charm.URL, supportedSeries []string, err error)
}

// InferRepository returns a charm repository inferred from the provided charm
// or bundle reference.
// Charm store references will use the provided parameters.
// Local references will use the provided path.
func InferRepository(ref *charm.URL, charmStoreParams NewCharmStoreParams, localRepoPath string) (Interface, error) {
	switch ref.Schema {
	case "cs":
		return NewCharmStore(charmStoreParams), nil
	case "local":
		return NewLocalRepository(localRepoPath)
	}
	// TODO fix this error message to reference bundles too?
	return nil, fmt.Errorf("unknown schema for charm reference %q", ref)
}
