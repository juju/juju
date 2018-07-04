// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle defines an API endpoint for functions dealing with bundles.
package bundle

import (
	"strings"

	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/description"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type Backend interface {
	migration.StateExporter
}

// bundleAPI implements the Bundle interface and is the concrete implementation
// of the API end point.
type BundleAPI struct {
	backend    Backend
	authorizer facade.Authorizer
	modelTag   names.ModelTag
}

func (b *BundleAPI) checkCanRead() error {
	canRead, err := b.authorizer.HasPermission(permission.ReadAccess, b.modelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

// NewFacade provides the required signature for facade registration.
func NewFacade(ctx facade.Context) (*BundleAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	tag, err := names.ParseModelTag(model.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &BundleAPI{
		authorizer: authorizer,
		backend:    getState(st, model),
		modelTag:   tag,
	}, nil
}

// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

var getState = func(st *state.State, m *state.Model) Backend {
	return stateShim{st, m}
}

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
func (b *BundleAPI) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
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
	verifyDevices := func(s string) error {
		_, err := devices.ParseConstraints(s)
		return err
	}
	if err := data.Verify(verifyConstraints, verifyStorage, verifyDevices); err != nil {
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
	changes, err := bundlechanges.FromData(
		bundlechanges.ChangesConfig{
			Bundle: data,
			Logger: loggo.GetLogger("juju.apiserver.bundlechanges"),
		})
	if err != nil {
		return results, err
	}
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

// ExportBundle exports the current model configuration as bundle.
func (b *BundleAPI) ExportBundle() ([]byte, error) {
	model, err := b.backend.Export()
	if err != nil {
		return nil, errors.Trace(err)
	}

	bytes, err := description.Serialize(model)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bytes, nil
}
