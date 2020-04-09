// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jujuclock "github.com/juju/clock"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/watcher/watchertest"

	"k8s.io/client-go/tools/cache"
)

func newKubernetesTestWatcher() (provider.KubernetesNotifyWatcher, func()) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return watchertest.NewMockNotifyWatcher(ch), func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func newKubernetesTestStringsWatcher() (provider.KubernetesStringsWatcher, func([]string)) {
	ch := make(chan []string, 1)
	ch <- []string{}
	return watchertest.NewMockStringsWatcher(ch), func(s []string) {
		select {
		case ch <- s:
		default:
		}
	}
}

func newK8sWatcherFunc(w provider.KubernetesNotifyWatcher) provider.NewK8sWatcherFunc {
	return func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (provider.KubernetesNotifyWatcher, error) {
		return w, nil
	}
}
