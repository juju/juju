// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"sort"

	"github.com/juju/errors"

	apiclient "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/api"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
)

type waitForCommandBase struct {
	modelcmd.ModelCommandBase

	newWatchAllAPIFunc func() (api.WatchAllAPI, error)
}

type watchAllAPIShim struct {
	*apiclient.Client
}

func (s watchAllAPIShim) WatchAll() (api.AllWatcher, error) {
	return s.Client.WatchAll()
}

// runQuery handles the more complex error handling of a query with a given
// scope.
func runQuery(q query.Query, scope Scope) (bool, error) {
	if res, err := q.BuiltinsRun(scope); query.IsInvalidIdentifierErr(err) {
		return false, invalidIdentifierError(scope, err)
	} else if query.IsRuntimeError(err) {
		return false, errors.Trace(err)
	} else if res && err == nil {
		return true, nil
	} else if err != nil {
		logger.Errorf("%v", err)
	}
	return false, nil
}

// Scope defines a local scope used to get identifiers of a given scope.
type Scope interface {
	query.Scope

	// GetIdents returns the identifers that are supported for a given scope.
	GetIdents() []string
}

func invalidIdentifierError(scope Scope, err error) error {
	if !query.IsInvalidIdentifierErr(err) {
		return errors.Trace(err)
	}

	identErr := errors.Cause(err).(*query.InvalidIdentifierError)
	name := identErr.Name()

	idents := scope.GetIdents()

	type Indexed = struct {
		Name  string
		Value int
	}
	matches := make([]Indexed, 0, len(idents))
	for _, ident := range idents {
		matches = append(matches, Indexed{
			Name:  ident,
			Value: levenshteinDistance(name, ident),
		})
	}
	// Find the smallest levenshtein distance. If two values are the same,
	// fallback to sorting on the name, which should give predictable results.
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Value < matches[j].Value {
			return true
		}
		if matches[i].Value > matches[j].Value {
			return false
		}
		return matches[i].Name < matches[j].Name
	})
	matchedName := matches[0].Name
	matchedValue := matches[0].Value

	if matchedName != "" && matchedValue <= len(matchedName)+1 {
		additional := errors.Errorf(`%s

Did you mean:
	%s
`, err.Error(), matchedName)
		return errors.Wrap(err, additional)
	}

	return errors.Trace(err)
}

// levenshteinDistance
// from https://groups.google.com/forum/#!topic/golang-nuts/YyH1f_qCZVc
// (no min, compute lengths once, 2 rows array)
// fastest profiled
func levenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)
	d := make([]int, la+1)
	var lastdiag, olddiag, temp int

	for i := 1; i <= la; i++ {
		d[i] = i
	}
	for i := 1; i <= lb; i++ {
		d[0] = i
		lastdiag = i - 1
		for j := 1; j <= la; j++ {
			olddiag = d[j]
			min := d[j] + 1
			if (d[j-1] + 1) < min {
				min = d[j-1] + 1
			}
			if a[j-1] == b[i-1] {
				temp = 0
			} else {
				temp = 1
			}
			if (lastdiag + temp) < min {
				min = lastdiag + temp
			}
			d[j] = min
			lastdiag = olddiag
		}
	}
	return d[la]
}
