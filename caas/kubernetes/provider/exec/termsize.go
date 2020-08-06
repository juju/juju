// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"os"

	"k8s.io/client-go/tools/remotecommand"
)

type sizeQueueInterface interface {
	Next() *remotecommand.TerminalSize
	stop()
	watch(int)
}

// SizeGetter defines methods for getting terminal size.
type SizeGetter interface {
	Get(int) *remotecommand.TerminalSize
}

func getFdInfo(in interface{}) (inFd int) {
	if file, ok := in.(*os.File); ok {
		inFd = int(file.Fd())
	}
	return inFd
}
