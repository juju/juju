// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/description/v12"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
)

// RegisterImportRelationNetworks registers the relation networks import
// operation with the given coordinator. The operation must be registered
// after the relations import operation, as relation networks are located by
// relation key and can only be imported once the relations exist.
func RegisterImportRelationNetworks(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importRelationNetworksOperation{
		clock:  clock,
		logger: logger,
	})
}

// RelationNetworkImportService provides a subset of the cross model relation
// domain service methods needed for relation networks import.
type RelationNetworkImportService interface {
	// ImportRelationNetworks adds the relation networks being migrated to
	// the current model.
	ImportRelationNetworks(ctx context.Context, networks []crossmodelrelation.RelationNetworkImport) error
}

type importRelationNetworksOperation struct {
	modelmigration.BaseOperation

	importService RelationNetworkImportService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importRelationNetworksOperation) Name() string {
	return "import relation networks"
}

// Setup implements Operation.
func (i *importRelationNetworksOperation) Setup(scope modelmigration.Scope) error {
	i.importService = service.NewMigrationService(
		modelstate.NewState(scope.ModelDB(), "", i.clock, i.logger),
		i.logger,
	)
	return nil
}

// Execute the import of the relation networks contained in the model.
func (i *importRelationNetworksOperation) Execute(ctx context.Context, model description.Model) error {
	networks, err := extractRelationNetworks(model)
	if err != nil {
		return errors.Errorf("extracting relation networks: %w", err)
	}

	// Each network is imported on its own, so that a network referencing a
	// relation that was not migrated, for example because the relation was
	// removed from the source model before the export, only skips that
	// network instead of failing the whole migration.
	for _, network := range networks {
		err := i.importService.ImportRelationNetworks(ctx, []crossmodelrelation.RelationNetworkImport{network})
		if errors.Is(err, relationerrors.RelationNotFound) {
			i.logger.Warningf(ctx, "skipping relation networks for relation %q: %v",
				network.RelationKey, err)
			continue
		}
		if err != nil {
			return errors.Errorf("importing relation networks for relation %q: %w",
				network.RelationKey, err)
		}
	}
	return nil
}

// extractRelationNetworks returns the relation networks to import, grouped by
// relation and direction. Older Juju versions store both the admin override
// and the original default networks of a relation, in which case only the
// override is effective, so it is the one imported.
func extractRelationNetworks(model description.Model) ([]crossmodelrelation.RelationNetworkImport, error) {
	type networkKey struct {
		RelationKey string
		Direction   crossmodelrelation.RelationNetworkDirection
	}
	type networkValue struct {
		Override bool
		CIDRs    []string
	}

	networks := make(map[networkKey]networkValue)
	for _, network := range model.RelationNetworks() {
		direction, override, err := parseRelationNetworkID(network.ID())
		if err != nil {
			return nil, errors.Errorf("parsing relation network ID %q: %w", network.ID(), err)
		}

		key := networkKey{
			RelationKey: network.RelationKey(),
			Direction:   direction,
		}
		if existing, ok := networks[key]; ok && existing.Override && !override {
			continue
		}
		networks[key] = networkValue{
			Override: override,
			CIDRs:    network.CIDRS(),
		}
	}

	result := make([]crossmodelrelation.RelationNetworkImport, 0, len(networks))
	for key, network := range networks {
		if len(network.CIDRs) == 0 {
			continue
		}
		relationKey, err := relation.NewKeyFromString(key.RelationKey)
		if err != nil {
			return nil, errors.Errorf("parsing relation key %q: %w", key.RelationKey, err)
		}
		result = append(result, crossmodelrelation.RelationNetworkImport{
			RelationKey: relationKey,
			Direction:   key.Direction,
			CIDRs:       network.CIDRs,
		})
	}
	return result, nil
}

// parseRelationNetworkID parses a relation network ID of the form
// "<relation key>:<direction>:<label>", where the direction is either
// "ingress" or "egress" and the label is either "default" or "override". The
// relation key itself contains colons, so the direction and label are always
// the last two colon separated parts of the ID.
func parseRelationNetworkID(id string) (direction crossmodelrelation.RelationNetworkDirection, override bool, err error) {
	parts := strings.Split(id, ":")
	if len(parts) < 3 {
		return 0, false, errors.Errorf("expected at least 3 colon separated parts")
	}
	directionPart, label := parts[len(parts)-2], parts[len(parts)-1]

	switch directionPart {
	case "ingress":
		direction = crossmodelrelation.RelationNetworkIngress
	case "egress":
		direction = crossmodelrelation.RelationNetworkEgress
	default:
		return 0, false, errors.Errorf("unknown direction %q", directionPart)
	}

	switch label {
	case "override":
		return direction, true, nil
	case "default":
		return direction, false, nil
	default:
		return 0, false, errors.Errorf("unknown label %q", label)
	}
}
