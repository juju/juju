// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package probe_test

import (
	stdtesting "testing"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/observability/probe"
)

func TestProbeNotImplemented(t *stdtesting.T) {
	status, err := probe.NotImplemented.Probe()
	if status {
		t.Errorf("expected probe.NotImplemented to return a false probe success")
	}

	if !errors.Is(err, errors.NotImplemented) {
		t.Errorf("expected probe.NotImplemented to return an error that satisfies errors.IsNotImplemented")
	}
}

func TestProbeSuccess(t *stdtesting.T) {
	status, err := probe.Success.Probe()
	if err != nil {
		t.Errorf("got unexpected error result: %v", err)
	}

	if !status {
		t.Errorf("expected success probe to return true")
	}
}
