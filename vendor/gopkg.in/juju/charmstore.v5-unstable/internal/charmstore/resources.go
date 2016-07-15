// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
import (
	"fmt"
	"io"
	"sort"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

// newResourceQuery returns a mongo query doc that will retrieve the
// given named resource and revision associated with the given charm URL.
// If revision is < 0, all revisions of the resource will be selected by
// the query.
func newResourceQuery(url *charm.URL, name string, revision int) bson.D {
	query := make(bson.D, 2, 3)
	query[0] = bson.DocElem{"baseurl", mongodoc.BaseURL(url)}
	query[1] = bson.DocElem{"name", name}
	if revision >= 0 {
		query = append(query, bson.DocElem{"revision", revision})
	}
	return query
}

// sortResources sorts the provided resource docs, The resources are
// sorted first by URL then by name and finally by revision.
func sortResources(resources []*mongodoc.Resource) {
	sort.Sort(resourcesByName(resources))
}

type resourcesByName []*mongodoc.Resource

func (sorted resourcesByName) Len() int      { return len(sorted) }
func (sorted resourcesByName) Swap(i, j int) { sorted[i], sorted[j] = sorted[j], sorted[i] }
func (sorted resourcesByName) Less(i, j int) bool {
	r0, r1 := sorted[i], sorted[j]
	if *r0.BaseURL != *r1.BaseURL {
		return r0.BaseURL.String() < r1.BaseURL.String()
	}
	if r0.Name != r1.Name {
		return r0.Name < r1.Name
	}
	return r0.Revision < r1.Revision
}

// ListResources returns the set of resources for the entity with the
// given id. If the unpublished channel is specified then set is
// composed of the latest revision for each resource. Otherwise it holds
// the revisions declared when the charm/channel pair was published.
func (s *Store) ListResources(id *router.ResolvedURL, channel params.Channel) ([]*mongodoc.Resource, error) {
	if channel == params.NoChannel {
		return nil, errgo.Newf("no channel specified")
	}
	if id.URL.Series == "bundle" {
		return nil, nil
	}
	entity, err := s.FindEntity(id, FieldSelector("charmmeta", "baseurl"))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if entity.CharmMeta == nil {
		return nil, errgo.Newf("entity missing charm metadata")
	}
	baseEntity, err := s.FindBaseEntity(entity.URL, FieldSelector("channelresources"))
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// get all of the resources associated with the charm first.
	resources, revisions, err := s.charmResources(entity.BaseURL)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if channel != params.UnpublishedChannel {
		revisions = mapRevisions(baseEntity.ChannelResources[channel])
	}
	var docs []*mongodoc.Resource
	for name := range entity.CharmMeta.Resources {
		revision, ok := revisions[name]
		var doc *mongodoc.Resource
		if !ok {
			// Create a placeholder for the missing resource.
			doc = &mongodoc.Resource{
				BaseURL:  baseEntity.URL,
				Name:     name,
				Revision: -1,
			}
		} else if doc = resources[name][revision]; doc == nil {
			return nil, errgo.Newf("published resource %q not found", fmt.Sprintf("%s/%d", name, revision))
		}
		docs = append(docs, doc)
	}
	sortResources(docs)
	return docs, nil
}

// charmResources returns all of the currently stored resources for a charm.
func (s *Store) charmResources(baseURL *charm.URL) (map[string]map[int]*mongodoc.Resource, map[string]int, error) {
	resources := make(map[string]map[int]*mongodoc.Resource)
	latest := make(map[string]int)
	iter := s.DB.Resources().Find(bson.D{{"baseurl", baseURL}}).Iter()
	var r mongodoc.Resource
	for iter.Next(&r) {
		resource := r
		if _, ok := resources[r.Name]; !ok {
			resources[r.Name] = make(map[int]*mongodoc.Resource)
		}
		resources[r.Name][r.Revision] = &resource
		if r.Revision >= latest[r.Name] {
			latest[r.Name] = r.Revision
		}
	}
	if err := iter.Close(); err != nil {
		return nil, nil, errgo.Mask(err)
	}
	return resources, latest, nil
}

// mapRevisions converts a list of ResourceRevisions into a map of
// resource name and revision.
func mapRevisions(resourceRevisions []mongodoc.ResourceRevision) map[string]int {
	revisions := make(map[string]int)
	for _, rr := range resourceRevisions {
		revisions[rr.Name] = rr.Revision
	}
	return revisions
}

// UploadResource add blob to the blob store and adds a new resource with
// the given name to the entity with the given id. The revision of the new resource
// will be calculated to be one higher than any existing resources.
//
// TODO consider restricting uploads so that if the hash matches the
// latest revision then a new revision isn't created. This would match
// the behaviour for charms and bundles.
func (s *Store) UploadResource(id *router.ResolvedURL, name string, blob io.Reader, blobHash string, size int64) (*mongodoc.Resource, error) {
	entity, err := s.FindEntity(id, FieldSelector("charmmeta", "baseurl"))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if !charmHasResource(entity.CharmMeta, name) {
		return nil, errgo.Newf("charm does not have resource %q", name)
	}
	blobName, _, err := s.putArchive(blob, size, blobHash)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	res, err := s.addResource(&mongodoc.Resource{
		BaseURL:    entity.BaseURL,
		Name:       name,
		Revision:   -1,
		BlobHash:   blobHash,
		Size:       size,
		BlobName:   blobName,
		UploadTime: time.Now().UTC(),
	})
	if err != nil {
		if err := s.BlobStore.Remove(blobName); err != nil {
			logger.Errorf("cannot remove blob %s after error: %v", blobName, err)
		}
		return nil, errgo.Mask(err)
	}
	return res, nil
}

// addResource adds r to the resources collection. If r does not specify
// a revision number will be one higher than any existing revisions. The
// inserted resource is returned on success.
func (s *Store) addResource(r *mongodoc.Resource) (*mongodoc.Resource, error) {
	if r.Revision < 0 {
		resource := *r
		var err error
		resource.Revision, err = s.nextResourceRevision(r.BaseURL, r.Name)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		r = &resource
	}
	if err := r.Validate(); err != nil {
		return nil, errgo.Mask(err)
	}
	if err := s.DB.Resources().Insert(r); err != nil {
		if mgo.IsDup(err) {
			return nil, errgo.WithCausef(nil, params.ErrDuplicateUpload, "")
		}
		return nil, errgo.Notef(err, "cannot insert resource")
	}
	return r, nil
}

// nextRevisionNumber calculates the next revision number to use for a
// resource.
func (s *Store) nextResourceRevision(baseURL *charm.URL, name string) (int, error) {
	var r mongodoc.Resource
	if err := s.DB.Resources().Find(newResourceQuery(baseURL, name, -1)).Sort("-revision").One(&r); err != nil {
		if err == mgo.ErrNotFound {
			return 0, nil
		}
		return -1, err
	}
	return r.Revision + 1, nil
}

// ResolveResource finds the resource specified. If a matching resource
// cannot be found an error with the cause params.ErrNotFound will be
// returned.
// If revision is negative, the most recently published revision
// for the given channel will be returned.
func (s *Store) ResolveResource(url *router.ResolvedURL, name string, revision int, channel params.Channel) (*mongodoc.Resource, error) {
	if channel == params.NoChannel {
		channel = params.StableChannel
	}
	if revision < 0 && channel != params.UnpublishedChannel {
		baseEntity, err := s.FindBaseEntity(&url.URL, FieldSelector("channelresources"))
		if err != nil {
			return nil, errgo.Mask(err)
		}
		var ok bool
		revision, ok = mapRevisions(baseEntity.ChannelResources[channel])[name]
		if !ok {
			return nil, errgo.WithCausef(nil, params.ErrNotFound, "%s has no %q resource on %s channel", url, name, channel)
		}
	}
	q := newResourceQuery(mongodoc.BaseURL(&url.URL), name, revision)
	var r mongodoc.Resource
	if err := s.DB.Resources().Find(q).Sort("-revision").One(&r); err != nil {
		if err == mgo.ErrNotFound {
			suffix := ""
			if revision >= 0 {
				suffix = fmt.Sprintf("/%d", revision)
			}
			return nil, errgo.WithCausef(nil, params.ErrNotFound, "%s has no %q resource", url, name+suffix)
		}
		return nil, errgo.Mask(err)
	}
	return &r, nil
}

func charmHasResource(meta *charm.Meta, name string) bool {
	if meta == nil {
		return false
	}
	_, ok := meta.Resources[name]
	return ok
}

// OpenResourceBlob returns the blob associated with the given resource.
func (s *Store) OpenResourceBlob(res *mongodoc.Resource) (*Blob, error) {
	r, size, err := s.BlobStore.Open(res.BlobName)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open archive data for %s resource %q", res.BaseURL, fmt.Sprintf("%s/%d", res.Name, res.Revision))
	}
	return &Blob{
		ReadSeekCloser: r,
		Size:           size,
		Hash:           res.BlobHash,
	}, nil
}
