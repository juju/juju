// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"maps"

	"github.com/juju/collections/set"
	"github.com/juju/description/v12"

	jujucloud "github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modelconfig/state"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/errors"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(
	coordinator Coordinator, defaultsProvider service.ModelDefaultsProvider, logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		defaultsProvider: defaultsProvider,
		logger:           logger,
	})
}

// ImportService represents the service methods needed for model config.
type ImportService interface {
	// SetModelConfig will remove any existing model config for the model and
	// replace with the new config provided. The new config will also be hydrated
	// with any model default attributes that have not been set on the config.
	SetModelConfig(
		ctx context.Context,
		cfg map[string]any,
	) error

	// GetModelConfigSchema returns the schema for the model config.
	GetModelConfigSchemaForCloudType(ctx context.Context, cloudType string) (configschema.Fields, error)
}

type importOperation struct {
	modelmigration.BaseOperation

	logger           logger.Logger
	service          ImportService
	defaultsProvider service.ModelDefaultsProvider
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import model configuration"
}

// Setup the import operation, this will ensure the service is created
// and ready to be used.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	st := state.NewState(scope.ModelDB())
	i.service = service.NewService(
		i.defaultsProvider,
		config.NoControllerAttributesValidator(),
		service.ProviderModelConfigGetter(),
		st,
		i.logger,
	)
	return nil
}

// Execute the import on the model config description.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	// The description returns its own config map; work on a copy so the
	// filtering below does not mutate the description, which other import
	// operations also read.
	attrs := maps.Clone(model.Config())

	// If we don't have any model config, then there is something seriously
	// wrong. In this case, we should return an error.
	if len(attrs) == 0 {
		return errors.Errorf("model config %w", coreerrors.NotValid)
	}

	cloudType, ok := attrs[config.TypeKey].(string)
	if !ok {
		return errors.Errorf("model config missing cloud type").Add(coreerrors.NotValid)
	}

	// Models migrated from 3.x Kubernetes clusters may nominate their
	// workload storage class with the workload-storage model config key,
	// removed in 4.0. Represent the nomination with the
	// storage-default-filesystem-source model config, resolving to the
	// Kubernetes storage pool that the storage pool import derives from
	// the same key. 3.6 recorded the seeded "kubernetes" pool as the
	// default source on every model and then ignored it in favour of
	// workload-storage when provisioning, so that value counts as unset
	// here; any other default filesystem source is the model's own and
	// is not overridden. A pool of the derived name provisioning from
	// another class is left alone, mirroring 3.6's rule that a
	// same-named pool is never clobbered: the nomination is dropped and
	// charm storage keeps falling back to the model's default.
	if cloudType == jujucloud.CloudTypeKubernetes {
		if class, ok := attrs[k8sconstants.WorkloadStorageKey].(string); ok && class != "" {
			source, hasSource := attrs[config.StorageDefaultFilesystemSourceKey].(string)
			sourceIsSentinel := !hasSource || source == "" ||
				source == string(k8sconstants.StorageProviderType)
			if sourceIsSentinel {
				poolName := storage.PoolNameForStorageClass(class)
				usable := true
				for _, pool := range model.StoragePools() {
					if pool.Name() != poolName {
						continue
					}
					poolClass, _ := pool.Attributes()[k8sconstants.StorageClass].(string)
					usable = poolClass == class
				}
				if usable {
					attrs[config.StorageDefaultFilesystemSourceKey] = poolName
				}
			}
		}
	}

	// Models imported from older controllers may contain config attributes
	// which have since been removed from use. Filter these out using the
	// schema.
	schema, err := i.service.GetModelConfigSchemaForCloudType(ctx, cloudType)
	if err != nil {
		return errors.Capture(err)
	}
	schemaAttrs := set.NewStrings()
	for k := range schema {
		schemaAttrs.Add(k)
	}

	for k, v := range attrs {
		if !schemaAttrs.Contains(k) {
			i.logger.Debugf(ctx, "model config attribute %s=%v is removed on import", k, v)
			delete(attrs, k)
		}
	}

	if err := i.service.SetModelConfig(ctx, attrs); err != nil {
		return errors.Capture(err)
	}
	return nil
}
