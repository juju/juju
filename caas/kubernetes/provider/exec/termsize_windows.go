// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"os"
	"os/signal"

	"github.com/juju/errors"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/remotecommand"
)

// TODO: implement window resizing for windows.
type sizeQueue struct{}

func newSizeQueue() sizeQueueInterface {
	logger.Warningf("terminal window resizing does not support on windows")
	return nil
}

var _ remotecommand.TerminalSizeQueue = (*sizeQueue)(nil)

// Next returns the new terminal size after the terminal has been resized. It returns nil when
// monitoring has been stopped.
func (s *sizeQueue) Next() *remotecommand.TerminalSize {
	return nil
}

func (s *sizeQueue) stop() {}

func (s *sizeQueue) push(size remotecommand.TerminalSize) {}

func (s *sizeQueue) watch(int) {}
