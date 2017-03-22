// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle defines an API endpoint for functions dealing with bundles.
package bundle

import (
	"strings"

	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// NewFacade provides the required signature for facade registration.
func NewFacade(_ *state.State, _ facade.Resources, auth facade.Authorizer) (Bundle, error) {
	return NewBundle(auth)
}

// NewBundle creates and returns a new Bundle API facade.
func NewBundle(auth facade.Authorizer) (Bundle, error) {
	if !auth.AuthClient() {
		return nil, common.ErrPerm
	}
	return &bundleAPI{}, nil
}

// Bundle defines the API endpoint used to retrieve bundle changes.
type Bundle interface {
	// GetChanges returns the list of changes required to deploy the given
	// bundle data.
	GetChanges(params.BundleChangesParams) (params.BundleChangesResults, error)
}

// bundleAPI implements the Bundle interface and is the concrete implementation
// of the API end point.
type bundleAPI struct{}

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
func (b *bundleAPI) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	var results params.BundleChangesResults
	data, err := charm.ReadBundleData(strings.NewReader(args.BundleDataYAML))
	if err != nil {
		return results, errors.Annotate(err, "cannot read bundle YAML")
	}
	verifyConstraints := func(s string) error {
		_, err := constraints.Parse(s)
		return err
	}
	verifyStorage := func(s string) error {
		_, err := storage.ParseConstraints(s)
		return err
	}
	if err := data.Verify(verifyConstraints, verifyStorage); err != nil {
		if err, ok := err.(*charm.VerificationError); ok {
			results.Errors = make([]string, len(err.Errors))
			for i, e := range err.Errors {
				results.Errors[i] = e.Error()
			}
			return results, nil
		}
		// This should never happen as Verify only returns verification errors.
		return results, errors.Annotate(err, "cannot verify bundle")
	}
	changes := bundlechanges.FromData(data)
	results.Changes = make([]*params.BundleChange, len(changes))
	for i, c := range changes {
		results.Changes[i] = &params.BundleChange{
			Id:       c.Id(),
			Method:   c.Method(),
			Args:     c.GUIArgs(),
			Requires: c.Requires(),
		}
	}
	return results, nil
}
