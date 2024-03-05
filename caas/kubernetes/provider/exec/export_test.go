// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"context"
	"os"

	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	ProcessEnv                   = processEnv
	NewForTest                   = newClient
	HandleContainerNotFoundError = handleContainerNotFoundError
	RandomString                 = &randomString
)

func (ep *ExecParams) Validate(ctx context.Context, podGetter typedcorev1.PodInterface) error {
	return ep.validate(ctx, podGetter)
}

func (fr *FileResource) Validate() error {
	return fr.validate()
}

func (cp *CopyParams) Validate() error {
	return cp.validate()
}

type SizeQueueInterface interface {
	Next() *remotecommand.TerminalSize
	Watch(int)
	Stop()
}

func (s *sizeQueue) Watch(fd int) {
	s.watch(fd)
}

func (s *sizeQueue) Stop() {
	s.stop()
}

func NewSizeQueueForTest(resizeChan chan remotecommand.TerminalSize, getSize SizeGetter, nCh chan os.Signal) SizeQueueInterface {
	return &sizeQueue{
		resizeChan: resizeChan,
		done:       make(chan struct{}),
		getSize:    getSize,
		nCh:        nCh,
	}
}
