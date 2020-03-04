// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"sync"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	"github.com/juju/juju/core/watcher"
)

func (k *kubernetesClient) deleteClusterScopeResourcesModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	labels := map[string]string{
		labelModel: k.namespace,
	}

	tasks := []teardownResources{
		k.deleteClusterRoleBindingsModelTeardown,
		k.deleteClusterRolesModelTeardown,
		k.deleteClusterScopeAPIExtensionResourcesModelTeardown,
		k.deleteMutatingWebhookConfigurationsModelTeardown,
		k.deleteValidatingWebhookConfigurationsModelTeardown,
		k.deleteStorageClassesModelTeardown,
	}
	var subwg sync.WaitGroup
	subwg.Add(len(tasks))
	defer subwg.Wait()

	for _, f := range tasks {
		go f(ctx, labels, k.clock, &subwg, errChan)
	}
}

type teardownResources func(
	context.Context,
	map[string]string,
	jujuclock.Clock,
	*sync.WaitGroup,
	chan<- error,
)

func (k *kubernetesClient) deleteClusterRoleBindingsModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, labels, clk, wg, errChan,
		k.deleteClusterRoleBindings, func(labels map[string]string) error {
			_, err := k.listClusterRoleBindings(labels)
			return err
		},
	)
}

func (k *kubernetesClient) deleteClusterRolesModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, labels, clk, wg, errChan,
		k.deleteClusterRoles, func(labels map[string]string) error {
			_, err := k.listClusterRoles(labels)
			return err
		},
	)
}

func (k *kubernetesClient) deleteClusterScopeAPIExtensionResourcesModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	defer wg.Done()

	var subwg sync.WaitGroup
	subwg.Add(2)
	defer subwg.Wait()
	// Delete CRs first then CRDs.
	k.deleteClusterScopeCustomResourcesModelTeardown(ctx, labels, clk, &subwg, errChan)
	k.deleteCustomResourceDefinitionsModelTeardown(ctx, labels, clk, &subwg, errChan)
}

func (k *kubernetesClient) deleteClusterScopeCustomResourcesModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	getLabels := func(crd apiextensionsv1beta1.CustomResourceDefinition) map[string]string {
		if !isCRDScopeNamespaced(crd.Spec.Scope) {
			// We only delete cluster scope CRs here, namespaced CRs are deleted by namespace destroy process.
			return labels
		}
		return nil
	}
	ensureResourcesDeletedFunc(ctx, labels, clk, wg, errChan,
		func(labels map[string]string) error {
			return k.deleteCustomResources(getLabels)
		},
		func(labels map[string]string) error {
			_, err := k.listCustomResources(getLabels)
			return err
		},
	)
}

func (k *kubernetesClient) deleteCustomResourceDefinitionsModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, labels, clk, wg, errChan,
		k.deleteCustomResourceDefinitions, func(labels map[string]string) error {
			_, err := k.listCustomResourceDefinitions(labels)
			return err
		},
	)
}

func (k *kubernetesClient) deleteMutatingWebhookConfigurationsModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, labels, clk, wg, errChan,
		k.deleteMutatingWebhookConfigurations, func(labels map[string]string) error {
			_, err := k.listMutatingWebhookConfigurations(labels)
			return err
		},
	)
}

func (k *kubernetesClient) deleteValidatingWebhookConfigurationsModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, labels, clk, wg, errChan,
		k.deleteValidatingWebhookConfigurations, func(labels map[string]string) error {
			_, err := k.listValidatingWebhookConfigurations(labels)
			return err
		},
	)
}
func (k *kubernetesClient) deleteStorageClassesModelTeardown(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, labels, clk, wg, errChan,
		k.deleteStorageClasses, func(labels map[string]string) error {
			_, err := k.listStorageClasses(labels)
			return err
		},
	)
}

type deleterChecker func(map[string]string) error

func ensureResourcesDeletedFunc(
	ctx context.Context,
	labels map[string]string,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
	deleter, checker deleterChecker,
) {
	defer wg.Done()

	var err error
	defer func() {
		if err != nil {
			select {
			case errChan <- err:
			default:
			}
		}
	}()

	if err = deleter(labels); err != nil {
		if errors.IsNotFound(err) {
			err = nil
		}
		return
	}

	interval := 1 * time.Second
	ticker := clk.NewTimer(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			err = errors.Trace(ctx.Err())
			return
		case <-ticker.Chan():
			err = checker(labels)
			if errors.IsNotFound(err) {
				// Deleted already.
				err = nil
				return
			}
			if err != nil {
				err = errors.Trace(err)
				return
			}
		}
		// Keep checking.
		ticker.Reset(interval)
	}
}

func (k *kubernetesClient) deleteNamespaceModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	var err error
	defer func() {
		if err != nil {
			select {
			case errChan <- err:
			default:
			}
		}
	}()

	var w watcher.NotifyWatcher
	if w, err = k.WatchNamespace(); err != nil {
		err = errors.Annotatef(err, "watching namespace %q", k.namespace)
		return
	}
	defer w.Kill()

	if err = k.deleteNamespace(); err != nil {
		err = errors.Annotatef(err, "deleting model namespace %q", k.namespace)
		return
	}
	for {
		select {
		case <-ctx.Done():
			err = errors.Annotatef(ctx.Err(), "tearing down namespace %q", k.namespace)
			return
		case <-w.Changes():
			// Ensures the namespace to be deleted - notfound error expected.
			_, err = k.GetNamespace(k.namespace)
			if errors.IsNotFound(err) {
				// Namespace has been deleted.
				err = nil
				return
			}
			if err != nil {
				err = errors.Trace(err)
				return
			}
			logger.Debugf("namespace %q is still been terminating", k.namespace)
		}
	}
}
