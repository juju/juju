// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"bytes"
)

type closingBuffer struct {
	bytes.Buffer
}

// Close implements io.Closer.
func (closingBuffer) Close() error {
	return nil
}
