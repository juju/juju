// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package probe_test

import (
	"errors"
	stdtesting "testing"

	"github.com/juju/juju/internal/observability/probe"
)

func TestAggregateProbeSuccess(t *stdtesting.T) {
	agg := probe.Aggregate{}
	agg.AddProber("1", probe.Success)
	agg.AddProber("2", probe.Success)
	agg.AddProber("3", probe.Success)
	agg.AddProber("4", probe.Success)

	val, n, err := agg.Probe()
	if !val {
		t.Errorf("expected aggregate probe of all success to provide a true value")
	}
	if n != 4 {
		t.Errorf("expected aggregate probe to return 4 probes read")
	}
	if err != nil {
		t.Errorf("unexpected error %v from all success probes to aggregate", err)
	}
}

func TestEmptyAggregateSuccess(t *stdtesting.T) {
	agg := probe.Aggregate{}
	val, n, err := agg.Probe()
	if !val {
		t.Errorf("expected empty aggregate probe to provide a true value")
	}
	if n != 0 {
		t.Errorf("expected aggregate probe to return 0 probes read")
	}
	if err != nil {
		t.Errorf("unexpected error %v from empty aggregate probe", err)
	}
}

func TestSingleFalseAggregateProbe(t *stdtesting.T) {
	agg := probe.Aggregate{}
	agg.AddProber("1", probe.Success)
	agg.AddProber("2", probe.ProberFn(func() (bool, error) {
		return false, nil
	}))
	agg.AddProber("3", probe.Success)
	val, n, err := agg.Probe()
	if val {
		t.Errorf("expected aggregate with false probe to to provide a false value")
	}
	if n != 3 {
		t.Errorf("expected aggregate probe to return 3 probes read")
	}
	if err != nil {
		t.Errorf("unexpected error %v from aggregate prober", err)
	}
}

func TestMultipleFalseAggregateProbe(t *stdtesting.T) {
	agg := probe.Aggregate{}
	agg.AddProber("1", probe.Success)
	agg.AddProber("2", probe.ProberFn(func() (bool, error) {
		return false, nil
	}))
	agg.AddProber("3", probe.Success)
	agg.AddProber("4", probe.ProberFn(func() (bool, error) {
		return false, nil
	}))
	agg.AddProber("5", probe.Success)
	val, n, err := agg.Probe()
	if val {
		t.Errorf("expected aggregate with false probes to to provide a false value")
	}
	if n != 5 {
		t.Errorf("expected aggregate probe to return 5 probes read")
	}
	if err != nil {
		t.Errorf("unexpected error %v from aggregate prober", err)
	}
}

func TestAggregateProbeWithError(t *stdtesting.T) {
	agg := probe.Aggregate{}
	agg.AddProber("1", probe.Success)
	agg.AddProber("2", probe.ProberFn(func() (bool, error) {
		return false, errors.New("test error")
	}))
	agg.AddProber("3", probe.Success)

	val, n, err := agg.Probe()
	if val {
		t.Errorf("expected aggregate with false probe to to provide a false value")
	}
	if n != 3 {
		t.Errorf("expected aggregate probe to return 3 probes read")
	}
	if err == nil {
		t.Errorf("expected error from aggregate prober")
	}
}

func TestAggregateProbeCallback(t *stdtesting.T) {
	agg := probe.Aggregate{}
	agg.AddProber("1", probe.Success)
	agg.AddProber("2", probe.ProberFn(func() (bool, error) {
		return false, errors.New("test error")
	}))
	agg.AddProber("3", probe.Success)

	val, n, err := agg.ProbeWithResultCallback(func(k string, val bool, err error) {
		expectedVal := true
		shouldError := false

		switch k {
		case "1":
		case "3":
			break
		case "2":
			expectedVal = false
			shouldError = true
		default:
			t.Errorf("unknown key %q supplied to aggregate callback", k)
		}

		if expectedVal != val {
			t.Errorf("expected %v for key %q and got %v", expectedVal, k, val)
		}

		if err != nil && !shouldError {
			t.Errorf("unexpected error %v", err)
		}
	})

	if val {
		t.Errorf("expected aggregate with false probe to to provide a false value")
	}
	if n != 3 {
		t.Errorf("expected aggregate probe to return 3 probes read")
	}
	if err == nil {
		t.Errorf("expected error from aggregate prober")
	}
}
