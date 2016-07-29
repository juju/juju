// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"time"

	jujuzip "github.com/juju/zip"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"

	"gopkg.in/juju/charmstore.v5-unstable/internal/blobstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
	"gopkg.in/juju/charmstore.v5-unstable/internal/series"
)

// addParams holds parameters held in common between the
// Store.addCharm and Store.addBundle methods.
type addParams struct {
	// url holds the id to be associated with the stored entity.
	// If URL.PromulgatedRevision is not -1, the entity will
	// be promulgated.
	url *router.ResolvedURL

	// blobName holds the name of the entity's archive blob.
	blobName string

	// blobHash holds the hash of the entity's archive blob.
	blobHash string

	// preV5BlobHash holds the hash of the entity's archive blob for
	// pre-v5 compatibility purposes.
	preV5BlobHash string

	// preV5BlobHash256 holds the SHA256 hash of the entity's archive blob for
	// pre-v5 compatibility purposes.
	preV5BlobHash256 string

	// preV5BlobSize holds the size of the entity's archive blob for
	// pre-v5 compatibility purposes.
	preV5BlobSize int64

	// blobHash256 holds the sha256 hash of the entity's archive blob.
	blobHash256 string

	// bobSize holds the size of the entity's archive blob.
	blobSize int64

	// chans holds the channels to associate with the entity.
	chans []params.Channel
}

// AddCharmWithArchive adds the given charm, which must
// be either a *charm.CharmDir or implement ArchiverTo,
// to the charmstore under the given URL.
//
// This method is provided for testing purposes only.
func (s *Store) AddCharmWithArchive(url *router.ResolvedURL, ch charm.Charm) error {
	return s.AddEntityWithArchive(url, ch)
}

// AddBundleWithArchive adds the given bundle, which must
// be either a *charm.BundleDir or implement ArchiverTo,
// to the charmstore under the given URL.
//
// This method is provided for testing purposes only.
func (s *Store) AddBundleWithArchive(url *router.ResolvedURL, b charm.Bundle) error {
	return s.AddEntityWithArchive(url, b)
}

// AddEntityWithArchive provides the implementation for
// both AddCharmWithArchive and AddBundleWithArchive.
// It accepts charm.Charm or charm.Bundle implementations
// defined in the charm package, and any that implement
// ArchiverTo.
func (s *Store) AddEntityWithArchive(url *router.ResolvedURL, archive interface{}) error {
	blob, err := getArchive(archive)
	if err != nil {
		return errgo.Notef(err, "cannot get archive")
	}
	defer blob.Close()
	hash := blobstore.NewHash()
	size, err := io.Copy(hash, blob)
	if err != nil {
		return errgo.Notef(err, "cannot copy archive")
	}
	if _, err := blob.Seek(0, 0); err != nil {
		return errgo.Notef(err, "cannot seek to start of archive")
	}
	if err := s.UploadEntity(url, blob, fmt.Sprintf("%x", hash.Sum(nil)), size, nil); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return nil
}

// UploadEntity reads the given blob, which should have the given hash
// and size, and uploads it to the charm store, associating it with
// the given channels (without actually making it current in any of them).
//
// The following error causes may be returned:
//	params.ErrDuplicateUpload if the URL duplicates an existing entity.
//	params.ErrEntityIdNotAllowed if the id may not be created.
//	params.ErrInvalidEntity if the provided blob is invalid.
func (s *Store) UploadEntity(url *router.ResolvedURL, blob io.Reader, blobHash string, size int64, chans []params.Channel) error {
	// Strictly speaking these tests are redundant, because a ResolvedURL should
	// always be canonical, but check just in case anyway, as this is
	// final gateway before a potentially invalid url might be stored
	// in the database.
	if url.URL.User == "" {
		return errgo.WithCausef(nil, params.ErrEntityIdNotAllowed, "entity id does not specify user")
	}
	if url.URL.Revision == -1 {
		return errgo.WithCausef(nil, params.ErrEntityIdNotAllowed, "entity id does not specify revision")
	}
	blobName, blobHash256, err := s.putArchive(blob, size, blobHash)
	if err != nil {
		return errgo.Mask(err)
	}
	r, _, err := s.BlobStore.Open(blobName)
	if err != nil {
		return errgo.Notef(err, "cannot open newly created blob")
	}
	defer r.Close()
	if err := s.addEntityFromReader(url, r, blobName, blobHash, blobHash256, size, chans); err != nil {
		if err1 := s.BlobStore.Remove(blobName); err1 != nil {
			logger.Errorf("cannot remove blob %s after error: %v", blobName, err1)
		}
		return errgo.Mask(err,
			errgo.Is(params.ErrDuplicateUpload),
			errgo.Is(params.ErrEntityIdNotAllowed),
			errgo.Is(params.ErrInvalidEntity),
		)
	}
	return nil
}

// putArchive reads the charm or bundle archive from the given reader and
// puts into the blob store. The archiveSize and hash must holds the length
// of the blob content and its SHA384 hash respectively.
func (s *Store) putArchive(blob io.Reader, blobSize int64, hash string) (blobName, blobHash256 string, err error) {
	name := bson.NewObjectId().Hex()

	// Calculate the SHA256 hash while uploading the blob in the blob store.
	hash256 := sha256.New()
	blob = io.TeeReader(blob, hash256)

	// Upload the actual blob, and make sure that it is removed
	// if we fail later.
	err = s.BlobStore.PutUnchallenged(blob, name, blobSize, hash)
	if err != nil {
		// TODO return error with ErrInvalidEntity cause when
		// there's a hash or size mismatch.
		return "", "", errgo.Notef(err, "cannot put archive blob")
	}
	return name, fmt.Sprintf("%x", hash256.Sum(nil)), nil
}

// addEntityFromReader adds the entity represented by the contents
// of the given reader, associating it with the given id.
func (s *Store) addEntityFromReader(id *router.ResolvedURL, r io.ReadSeeker, blobName, hash, hash256 string, blobSize int64, chans []params.Channel) error {
	p := addParams{
		url:              id,
		blobName:         blobName,
		blobHash:         hash,
		blobHash256:      hash256,
		blobSize:         blobSize,
		preV5BlobHash:    hash,
		preV5BlobHash256: hash256,
		preV5BlobSize:    blobSize,
		chans:            chans,
	}
	if id.URL.Series == "bundle" {
		b, err := s.newBundle(id, r, blobSize)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrInvalidEntity), errgo.Is(params.ErrDuplicateUpload), errgo.Is(params.ErrEntityIdNotAllowed))
		}
		if err := s.addBundle(b, p); err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrDuplicateUpload), errgo.Is(params.ErrEntityIdNotAllowed))
		}
		return nil
	}
	ch, err := s.newCharm(id, r, blobSize)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrInvalidEntity), errgo.Is(params.ErrDuplicateUpload), errgo.Is(params.ErrEntityIdNotAllowed))
	}
	if len(ch.Meta().Series) > 0 {
		if _, err := r.Seek(0, 0); err != nil {
			return errgo.Notef(err, "cannot seek to start of archive")
		}
		logger.Infof("adding pre-v5 compat blob for %#v", id)
		info, err := addPreV5CompatibilityHackBlob(s.BlobStore, r, p.blobName, p.blobSize)
		if err != nil {
			return errgo.Notef(err, "cannot add pre-v5 compatibility blob")
		}
		p.preV5BlobHash = info.hash
		p.preV5BlobHash256 = info.hash256
		p.preV5BlobSize = info.size
	}
	err = s.addCharm(ch, p)
	if err != nil && len(ch.Meta().Series) > 0 {
		// We added a compatibility blob so we need to remove it.
		compatBlobName := preV5CompatibilityBlobName(p.blobName)
		if err1 := s.BlobStore.Remove(compatBlobName); err1 != nil {
			logger.Errorf("cannot remove blob %s after error: %v", compatBlobName, err1)
		}
	}
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrDuplicateUpload), errgo.Is(params.ErrEntityIdNotAllowed))
	}
	return nil
}

type preV5CompatibilityHackBlobInfo struct {
	hash    string
	hash256 string
	size    int64
}

// addPreV5CompatibilityHackBlob adds a second blob to the blob store that
// contains a suffix to the zipped charm archive file that updates the zip
// index to point to an updated version of metadata.yaml that does
// not have a series field. The original blob is held in r.
// It updates the fields in p accordingly.
//
// We do this because earlier versions of the charm package have a version
// of the series field that holds a single string rather than a slice of string
// so will fail when reading the new slice-of-string form, and we
// don't want to change the field name from "series".
func addPreV5CompatibilityHackBlob(blobStore *blobstore.Store, r io.ReadSeeker, blobName string, blobSize int64) (*preV5CompatibilityHackBlobInfo, error) {
	readerAt := ReaderAtSeeker(r)
	z, err := jujuzip.NewReader(readerAt, blobSize)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open charm archive")
	}
	var metadataf *jujuzip.File
	for _, f := range z.File {
		if f.Name == "metadata.yaml" {
			metadataf = f
			break
		}
	}
	if metadataf == nil {
		return nil, errgo.New("no metadata.yaml file found")
	}
	fr, err := metadataf.Open()
	if err != nil {
		return nil, errgo.Notef(err, "cannot open metadata.yaml from archive")
	}
	defer fr.Close()
	data, err := removeSeriesField(fr)
	if err != nil {
		return nil, errgo.Notef(err, "cannot remove series field from metadata")
	}
	var appendedBlob bytes.Buffer
	zw := z.Append(&appendedBlob)
	updatedf := metadataf.FileHeader // Work around invalid duplicate FileHeader issue.
	zwf, err := zw.CreateHeader(&updatedf)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create appended metadata entry")
	}
	if _, err := zwf.Write(data); err != nil {
		return nil, errgo.Notef(err, "cannot write appended metadata data")
	}
	if err := zw.Close(); err != nil {
		return nil, errgo.Notef(err, "cannot close zip file")
	}
	data = appendedBlob.Bytes()
	sha384sum := sha512.Sum384(data)

	err = blobStore.PutUnchallenged(&appendedBlob, preV5CompatibilityBlobName(blobName), int64(len(data)), fmt.Sprintf("%x", sha384sum[:]))
	if err != nil {
		return nil, errgo.Notef(err, "cannot put archive blob")
	}

	sha384w := sha512.New384()
	sha256w := sha256.New()
	hashw := io.MultiWriter(sha384w, sha256w)
	if _, err := r.Seek(0, 0); err != nil {
		return nil, errgo.Notef(err, "cannnot seek to start of blob")
	}
	if _, err := io.Copy(hashw, r); err != nil {
		return nil, errgo.Notef(err, "cannot recalculate blob checksum")
	}
	hashw.Write(data)
	return &preV5CompatibilityHackBlobInfo{
		size:    blobSize + int64(len(data)),
		hash256: fmt.Sprintf("%x", sha256w.Sum(nil)),
		hash:    fmt.Sprintf("%x", sha384w.Sum(nil)),
	}, nil
}

// preV5CompatibilityBlobName returns the name of the zip file suffix used
// to overwrite the metadata.yaml file for pre-v5 compatibility purposes.
func preV5CompatibilityBlobName(blobName string) string {
	return blobName + ".pre-v5-suffix"
}

func removeSeriesField(r io.Reader) ([]byte, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	var meta map[string]interface{}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal metadata.yaml")
	}
	delete(meta, "series")
	data, err = yaml.Marshal(meta)
	if err != nil {
		return nil, errgo.Notef(err, "cannot re-marshal metadata.yaml")
	}
	return data, nil
}

// newCharm returns a new charm implementation from the archive blob
// read from r, that should have the given size and will
// be named with the given id.
//
// The charm is checked for validity before returning.
func (s *Store) newCharm(id *router.ResolvedURL, r io.ReadSeeker, blobSize int64) (charm.Charm, error) {
	readerAt := ReaderAtSeeker(r)
	ch, err := charm.ReadCharmArchiveFromReader(readerAt, blobSize)
	if err != nil {
		return nil, zipReadError(err, "cannot read charm archive")
	}
	if err := checkCharmIsValid(ch); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrInvalidEntity))
	}
	if err := checkIdAllowed(id, ch); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrEntityIdNotAllowed))
	}
	return ch, nil
}

func checkCharmIsValid(ch charm.Charm) error {
	m := ch.Meta()
	for _, rels := range []map[string]charm.Relation{m.Provides, m.Requires, m.Peers} {
		if err := checkRelationsAreValid(rels); err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrInvalidEntity))
		}
	}
	if err := checkConsistentSeries(m.Series); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrInvalidEntity))
	}
	return nil
}

func checkRelationsAreValid(rels map[string]charm.Relation) error {
	for _, rel := range rels {
		if rel.Name == "relation-name" {
			return errgo.WithCausef(nil, params.ErrInvalidEntity, "relation %s has almost certainly not been changed from the template", rel.Name)
		}
		if rel.Interface == "interface-name" {
			return errgo.WithCausef(nil, params.ErrInvalidEntity, "interface %s in relation %s has almost certainly not been changed from the template", rel.Interface, rel.Name)
		}
	}
	return nil
}

// checkConsistentSeries ensures that all of the series listed in the
// charm metadata come from the same distribution. If an error is
// returned it will have a cause of params.ErrInvalidEntity.
func checkConsistentSeries(metadataSeries []string) error {
	var dist series.Distribution
	for _, s := range metadataSeries {
		d := series.Series[s].Distribution
		if d == "" {
			return errgo.WithCausef(nil, params.ErrInvalidEntity, "unrecognized series %q in metadata", s)
		}
		if dist == "" {
			dist = d
		} else if dist != d {
			return errgo.WithCausef(nil, params.ErrInvalidEntity, "cannot mix series from %s and %s in single charm", dist, d)
		}
	}
	return nil
}

// checkIdAllowed ensures that the given id may be used for the provided
// charm. If an error is returned it will have a cause of
// params.ErrEntityIdNotAllowed.
func checkIdAllowed(id *router.ResolvedURL, ch charm.Charm) error {
	m := ch.Meta()
	if id.URL.Series == "" && len(m.Series) == 0 {
		return errgo.WithCausef(nil, params.ErrEntityIdNotAllowed, "series not specified in url or charm metadata")
	} else if id.URL.Series == "" || len(m.Series) == 0 {
		return nil
	}
	// if we get here we have series in both the id and metadata, ensure they agree.
	for _, s := range m.Series {
		if s == id.URL.Series {
			return nil
		}
	}
	return errgo.WithCausef(nil, params.ErrEntityIdNotAllowed, "%q series not listed in charm metadata", id.URL.Series)
}

// addCharm adds a charm to the entities collection with the given parameters.
// If p.URL cannot be used as a name for the charm then the returned
// error will have the cause params.ErrEntityIdNotAllowed. If the charm
// duplicates an existing charm then the returned error will have the
// cause params.ErrDuplicateUpload.
func (s *Store) addCharm(c charm.Charm, p addParams) (err error) {
	// Strictly speaking this test is redundant, because a ResolvedURL should
	// always be canonical, but check just in case anyway, as this is
	// final gateway before a potentially invalid url might be stored
	// in the database.
	id := p.url.URL
	logger.Infof("add charm url %s; promulgated rev %d", &id, p.url.PromulgatedRevision)
	entity := &mongodoc.Entity{
		URL:                     &id,
		PromulgatedURL:          p.url.PromulgatedURL(),
		BlobHash:                p.blobHash,
		BlobHash256:             p.blobHash256,
		BlobName:                p.blobName,
		PreV5BlobSize:           p.preV5BlobSize,
		PreV5BlobHash:           p.preV5BlobHash,
		PreV5BlobHash256:        p.preV5BlobHash256,
		Size:                    p.blobSize,
		UploadTime:              time.Now(),
		CharmMeta:               c.Meta(),
		CharmConfig:             c.Config(),
		CharmActions:            c.Actions(),
		CharmProvidedInterfaces: interfacesForRelations(c.Meta().Provides),
		CharmRequiredInterfaces: interfacesForRelations(c.Meta().Requires),
		SupportedSeries:         c.Meta().Series,
	}
	denormalizeEntity(entity)
	setEntityChannels(entity, p.chans)

	// Check that we're not going to create a charm that duplicates
	// the name of a bundle. This is racy, but it's the best we can
	// do. Also check that there isn't an existing multi-series charm
	// that would be replaced by this one.
	entities, err := s.FindEntities(entity.BaseURL, nil)
	if err != nil {
		return errgo.Notef(err, "cannot check for existing entities")
	}
	for _, entity := range entities {
		if entity.URL.Series == "bundle" {
			return errgo.WithCausef(err, params.ErrEntityIdNotAllowed, "charm name duplicates bundle name %v", entity.URL)
		}
		if id.Series != "" && entity.URL.Series == "" {
			return errgo.WithCausef(err, params.ErrEntityIdNotAllowed, "charm name duplicates multi-series charm name %v", entity.URL)
		}
	}
	if err := s.addEntity(entity); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrDuplicateUpload))
	}
	return nil
}

// setEntityChannels associates the entity with the given channels, ignoring
// unknown channels.
func setEntityChannels(entity *mongodoc.Entity, chans []params.Channel) {
	for _, c := range chans {
		switch c {
		case params.DevelopmentChannel:
			entity.Development = true
		case params.StableChannel:
			entity.Stable = true
		}
	}
}

// addBundle adds a bundle to the entities collection with the given
// parameters. If p.URL cannot be used as a name for the bundle then the
// returned error will have the cause params.ErrEntityIdNotAllowed. If
// the bundle duplicates an existing bundle then the returned error will
// have the cause params.ErrDuplicateUpload.
func (s *Store) addBundle(b charm.Bundle, p addParams) error {
	bundleData := b.Data()
	urls, err := bundleCharms(bundleData)
	if err != nil {
		return errgo.Mask(err)
	}
	entity := &mongodoc.Entity{
		URL:                &p.url.URL,
		BlobHash:           p.blobHash,
		BlobHash256:        p.blobHash256,
		BlobName:           p.blobName,
		PreV5BlobSize:      p.preV5BlobSize,
		PreV5BlobHash:      p.preV5BlobHash,
		PreV5BlobHash256:   p.preV5BlobHash256,
		Size:               p.blobSize,
		UploadTime:         time.Now(),
		BundleData:         bundleData,
		BundleUnitCount:    newInt(bundleUnitCount(bundleData)),
		BundleMachineCount: newInt(bundleMachineCount(bundleData)),
		BundleReadMe:       b.ReadMe(),
		BundleCharms:       urls,
		PromulgatedURL:     p.url.PromulgatedURL(),
	}
	denormalizeEntity(entity)
	setEntityChannels(entity, p.chans)

	// Check that we're not going to create a bundle that duplicates
	// the name of a charm. This is racy, but it's the best we can do.
	entities, err := s.FindEntities(entity.BaseURL, nil)
	if err != nil {
		return errgo.Notef(err, "cannot check for existing entities")
	}
	for _, entity := range entities {
		if entity.URL.Series != "bundle" {
			return errgo.WithCausef(err, params.ErrEntityIdNotAllowed, "bundle name duplicates charm name %s", entity.URL)
		}
	}
	if err := s.addEntity(entity); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrDuplicateUpload))
	}
	return nil
}

// addEntity actually adds the entity (and its base entity if required) to
// the database. It assumes that the blob associated with the
// entity has already been validated and stored.
func (s *Store) addEntity(entity *mongodoc.Entity) (err error) {
	// Add the base entity to the database.
	perms := []string{entity.User}
	acls := mongodoc.ACL{
		Read:  perms,
		Write: perms,
	}
	baseEntity := &mongodoc.BaseEntity{
		URL:  entity.BaseURL,
		User: entity.User,
		Name: entity.Name,
		ChannelACLs: map[params.Channel]mongodoc.ACL{
			params.UnpublishedChannel: acls,
			params.DevelopmentChannel: acls,
			params.StableChannel:      acls,
		},
		Promulgated: entity.PromulgatedURL != nil,
	}
	err = s.DB.BaseEntities().Insert(baseEntity)
	if err != nil && !mgo.IsDup(err) {
		return errgo.Notef(err, "cannot insert base entity")
	}

	// Add the entity to the database.
	err = s.DB.Entities().Insert(entity)
	if mgo.IsDup(err) {
		return params.ErrDuplicateUpload
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert entity")
	}
	return nil
}

// denormalizeEntity sets all denormalized fields in e
// from their associated canonical fields.
//
// It is the responsibility of the caller to set e.SupportedSeries
// if the entity URL does not contain a series. If the entity
// URL *does* contain a series, e.SupportedSeries will
// be overwritten.
func denormalizeEntity(e *mongodoc.Entity) {
	e.BaseURL = mongodoc.BaseURL(e.URL)
	e.Name = e.URL.Name
	e.User = e.URL.User
	e.Revision = e.URL.Revision
	e.Series = e.URL.Series
	if e.URL.Series != "" {
		if e.URL.Series == "bundle" {
			e.SupportedSeries = nil
		} else {
			e.SupportedSeries = []string{e.URL.Series}
		}
	}
	if e.PromulgatedURL == nil {
		e.PromulgatedRevision = -1
	} else {
		e.PromulgatedRevision = e.PromulgatedURL.Revision
	}
}

// newBundle returns a new bundle implementation from the archive blob
// read from r, that should have the given size and will
// be named with the given id.
//
// The bundle is checked for validity before returning.
func (s *Store) newBundle(id *router.ResolvedURL, r io.ReadSeeker, blobSize int64) (charm.Bundle, error) {
	readerAt := ReaderAtSeeker(r)
	b, err := charm.ReadBundleArchiveFromReader(readerAt, blobSize)
	if err != nil {
		return nil, zipReadError(err, "cannot read bundle archive")
	}
	bundleData := b.Data()
	charms, err := s.bundleCharms(bundleData.RequiredCharms())
	if err != nil {
		return nil, errgo.Notef(err, "cannot retrieve bundle charms")
	}
	if err := bundleData.VerifyWithCharms(verifyConstraints, verifyStorage, charms); err != nil {
		// TODO frankban: use multiError (defined in internal/router).
		return nil, errgo.NoteMask(verificationError(err), "bundle verification failed", errgo.Is(params.ErrInvalidEntity))
	}
	return b, nil
}

func (s *Store) bundleCharms(ids []string) (map[string]charm.Charm, error) {
	numIds := len(ids)
	urls := make([]*charm.URL, 0, numIds)
	idKeys := make([]string, 0, numIds)
	// TODO resolve ids concurrently.
	for _, id := range ids {
		url, err := charm.ParseURL(id)
		if err != nil {
			// Ignore this error. This will be caught in the bundle
			// verification process (see bundleData.VerifyWithCharms) and will
			// be returned to the user along with other bundle errors.
			continue
		}
		e, err := s.FindBestEntity(url, params.NoChannel, map[string]int{})
		if err != nil {
			if errgo.Cause(err) == params.ErrNotFound {
				// Ignore this error too, for the same reasons
				// described above.
				continue
			}
			return nil, err
		}
		urls = append(urls, e.URL)
		idKeys = append(idKeys, id)
	}
	var entities []mongodoc.Entity
	if err := s.DB.Entities().
		Find(bson.D{{"_id", bson.D{{"$in", urls}}}}).
		All(&entities); err != nil {
		return nil, err
	}

	entityCharms := make(map[charm.URL]charm.Charm, len(entities))
	for i, entity := range entities {
		entityCharms[*entity.URL] = &entityCharm{entities[i]}
	}
	charms := make(map[string]charm.Charm, len(urls))
	for i, url := range urls {
		if ch, ok := entityCharms[*url]; ok {
			charms[idKeys[i]] = ch
		}
	}
	return charms, nil
}

// bundleCharms returns all the charm URLs used by a bundle,
// without duplicates.
// TODO this seems to overlap slightly with Store.bundleCharms.
func bundleCharms(data *charm.BundleData) ([]*charm.URL, error) {
	// Use a map to de-duplicate the URL list: a bundle can include services
	// deployed by the same charm.
	urlMap := make(map[string]*charm.URL)
	for _, application := range data.Applications {
		url, err := charm.ParseURL(application.Charm)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		urlMap[url.String()] = url
		// Also add the corresponding base URL.
		base := mongodoc.BaseURL(url)
		urlMap[base.String()] = base
	}
	urls := make([]*charm.URL, 0, len(urlMap))
	for _, url := range urlMap {
		urls = append(urls, url)
	}
	return urls, nil
}

func newInt(x int) *int {
	return &x
}

// bundleUnitCount returns the number of units created by the bundle.
func bundleUnitCount(b *charm.BundleData) int {
	count := 0
	for _, application := range b.Applications {
		count += application.NumUnits
	}
	return count
}

// bundleMachineCount returns the number of machines
// that will be created or used by the bundle.
func bundleMachineCount(b *charm.BundleData) int {
	count := len(b.Machines)
	for _, applications := range b.Applications {
		// The default placement is "new".
		placement := &charm.UnitPlacement{
			Machine: "new",
		}
		// Check for "new" placements, which means a new machine
		// must be added.
		for _, location := range applications.To {
			var err error
			placement, err = charm.ParsePlacement(location)
			if err != nil {
				// Ignore invalid placements - a bundle should always
				// be verified before adding to the charm store so this
				// should never happen in practice.
				continue
			}
			if placement.Machine == "new" {
				count++
			}
		}
		// If there are less elements in To than NumUnits, the last placement
		// element is replicated. For this reason, if the last element is
		// "new", we need to add more machines.
		if placement != nil && placement.Machine == "new" {
			count += applications.NumUnits - len(applications.To)
		}
	}
	return count
}

func interfacesForRelations(rels map[string]charm.Relation) []string {
	// Eliminate duplicates by storing interface names into a map.
	interfaces := make(map[string]bool)
	for _, rel := range rels {
		interfaces[rel.Interface] = true
	}
	result := make([]string, 0, len(interfaces))
	for iface := range interfaces {
		result = append(result, iface)
	}
	return result
}

// zipReadError creates an appropriate error for errors in reading an
// uploaded archive. If the archive could not be read because the data
// uploaded is invalid then an error with a cause of
// params.ErrInvalidEntity will be returned. The given message will be
// added as context.
func zipReadError(err error, msg string) error {
	switch errgo.Cause(err) {
	case zip.ErrFormat, zip.ErrAlgorithm, zip.ErrChecksum:
		return errgo.WithCausef(err, params.ErrInvalidEntity, msg)
	}
	return errgo.Notef(err, msg)
}

func verifyConstraints(s string) error {
	// TODO(rog) provide some actual constraints checking here.
	return nil
}

func verifyStorage(s string) error {
	// TODO(frankban) provide some actual storage checking here.
	return nil
}

// verificationError returns an error whose string representation is a list of
// all the verification error messages stored in err, in JSON format.
// Note that err must be a *charm.VerificationError.
func verificationError(err error) error {
	verr, ok := err.(*charm.VerificationError)
	if !ok {
		return err
	}
	messages := make([]string, len(verr.Errors))
	for i, err := range verr.Errors {
		messages[i] = err.Error()
	}
	sort.Strings(messages)
	encodedMessages, err := json.Marshal(messages)
	if err != nil {
		// This should never happen.
		return err
	}
	return errgo.WithCausef(nil, params.ErrInvalidEntity, string(encodedMessages))
}

// entityCharm implements charm.Charm.
type entityCharm struct {
	mongodoc.Entity
}

func (e *entityCharm) Meta() *charm.Meta {
	return e.CharmMeta
}

func (e *entityCharm) Metrics() *charm.Metrics {
	return nil
}

func (e *entityCharm) Config() *charm.Config {
	return e.CharmConfig
}

func (e *entityCharm) Actions() *charm.Actions {
	return e.CharmActions
}

func (e *entityCharm) Revision() int {
	return e.URL.Revision
}
