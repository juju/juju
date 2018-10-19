// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interact

import (
	"io"

	"github.com/juju/ansiterm"
)

// NewErrWriter wraps w in a type that will cause all writes to be written as
// ansi terminal BrightRed.
func NewErrWriter(w io.Writer) io.Writer {
	return errWriter{ansiterm.NewWriter(w)}
}

// errWriter is a little type that ensures that anything written to it is
// written in BrightRed.
type errWriter struct {
	w *ansiterm.Writer
}

func (w errWriter) Write(b []byte) (n int, err error) {
	w.w.SetForeground(ansiterm.BrightRed)
	defer w.w.Reset()
	return w.w.Write(b)
}
