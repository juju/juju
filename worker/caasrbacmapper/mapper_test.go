// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasrbacmapper_test

import (
	"sync"
	"testing"
	"time"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	jujutest "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasrbacmapper"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

type MapperSuite struct {
	ctrl                    *gomock.Controller
	mockSAInformer          *mocks.MockServiceAccountInformer
	mockSALister            *mocks.MockServiceAccountLister
	mockSANamespaceLister   *mocks.MockServiceAccountNamespaceLister
	mockSharedIndexInformer *mocks.MockSharedIndexInformer
}

var _ = gc.Suite(&MapperSuite{})

func TestMapperSuite(t *testing.T) { gc.TestingT(t) }

func (m *MapperSuite) SetUpTest(c *gc.C) {
	m.ctrl = gomock.NewController(c)
	m.mockSAInformer = mocks.NewMockServiceAccountInformer(m.ctrl)
	m.mockSharedIndexInformer = mocks.NewMockSharedIndexInformer(m.ctrl)
	m.mockSharedIndexInformer.EXPECT().HasSynced().AnyTimes().Return(true)
	m.mockSALister = mocks.NewMockServiceAccountLister(m.ctrl)
	m.mockSANamespaceLister = mocks.NewMockServiceAccountNamespaceLister(m.ctrl)
	m.mockSharedIndexInformer.EXPECT().Run(gomock.Any()).AnyTimes()
	m.mockSAInformer.EXPECT().Informer().AnyTimes().Return(m.mockSharedIndexInformer)
	m.mockSAInformer.EXPECT().Lister().AnyTimes().Return(m.mockSALister)
}

func (m *MapperSuite) TestMapperAdditionSync(c *gc.C) {
	defer m.ctrl.Finish()
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(1)
	var eventHandlers cache.ResourceEventHandlerFuncs
	m.mockSharedIndexInformer.EXPECT().AddEventHandler(gomock.Any()).
		DoAndReturn(func(h cache.ResourceEventHandlerFuncs) {
			eventHandlers = h
			waitGroup.Done()
		})

	mapper, err := caasrbacmapper.NewMapper(loggo.Logger{}, m.mockSAInformer)
	c.Assert(err, jc.ErrorIsNil)
	waitGroup.Wait()

	appName := "test"
	name := "test-sa"
	namespace := "test-model"
	uid := types.UID("123")
	sa := &core.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    provider.RBACLabels(appName, "test-model", false),
			UID:       uid,
		},
	}

	waitGroup.Add(1)
	m.mockSALister.EXPECT().ServiceAccounts(gomock.Eq(namespace)).
		Return(m.mockSANamespaceLister)
	m.mockSANamespaceLister.EXPECT().Get(gomock.Eq(name)).
		DoAndReturn(func(_ string) (*core.ServiceAccount, error) {
			waitGroup.Done()
			return sa, nil
		})

	eventHandlers.OnAdd(sa)
	waitGroup.Wait()

	mapper.Kill()
	mapper.Wait()

	time.Sleep(jujutest.ShortWait)
	rAppName, err := mapper.AppNameForServiceAccount(uid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAppName, gc.Equals, appName)
}

func (m *MapperSuite) TestRBACMapperUpdateSync(c *gc.C) {
	defer m.ctrl.Finish()
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(1)
	var eventHandlers cache.ResourceEventHandlerFuncs
	m.mockSharedIndexInformer.EXPECT().AddEventHandler(gomock.Any()).
		DoAndReturn(func(h cache.ResourceEventHandlerFuncs) {
			eventHandlers = h
			waitGroup.Done()
		})

	mapper, err := caasrbacmapper.NewMapper(loggo.Logger{}, m.mockSAInformer)
	c.Assert(err, jc.ErrorIsNil)
	waitGroup.Wait()

	appName := "test"
	name := "test-sa"
	namespace := "test-model"
	uid := types.UID("123")
	sa := &core.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    provider.RBACLabels(appName, "test-model", false),
			UID:       uid,
		},
	}

	waitGroup.Add(1)
	m.mockSALister.EXPECT().ServiceAccounts(gomock.Eq(namespace)).
		Return(m.mockSANamespaceLister).AnyTimes()
	m.mockSANamespaceLister.EXPECT().Get(gomock.Eq(name)).
		DoAndReturn(func(_ string) (*core.ServiceAccount, error) {
			waitGroup.Done()
			return sa, nil
		})

	eventHandlers.OnAdd(sa)
	waitGroup.Wait()

	rAppName, err := mapper.AppNameForServiceAccount(uid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAppName, gc.Equals, appName)

	// Update SA with a new app name to check propagation
	appName = "test-2"
	sa2 := sa.DeepCopy()
	sa2.ObjectMeta.Labels = provider.RBACLabels(appName, "test-model", false)
	waitGroup.Add(1)
	m.mockSANamespaceLister.EXPECT().Get(gomock.Eq(name)).
		DoAndReturn(func(_ string) (*core.ServiceAccount, error) {
			waitGroup.Done()
			return sa2, nil
		})

	//Send the update event, oldObj, newObj
	eventHandlers.OnUpdate(sa, sa2)
	waitGroup.Wait()

	mapper.Kill()
	mapper.Wait()

	time.Sleep(jujutest.ShortWait)
	rAppName, err = mapper.AppNameForServiceAccount(uid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAppName, gc.Equals, appName)
}

func (m *MapperSuite) TestRBACMapperDeleteSync(c *gc.C) {
	defer m.ctrl.Finish()
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(1)
	var eventHandlers cache.ResourceEventHandlerFuncs
	m.mockSharedIndexInformer.EXPECT().AddEventHandler(gomock.Any()).
		DoAndReturn(func(h cache.ResourceEventHandlerFuncs) {
			eventHandlers = h
			waitGroup.Done()
		})

	mapper, err := caasrbacmapper.NewMapper(loggo.Logger{}, m.mockSAInformer)
	c.Assert(err, jc.ErrorIsNil)
	waitGroup.Wait()

	appName := "test"
	name := "test-sa"
	namespace := "test-model"
	uid := types.UID("123")
	sa := &core.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    provider.RBACLabels(appName, "test-model", false),
			UID:       uid,
		},
	}

	waitGroup.Add(1)
	m.mockSALister.EXPECT().ServiceAccounts(gomock.Eq(namespace)).
		Return(m.mockSANamespaceLister).AnyTimes()
	m.mockSANamespaceLister.EXPECT().Get(gomock.Eq(name)).
		DoAndReturn(func(_ string) (*core.ServiceAccount, error) {
			waitGroup.Done()
			return sa, nil
		})

	eventHandlers.OnAdd(sa)
	waitGroup.Wait()

	rAppName, err := mapper.AppNameForServiceAccount(uid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAppName, gc.Equals, appName)

	// Update SA with a new app name to check propagation
	waitGroup.Add(1)
	m.mockSANamespaceLister.EXPECT().Get(gomock.Eq(name)).
		DoAndReturn(func(_ string) (*core.ServiceAccount, error) {
			waitGroup.Done()
			return nil, k8serrors.NewNotFound(core.Resource("serviceaccount"), name)
		})

	//Send the delete event for the service account
	eventHandlers.OnDelete(sa)
	waitGroup.Wait()

	mapper.Kill()
	mapper.Wait()

	time.Sleep(jujutest.ShortWait)
	_, err = mapper.AppNameForServiceAccount(uid)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (m *MapperSuite) TestRBACMapperNotFound(c *gc.C) {
	defer m.ctrl.Finish()
	m.mockSharedIndexInformer.EXPECT().AddEventHandler(gomock.Any())

	mapper, err := caasrbacmapper.NewMapper(loggo.Logger{}, m.mockSAInformer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = mapper.AppNameForServiceAccount(types.UID("testing"))
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}
