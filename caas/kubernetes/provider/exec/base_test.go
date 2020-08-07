// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec_test

import (
	"io"
	"net/url"
	"reflect"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	execmocks "github.com/juju/juju/caas/kubernetes/provider/exec/mocks"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	namespace             string
	k8sClient             *mocks.MockInterface
	restClient            *mocks.MockRestClientInterface
	execClient            exec.Executor
	mockPodGetter         *mocks.MockPodInterface
	mockNamespaces        *mocks.MockNamespaceInterface
	mockRemoteCmdExecutor *execmocks.MockExecutor
	suiteMocks            *suiteMocks

	pipReader io.Reader
	pipWriter io.WriteCloser
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.namespace = "test"
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.pipReader, s.pipWriter = io.Pipe()
}

func (s *BaseSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.pipWriter.Close()
}

func (s *BaseSuite) setupExecClient(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.k8sClient = mocks.NewMockInterface(ctrl)

	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)

	s.restClient = mocks.NewMockRestClientInterface(ctrl)
	mockCoreV1.EXPECT().RESTClient().AnyTimes().Return(s.restClient)

	s.mockPodGetter = mocks.NewMockPodInterface(ctrl)
	mockCoreV1.EXPECT().Pods(s.namespace).AnyTimes().Return(s.mockPodGetter)

	s.mockNamespaces = mocks.NewMockNamespaceInterface(ctrl)
	mockCoreV1.EXPECT().Namespaces().AnyTimes().Return(s.mockNamespaces)

	s.mockRemoteCmdExecutor = execmocks.NewMockExecutor(ctrl)

	s.suiteMocks = newSuiteMocks(ctrl)
	s.execClient = exec.NewForTest(
		s.namespace,
		s.k8sClient,
		&rest.Config{},
		func(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
			return s.suiteMocks.RemoteCmdExecutorGetter(config, method, url)
		},
		func() (io.Reader, io.WriteCloser) {
			return s.pipReader, s.pipWriter
		},
	)
	return ctrl
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

type suiteMocks struct {
	ctrl     *gomock.Controller
	recorder *suiteMocksRecorder
}

type suiteMocksRecorder struct {
	mock *suiteMocks
}

func newSuiteMocks(ctrl *gomock.Controller) *suiteMocks {
	mock := &suiteMocks{ctrl: ctrl}
	mock.recorder = &suiteMocksRecorder{mock}
	return mock
}

func (m *suiteMocks) EXPECT() *suiteMocksRecorder {
	return m.recorder
}

func (m *suiteMocks) RemoteCmdExecutorGetter(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
	ret := m.ctrl.Call(m, "RemoteCmdExecutorGetter", config, method, url)
	ret0, _ := ret[0].(remotecommand.Executor)
	ret1, _ := ret[0].(error)
	return ret0, ret1
}

func (mr *suiteMocksRecorder) RemoteCmdExecutorGetter(config interface{}, method interface{}, url interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoteCmdExecutorGetter", reflect.TypeOf((*suiteMocks)(nil).RemoteCmdExecutorGetter), config, method, url)
}
