// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repositories

import (
	"io"

	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
)

// EntityRepository exposes the functionality needed to access data from
// a repository.  If the data is not in state, get it from charm hub or
// charm store and store in state. The operations apply to a single application
// (or unit).
type EntityRepository interface {
	// GetResource returns the resource data for the identified resource.
	GetResource(name string) (resource.Resource, error)

	// SetResource stores the resource in the local cache.
	SetResource(res charmresource.Resource, reader io.Reader, _ state.IncrementCharmModifiedVersionType) (resource.Resource, error)

	// OpenResource returns metadata about the resource, and a reader
	// for the resource.
	OpenResource(name string) (resource.Resource, io.ReadCloser, error)
}

// operationsRepository is a wrapper around EntityRepository. It supports
// the operations type.
type operationsRepository struct {
	repo EntityRepository
}

// get retrieves the resource info and data from a repo. If only
// the info is found then the returned reader will be nil. If a
// repo is not in use then errors.NotFound is returned.
func (cfo operationsRepository) get(name string) (resource.Resource, io.ReadCloser, error) {
	if cfo.repo == nil {
		return resource.Resource{}, nil, errors.NotFoundf("resource %q", name)
	}

	res, reader, err := cfo.repo.OpenResource(name)
	if errors.IsNotFound(err) {
		reader = nil
		res, err = cfo.repo.GetResource(name)
	}
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}

// set stores the resource info and data in a repo, if there is one.
// If no repo is in use then this is a no-op. Note that the returned
// reader may or may not be the same one that was passed in.
func (cfo operationsRepository) set(chRes charmresource.Resource, reader io.ReadCloser, incrementCharmModifiedVersion state.IncrementCharmModifiedVersionType) (_ resource.Resource, _ io.ReadCloser, err error) {
	if cfo.repo == nil {
		res := resource.Resource{
			Resource: chRes,
		}
		return res, reader, nil // a no-op
	}
	defer func() {
		if err != nil {
			// With no err, the reader was closed down in unitSetter Read().
			// Closing here with no error leads to a panic in Read, and the
			// unit's resource doc is never cleared of it's pending status.
			_ = reader.Close()
		}
	}()
	res, err := cfo.repo.SetResource(chRes, reader, incrementCharmModifiedVersion)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	// Make sure to use the potentially updated resource details.
	res, reader, err = cfo.repo.OpenResource(res.Name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}
