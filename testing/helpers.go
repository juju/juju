// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
)

// CloseableBuffer is a closeable wrapper around a bytes.Buffer,
// allowing a buffer to be used as an io.ReadCloser.
type CloseableBuffer struct {
	*bytes.Buffer
}

func NewCloseableBufferString(data string) *CloseableBuffer {
	return &CloseableBuffer{bytes.NewBufferString(data)}
}

func (f *CloseableBuffer) Close() error {
	return nil
}
