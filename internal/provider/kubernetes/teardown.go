// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"sync"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

func (k *kubernetesClient) deleteClusterScopeResourcesModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	labels := utils.LabelsForModel(k.ModelName(), k.ModelUUID(), k.ControllerUUID(), k.LabelVersion())
	selector := k8slabels.NewSelector().Add(
		labelSetToRequirements(labels)...,
	)

	// TODO(caas): Fix to only delete cluster wide resources created by this controller.
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
		go f(ctx, selector, k.clock, &subwg, errChan)
	}
}

type teardownResources func(
	context.Context,
	k8slabels.Selector,
	jujuclock.Clock,
	*sync.WaitGroup,
	chan<- error,
)

func (k *kubernetesClient) deleteClusterRoleBindingsModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
		k.deleteClusterRoleBindings, func(ctx context.Context, selector k8slabels.Selector) error {
			_, err := k.listClusterRoleBindings(ctx, selector)
			return err
		},
	)
}

func (k *kubernetesClient) deleteClusterRolesModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
		k.deleteClusterRoles, func(ctx context.Context, selector k8slabels.Selector) error {
			_, err := k.listClusterRoles(ctx, selector)
			return err
		},
	)
}

func (k *kubernetesClient) deleteClusterScopeAPIExtensionResourcesModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	defer wg.Done()

	var subwg sync.WaitGroup
	subwg.Add(2)
	defer subwg.Wait()

	selector = mergeSelectors(selector, lifecycleModelTeardownSelector)
	// Delete CRs everywhere first then CRDs. Finalizers are stripped from all
	// matching CRs before deletion so that resources are not left stuck in a
	// terminating state.
	k.deleteAllNamespacesCustomResourcesModelTeardown(ctx, selector, clk, &subwg, errChan)
	k.deleteCustomResourceDefinitionsModelTeardown(ctx, selector, clk, &subwg, errChan)
}

// deleteAllNamespacesCustomResourcesModelTeardown deletes custom resources
// matching the selector everywhere. Before issuing the delete it strips all
// finalizers from every matching CR so that nothing is left stuck in a
// terminating state. This must only ever be called during model teardown.
func (k *kubernetesClient) deleteAllNamespacesCustomResourcesModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
<<<<<<< HEAD
		func(ctx context.Context, _ k8slabels.Selector) error {
			return k.deleteCustomResources(ctx, getSelector)
		},
		func(ctx context.Context, _ k8slabels.Selector) error {
			_, err := k.listCustomResources(ctx, getSelector)
=======
		func(selector k8slabels.Selector) error {
			// Remove finalizers first so that the subsequent DeleteCollection
			// is not blocked by termination hooks.
			err := k.removeAllCustomResourceFinalizers(ctx, selector)
			if err != nil {
				return err
			}
			return k.deleteAllCustomResourcesAllNamespaces(ctx, selector)
		},
		func(selector k8slabels.Selector) error {
			_, err := k.listAllCustomResourcesAllNamespaces(ctx, selector)
>>>>>>> 3.6
			return err
		},
	)
}

func (k *kubernetesClient) deleteCustomResourceDefinitionsModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
		k.deleteCustomResourceDefinitions, func(ctx context.Context, selector k8slabels.Selector) error {
			_, err := k.listCustomResourceDefinitions(ctx, selector)
			return err
		},
	)
}

func (k *kubernetesClient) deleteMutatingWebhookConfigurationsModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
		k.deleteMutatingWebhookConfigurations, func(ctx context.Context, selector k8slabels.Selector) error {
			_, err := k.listMutatingWebhookConfigurations(ctx, selector)
			return err
		},
	)
}

func (k *kubernetesClient) deleteValidatingWebhookConfigurationsModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
		k.deleteValidatingWebhookConfigurations, func(ctx context.Context, selector k8slabels.Selector) error {
			_, err := k.listValidatingWebhookConfigurations(ctx, selector)
			return err
		},
	)
}
func (k *kubernetesClient) deleteStorageClassesModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
		k.deleteStorageClasses, func(ctx context.Context, selector k8slabels.Selector) error {
			_, err := k.ListStorageClasses(ctx, selector)
			return err
		},
	)
}

type deleterChecker func(context.Context, k8slabels.Selector) error

func ensureResourcesDeletedFunc(
	ctx context.Context,
	selector k8slabels.Selector,
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

	if err = deleter(ctx, selector); err != nil {
		if errors.Is(err, errors.NotFound) {
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
			err = checker(ctx, selector)
			if errors.Is(err, errors.NotFound) {
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

	if err = k.deleteNamespace(ctx); err != nil {
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
			_, err = k.GetNamespace(ctx, k.namespace)
			if errors.Is(err, errors.NotFound) {
				// Namespace has been deleted.
				err = nil
				return
			}
			if err != nil {
				err = errors.Trace(err)
				return
			}
			logger.Debugf(context.TODO(), "namespace %q is still been terminating", k.namespace)
		}
	}
}
