// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
)

// Predicate defines a function that takes a entity info delta and works out
// if it's valid or not.
type Predicate func(params.EntityInfo) bool

// PredicateInterpreter takes a query and generates a predicate from that query.
func PredicateInterpreter(query Query) (Predicate, error) {
	names := make([]string, 0, len(query.Arguments))
	for name := range query.Arguments {
		names = append(names, name)
	}
	sort.Strings(names)

	predicates := make([]Predicate, 0)
	for _, name := range names {
		args := query.Arguments[name]
		switch name {
		case "life":
			predicates = append(predicates, MatchesLife(args))
		case "status":
			predicates = append(predicates, MatchesStatus(args))
		default:
			return nil, errors.Errorf("unexpected query name %q", name)
		}
	}
	return ComposePredicates(predicates), nil
}

// BoolPredicate will take a boolean value and always return that given value
// no matter what the delta is.
func BoolPredicate(value bool) Predicate {
	return func(params.EntityInfo) bool {
		return value
	}
}

// MatchesLife takes a life and attempts to determine if the given entity
// delta correctly matches the life.
func MatchesLife(lives []string) Predicate {
	return func(info params.EntityInfo) bool {
		switch entityInfo := info.(type) {
		case *params.ModelUpdate:
			return isOneOf(lives, string(entityInfo.Life))
		case *params.ApplicationInfo:
			return isOneOf(lives, string(entityInfo.Life))
		}
		return false
	}
}

// MatchesStatus takes a status and attempts to match it with a given entity
// delta.
func MatchesStatus(statuses []string) Predicate {
	return func(info params.EntityInfo) bool {
		switch entityInfo := info.(type) {
		case *params.ModelUpdate:
			return isOneOf(statuses, entityInfo.Status.Current.String())
		case *params.ApplicationInfo:
			return isOneOf(statuses, entityInfo.Status.Current.String())
		}
		return false
	}
}

// ComposePredicates will compose a series of predicates into one. The first
// failure will cause the predicate to fail and subsequent predicates will not
// run.
func ComposePredicates(predicates []Predicate) Predicate {
	return func(info params.EntityInfo) bool {
		for _, fn := range predicates {
			if res := fn(info); !res {
				return false
			}
		}
		return true
	}
}

func isOneOf(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
