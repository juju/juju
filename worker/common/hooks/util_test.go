// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hooks_test

import (
	"bytes"
	"io"
)

const (
	formatYaml = iota
	formatJson
)

func bufferBytes(stream io.Writer) []byte {
	return stream.(*bytes.Buffer).Bytes()
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}
