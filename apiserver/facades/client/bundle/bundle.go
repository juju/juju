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

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.apiserver.bundle")

// APIv1 provides the Bundle API facade for version 1.
type APIv1 struct {
	*APIv2
}

// APIv2 provides the Bundle API facade for version 2.
type APIv2 struct {
	*BundleAPI
}

// BundleAPI implements the Bundle interface and is the concrete implementation
// of the API end point.
type BundleAPI struct {
	backend    Backend
	authorizer facade.Authorizer
	modelTag   names.ModelTag
}

// NewFacadeV1 provides the signature required for facade registration
// version 1.
func NewFacadeV1(ctx facade.Context) (*APIv1, error) {
	api, err := NewFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &APIv1{api}, nil
}

// NewFacadeV2 provides the signature required for facade registration
// for version 2.
func NewFacadeV2(ctx facade.Context) (*APIv2, error) {
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// NewFacade provides the required signature for facade registration.
func newFacade(ctx facade.Context) (*BundleAPI, error) {
	authorizer := ctx.Auth()
	st := ctx.State()

	return NewBundleAPI(
		st,
		authorizer,
		names.NewModelTag(st.ModelUUID()),
	)
}

// NewBundleAPI returns the new Bundle API facade.
func NewBundleAPI(
	st Backend,
	auth facade.Authorizer,
	tag names.ModelTag,
) (*BundleAPI, error) {
	if !auth.AuthClient() {
		return nil, common.ErrPerm
	}

	return &BundleAPI{
		backend:    st,
		authorizer: auth,
		modelTag:   tag,
	}, nil
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

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
// V1 GetChanges did not support device.
func (b *APIv1) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
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
	if err := data.Verify(verifyConstraints, verifyStorage, nil); err != nil {
		if verificationError, ok := err.(*charm.VerificationError); ok {
			results.Errors = make([]string, len(verificationError.Errors))
			for i, e := range verificationError.Errors {
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
		var guiArgs []interface{}
		switch c := c.(type) {
		case *bundlechanges.AddApplicationChange:
			guiArgs = c.GUIArgsLegacy()
		default:
			guiArgs = c.GUIArgs()
		}
		results.Changes[i] = &params.BundleChange{
			Id:       c.Id(),
			Method:   c.Method(),
			Args:     guiArgs,
			Requires: c.Requires(),
		}
	}
	return results, nil
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
		if verificationError, ok := err.(*charm.VerificationError); ok {
			results.Errors = make([]string, len(verificationError.Errors))
			for i, e := range verificationError.Errors {
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
func (b *BundleAPI) ExportBundle() (params.StringResult, error) {
	return params.StringResult{}, nil
}

// ExportBundle is not in V1 API.
// Mask the new method from V1 API.
func (b *APIv1) ExportBundle() (_, _ struct{}) { return }
