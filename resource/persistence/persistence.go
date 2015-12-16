// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/resource"
)

var logger = loggo.GetLogger("juju.resource.persistence")

// PersistenceBase exposes the core persistence functionality needed
// for resources.
type PersistenceBase interface {
	// All populates docs with the list of the documents corresponding
	// to the provided query.
	All(collName string, query, docs interface{}) error
}

// Persistence provides the persistence functionality for the
// Juju environment as a whole.
type Persistence struct {
	base PersistenceBase
}

// NewPersistence wraps the base in a new Persistence.
func NewPersistence(base PersistenceBase) *Persistence {
	return &Persistence{
		base: base,
	}
}

// ListResources returns the resource data for the given service ID.
func (p Persistence) ListResources(serviceID string) ([]resource.Resource, error) {
	logger.Tracef("listing all resources for service %q", serviceID)

	// TODO(ericsnow) Ensure that the service is still there?

	docs, err := p.resources(serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []resource.Resource
	for _, doc := range docs {
		res, err := doc.resource()
		if err != nil {
			return nil, errors.Trace(err)
		}
		results = append(results, res)
	}
	return results, nil
}
