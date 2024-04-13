// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build aix || dragonfly || freebsd || (js && wasm) || linux || netbsd || openbsd || solaris

package exec_test

import (
	"os"
	"time"

	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"
	gc "gopkg.in/check.v1"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/exec"
	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/exec/mocks"
	"github.com/juju/juju/testing"
)

type termSizeSuite struct {
	testing.BaseSuite

	sizeQueue exec.SizeQueueInterface
	getSize   *mocks.MockSizeGetter
	nCh       chan os.Signal
}

var _ = gc.Suite(&termSizeSuite{})

func (s *termSizeSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)

	if s.sizeQueue != nil {
		s.sizeQueue.Stop()
		s.sizeQueue = nil
	}
	s.getSize = nil
	s.nCh = nil
}

func (s *termSizeSuite) setupQ(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.getSize = mocks.NewMockSizeGetter(ctrl)
	s.nCh = make(chan os.Signal, 1)
	s.sizeQueue = exec.NewSizeQueueForTest(
		make(chan remotecommand.TerminalSize, 1),
		s.getSize, s.nCh,
	)
	return ctrl
}

func (s *termSizeSuite) TestWatch(c *gc.C) {
	ctrl := s.setupQ(c)
	defer ctrl.Finish()

	go func() {
		// window size changed.
		s.nCh <- unix.SIGWINCH
	}()

	outChan := make(chan *remotecommand.TerminalSize, 1)
	done := make(chan struct{})
	defer close(done)

	go func(sizeQueue exec.SizeQueueInterface) {
		for {
			select {
			case outChan <- sizeQueue.Next():
			case <-done:
				return
			}
		}
	}(s.sizeQueue)

	size1 := &remotecommand.TerminalSize{Width: 111, Height: 222}
	size2 := &remotecommand.TerminalSize{Width: 333, Height: 666}

	gomock.InOrder(
		s.getSize.EXPECT().Get(gomock.Any()).DoAndReturn(
			// get initial window size.
			func(fd int) *remotecommand.TerminalSize {
				c.Assert(fd, gc.DeepEquals, 1)
				return size1
			},
		),
		s.getSize.EXPECT().Get(gomock.Any()).DoAndReturn(
			func(fd int) *remotecommand.TerminalSize {
				// get the latest changed window size.
				c.Assert(fd, gc.DeepEquals, 1)
				return size2
			},
		),
	)

	s.sizeQueue.Watch(1)

	for _, expected := range []*remotecommand.TerminalSize{
		size1, size2,
	} {
		select {
		case o := <-outChan:
			c.Assert(o, gc.DeepEquals, expected)
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for result")
		}
	}

}
