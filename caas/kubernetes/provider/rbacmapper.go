// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	core "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type RBACMapper interface {
	worker.Worker
	AppNameForServiceAccount(types.UID) (string, error)
}

type rbacMapper struct {
	lock       *sync.RWMutex
	saInformer core.ServiceAccountInformer
	saMap      map[types.UID]string
	stopCh     chan struct{}
	workQueue  workqueue.RateLimitingInterface
}

func (r *rbacMapper) AppNameForServiceAccount(id types.UID) (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	appName, found := r.saMap[id]
	if !found {
		return "", errors.NotFoundf("no service account for app found with id %v", id)
	}
	return appName, nil
}

func (r *rbacMapper) enqueueServiceAccount(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		//TODO Handle error
	}
	r.workQueue.Add(key)
}

func (r *rbacMapper) Kill() {
	close(r.stopCh)
}

func newRBACMapper(informer core.ServiceAccountInformer) *rbacMapper {
	return &rbacMapper{
		lock:       new(sync.RWMutex),
		saInformer: informer,
		saMap:      map[types.UID]string{},
		workQueue:  workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
}

func (r *rbacMapper) processNextQueueItem() bool {
	obj, shutdown := r.workQueue.Get()
	if shutdown {
		//TODO
		return false
	}

	defer r.workQueue.Done(obj)
	key, ok := obj.(string)
	if !ok {
		//TODO
		return false
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		//TODO
		return false
	}

	sa, err := r.saInformer.Lister().ServiceAccounts(namespace).Get(name)
	if err != nil {
		//TODO
		return false
	}

	appName, err := getRBACAppName(sa)
	if errors.IsNotFound(err) {
		return true
	} else if err != nil {
		//TODO
		return false
	}

	r.lock.Lock()
	defer r.lock.Unlock()
	r.saMap[sa.UID] = appName
	return true
}

func (r *rbacMapper) Wait() error {
	r.stopCh = make(chan struct{})

	r.saInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.enqueueServiceAccount,
		DeleteFunc: r.enqueueServiceAccount,
		UpdateFunc: func(_, newObj interface{}) {
			r.enqueueServiceAccount(newObj)
		},
	})
	go r.saInformer.Informer().Run(r.stopCh)
	go wait.Until(func() {
		for r.processNextQueueItem() {
		}
	}, time.Second, r.stopCh)

	select {
	case <-r.stopCh:
	}
	return nil
}
