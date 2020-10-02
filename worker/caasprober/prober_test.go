// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober_test

import (
	"errors"
	"testing"

	"github.com/juju/juju/worker/caasprober"
)

func TestProbeNotImplemented(t *testing.T) {
	prober := caasprober.ProbeNotImplemented{}
	res, err := prober.Probe()
	if res {
		t.Errorf("expected bool result from ProberNotImplemented to be false")
	}
	if !errors.Is(err, caasprober.ErrorProbeNotImplemented) {
		t.Errorf("expected ProberNotImplemented to return ErrorProberNotImplemented error")
	}
}

func TestProbeSuccess(t *testing.T) {
	prober := caasprober.ProbeSuccess{}
	res, err := prober.Probe()
	if err != nil {
		t.Errorf("expected ProbeSuccess to not produce error result")
	}
	if !res {
		t.Errorf("expected ProberSuccess to always return true for probe")
	}
}
