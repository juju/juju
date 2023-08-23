// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package probe_test

import (
	"errors"
	"testing"

	"github.com/juju/juju/internal/observability/probe"
)

func TestAggregateProbeSuccess(t *testing.T) {
	agg := probe.Aggregate{
		Probes: map[string]probe.Prober{
			"1": probe.Success,
			"2": probe.Success,
			"3": probe.Success,
			"4": probe.Success,
		},
	}

	val, err := agg.Probe()
	if !val {
		t.Errorf("expected aggregate probe of all success to provide a true value")
	}

	if err != nil {
		t.Errorf("unexpected error %v from all success probes to aggregate", err)
	}
}

func TestEmptyAggregateSuccess(t *testing.T) {
	agg := probe.Aggregate{}
	val, err := agg.Probe()
	if !val {
		t.Errorf("expected empty aggregate probe to provide a true value")
	}

	if err != nil {
		t.Errorf("unexpected error %v from empty aggregate probe", err)
	}
}

func TestSingleFalseAggregateProbe(t *testing.T) {
	agg := probe.Aggregate{
		Probes: map[string]probe.Prober{
			"1": probe.Success,
			"2": probe.ProberFn(func() (bool, error) {
				return false, nil
			}),
			"3": probe.Success,
		},
	}
	val, err := agg.Probe()
	if val {
		t.Errorf("expected aggregate with false probe to to provide a false value")
	}
	if err != nil {
		t.Errorf("unexpected error %v from aggregate prober", err)
	}
}

func TestMultipleFalseAggregateProbe(t *testing.T) {
	agg := probe.Aggregate{
		Probes: map[string]probe.Prober{
			"1": probe.Success,
			"2": probe.ProberFn(func() (bool, error) {
				return false, nil
			}),
			"3": probe.Success,
			"4": probe.ProberFn(func() (bool, error) {
				return false, nil
			}),
			"5": probe.Success,
		},
	}
	val, err := agg.Probe()
	if val {
		t.Errorf("expected aggregate with false probes to to provide a false value")
	}
	if err != nil {
		t.Errorf("unexpected error %v from aggregate prober", err)
	}
}

func TestAggregateProbeWithError(t *testing.T) {
	agg := probe.Aggregate{
		Probes: map[string]probe.Prober{
			"1": probe.Success,
			"2": probe.ProberFn(func() (bool, error) {
				return false, errors.New("test error")
			}),
			"3": probe.Success,
		},
	}
	val, err := agg.Probe()
	if val {
		t.Errorf("expected aggregate with false probe to to provide a false value")
	}
	if err == nil {
		t.Errorf("expected error from aggregate prober")
	}
}

func TestAggregateProbeCallback(t *testing.T) {
	agg := probe.Aggregate{
		Probes: map[string]probe.Prober{
			"1": probe.Success,
			"2": probe.ProberFn(func() (bool, error) {
				return false, errors.New("test error")
			}),
			"3": probe.Success,
		},
	}

	val, err := agg.ProbeWithResultCallback(func(k string, val bool, err error) {
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
	if err == nil {
		t.Errorf("expected error from aggregate prober")
	}
}
