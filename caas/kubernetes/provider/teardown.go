// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"sync"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/watcher"
)

func (k *kubernetesClient) deleteClusterScropeResourcesModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	labels := map[string]string{
		labelModel: k.namespace,
	}
	logger.Criticalf("--> deleteClusterScropeResourcesModelTeardown")

	tasks := []teardownResources{
		// Order matters.
		k.deleteClusterRoleBindingsModelTeardown,
		k.deleteClusterRolesModelTeardown,
		// delete CRDs will delete CRs that were created by this CRD automatically.
		k.deleteCustomResourceDefinitionsModelTeardown,
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
	ensureResourcesDeletedfunc(ctx, labels, clk, wg, errChan,
		k.deleteClusterRoleBindings, func(labels map[string]string) error {
			_, err := k.listClusterRoleBindings(labels)
			logger.Criticalf("listClusterRoleBindings err -> %#v", err)
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
	ensureResourcesDeletedfunc(ctx, labels, clk, wg, errChan,
		k.deleteClusterRoles, func(labels map[string]string) error {
			_, err := k.listClusterRoles(labels)
			logger.Criticalf("listClusterRoles err -> %#v", err)
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
	ensureResourcesDeletedfunc(ctx, labels, clk, wg, errChan,
		k.deleteCustomResourceDefinitions, func(labels map[string]string) error {
			_, err := k.listCustomResourceDefinitions(labels)
			logger.Criticalf("listCustomResourceDefinitions err -> %#v", err)
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
	ensureResourcesDeletedfunc(ctx, labels, clk, wg, errChan,
		k.deleteMutatingWebhookConfigurations, func(labels map[string]string) error {
			_, err := k.listMutatingWebhookConfigurations(labels)
			logger.Criticalf("listMutatingWebhookConfigurations err -> %#v", err)
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
	ensureResourcesDeletedfunc(ctx, labels, clk, wg, errChan,
		k.deleteValidatingWebhookConfigurations, func(labels map[string]string) error {
			_, err := k.listValidatingWebhookConfigurations(labels)
			logger.Criticalf("listValidatingWebhookConfigurations err -> %#v", err)
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
	ensureResourcesDeletedfunc(ctx, labels, clk, wg, errChan,
		k.deleteStorageClasses, func(labels map[string]string) error {
			_, err := k.listStorageClasses(labels)
			logger.Criticalf("listStorageClasses err -> %#v", err)
			return err
		},
	)
}

type deleterChecker func(map[string]string) error

func ensureResourcesDeletedfunc(
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
			}
		}
	}()
	if err = deleter(labels); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			err = errors.Trace(ctx.Err())
			return
		default:
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
			// Keep checking.
		}
	}
}

func (k *kubernetesClient) deleteNamespaceModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	var err error
	defer func() {
		if err != nil {
			select {
			case errChan <- err:
			}
		}
	}()

	logger.Criticalf("--> deleteNamespaceModelTeardown")

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
			_, err := k.GetNamespace(k.namespace)
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
