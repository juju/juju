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
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/watcher"
)

func (k *kubernetesClient) deleteClusterScopeResourcesModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	labels := utils.LabelsForModel(k.CurrentModel(), k.IsLegacyLabels())
	selector := k8slabels.NewSelector().Add(
		labelSetToRequirements(labels)...,
	)

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
		k.deleteClusterRoleBindings, func(selector k8slabels.Selector) error {
			_, err := k.listClusterRoleBindings(selector)
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
		k.deleteClusterRoles, func(selector k8slabels.Selector) error {
			_, err := k.listClusterRoles(selector)
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
	// Delete CRs first then CRDs.
	k.deleteClusterScopeCustomResourcesModelTeardown(ctx, selector, clk, &subwg, errChan)
	k.deleteCustomResourceDefinitionsModelTeardown(ctx, selector, clk, &subwg, errChan)
}

func (k *kubernetesClient) deleteClusterScopeCustomResourcesModelTeardown(
	ctx context.Context,
	selector k8slabels.Selector,
	clk jujuclock.Clock,
	wg *sync.WaitGroup,
	errChan chan<- error,
) {
	getSelector := func(crd apiextensionsv1beta1.CustomResourceDefinition) k8slabels.Selector {
		if !isCRDScopeNamespaced(crd.Spec.Scope) {
			// We only delete cluster scope CRs here, namespaced CRs are deleted by namespace destroy process.
			return selector
		}
		return k8slabels.NewSelector()
	}
	ensureResourcesDeletedFunc(ctx, selector, clk, wg, errChan,
		func(_ k8slabels.Selector) error {
			return k.deleteCustomResources(getSelector)
		},
		func(_ k8slabels.Selector) error {
			_, err := k.listCustomResources(getSelector)
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
		k.deleteCustomResourceDefinitions, func(selector k8slabels.Selector) error {
			_, err := k.listCustomResourceDefinitions(selector)
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
		k.deleteMutatingWebhookConfigurations, func(selector k8slabels.Selector) error {
			_, err := k.listMutatingWebhookConfigurations(selector)
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
		k.deleteValidatingWebhookConfigurations, func(selector k8slabels.Selector) error {
			_, err := k.listValidatingWebhookConfigurations(selector)
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
		k.deleteStorageClasses, func(selector k8slabels.Selector) error {
			_, err := k.listStorageClasses(selector)
			return err
		},
	)
}

type deleterChecker func(k8slabels.Selector) error

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

	if err = deleter(selector); err != nil {
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
			err = checker(selector)
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
