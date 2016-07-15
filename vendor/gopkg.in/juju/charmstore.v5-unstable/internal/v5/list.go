// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/entitycache"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
)

// GET list[?filter=value…][&include=meta][&sort=field[+dir]]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-list
func (h *ReqHandler) serveList(_ http.Header, req *http.Request) (interface{}, error) {
	sp, err := ParseSearchParams(req)
	if err != nil {
		return "", err
	}
	h.WillIncludeMetadata(sp.Include)

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
	return params.ListResponse{
		Results: r,
	}, nil
}

type entityCacheListQuery struct {
	q *charmstore.ListQuery
}

func (q entityCacheListQuery) Iter(fields map[string]int) entitycache.StoreIter {
	return q.q.Iter(fields)
}
