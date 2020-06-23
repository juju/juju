// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/worker/logsender"
)

func NewForTest(ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (cmd.Command, error) {
	// TODO
	return &k8sUnitAgent{}, nil
}
