// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"io"

	"github.com/juju/juju/worker/common/hookcommands"
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

func cmdString(cmd string) string {
	return cmd + hookcommands.CmdSuffix
}
