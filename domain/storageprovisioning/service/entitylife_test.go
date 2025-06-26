// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"iter"
	"slices"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
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

// initialEntityLifeQuery is a helper function that returns an eventsource query
// that will return the data supplied when called.
func initialEntityLifeQuery(data map[string]life.Life) eventsource.Query[map[string]life.Life] {
	return func(_ context.Context, _ database.TxnRunner) (map[string]life.Life, error) {
		return data, nil
	}
}

// TestEntityLifeMapper is a test of tests for making sure that the
// [EntityLifeMapperFunc] correctly handles changes in values over time. i.e the
// caller is correctly notified of the right ids when change has occurred.
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

			initialLifeFn := func(_ context.Context) (map[string]life.Life, error) {
				return test.InitialLife, nil
			}
			mapper := EntityLifeMapperFunc(initialLifeFn, getter)

			for _, expected := range test.Expected {
				changes, err := mapper(t.Context(), nil)
				tc.Check(t, err, tc.ErrorIsNil)
				tc.Check(t, changes, tc.SameContents, expected)
			}
		})
	}
}

// TestMakeEntityLifePrerequisites tests the common case of the mapper and
// initial returned by [MakeEntityLifePrerequisites].
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

	initData := map[string]life.Life{
		"l1": life.Alive,
		"l2": life.Dying,
	}

	initQuery, mapper := MakeEntityLifePrerequisites(
		initialEntityLifeQuery(initData), lifeGetter,
	)

	initVals, err := initQuery(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, initVals, tc.SameContents, []string{"l1", "l2"})

	changes, err := mapper(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, changes, tc.SameContents, []string{"l8"})

	changes, err = mapper(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, changes, tc.SameContents, []string{"l2", "l8", "l9"})
}

// TestMakeEntityLifePrerequisitesNoDeadLock tests that if the initial query
// provided by [MakeEntityLifePrerequisites] does not get called so that the
// initial values to the mapper are not available, the mapper keeps working and
// starts from an empty set of values.
func TestMakeEntityLifePrerequisitesNoDeadLock(t *testing.T) {
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

	_, mapper := MakeEntityLifePrerequisites(
		initialEntityLifeQuery(nil), lifeGetter,
	)

	changes, err := mapper(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, changes, tc.SameContents, []string{"l1", "l2", "l8"})

	changes, err = mapper(t.Context(), nil)
	tc.Check(t, err, tc.ErrorIsNil)
	tc.Check(t, changes, tc.SameContents, []string{"l2", "l8", "l9"})
}
