// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"iter"
	"slices"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// entityLifeGetter is a testing implementation of [EntityLifeGetter] that
// returns the values supplied in the sequence each time the getter is called. A
// stop function is returned that must be called when the getter is no longer
// required.
func entityLifeGetter(
	seq iter.Seq[map[string]life.Life],
) (EntityLifeGetter, func()) {
	next, stop := iter.Pull(seq)
	return func(_ context.Context) (map[string]life.Life, error) {
		vals, valid := next()
		if !valid {
			return nil, errors.Errorf("no more values in sequence")
		}
		return vals, nil
	}, stop
}

// TestEntityLifeInitialQuery tests that the [EntityLifeInitialQuery] correctly
// returns the initial id values from the provided initial life map.
func TestEntityLifeInitialQuery(t *testing.T) {
	initialLife := map[string]life.Life{
		"l1": life.Alive,
		"l2": life.Dead,
	}

	query := EntityLifeInitialQuery(initialLife)
	vals, err := query(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, vals, tc.SameContents, []string{"l1", "l2"})
}

// TestEntityLifeInitialQueryEmpty tests that the [EntityLifeInitialQuery]
// correctly returns an empty slice when the initial life map is empty.
func TestEntityLifeInitialQueryEmpty(t *testing.T) {
	query := EntityLifeInitialQuery(nil)
	vals, err := query(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, vals, tc.HasLen, 0)
}

// TestEntityLifeMapper is a test of tests for making sure that the
// [EntityLifeMapperFunc] correctly handles changes in values over time. i.e the
// caller is correctly notified of the right ids when change has occured.
func TestEntityLifeMapper(t *testing.T) {
	test := []struct {
		Name        string
		InitialLife map[string]life.Life
		LifeStages  []map[string]life.Life
		Expected    [][]string
	}{
		{
			// We want to see that over multiple calls to the getter the mapper
			// picks up new id values over time.
			Name: "mapper picks up new id values",
			InitialLife: map[string]life.Life{
				"1": life.Alive,
			},
			LifeStages: []map[string]life.Life{
				{
					"1": life.Alive,
					"2": life.Alive,
				},
				{
					"1": life.Alive,
					"2": life.Alive,
					"3": life.Alive,
				},
			},
			Expected: [][]string{
				{"2"},
				{"3"},
			},
		},
		// We want to see that over multiple calls to the getter the mapper
		// picks up removed id values over time.
		{
			Name: "mapper picks up removed id values",
			InitialLife: map[string]life.Life{
				"1": life.Alive,
				"2": life.Alive,
				"3": life.Alive,
			},
			LifeStages: []map[string]life.Life{
				{
					"1": life.Alive,
					"2": life.Alive,
				},
				{
					"1": life.Alive,
				},
			},
			Expected: [][]string{
				{"3"},
				{"2"},
			},
		},
		{
			Name: "mapper picks up changed id values",
			InitialLife: map[string]life.Life{
				"1": life.Alive,
				"2": life.Alive,
				"3": life.Alive,
			},
			LifeStages: []map[string]life.Life{
				{
					"1": life.Dead,
					"2": life.Dying,
					"3": life.Dying,
				},
				{},
			},
			Expected: [][]string{
				{"1", "2", "3"},
				{"1", "2", "3"},
			},
		},
		{
			Name: "mapper mixed changed",
			InitialLife: map[string]life.Life{
				"fs1": life.Alive,
				"fs2": life.Alive,
				"fs3": life.Alive,
			},
			LifeStages: []map[string]life.Life{
				{
					"fs1": life.Dead,
					"fs2": life.Alive,
					"fs3": life.Alive,
					"fs4": life.Alive,
				},
				{
					"fs2": life.Dying,
					"fs3": life.Alive,
					"fs4": life.Alive,
				},
			},
			Expected: [][]string{
				{"fs1", "fs4"},
				{"fs1", "fs2"},
			},
		},
		{
			Name:        "nil getter values",
			InitialLife: map[string]life.Life{},
			LifeStages: []map[string]life.Life{
				{},
			},
			Expected: [][]string{
				{},
			},
		},
	}

	for _, test := range test {
		t.Run(test.Name, func(t *testing.T) {
			seq := slices.Values(test.LifeStages)
			getter, stop := entityLifeGetter(seq)
			defer stop()
			mapper := EntityLifeMapperFunc(test.InitialLife, getter)

			for _, expected := range test.Expected {
				changes, err := mapper(t.Context(), nil)
				tc.Check(t, err, tc.ErrorIsNil)
				tc.Check(t, changes, tc.SameContents, expected)
			}
		})
	}
}

func TestMakeEntityLifePrerequisties(t *testing.T) {
	lifeGetter, stop := entityLifeGetter(slices.Values([]map[string]life.Life{
		{
			"l1": life.Alive,
			"l2": life.Dying,
			"l8": life.Alive,
		},
		{
			"l1": life.Alive,
			"l8": life.Dying,
			"l9": life.Alive,
		},
	}))
	defer stop()

	initQuery, mapper, err := MakeEntityLifePrerequisites(t.Context(), lifeGetter)
	tc.Check(t, err, tc.ErrorIsNil)

	initVals, err := initQuery(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, initVals, tc.SameContents, []string{"l1", "l2", "l8"})

	changes, err := mapper(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, changes, tc.SameContents, []string{"l2", "l8", "l9"})
}
