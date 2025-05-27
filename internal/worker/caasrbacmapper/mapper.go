// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasrbacmapper

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	core "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/provider/kubernetes"
)

// Mapper describes an interface for mapping k8s service account UID's to juju
// application names.
type Mapper interface {
	// AppNameForServiceAccount fetches the juju application name associated
	// with a given kubernetes service account UID. If no result is found
	// errors.NotFound is returned. All other errors should be considered
	// internal to the interface operation.
	AppNameForServiceAccount(types.UID) (string, error)
}

// MapperWorker is a Mapper that also implements the worker interface
type MapperWorker interface {
	Mapper
	worker.Worker
}

// DefaultMapper is a default implementation of the MapperWorker interface. It's
// responsible for watching ServiceAccounts on a given model and
type DefaultMapper struct {
	catacomb     catacomb.Catacomb
	lock         *sync.RWMutex
	logger       logger.Logger
	saInformer   core.ServiceAccountInformer
	saNameUIDMap map[string]types.UID
	saUIDAppMap  map[types.UID]string
	workQueue    workqueue.RateLimitingInterface
}

// NewMapper constructs a new DefaultMapper for the supplied logger and
// ServiceAccountInformer
func NewMapper(logger logger.Logger, informer core.ServiceAccountInformer) (*DefaultMapper, error) {
	dm := &DefaultMapper{
		lock:         new(sync.RWMutex),
		logger:       logger,
		saInformer:   informer,
		saNameUIDMap: map[string]types.UID{},
		saUIDAppMap:  map[types.UID]string{},
		workQueue:    workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	_, err := dm.saInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    dm.enqueueServiceAccount,
		DeleteFunc: dm.enqueueServiceAccount,
		UpdateFunc: func(_, newObj interface{}) {
			dm.enqueueServiceAccount(newObj)
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-rbac-mapper",
		Site: &dm.catacomb,
		Work: dm.loop,
	}); err != nil {
		return dm, errors.Trace(err)
	}
	return dm, nil
}

// AppNameForServiceAccount implements Mapper interface
func (d *DefaultMapper) AppNameForServiceAccount(id types.UID) (string, error) {
	d.lock.RLock()
	defer d.lock.RUnlock()
	appName, found := d.saUIDAppMap[id]
	if !found {
		return "", errors.NotFoundf("service account for app with id %v", id)
	}
	return appName, nil
}

func (d *DefaultMapper) enqueueServiceAccount(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		d.logger.Errorf(context.Background(), "failed enqueuing service account: %v", err)
		return
	}
	d.workQueue.Add(key)
}

// Kill implements Kill() from the Worker interface
func (d *DefaultMapper) Kill() {
	d.catacomb.Kill(nil)
}

// Wait implements Wait() from the Worker interface
func (d *DefaultMapper) Wait() error {
	return d.catacomb.Wait()
}

func (d *DefaultMapper) loop() error {
	defer d.workQueue.ShutDown()

	ctx, cancel := d.scopedContext()
	defer cancel()

	go d.saInformer.Informer().Run(d.catacomb.Dying())

	if ok := cache.WaitForCacheSync(
		d.catacomb.Dying(), d.saInformer.Informer().HasSynced); !ok {
		return errors.New("failed to wait for cache to sync")
	}

	// Wait until runs the below for loop every one second until the catacomb
	// dies. The for loop processes all items in the queue till it's empty and
	// the cycle repeats. This is to stop checking thrashing about and is the
	// prescribed k8s way to process.
	go wait.Until(func() {
		for d.processNextQueueItem(ctx) {
		}
	}, time.Second, d.catacomb.Dying())

	select {
	case <-d.catacomb.Dying():
		return d.catacomb.ErrDying()
	}
}

func (d *DefaultMapper) processNextQueueItem(ctx context.Context) bool {
	obj, shutdown := d.workQueue.Get()
	if shutdown {
		return false
	}

	defer d.workQueue.Done(obj)
	key, ok := obj.(string)
	if !ok {
		d.workQueue.Forget(obj)
		d.logger.Errorf(ctx, "failed converting service account queue item to string")
		return true
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		d.workQueue.Forget(obj)
		d.logger.Errorf(ctx, "failed spliting key into namespace and name for service account queue: %v", err)
		return true
	}

	sa, err := d.saInformer.Lister().ServiceAccounts(namespace).Get(name)
	if k8serrors.IsNotFound(err) {
		d.lock.Lock()
		defer d.lock.Unlock()
		uid, exists := d.saNameUIDMap[key]
		if !exists {
			return true
		}
		delete(d.saUIDAppMap, uid)
		delete(d.saNameUIDMap, name)
		return true
	}
	if err != nil {
		d.workQueue.Forget(obj)
		d.logger.Errorf(ctx, "failed fetching service account for %s/%s", namespace, name)
		return true
	}

	appName, err := kubernetes.AppNameForServiceAccount(sa)
	if errors.Is(err, errors.NotFound) {
		return true
	} else if err != nil {
		d.logger.Errorf(ctx, "failure getting app name for service account: %v", err)
		return true
	}

	d.lock.Lock()
	defer d.lock.Unlock()
	d.saNameUIDMap[key] = sa.UID
	d.saUIDAppMap[sa.UID] = appName
	return true
}

func (w *DefaultMapper) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
