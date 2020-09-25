// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import "github.com/juju/juju/apiserver/params"

type Predicate func(params.EntityInfo) bool

func LifePredicate(life string) Predicate {
	return func(info params.EntityInfo) bool {
		switch entityInfo := info.(type) {
		case *params.ModelUpdate:
			return string(entityInfo.Life) == life
		case *params.ApplicationInfo:
			return string(entityInfo.Life) == life
		}
		return false
	}
}

func StatusPredicate(status string) Predicate {
	return func(info params.EntityInfo) bool {
		switch entityInfo := info.(type) {
		case *params.ModelUpdate:
			return entityInfo.Status.Current.String() == status
		case *params.ApplicationInfo:
			return entityInfo.Status.Current.String() == status
		}
		return false
	}
}

func ComposePredicates(predicates map[string]Predicate) Predicate {
	return func(info params.EntityInfo) bool {
		for _, fn := range predicates {
			if res := fn(info); !res {
				return false
			}
		}
		return true
	}
}
