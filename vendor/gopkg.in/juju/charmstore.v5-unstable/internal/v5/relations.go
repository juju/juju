// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"net/http"
	"net/url"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/entitycache"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

// GET id/meta/charm-related[?include=meta[&include=meta…]]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetacharm-related
func (h *ReqHandler) metaCharmRelated(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	if id.URL.Series == "bundle" {
		return nil, nil
	}
	// If the charm does not define any relation we can just return without
	// hitting the db.
	if len(entity.CharmProvidedInterfaces)+len(entity.CharmRequiredInterfaces) == 0 {
		return &params.RelatedResponse{}, nil
	}
	fields := charmstore.FieldSelector(
		"supportedseries",
		"charmrequiredinterfaces",
		"charmprovidedinterfaces",
		"promulgated-url",
		"promulgated-revision",
	)
	query := h.Store.MatchingInterfacesQuery(entity.CharmProvidedInterfaces, entity.CharmRequiredInterfaces)
	iter := h.Cache.Iter(query.Sort("_id"), fields)
	var entities []*mongodoc.Entity
	for iter.Next() {
		entities = append(entities, iter.Entity())
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot retrieve the related charms")
	}

	// If no entities are found there is no need for further processing the
	// results.
	if len(entities) == 0 {
		return &params.RelatedResponse{}, nil
	}

	// Build the results, by grouping entities based on their relations' roles
	// and interfaces.
	includes := flags["include"]
	requires, err := h.getRelatedCharmsResponse(entity.CharmProvidedInterfaces, entities, func(e *mongodoc.Entity) []string {
		return e.CharmRequiredInterfaces
	}, includes, req)
	if err != nil {
		return nil, errgo.Notef(err, "cannot retrieve the charm requires")
	}
	provides, err := h.getRelatedCharmsResponse(entity.CharmRequiredInterfaces, entities, func(e *mongodoc.Entity) []string {
		return e.CharmProvidedInterfaces
	}, includes, req)
	if err != nil {
		return nil, errgo.Notef(err, "cannot retrieve the charm provides")
	}

	// Return the response.
	return &params.RelatedResponse{
		Requires: requires,
		Provides: provides,
	}, nil
}

// allEntities returns all the entities from the given iterator. It may
// return some entities and an error if some were read before the
// iterator completed.
func allEntities(iter *entitycache.Iter) ([]*mongodoc.Entity, error) {
	var entities []*mongodoc.Entity
	for iter.Next() {
		entities = append(entities, iter.Entity())
	}
	return entities, iter.Err()
}

type entityRelatedInterfacesGetter func(*mongodoc.Entity) []string

// getRelatedCharmsResponse returns a response mapping interfaces to related
// charms. For instance:
//   map[string][]params.MetaAnyResponse{
//       "http": []params.MetaAnyResponse{
//           {Id: "cs:utopic/django-42", Meta: ...},
//           {Id: "cs:trusty/wordpress-47", Meta: ...},
//       },
//       "memcache": []params.MetaAnyResponse{
//           {Id: "cs:utopic/memcached-0", Meta: ...},
//       },
//   }
func (h *ReqHandler) getRelatedCharmsResponse(
	ifaces []string,
	entities []*mongodoc.Entity,
	getInterfaces entityRelatedInterfacesGetter,
	includes []string,
	req *http.Request,
) (map[string][]params.EntityResult, error) {
	results := make(map[string][]params.EntityResult, len(ifaces))
	for _, iface := range ifaces {
		responses, err := h.getRelatedIfaceResponses(iface, entities, getInterfaces, includes, req)
		if err != nil {
			return nil, err
		}
		if len(responses) > 0 {
			results[iface] = responses
		}
	}
	return results, nil
}

func (h *ReqHandler) getRelatedIfaceResponses(
	iface string,
	entities []*mongodoc.Entity,
	getInterfaces entityRelatedInterfacesGetter,
	includes []string,
	req *http.Request,
) ([]params.EntityResult, error) {
	// Build a list of responses including only entities which are related
	// to the given interface.
	usesInterface := func(e *mongodoc.Entity) bool {
		for _, entityIface := range getInterfaces(e) {
			if entityIface == iface {
				return true
			}
		}
		return false
	}
	resp, err := h.getMetadataForEntities(entities, includes, req, usesInterface)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return resp, nil
}

// GET id/meta/bundles-containing[?include=meta[&include=meta…]][&any-series=1][&any-revision=1][&all-results=1]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetabundles-containing
func (h *ReqHandler) metaBundlesContaining(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	if id.URL.Series == "bundle" {
		return nil, nil
	}

	// Validate the URL query values.
	anySeries, err := router.ParseBool(flags.Get("any-series"))
	if err != nil {
		return nil, badRequestf(err, "invalid value for any-series")
	}
	anyRevision, err := router.ParseBool(flags.Get("any-revision"))
	if err != nil {
		return nil, badRequestf(err, "invalid value for any-revision")
	}
	allResults, err := router.ParseBool(flags.Get("all-results"))
	if err != nil {
		return nil, badRequestf(err, "invalid value for all-results")
	}

	// Mutate the reference so that it represents a base URL if required.
	prefURL := id.PreferredURL()
	searchId := *prefURL
	if anySeries || anyRevision {
		searchId.Revision = -1
		searchId.Series = ""
	}

	// Retrieve the bundles containing the resulting charm id.
	q := h.Store.DB.Entities().Find(bson.D{{"bundlecharms", &searchId}})
	iter := h.Cache.Iter(q, charmstore.FieldSelector("bundlecharms", "promulgated-url"))
	entities, err := allEntities(iter)
	if err != nil {
		return nil, errgo.Notef(err, "cannot retrieve the related bundles")
	}

	// Further filter the entities if required, by only including latest
	// bundle revisions and/or excluding specific charm series or revisions.

	// Filter entities so it contains only entities that actually
	// match the desired search criteria.
	filterEntities(&entities, func(e *mongodoc.Entity) bool {
		if anySeries == anyRevision {
			// If neither anySeries or anyRevision are true, then
			// the search will be exact and therefore e must be
			// matched.
			// If both anySeries and anyRevision are true, then
			// the base entity that we are searching for is exactly
			// what we want to search for, therefore e must be matched.
			return true
		}
		for _, charmId := range e.BundleCharms {
			if charmId.Name == prefURL.Name &&
				charmId.User == prefURL.User &&
				(anySeries || charmId.Series == prefURL.Series) &&
				(anyRevision || charmId.Revision == prefURL.Revision) {
				return true
			}
		}
		return false
	})

	var latest map[charm.URL]int
	if !allResults {
		// Include only the latest revision of any bundle.
		// This is made somewhat tricky by the fact that
		// each bundle can have two URLs, its canonical
		// URL (with user) and its promulgated URL.
		//
		// We want to maximise the URL revision regardless of
		// whether the URL is promulgated or not, so we
		// we build a map holding the latest revision for both
		// promulgated and non-promulgated revisions
		// and then include entities that have the latest
		// revision for either.
		latest = make(map[charm.URL]int)

		// updateLatest updates the latest revision for u
		// without its revision if it's greater than the existing
		// entry.
		updateLatest := func(u *charm.URL) {
			u1 := *u
			u1.Revision = -1
			if rev, ok := latest[u1]; !ok || rev < u.Revision {
				latest[u1] = u.Revision
			}
		}
		for _, e := range entities {
			updateLatest(e.URL)
			if e.PromulgatedURL != nil {
				updateLatest(e.PromulgatedURL)
			}
		}
		filterEntities(&entities, func(e *mongodoc.Entity) bool {
			if e.PromulgatedURL != nil {
				u := *e.PromulgatedURL
				u.Revision = -1
				if latest[u] == e.PromulgatedURL.Revision {
					return true
				}
			}
			u := *e.URL
			u.Revision = -1
			return latest[u] == e.URL.Revision
		})
	}
	resp, err := h.getMetadataForEntities(entities, flags["include"], req, nil)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return resp, nil
}

func (h *ReqHandler) getMetadataForEntities(entities []*mongodoc.Entity, includes []string, req *http.Request, includeEntity func(*mongodoc.Entity) bool) ([]params.EntityResult, error) {
	for _, inc := range includes {
		if h.Router.MetaHandler(inc) == nil {
			return nil, errgo.Newf("unrecognized metadata name %q", inc)
		}
	}
	response := make([]params.EntityResult, 0, len(entities))
	for _, e := range entities {
		if includeEntity != nil && !includeEntity(e) {
			continue
		}
		meta, err := h.getMetadataForEntity(e, includes, req)
		if err == errMetadataUnauthorized {
			continue
		}
		if err != nil {
			// Unfortunately it is possible to get errors here due to
			// internal inconsistency, so rather than throwing away
			// all the search results, we just log the error and move on.
			logger.Errorf("cannot retrieve metadata for %v: %v", e.PreferredURL(true), err)
			continue
		}
		response = append(response, params.EntityResult{
			Id:   e.PreferredURL(true),
			Meta: meta,
		})
	}
	return response, nil
}

var errMetadataUnauthorized = errgo.Newf("metadata unauthorized")

func (h *ReqHandler) getMetadataForEntity(e *mongodoc.Entity, includes []string, req *http.Request) (map[string]interface{}, error) {
	rurl := charmstore.EntityResolvedURL(e)
	// Ignore entities that aren't readable by the current user.
	if err := h.AuthorizeEntity(rurl, req); err != nil {
		return nil, errMetadataUnauthorized
	}
	return h.Router.GetMetadata(rurl, includes, req)
}

// filterEntities deletes all entities from *entities for which
// the given predicate returns false.
func filterEntities(entities *[]*mongodoc.Entity, predicate func(*mongodoc.Entity) bool) {
	entities1 := *entities
	j := 0
	for _, e := range entities1 {
		if predicate(e) {
			entities1[j] = e
			j++
		}
	}
	*entities = entities1[0:j]
}
