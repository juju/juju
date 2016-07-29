// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v4 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v4"

import (
	"net/http"
	"net/url"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

// GET id/meta/charm-related[?include=meta[&include=meta…]]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetacharm-related
func (h ReqHandler) metaCharmRelated(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
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
func (h ReqHandler) getRelatedCharmsResponse(
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

func (h ReqHandler) getRelatedIfaceResponses(
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

func (h ReqHandler) getMetadataForEntities(entities []*mongodoc.Entity, includes []string, req *http.Request, includeEntity func(*mongodoc.Entity) bool) ([]params.EntityResult, error) {
	response := make([]params.EntityResult, 0, len(entities))
	for _, inc := range includes {
		if h.Router.MetaHandler(inc) == nil {
			return nil, errgo.Newf("unrecognized metadata name %q", inc)
		}
	}
	err := expandMultiSeries(entities, func(series string, e *mongodoc.Entity) error {
		if includeEntity != nil && !includeEntity(e) {
			return nil
		}
		meta, err := h.getMetadataForEntity(e, includes, req)
		if err == errMetadataUnauthorized {
			return nil
		}
		if err != nil {
			// Unfortunately it is possible to get errors here due to
			// internal inconsistency, so rather than throwing away
			// all the search results, we just log the error and move on.
			logger.Errorf("cannot retrieve metadata for %v: %v", e.PreferredURL(true), err)
			return nil
		}
		id := e.PreferredURL(true)
		id.Series = series
		response = append(response, params.EntityResult{
			Id:   id,
			Meta: meta,
		})
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return response, nil
}

var errMetadataUnauthorized = errgo.Newf("metadata unauthorized")

func (h ReqHandler) getMetadataForEntity(e *mongodoc.Entity, includes []string, req *http.Request) (map[string]interface{}, error) {
	rurl := charmstore.EntityResolvedURL(e)
	// Ignore entities that aren't readable by the current user.
	if err := h.AuthorizeEntity(rurl, req); err != nil {
		return nil, errMetadataUnauthorized
	}
	return h.Router.GetMetadata(rurl, includes, req)
}
