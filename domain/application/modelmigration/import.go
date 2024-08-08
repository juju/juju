// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v8"
	"github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, registry storage.ProviderRegistry, logger logger.Logger) {
	coordinator.Add(&importOperation{
		registry:     registry,
		logger:       logger,
		charmOrigins: make(map[string]*corecharm.Origin),
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	logger logger.Logger

	service  ImportService
	registry storage.ProviderRegistry

	charmOrigins map[string]*corecharm.Origin
}

// ImportService defines the application service used to import applications
// from another controller model to this controller.
type ImportService interface {
	// CreateApplication registers the existence of an application in the model.
	CreateApplication(context.Context, string, internalcharm.Charm, corecharm.Origin, service.AddApplicationArgs, ...service.AddUnitArg) (coreapplication.ID, error)
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import applications"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB(), i.logger),
		i.registry,
		i.logger,
	)
	return nil
}

func ptr[T any](v T) *T {
	return &v
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		unitArgs := make([]service.AddUnitArg, 0, len(app.Units()))
		for _, unit := range app.Units() {
			arg := service.AddUnitArg{
				UnitName: ptr(unit.Name()),
			}
			if unit.PasswordHash() != "" {
				arg.PasswordHash = ptr(unit.PasswordHash())
			}
			if cc := unit.CloudContainer(); cc != nil {
				cldContainer := &service.CloudContainerParams{}
				cldContainer.Address, cldContainer.AddressOrigin = i.makeAddress(cc.Address())
				if cc.ProviderId() != "" {
					cldContainer.ProviderId = ptr(cc.ProviderId())
				}
				if len(cc.Ports()) > 0 {
					cldContainer.Ports = ptr(cc.Ports())
				}
				arg.CloudContainer = cldContainer
			}
			unitArgs = append(unitArgs, arg)
		}

		// TODO (stickupkid): This is a temporary solution until we have a
		// charms in the description model.
		url, err := internalcharm.ParseURL(app.CharmURL())
		if err != nil {
			return fmt.Errorf("parse charm URL %q: %w", app.CharmURL(), err)
		}

		origin, err := i.makeCharmOrigin(app)
		if err != nil {
			return fmt.Errorf("parse charm origin %v: %w", app.CharmOrigin(), err)
		}

		_, err = i.service.CreateApplication(
			ctx, app.Name(), &stubCharm{
				name:     url.Name,
				revision: url.Revision,
			}, *origin, service.AddApplicationArgs{}, unitArgs...,
		)
		if err != nil {
			return fmt.Errorf(
				"import model application %q with %d units: %w",
				app.Name(), len(app.Units()), err,
			)
		}
	}

	return nil
}

// makeCharmOrigin returns the charm origin for an application
//
// Ensure ID, Hash and Channel are dropped from local charm.
// Due to LP:1986547: where the track is missing from the effective channel it implicitly
// resolves to 'latest' if the charm does not have a default channel defined. So if the
// received channel has no track, we can be confident it should be 'latest'
func (i *importOperation) makeCharmOrigin(a description.Application) (*corecharm.Origin, error) {
	sourceOrigin := a.CharmOrigin()
	if sourceOrigin == nil {
		return nil, errors.Errorf("nil charm origin importing application %q", a.Name())
	}
	_, err := internalcharm.ParseURL(a.CharmURL())
	if err != nil {
		return nil, errors.Trace(err)
	}

	if foundOrigin, ok := i.charmOrigins[a.CharmURL()]; ok {
		return foundOrigin, nil
	}

	var channel *internalcharm.Channel
	serialized := sourceOrigin.Channel()
	if serialized != "" && corecharm.CharmHub.Matches(sourceOrigin.Source()) {
		c, err := internalcharm.ParseChannelNormalize(serialized)
		if err != nil {
			return nil, errors.Trace(err)
		}
		track := c.Track
		if track == "" {
			track = "latest"
		}
		channel = &internalcharm.Channel{
			Track:  track,
			Risk:   c.Risk,
			Branch: c.Branch,
		}
	}

	p, err := corecharm.ParsePlatformNormalize(sourceOrigin.Platform())
	if err != nil {
		return nil, errors.Trace(err)
	}
	platform := corecharm.Platform{
		Architecture: p.Architecture,
		OS:           p.OS,
		Channel:      p.Channel,
	}

	rev := sourceOrigin.Revision()
	// We can hardcode type to charm as we never store bundles in state.
	var origin *corecharm.Origin
	if corecharm.Local.Matches(sourceOrigin.Source()) {
		origin = &corecharm.Origin{
			Source:   corecharm.Local,
			Type:     "charm",
			Revision: &rev,
			Platform: platform,
		}
	} else if corecharm.CharmHub.Matches(sourceOrigin.Source()) {
		origin = &corecharm.Origin{
			Source:   corecharm.CharmHub,
			Type:     "charm",
			Revision: &rev,
			Platform: platform,
			ID:       sourceOrigin.ID(),
			Hash:     sourceOrigin.Hash(),
			Channel:  channel,
		}
	} else {
		return nil, errors.Errorf("unrecognised charm origin %q", sourceOrigin.Source())
	}

	i.charmOrigins[a.CharmURL()] = origin
	return origin, nil
}

func (i *importOperation) makeAddress(addr description.Address) (*network.SpaceAddress, *network.Origin) {
	if addr == nil {
		return nil, nil
	}

	result := &network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: addr.Value(),
			Type:  network.AddressType(addr.Type()),
			Scope: network.Scope(addr.Scope()),
		},
		SpaceID: addr.SpaceID(),
	}

	// Addresses are placed in the default space if no space ID is set.
	if result.SpaceID == "" {
		result.SpaceID = network.AlphaSpaceId
	}

	return result, ptr(network.Origin(addr.Origin()))
}

type stubCharm struct {
	name     string
	revision int
}

var _ internalcharm.Charm = stubCharm{}

func (s stubCharm) Meta() *internalcharm.Meta {
	return &internalcharm.Meta{
		Name: s.name,
	}
}

func (s stubCharm) Actions() *internalcharm.Actions {
	return &internalcharm.Actions{}
}

func (s stubCharm) Config() *internalcharm.Config {
	return &internalcharm.Config{}
}

func (s stubCharm) Manifest() *internalcharm.Manifest {
	return &internalcharm.Manifest{}
}

func (s stubCharm) Revision() int {
	return s.revision
}
