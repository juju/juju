// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/elasticsearch"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
	"gopkg.in/juju/charmstore.v5-unstable/internal/series"
)

type SearchIndex struct {
	*elasticsearch.Database
	Index string
}

const typeName = "entity"

// seriesBoost defines how much the results for each
// series will be boosted. Series are currently ranked in
// reverse order of LTS releases, followed by the latest
// non-LTS release, followed by everything else.
var seriesBoost = func() map[string]float64 {
	m := make(map[string]float64)
	for k, v := range series.Series {
		if !v.SearchIndex {
			continue
		}
		m[k] = v.SearchBoost
	}
	return m
}()

// SearchDoc is a mongodoc.Entity with additional fields useful for searching.
// This is the document that is stored in the search index.
type SearchDoc struct {
	*mongodoc.Entity
	TotalDownloads int64
	ReadACLs       []string
	Series         []string

	// SingleSeries is true if the document referes to an entity that
	// describes a single series. This will either be a bundle, a
	// single-series charm or an expanded record for a multi-series
	// charm.
	SingleSeries bool

	// AllSeries is true if the document referes to an entity that
	// describes all series supported by the entity. This will either
	// be a bundle, a single-series charm or the canonical record for
	// a multi-series charm.
	AllSeries bool
}

// UpdateSearchAsync will update the search record for the entity
// reference r in the backgroud.
func (s *Store) UpdateSearchAsync(r *router.ResolvedURL) {
	s.Go(func(s *Store) {
		if err := s.UpdateSearch(r); err != nil {
			logger.Errorf("cannot update search record for %v: %s", r, err)
		}
	})
}

// UpdateSearch updates the search record for the entity reference r. The
// search index only includes the latest stable revision of each entity
// so the latest stable revision of the charm specified by r will be
// indexed.
func (s *Store) UpdateSearch(r *router.ResolvedURL) error {
	if s.ES == nil || s.ES.Database == nil {
		return nil
	}
	// For multi-series charms update the whole base URL.
	if r.URL.Series == "" {
		return s.UpdateSearchBaseURL(&r.URL)
	}

	if !series.Series[r.URL.Series].SearchIndex {
		return nil
	}
	baseEntity, err := s.FindBaseEntity(&r.URL, nil)
	if err != nil {
		return errgo.NoteMask(err, fmt.Sprintf("cannot update search record for %q", &r.URL), errgo.Is(params.ErrNotFound))
	}
	series := r.URL.Series
	entityURL := baseEntity.ChannelEntities[params.StableChannel][series]
	if entityURL == nil {
		// There is no stable version of the entity to index.
		return nil
	}
	entity, err := s.FindEntity(&router.ResolvedURL{URL: *entityURL}, nil)
	if err != nil {
		return errgo.Notef(err, "cannot update search record for %q", entityURL)
	}
	if err := s.updateSearchEntity(entity, baseEntity); err != nil {
		return errgo.Notef(err, "cannot update search record for %q", entityURL)
	}
	return nil
}

// UpdateSearchBaseURL updates the search record for all entities with
// the specified base URL. It must be called whenever the entry for the
// given URL in the BaseEntitites collection has changed.
func (s *Store) UpdateSearchBaseURL(baseURL *charm.URL) error {
	if s.ES == nil || s.ES.Database == nil {
		return nil
	}
	baseEntity, err := s.FindBaseEntity(baseURL, nil)
	if err != nil {
		return errgo.NoteMask(err, fmt.Sprintf("cannot index %s", baseURL), errgo.Is(params.ErrNotFound))
	}
	stableEntities := baseEntity.ChannelEntities[params.StableChannel]
	updated := make(map[string]bool, len(stableEntities))
	for urlSeries, url := range stableEntities {
		if !series.Series[urlSeries].SearchIndex {
			continue
		}
		if updated[url.String()] {
			continue
		}
		updated[url.String()] = true
		entity, err := s.FindEntity(&router.ResolvedURL{URL: *url}, nil)
		if err != nil {
			return errgo.Notef(err, "cannot update search record for %q", url)
		}
		if err := s.updateSearchEntity(entity, baseEntity); err != nil {
			return errgo.Notef(err, "cannot update search record for %q", url)
		}
	}
	return nil
}

func (s *Store) updateSearchEntity(entity *mongodoc.Entity, baseEntity *mongodoc.BaseEntity) error {
	doc, err := s.searchDocFromEntity(entity, baseEntity)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := s.ES.update(doc); err != nil {
		return errgo.Notef(err, "cannot update search index")
	}
	return nil
}

// UpdateSearchFields updates the search record for the entity reference r
// with the updated values in fields.
func (s *Store) UpdateSearchFields(r *router.ResolvedURL, fields map[string]interface{}) error {
	if s.ES == nil || s.ES.Database == nil {
		return nil
	}
	var needUpdate bool
	for k := range fields {
		// Add any additional fields here that should update the search index.
		if k == "extrainfo.legacy-download-stats" {
			needUpdate = true
		}
	}
	if !needUpdate {
		return nil
	}
	if err := s.UpdateSearch(r); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// searchDocFromEntity performs the processing required to convert a
// mongodoc.Entity and the corresponding mongodoc.BaseEntity to an esDoc
// for indexing.
func (s *Store) searchDocFromEntity(e *mongodoc.Entity, be *mongodoc.BaseEntity) (*SearchDoc, error) {
	doc := SearchDoc{Entity: e}
	doc.ReadACLs = be.ChannelACLs[params.StableChannel].Read
	// There should only be one record for the promulgated entity, which
	// should be the latest promulgated revision. In the case that the base
	// entity is not promulgated assume that there is a later promulgated
	// entity.
	if !be.Promulgated {
		doc.Entity.PromulgatedURL = nil
		doc.Entity.PromulgatedRevision = -1
	}
	_, allRevisions, err := s.ArchiveDownloadCounts(EntityResolvedURL(e).PreferredURL(), false)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	doc.TotalDownloads = allRevisions.Total
	if doc.Entity.Series == "bundle" {
		doc.Series = []string{"bundle"}
	} else {
		doc.Series = doc.Entity.SupportedSeries
	}
	doc.AllSeries = true
	doc.SingleSeries = doc.Entity.Series != ""
	return &doc, nil
}

// update inserts an entity into elasticsearch if elasticsearch
// is configured. The entity with id r is extracted from mongodb
// and written into elasticsearch.
func (si *SearchIndex) update(doc *SearchDoc) error {
	if si == nil || si.Database == nil {
		return nil
	}
	err := si.PutDocumentVersionWithType(
		si.Index,
		typeName,
		si.getID(doc.URL),
		int64(doc.URL.Revision),
		elasticsearch.ExternalGTE,
		doc)
	if err != nil && err != elasticsearch.ErrConflict {
		return errgo.Mask(err)
	}
	if doc.Entity.URL.Series != "" {
		return nil
	}
	// This document represents a multi-series charm. Expand the
	// document for each of the supported series.
	for _, series := range doc.Entity.SupportedSeries {
		u := *doc.Entity.URL
		u.Series = series
		doc.Entity.URL = &u
		if doc.PromulgatedURL != nil {
			u := *doc.Entity.PromulgatedURL
			u.Series = series
			doc.Entity.PromulgatedURL = &u
		}
		doc.Series = []string{series}
		doc.AllSeries = false
		doc.SingleSeries = true
		if err := si.update(doc); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

// getID returns an ID for the elasticsearch document based on the contents of the
// mongoDB document. This is to allow elasticsearch documents to be replaced with
// updated versions when charm data is changed.
func (si *SearchIndex) getID(r *charm.URL) string {
	ref := *r
	ref.Revision = -1
	b := sha1.Sum([]byte(ref.String()))
	s := base64.URLEncoding.EncodeToString(b[:])
	// Cut off any trailing = as there is no need for them and they will get URL escaped.
	return strings.TrimRight(s, "=")
}

// Search searches for matching entities in the configured elasticsearch index.
// If there is no elasticsearch index configured then it will return an empty
// SearchResult, as if no results were found.
func (si *SearchIndex) search(sp SearchParams) (SearchResult, error) {
	if si == nil || si.Database == nil {
		return SearchResult{}, nil
	}
	q := createSearchDSL(sp)
	q.Fields = append(q.Fields, "URL", "PromulgatedURL", "Series")
	esr, err := si.Search(si.Index, typeName, q)
	if err != nil {
		return SearchResult{}, errgo.Mask(err)
	}
	r := SearchResult{
		SearchTime: time.Duration(esr.Took) * time.Millisecond,
		Total:      esr.Hits.Total,
		Results:    make([]*mongodoc.Entity, 0, len(esr.Hits.Hits)),
	}
	for _, h := range esr.Hits.Hits {
		urlStr := h.Fields.GetString("URL")
		url, err := charm.ParseURL(urlStr)
		if err != nil {
			return SearchResult{}, errgo.Notef(err, "invalid URL in result %q", urlStr)
		}
		e := &mongodoc.Entity{
			URL: url,
		}
		if url.Series == "" {
			series := make([]string, len(h.Fields["Series"]))
			for i, s := range h.Fields["Series"] {
				series[i] = s.(string)
			}
			e.SupportedSeries = series
		} else if url.Series != "bundle" {
			e.SupportedSeries = []string{url.Series}
		}
		if purlStr := h.Fields.GetString("PromulgatedURL"); purlStr != "" {
			purl, err := charm.ParseURL(purlStr)
			if err != nil {
				return SearchResult{}, errgo.Notef(err, "invalid promulgated URL in result %q", purlStr)
			}
			e.PromulgatedURL = purl
			e.PromulgatedRevision = purl.Revision
		} else {
			e.PromulgatedURL = nil
			e.PromulgatedRevision = -1
		}
		r.Results = append(r.Results, e)
	}
	return r, nil
}

// GetSearchDocument retrieves the current search record for the charm
// reference id.
func (si *SearchIndex) GetSearchDocument(id *charm.URL) (*SearchDoc, error) {
	if si == nil || si.Database == nil {
		return &SearchDoc{}, nil
	}
	var s SearchDoc
	err := si.GetDocument(si.Index, "entity", si.getID(id), &s)
	if err != nil {
		return nil, errgo.Notef(err, "cannot retrieve search document for %v", id)
	}
	return &s, nil
}

// version is a document that stores the structure information
// in the elasticsearch database.
type version struct {
	Version int64
	Index   string
}

const versionIndex = ".versions"
const versionType = "version"

// ensureIndexes makes sure that the required indexes exist and have the right
// settings. If force is true then ensureIndexes will create new indexes irrespective
// of the status of the current index.
func (si *SearchIndex) ensureIndexes(force bool) error {
	if si == nil || si.Database == nil {
		return nil
	}
	old, dv, err := si.getCurrentVersion()
	if err != nil {
		return errgo.Notef(err, "cannot get current version")
	}
	if !force && old.Version >= esSettingsVersion {
		return nil
	}
	index, err := si.newIndex()
	if err != nil {
		return errgo.Notef(err, "cannot create index")
	}
	new := version{
		Version: esSettingsVersion,
		Index:   index,
	}
	updated, err := si.updateVersion(new, dv)
	if err != nil {
		return errgo.Notef(err, "cannot update version")
	}
	if !updated {
		// Update failed so delete the new index
		if err := si.DeleteIndex(index); err != nil {
			return errgo.Notef(err, "cannot delete index")
		}
		return nil
	}
	// Update succeeded - update the aliases
	if err := si.Alias(index, si.Index); err != nil {
		return errgo.Notef(err, "cannot create alias")
	}
	// Delete the old unused index
	if old.Index != "" {
		if err := si.DeleteIndex(old.Index); err != nil {
			return errgo.Notef(err, "cannot delete index")
		}
	}
	return nil
}

// getCurrentVersion gets the version of elasticsearch settings, if any
// that are deployed to elasticsearch.
func (si *SearchIndex) getCurrentVersion() (version, int64, error) {
	var v version
	d, err := si.GetESDocument(versionIndex, versionType, si.Index)
	if err != nil && err != elasticsearch.ErrNotFound {
		return version{}, 0, errgo.Notef(err, "cannot get settings version")
	}
	if d.Found {
		if err := json.Unmarshal(d.Source, &v); err != nil {
			return version{}, 0, errgo.Notef(err, "invalid version")
		}
	}
	return v, d.Version, nil
}

// newIndex creates a new index with current elasticsearch settings.
// The new Index will have a randomized name based on si.Index.
func (si *SearchIndex) newIndex() (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errgo.Notef(err, "cannot create index name")
	}
	index := si.Index + "-" + uuid.String()
	if err := si.PutIndex(index, esIndex); err != nil {
		return "", errgo.Notef(err, "cannot set index settings")
	}
	if err := si.PutMapping(index, "entity", esMapping); err != nil {
		return "", errgo.Notef(err, "cannot set index mapping")
	}
	return index, nil
}

// updateVersion attempts to atomically update the document specifying the version of
// the elasticsearch settings. If it succeeds then err will be nil, if the update could not be
// made atomically then err will be elasticsearch.ErrConflict, otherwise err is a non-nil
// error.
func (si *SearchIndex) updateVersion(v version, dv int64) (bool, error) {
	var err error
	if dv == 0 {
		err = si.CreateDocument(versionIndex, versionType, si.Index, v)
	} else {
		err = si.PutDocumentVersion(versionIndex, versionType, si.Index, dv, v)
	}
	if err != nil {
		if errgo.Cause(err) == elasticsearch.ErrConflict {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// syncSearch populates the SearchIndex with all the data currently stored in
// mongodb. If the SearchIndex is not configured then this method returns a nil error.
func (s *Store) syncSearch() error {
	if s.ES == nil || s.ES.Database == nil {
		return nil
	}
	var result mongodoc.Entity
	// Only get the IDs here, UpdateSearch will get the full document
	// if it is in a series that is indexed.
	iter := s.DB.Entities().Find(nil).Select(bson.M{"_id": 1, "promulgated-url": 1}).Iter()
	defer iter.Close() // Make sure we always close on error.
	for iter.Next(&result) {
		rurl := EntityResolvedURL(&result)
		if err := s.UpdateSearch(rurl); err != nil {
			return errgo.Notef(err, "cannot index %s", rurl)
		}
	}
	logger.Infof("finished sync search")
	if err := iter.Close(); err != nil {
		return err
	}
	return nil
}

// SearchParams represents the search parameters used to search the store.
type SearchParams struct {
	// The text to use in the full text search query.
	Text string
	// If autocomplete is specified, the search will return only charms and
	// bundles with a name that has text as a prefix.
	AutoComplete bool
	// Limit the search to items with attributes that match the specified filter value.
	Filters map[string][]string
	// Limit the number of returned items to the specified count.
	Limit int
	// Include the following metadata items in the search results.
	Include []string
	// Start the the returned items at a specific offset.
	Skip int
	// ACL values to search in addition to everyone. ACL values may represent user names
	// or group names.
	Groups []string
	// Admin searches will not filter on the ACL and will show results for all matching
	// charms.
	Admin bool
	// Sort the returned items.
	sort []sortParam
	// ExpandedMultiSeries returns a number of entries for
	// multi-series charms, one for each entity.
	ExpandedMultiSeries bool
}

var allowedSortFields = map[string]bool{
	"name":      true,
	"owner":     true,
	"series":    true,
	"downloads": true,
}

func (sp *SearchParams) ParseSortFields(f ...string) error {
	for _, s := range f {
		for _, s := range strings.Split(s, ",") {
			var sort sortParam
			if strings.HasPrefix(s, "-") {
				sort.Order = sortDescending
				s = s[1:]
			}
			if !allowedSortFields[s] {
				return errgo.Newf("unrecognized sort parameter %q", s)
			}
			sort.Field = s
			sp.sort = append(sp.sort, sort)
		}
	}

	return nil
}

// sortOrder defines the order in which a field should be sorted.
type sortOrder int

const (
	sortAscending sortOrder = iota
	sortDescending
)

// sortParam represents a field and direction on which results should be sorted.
type sortParam struct {
	Field string
	Order sortOrder
}

// SearchResult represents the result of performing a search. The entites
// in Results will have the following fields completed:
// 	- URL
// 	- SupportedSeries
// 	- PromulgatedURL
// 	- PromulgatedRevision
type SearchResult struct {
	SearchTime time.Duration
	Total      int
	Results    []*mongodoc.Entity
}

// ListResult represents the result of performing a list.
type ListResult struct {
	Results []*mongodoc.Entity
}

// queryFields provides a map of fields to weighting to use with the
// elasticsearch query.
func queryFields(sp SearchParams) map[string]float64 {
	var fields map[string]float64
	if sp.AutoComplete {
		fields = map[string]float64{
			"Name.ngrams": 10,
		}
	} else {
		fields = map[string]float64{
			"Name":                    10,
			"User":                    7,
			"CharmMeta.Categories":    5,
			"CharmMeta.Tags":          5,
			"BundleData.Tags":         5,
			"Series":                  5,
			"CharmProvidedInterfaces": 3,
			"CharmRequiredInterfaces": 3,
			"CharmMeta.Description":   1,
			"BundleReadMe":            1,
		}
	}
	return fields
}

// encodeFields takes a map of field name to weight and builds a slice of strings
// representing those weighted fields for a MultiMatchQuery.
func encodeFields(fields map[string]float64) []string {
	fs := make([]string, 0, len(fields))
	for k, v := range fields {
		fs = append(fs, elasticsearch.BoostField(k, v))
	}
	return fs
}

// createSearchDSL builds an elasticsearch query from the query parameters.
// http://www.elasticsearch.org/guide/en/elasticsearch/reference/current/query-dsl.html
func createSearchDSL(sp SearchParams) elasticsearch.QueryDSL {
	qdsl := elasticsearch.QueryDSL{
		From: sp.Skip,
		Size: sp.Limit,
	}

	// Full text search
	var q elasticsearch.Query
	if sp.Text == "" {
		q = elasticsearch.MatchAllQuery{}
	} else {
		q = elasticsearch.MultiMatchQuery{
			Query:  sp.Text,
			Fields: encodeFields(queryFields(sp)),
		}
	}

	// Boosting
	f := []elasticsearch.Function{
		// TODO(mhilton) review this function in future if downloads get sufficiently
		// large that the order becomes undesirable.
		elasticsearch.FieldValueFactorFunction{
			Field:    "TotalDownloads",
			Factor:   0.000001,
			Modifier: "ln2p",
		},
		elasticsearch.BoostFactorFunction{
			Filter:      promulgatedFilter("1"),
			BoostFactor: 1.25,
		},
	}
	for k, v := range seriesBoost {
		f = append(f, elasticsearch.BoostFactorFunction{
			Filter:      seriesFilter(k),
			BoostFactor: v,
		})
	}
	q = elasticsearch.FunctionScoreQuery{
		Query:     q,
		Functions: f,
	}

	// Filters
	qdsl.Query = elasticsearch.FilteredQuery{
		Query:  q,
		Filter: createFilters(sp),
	}

	// Sorting
	for _, s := range sp.sort {
		qdsl.Sort = append(qdsl.Sort, createElasticSort(s))
	}

	return qdsl
}

// createFilters converts the filters requested with the search API into
// filters in the elasticsearch query DSL.
// See https://github.com/juju/charmstore/blob/v4/docs/API.md#get-search
// for details of how filters are specified in the API. For each key in f a
// filter is created that matches any one of the set of values specified for
// that key. The created filter will only match when at least one of the
// requested values matches for all of the requested keys. Any filter names
// that are not defined in the filters map will be silently skipped
func createFilters(sp SearchParams) elasticsearch.Filter {
	af := make(elasticsearch.AndFilter, 1, len(sp.Filters)+2)
	if sp.ExpandedMultiSeries {
		af[0] = elasticsearch.TermFilter{
			Field: "SingleSeries",
			Value: "true",
		}
	} else {
		af[0] = elasticsearch.TermFilter{
			Field: "AllSeries",
			Value: "true",
		}
	}
	for k, vals := range sp.Filters {
		filter, ok := filters[k]
		if !ok {
			continue
		}
		of := make(elasticsearch.OrFilter, 0, len(vals))
		for _, v := range vals {
			of = append(of, filter(v))
		}
		af = append(af, of)
	}
	if sp.Admin {
		return af
	}
	gf := make(elasticsearch.OrFilter, 0, len(sp.Groups)+1)
	gf = append(gf, elasticsearch.TermFilter{
		Field: "ReadACLs",
		Value: params.Everyone,
	})
	for _, g := range sp.Groups {
		gf = append(gf, elasticsearch.TermFilter{
			Field: "ReadACLs",
			Value: g,
		})
	}
	af = append(af, gf)
	return af
}

// filters contains a mapping from a filter parameter in the API to a
// function that will generate an elasticsearch query DSL filter for the
// given value.
var filters = map[string]func(string) elasticsearch.Filter{
	"description": descriptionFilter,
	"name":        nameFilter,
	"owner":       ownerFilter,
	"promulgated": promulgatedFilter,
	"provides":    termFilter("CharmProvidedInterfaces"),
	"requires":    termFilter("CharmRequiredInterfaces"),
	"series":      seriesFilter,
	"summary":     summaryFilter,
	"tags":        tagsFilter,
	"type":        typeFilter,
}

// descriptionFilter generates a filter that will match against the
// description field of the charm data.
func descriptionFilter(value string) elasticsearch.Filter {
	return elasticsearch.QueryFilter{
		Query: elasticsearch.MatchQuery{
			Field: "CharmMeta.Description",
			Query: value,
			Type:  "phrase",
		},
	}
}

// nameFilter generates a filter that will match against the
// name of the charm or bundle.
func nameFilter(value string) elasticsearch.Filter {
	return elasticsearch.QueryFilter{
		Query: elasticsearch.MatchQuery{
			Field: "Name",
			Query: value,
			Type:  "phrase",
		},
	}
}

// ownerFilter generates a filter that will match against the
// owner taken from the URL.
func ownerFilter(value string) elasticsearch.Filter {
	if value == "" {
		return promulgatedFilter("1")
	}
	return elasticsearch.QueryFilter{
		Query: elasticsearch.MatchQuery{
			Field: "User",
			Query: value,
			Type:  "phrase",
		},
	}
}

// promulgatedFilter generates a filter that will match against the
// existence of a promulgated URL.
func promulgatedFilter(value string) elasticsearch.Filter {
	f := elasticsearch.ExistsFilter("PromulgatedURL")
	if value == "1" {
		return f
	}
	return elasticsearch.NotFilter{f}
}

// seriesFilter generates a filter that will match against the
// series taken from the URL.
func seriesFilter(value string) elasticsearch.Filter {
	return elasticsearch.QueryFilter{
		Query: elasticsearch.MatchQuery{
			Field: "Series",
			Query: value,
			Type:  "phrase",
		},
	}
}

// summaryFilter generates a filter that will match against the
// summary field from the charm data.
func summaryFilter(value string) elasticsearch.Filter {
	return elasticsearch.QueryFilter{
		Query: elasticsearch.MatchQuery{
			Field: "CharmMeta.Summary",
			Query: value,
			Type:  "phrase",
		},
	}
}

// tagsFilter generates a filter that will match against the "tags" field
// in the data. For charms this is the Categories field and for bundles this
// is the Tags field.
func tagsFilter(value string) elasticsearch.Filter {
	tags := strings.Split(value, " ")
	af := make(elasticsearch.AndFilter, 0, len(tags))
	for _, t := range tags {
		if t == "" {
			continue
		}
		af = append(af, elasticsearch.OrFilter{
			elasticsearch.TermFilter{
				Field: "CharmMeta.Categories",
				Value: t,
			},
			elasticsearch.TermFilter{
				Field: "CharmMeta.Tags",
				Value: t,
			},
			elasticsearch.TermFilter{
				Field: "BundleData.Tags",
				Value: t,
			},
		})
	}
	return af
}

// termFilter creates a function that generates a filter on the specified
// document field.
func termFilter(field string) func(string) elasticsearch.Filter {
	return func(value string) elasticsearch.Filter {
		terms := strings.Split(value, " ")
		af := make(elasticsearch.AndFilter, 0, len(terms))
		for _, t := range terms {
			if t == "" {
				continue
			}
			af = append(af, elasticsearch.TermFilter{
				Field: field,
				Value: t,
			})
		}
		return af
	}
}

// bundleFilter is a filter that matches against bundles, based on
// the URL.
var bundleFilter = seriesFilter("bundle")

// typeFilter generates a filter that is used to match either only charms,
// or only bundles.
func typeFilter(value string) elasticsearch.Filter {
	if value == "bundle" {
		return bundleFilter
	}
	return elasticsearch.NotFilter{bundleFilter}
}

// sortFields contains a mapping from api fieldnames to the entity fields to search.
var sortESFields = map[string]string{
	"name":      "Name",
	"owner":     "User",
	"series":    "Series",
	"downloads": "TotalDownloads",
}

// createSort creates an elasticsearch.Sort query parameter out of a Sort parameter.
func createElasticSort(s sortParam) elasticsearch.Sort {
	sort := elasticsearch.Sort{
		Field: sortESFields[s.Field],
		Order: elasticsearch.Ascending,
	}
	if s.Order == sortDescending {
		sort.Order = elasticsearch.Descending
	}
	return sort
}
