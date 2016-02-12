// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
)

type stubHookContext struct {
	stub *testing.Stub

	ReturnDownload string
}

func (s *stubHookContext) Download(name string) (string, error) {
	s.stub.AddCall("Download", name)
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return s.ReturnDownload, nil
}
