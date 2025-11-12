// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"context"
	"fmt"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	jujukubernetes "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
	pool                   *state.StatePool
	getStorageUniqueIDFunc func() func(appName string, model *state.Model) (string, error)
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

func (s stateBackend) PopulateApplicationStorageUniqueID() error {
	return state.PopulateApplicationStorageUniqueID(s.pool, GetStorageUniqueID(NewK8sClient))
}

func NewK8sClient(model *state.Model) (kubernetes.Interface, *rest.Config, error) {
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

type kubernetesClient struct {
	kubernetes.Interface
	config *rest.Config
}

// GetStorageUniqueID attempts to grab the storage unique ID for each app
// in the given model.
// The storage unique ID is saved in annotations in a statefulset and for legacy
// applications are saved in a deploymen or daemonset.
func GetStorageUniqueID(newK8sClient newK8sFunc) func(
	apps []state.AppAndStorageID,
	model *state.Model,
) ([]state.AppAndStorageID, error) {
	return func(apps []state.AppAndStorageID,
		model *state.Model) ([]state.AppAndStorageID, error) {
		k8sClient, cfg, err := newK8sClient(model)
		if err != nil {
			return nil, err
		}
		k8sClient = kubernetesClient{k8sClient, cfg}
		logger.Infof("[adis] creating a k8s client for model %q", model.UUID())

		namespace, err := jujukubernetes.NamespaceForModel(model.Name(), model.ControllerUUID(), cfg)
		if err != nil {
			return nil, err
		}

		isLegacyDeployment := func(appName string) (bool, error) {
			legacyName := "juju-operator-" + appName
			_, getErr := k8sClient.AppsV1().
				StatefulSets(namespace).
				Get(context.Background(), legacyName, v1.GetOptions{})

			if getErr == nil {
				return true, nil
			}
			if k8serrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		getUniqueIDFromAnnotation := func(annotations map[string]string) (string, error) {
			labelVersion, err := utils.MatchModelLabelVersion(namespace, model.Name(), model.UUID(),
				model.ControllerUUID(), k8sClient.CoreV1().Namespaces())
			if err != nil {
				return "", err
			}

			appIDKey := utils.AnnotationKeyApplicationUUID(labelVersion)
			storageUniqueID, ok := annotations[appIDKey]
			if !ok {
				return state.RandomPrefix()
			}
			return storageUniqueID, nil
		}
		k8sApps := make([]state.AppAndStorageID, 0, len(apps))

		for _, a := range apps {
			foundDeployment := false
			deploymentName := a.Name
			isLegacy, err := isLegacyDeployment(a.Name)
			if err != nil {
				return nil, err
			}
			if isLegacy {
				deploymentName = "juju-operator-" + a.Name
			}

			// We have to find the resource where the storageUniqueID
			// is saved in annotation. While modern charms are deployed as
			// statefulsets, due to legacy deployments, we also have to check
			// for deployments and daemonsets resource.
			sts, err := k8sClient.AppsV1().
				StatefulSets(namespace).
				Get(context.Background(), deploymentName, v1.GetOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, err
			}
			if err == nil {
				logger.Infof("[adis] found sts for app %q with annotations %+v", a.Name, sts.Annotations)
				storageUniqueID, err := getUniqueIDFromAnnotation(sts.Annotations)
				if err != nil {
					return nil, err
				}
				k8sApps = append(k8sApps, state.AppAndStorageID{
					Id:              a.Id,
					Name:            a.Name,
					StorageUniqueID: storageUniqueID,
				})
				foundDeployment = true
			}

			// The app doesn't exist in statefulset so we look at the deployment
			// resource.
			deployment, err := k8sClient.AppsV1().
				Deployments(namespace).
				Get(context.Background(), deploymentName, v1.GetOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, err
			}
			if err == nil {
				logger.Infof("[adis] found deployment for app %q with annotations %+v", a.Name, deployment.Annotations)
				storageUniqueID, err := getUniqueIDFromAnnotation(deployment.Annotations)
				if err != nil {
					return nil, err
				}
				k8sApps = append(k8sApps, state.AppAndStorageID{
					Id:              a.Id,
					Name:            a.Name,
					StorageUniqueID: storageUniqueID,
				})
				foundDeployment = true
			}

			// The app doesn't exist in deployment so we look at the daemonset
			// resource.
			daemonSet, err := k8sClient.AppsV1().
				DaemonSets(namespace).
				Get(context.Background(), deploymentName, v1.GetOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, err
			}
			if err == nil {
				logger.Infof("[adis] found daemonset for app %q with annotations %+v", a.Name, daemonSet.Annotations)
				storageUniqueID, err := getUniqueIDFromAnnotation(daemonSet.Annotations)
				if err != nil {
					return nil, err
				}
				k8sApps = append(k8sApps, state.AppAndStorageID{
					Id:              a.Id,
					Name:            a.Name,
					StorageUniqueID: storageUniqueID,
				})
				foundDeployment = true
			}

			if !foundDeployment {
				return nil, fmt.Errorf("cannot find k8s deployment for app %q in model %q", a.Name, model.Name())
			}
		}
		return k8sApps, nil
	}
}
