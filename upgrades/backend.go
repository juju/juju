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

func GetStorageUniqueID(newK8sClient newK8sFunc) func(appName string, model *state.Model) (string, error) {
	k8sClientsByModel := make(map[string]kubernetesClient)
	return func(appName string, model *state.Model) (string, error) {
		// We open a k8s client for each model.
		if _, ok := k8sClientsByModel[model.UUID()]; !ok {
			k8sClient, cfg, err := newK8sClient(model)
			if err != nil {
				return "", err
			}
			k8sClientsByModel[model.UUID()] = kubernetesClient{k8sClient, cfg}
			logger.Infof("[adis] creating a k8s client for model %q", model.UUID())
		}

		k8sClient := k8sClientsByModel[model.UUID()]
		namespace, err := jujukubernetes.NamespaceForModel(model.Name(), model.ControllerUUID(), k8sClient.config)
		if err != nil {
			return "", err
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

		deploymentName := appName
		isLegacy, err := isLegacyDeployment(appName)
		if err != nil {
			return "", err
		}
		if isLegacy {
			deploymentName = "juju-operator-" + appName
		}

		// We have to find the resource where the storageUniqueID
		// is saved in annotation. While modern charms are deployed as
		// statefulsets, due to legacy deployments, we also have to check
		// for deployments and daemonsets resource.
		sts, err := k8sClient.AppsV1().
			StatefulSets(namespace).
			Get(context.Background(), deploymentName, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return "", err
		}
		if err == nil {
			logger.Infof("[adis] found sts for app %q with annotations %+v", appName, sts.Annotations)
			return getUniqueIDFromAnnotation(sts.Annotations)
		}

		deployment, err := k8sClient.AppsV1().
			Deployments(namespace).
			Get(context.Background(), deploymentName, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return "", err
		}
		if err == nil {
			logger.Infof("[adis] found deployment for app %q with annotations %+v", appName, deployment.Annotations)
			return getUniqueIDFromAnnotation(deployment.Annotations)
		}

		daemonSet, err := k8sClient.AppsV1().
			DaemonSets(namespace).
			Get(context.Background(), deploymentName, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return "", err
		}
		if err == nil {
			logger.Infof("[adis] found daemonset for app %q with annotations %+v", appName, daemonSet.Annotations)
			return getUniqueIDFromAnnotation(daemonSet.Annotations)
		}

		return "", fmt.Errorf("cannot find k8s deployment for app %q", appName)
	}
}
