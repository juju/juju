// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec_test

import (
	"io"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/internal/provider/kubernetes/exec"
	execmocks "github.com/juju/juju/internal/provider/kubernetes/exec/mocks"
	"github.com/juju/juju/internal/provider/kubernetes/mocks"
	"github.com/juju/juju/internal/testing"
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
	mockRemoteCMDGetter   *execmocks.MockRemoteCMDExecutorGetter

	clock     *testclock.Clock
	pipReader io.Reader
	pipWriter io.WriteCloser
}

func (s *BaseSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.namespace = "test"
}

func (s *BaseSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.pipReader, s.pipWriter = io.Pipe()
}

func (s *BaseSuite) TearDownTest(c *tc.C) {
	s.BaseSuite.TearDownTest(c)

	s.k8sClient = nil
	s.restClient = nil
	s.execClient = nil
	s.mockPodGetter = nil
	s.mockRemoteCmdExecutor = nil
	s.mockRemoteCMDGetter = nil
	s.clock = nil
	s.pipReader = nil
	if s.pipWriter != nil {
		s.pipWriter.Close()
		s.pipWriter = nil
	}
}

func (s *BaseSuite) setupExecClient(c *tc.C) *gomock.Controller {
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
	s.mockRemoteCMDGetter = execmocks.NewMockRemoteCMDExecutorGetter(ctrl)
	s.clock = testclock.NewClock(time.Time{})

	s.execClient = exec.NewForTest(
		s.namespace,
		s.k8sClient,
		&rest.Config{},
		s.mockRemoteCMDGetter.RemoteCmdExecutorGetter,
		func() (io.Reader, io.WriteCloser) {
			return s.pipReader, s.pipWriter
		},
		s.clock,
	)
	return ctrl
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}
