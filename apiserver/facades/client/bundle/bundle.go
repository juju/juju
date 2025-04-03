// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"context"
	"strings"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	bundlechanges "github.com/juju/juju/internal/bundle/changes"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetCharm returns the charm by name, source and revision. Calling this method
	// will return all the data associated with the charm. It is not expected to
	// call this method for all calls, instead use the move focused and specific
	// methods. That's because this method is very expensive to call. This is
	// implemented for the cases where all the charm data is needed; model
	// migration, charm export, etc.
	GetCharm(ctx context.Context, locator applicationcharm.CharmLocator) (charm.Charm, applicationcharm.CharmLocator, bool, error)
}

// APIv8 provides the Bundle API facade for version 8. It drops IncludeSeries
// from ExportBundle params, and drops series entirely from ExportBundle output
type APIv8 struct {
	*BundleAPI
}

// BundleAPI implements the Bundle interface and is the concrete implementation
// of the API end point.
type BundleAPI struct {
	backend            Backend
	store              objectstore.ObjectStore
	authorizer         facade.Authorizer
	networkService     NetworkService
	applicationService ApplicationService
	logger             corelogger.Logger
}

// NewFacade provides the required signature for facade registration.
func newFacade(ctx facade.ModelContext) (*BundleAPI, error) {
	authorizer := ctx.Auth()
	st := ctx.State()

	return NewBundleAPI(
		NewStateShim(st),
		ctx.ObjectStore(),
		authorizer,
		ctx.DomainServices().Network(),
		ctx.DomainServices().Application(),
		ctx.Logger().Child("bundlechanges"),
	)
}

// NewBundleAPI returns the new Bundle API facade.
func NewBundleAPI(
	st Backend,
	store objectstore.ObjectStore,
	auth facade.Authorizer,
	networkService NetworkService,
	applicationService ApplicationService,
	logger corelogger.Logger,
) (*BundleAPI, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &BundleAPI{
		backend:            st,
		store:              store,
		authorizer:         auth,
		networkService:     networkService,
		applicationService: applicationService,
		logger:             logger,
	}, nil
}

type validators struct {
	verifyConstraints func(string) error
	verifyStorage     func(string) error
	verifyDevices     func(string) error
}

func (b *BundleAPI) doGetBundleChanges(
	ctx context.Context,
	args params.BundleChangesParams,
	vs validators,
) ([]bundlechanges.Change, []error, error) {
	dataSource, _ := charm.StreamBundleDataSource(strings.NewReader(args.BundleDataYAML), args.BundleURL)
	data, err := charm.ReadAndMergeBundleData(dataSource)
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot read bundle YAML")
	}
	if err := data.Verify(vs.verifyConstraints, vs.verifyStorage, vs.verifyDevices); err != nil {
		if verificationError, ok := err.(*charm.VerificationError); ok {
			validationErrors := make([]error, len(verificationError.Errors))
			for i, e := range verificationError.Errors {
				validationErrors[i] = e
			}
			return nil, validationErrors, nil
		}
		// This should never happen as Verify only returns verification errors.
		return nil, nil, errors.Annotate(err, "cannot verify bundle")
	}
	changes, err := bundlechanges.FromData(
		ctx,
		bundlechanges.ChangesConfig{
			Bundle:    data,
			BundleURL: args.BundleURL,
			Logger:    b.logger,
		})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return changes, nil, nil
}

// GetChangesMapArgs returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
// V4 GetChangesMapArgs is not supported on anything less than v4
func (b *BundleAPI) GetChangesMapArgs(ctx context.Context, args params.BundleChangesParams) (params.BundleChangesMapArgsResults, error) {
	vs := validators{
		verifyConstraints: func(s string) error {
			_, err := constraints.Parse(s)
			return err
		},
		verifyStorage: func(s string) error {
			_, err := storage.ParseDirective(s)
			return err
		},
		verifyDevices: func(s string) error {
			_, err := devices.ParseConstraints(s)
			return err
		},
	}
	return b.doGetBundleChangesMapArgs(ctx, args, vs, func(changes []bundlechanges.Change, results *params.BundleChangesMapArgsResults) error {
		results.Changes = make([]*params.BundleChangesMapArgs, len(changes))
		results.Errors = make([]string, len(changes))
		for i, c := range changes {
			args, err := c.Args()
			if err != nil {
				results.Errors[i] = err.Error()
				continue
			}
			results.Changes[i] = &params.BundleChangesMapArgs{
				Id:       c.Id(),
				Method:   c.Method(),
				Args:     args,
				Requires: c.Requires(),
			}
		}
		return nil
	})
}

func (b *BundleAPI) doGetBundleChangesMapArgs(
	ctx context.Context,
	args params.BundleChangesParams,
	vs validators,
	postProcess func([]bundlechanges.Change, *params.BundleChangesMapArgsResults) error,
) (params.BundleChangesMapArgsResults, error) {
	var results params.BundleChangesMapArgsResults
	changes, validationErrors, err := b.doGetBundleChanges(ctx, args, vs)
	if err != nil {
		return results, errors.Trace(err)
	}
	if len(validationErrors) > 0 {
		results.Errors = make([]string, len(validationErrors))
		for k, v := range validationErrors {
			results.Errors[k] = v.Error()
		}
		return results, nil
	}
	err = postProcess(changes, &results)
	return results, err
}

// ExportBundle exports the current model configuration as bundle.
func (*BundleAPI) ExportBundle(context.Context, params.ExportBundleParams) (params.StringResult, error) {
	return params.StringResult{}, apiservererrors.ServerError(internalerrors.Errorf(
		"Juju 4.0 doesn't support exporting bundles").Add(coreerrors.NotImplemented))
}
