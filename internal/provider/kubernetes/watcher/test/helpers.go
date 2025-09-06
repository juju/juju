// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8swatchertest

import (
	jujuclock "github.com/juju/clock"
	"k8s.io/client-go/tools/cache"

	"github.com/juju/juju/core/watcher/watchertest"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
)

func NewKubernetesTestWatcher() (k8swatcher.KubernetesNotifyWatcher, func()) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return watchertest.NewMockNotifyWatcher(ch), func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func NewKubernetesTestStringsWatcher() (k8swatcher.KubernetesStringsWatcher, func([]string)) {
	ch := make(chan []string, 1)
	ch <- []string{}
	return watchertest.NewMockStringsWatcher(ch), func(s []string) {
		select {
		case ch <- s:
		default:
		}
	}
}

func NewKubernetesTestWatcherFunc(w k8swatcher.KubernetesNotifyWatcher) k8swatcher.NewK8sWatcherFunc {
	return func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		return w, nil
	}
}
