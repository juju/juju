// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5/internal/v5"

import (
	"net/http"
	"sort"
	"strings"

	"gopkg.in/errgo.v1"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient/params"

	"gopkg.in/juju/charmstore.v5/internal/charmstore"
	"gopkg.in/juju/charmstore.v5/internal/entitycache"
	"gopkg.in/juju/charmstore.v5/internal/mongodoc"
)

// GET list[?filter=value…][&include=meta][&sort=field[+dir]]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-list
func (h *ReqHandler) serveList(_ http.Header, req *http.Request) (interface{}, error) {
	sp, err := ParseSearchParams(req)
	sp.AutoComplete = false
	if err != nil {
		return nil, err
	}
	h.WillIncludeMetadata(sp.Include)
	less, err := entityResultLess(sp.Sort)
	if err != nil {
		return nil, badRequestf(err, "")
	}
	lq, err := h.Store.ListQuery(sp)
	if err != nil {
		return nil, badRequestf(err, "")
	}
	var results []*mongodoc.Entity
	iter := h.Cache.CustomIter(entityCacheListQuery{lq}, nil)
	for iter.Next() {
		results = append(results, iter.Entity())
	}
	if iter.Err() != nil {
		return nil, errgo.Notef(err, "error listing charms and bundles")
	}
	r, err := h.getMetadataForEntities(results, sp.Include, req, nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get metadata")
	}
	sort.Sort(&entityResultsByOrder{
		less:    less,
		results: r,
	})
	return params.ListResponse{
		Results: uniqueEntityResults(r),
	}, nil
}

// uniqueEntityResults removes all adjacent entries that have the same URL but different
// revisions. Note that this relies on the fact that entityResultsByOrder sorts highest
// revision first.
func uniqueEntityResults(r []params.EntityResult) []params.EntityResult {
	var prev *params.EntityResult
	j := 0
	for i := range r {
		curr := &r[i]
		if prev != nil && eqURLWithoutRev(curr.Id, prev.Id) {
			continue
		}
		if i != j {
			r[j] = *curr
		}
		prev = curr
		j++
	}
	return r[0:j]
}

type entityResultsByOrder struct {
	less    func(r0, r1 *params.EntityResult) bool
	results []params.EntityResult
}

func (r *entityResultsByOrder) Less(i, j int) bool {
	return r.less(&r.results[i], &r.results[j])
}

func (r *entityResultsByOrder) Swap(i, j int) {
	r.results[i], r.results[j] = r.results[j], r.results[i]
}

func (r *entityResultsByOrder) Len() int {
	return len(r.results)
}

func entityResultLess(sp []charmstore.SortParam) (func(r0, r1 *params.EntityResult) bool, error) {
	comparers := make([]func(r0, r1 *params.EntityResult) int, 0, 4)
	for _, p := range sp {
		if !allowedListSortFields[p.Field] {
			return nil, errgo.Newf("sort %q not allowed", p.Field)
		}
		comparers = append(comparers, fieldCompare(p))
	}
	// Finally sort by id and decending revision when all other criteria are equal.
	comparers = append(comparers, fieldCompare(charmstore.SortParam{
		Field: "id",
	}))
	comparers = append(comparers, func(r0, r1 *params.EntityResult) int {
		return r1.Id.Revision - r0.Id.Revision
	})
	return func(r0, r1 *params.EntityResult) bool {
		for _, cmp := range comparers {
			if c := cmp(r0, r1); c != 0 {
				return c < 0
			}
		}
		return false
	}, nil
}

func fieldCompare(p charmstore.SortParam) func(r0, r1 *params.EntityResult) int {
	accessor := fieldAccessors[p.Field]
	return func(r0, r1 *params.EntityResult) int {
		r := strings.Compare(accessor(r0), accessor(r1))
		if p.Descending {
			return -r
		}
		return r
	}
}

var allowedListSortFields = map[string]bool{
	"name":   true,
	"owner":  true,
	"series": true,
}

var fieldAccessors = map[string]func(*params.EntityResult) string{
	"name": func(r *params.EntityResult) string {
		return r.Id.Name
	},
	"owner": func(r *params.EntityResult) string {
		return r.Id.User
	},
	"series": func(r *params.EntityResult) string {
		return r.Id.Series
	},
	"id": func(r *params.EntityResult) string {
		return r.Id.WithRevision(-1).String()
	},
}

func eqURLWithoutRev(u1, u2 *charm.URL) bool {
	return u1.Schema == u2.Schema &&
		u1.User == u2.User &&
		u1.Name == u2.Name &&
		u1.Series == u2.Series
}

type entityCacheListQuery struct {
	q *charmstore.ListQuery
}

func (q entityCacheListQuery) Iter(fields map[string]int) entitycache.StoreIter {
	return q.q.Iter(fields)
}
