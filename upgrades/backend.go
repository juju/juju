// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	jujukubernetes "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	AddVirtualHostKeys() error
	SplitMigrationStatusMessages() error
	PopulateApplicationStorageUniqueID() error
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.StatePool object.
func NewStateBackend(pool *state.StatePool) StateBackend {
	return stateBackend{
		pool: pool,
	}
}

type stateBackend struct {
	pool *state.StatePool
}

// AddVirtualHostKeys runs an upgrade to
// create missing virtual host keys.
func (s stateBackend) AddVirtualHostKeys() error {
	return state.AddVirtualHostKeys(s.pool)
}

// SplitMigrationStatusMessages runs an upgrade to
// split migration status messages.
func (s stateBackend) SplitMigrationStatusMessages() error {
	return state.SplitMigrationStatusMessages(s.pool)
}

// PopulateApplicationStorageUniqueID runs an upgrade to backfill CAAS apps
// storage unique IDs.
func (s stateBackend) PopulateApplicationStorageUniqueID() error {
	return state.PopulateApplicationStorageUniqueID(s.pool, GetStorageUniqueIDs(
		newK8sClient,
		jujukubernetes.NamespaceForModel),
	)
}

// newK8sClient initializes a new k8s client for a given model.
func newK8sClient(model *state.Model) (kubernetes.Interface, *rest.Config, error) {
	g := stateenvirons.EnvironConfigGetter{Model: model}
	cloudSpec, err := g.CloudSpec()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := jujukubernetes.CloudSpecToK8sRestConfig(cloudSpec)
	if err != nil {
		return nil, nil, err
	}

	k8sClient, err := kubernetes.NewForConfig(cfg)
	return k8sClient, cfg, err
}

type newK8sFunc func(model *state.Model) (kubernetes.Interface, *rest.Config, error)
type namespaceForModelFunc func(modelName string, controllerUUID string,
	k8sRestConfig *rest.Config) (string, error)

// GetStorageUniqueIDs attempts to grab the storage unique ID for each app
// in the given model.
// The storage unique ID is saved in annotations in a statefulset and for legacy
// applications are saved in a deployment or daemonset.
func GetStorageUniqueIDs(
	newK8sClient newK8sFunc,
	namespaceForModel namespaceForModelFunc,
) func(
	ctx context.Context,
	apps []state.AppAndStorageID,
	model *state.Model,
) ([]state.AppAndStorageID, error) {
	return func(
		ctx context.Context,
		apps []state.AppAndStorageID,
		model *state.Model,
	) ([]state.AppAndStorageID, error) {
		k8sClient, cfg, err := newK8sClient(model)
		if err != nil {
			return nil, err
		}

		namespace, err := namespaceForModel(
			model.Name(), model.ControllerUUID(), cfg)
		if err != nil {
			return nil, err
		}

		k8sApps := make([]state.AppAndStorageID, 0, len(apps))

		for _, a := range apps {
			found := false
			for _, findStorageUniqueID := range jujukubernetes.StorageUniqueIDFinder {
				storageUniqueID, err := findStorageUniqueID(
					ctx, k8sClient,
					namespace, a.Name, model.Name(), model.UUID(),
					model.ControllerUUID(),
				)

				if err == nil {
					k8sApps = append(k8sApps, state.AppAndStorageID{
						Id:              a.Id,
						Name:            a.Name,
						StorageUniqueID: storageUniqueID,
					})
					found = true
					break
				}
				if k8serrors.IsNotFound(err) {
					continue
				}
				return nil, errors.Annotate(err, "finding storage unique ID")
			}

			if !found {
				return nil, fmt.Errorf("cannot find k8s artefact for "+
					"app %q in model %q", a.Name, model.Name())
			}
		}
		return k8sApps, nil
	}
}
