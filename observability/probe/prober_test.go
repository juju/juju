// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package probe_test

import (
	"testing"

	"github.com/juju/errors"

	"github.com/juju/juju/observability/probe"
)

func TestProbeNotImplemented(t *testing.T) {
	status, err := probe.NotImplemented.Probe()
	if status {
		t.Errorf("expected probe.NotImplemented to return a false probe success")
	}

	if !errors.IsNotImplemented(err) {
		t.Errorf("expected probe.NotImplemented to return an error that satisfies errors.IsNotImplemented")
	}
	if err.Error() != "probe not implemented" {
		t.Errorf(`expected probe.NotImplemented to string to "probe not implemented" got: %q`, err.Error())
	}
}

func TestProbeSuccess(t *testing.T) {
	status, err := probe.Success.Probe()
	if err != nil {
		t.Errorf("got unexpected error result: %v", err)
	}

	if !status {
		t.Errorf("expected success probe to return true")
	}
}
