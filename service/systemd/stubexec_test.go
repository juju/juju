// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"github.com/juju/testing"
	"github.com/juju/utils/exec"
)

type StubExec struct {
	*testing.Stub

	Responses []exec.ExecResponse
}

func (se *StubExec) SetResponses(resp ...exec.ExecResponse) {
	se.Responses = resp
}

func (se *StubExec) RunCommand(args exec.RunParams) (*exec.ExecResponse, error) {
	se.AddCall("RunCommand", args)

	var response exec.ExecResponse
	if len(se.Responses) > 0 {
		response = se.Responses[0]
		se.Responses = se.Responses[1:]
	}
	err := se.NextErr()
	if err != nil {
		return nil, err
	}
	return &response, nil
}
