// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build windows

package exec

import "context"

// TODO: implement window resizing for windows.
func newSizeQueue() sizeQueueInterface {
	logger.Warningf(context.TODO(), "terminal window resizing does not support on windows")
	return nil
}
