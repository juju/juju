// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var logger = loggo.GetLogger("juju.state.cloudimagemetadata")

type storage struct {
	collection string
	store      DataStore
}

var _ Storage = (*storage)(nil)

// expiryTime is the time after which non-custom image metadata
// records will be deleted from the cache.
const expiryTime = 5 * time.Minute

// MongoIndexes returns the indexes to apply to the clouldimagemetadata collection.
// We return an index that expires records containing a created-at field after 5 minutes.
func MongoIndexes() []mgo.Index {
	return []mgo.Index{{
		Key:         []string{"expire-at"},
		ExpireAfter: expiryTime,
		Sparse:      true,
	}}
}

// NewStorage constructs a new Storage that stores image metadata
// in the provided data store.
func NewStorage(collectionName string, store DataStore) Storage {
	return &storage{collectionName, store}
}

var emptyMetadata = Metadata{}

// SaveMetadata implements Storage.SaveMetadata and behaves as save-or-update.
// Non custom records will expire after a set time.
func (s *storage) SaveMetadata(metadata []Metadata) error {
	if len(metadata) == 0 {
		return nil
	}

	newDocs := make([]imagesMetadataDoc, len(metadata))
	for i, m := range metadata {
		newDoc := s.mongoDoc(m)
		if err := validateMetadata(&newDoc); err != nil {
			return err
		}
		newDocs[i] = newDoc
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		seen := set.NewStrings()
		var ops []txn.Op
		for _, newDoc := range newDocs {
			newDocCopy := newDoc
			if seen.Contains(newDocCopy.Id) {
				return nil, errors.Errorf(
					"duplicate metadata record for image id %s (key=%q)",
					newDocCopy.ImageId, newDocCopy.Id)
			}
			op := txn.Op{
				C:  s.collection,
				Id: newDocCopy.Id,
			}

			// Check if this image metadata is already known.
			existing, err := s.getMetadata(newDocCopy.Id)
			if errors.IsNotFound(err) {
				op.Assert = txn.DocMissing
				op.Insert = &newDocCopy
				ops = append(ops, op)
				logger.Debugf("inserting cloud image metadata for %v", newDocCopy.Id)
			} else if err != nil {
				return nil, errors.Trace(err)
			} else if existing.ImageId != newDocCopy.ImageId {
				// need to update imageId
				op.Assert = txn.DocExists
				op.Update = bson.D{{"$set", bson.D{{"image_id", newDocCopy.ImageId}}}}
				ops = append(ops, op)
				logger.Debugf("updating cloud image id for metadata %v", newDocCopy.Id)
			}
			seen.Add(newDocCopy.Id)
		}
		if len(ops) == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		return ops, nil
	}

	err := s.store.RunTransaction(buildTxn)
	if err != nil {
		return errors.Annotate(err, "cannot save cloud image metadata")
	}
	return nil
}

// DeleteMetadata implements Storage.DeleteMetadata.
func (s *storage) DeleteMetadata(imageId string) error {
	deleteOperation := func(docId string) txn.Op {
		logger.Debugf("deleting metadata (ID=%v) for image (ID=%v)", docId, imageId)
		return txn.Op{
			C:      s.collection,
			Id:     docId,
			Assert: txn.DocExists,
			Remove: true,
		}
	}

	noOp := func() ([]txn.Op, error) {
		logger.Debugf("no metadata for image ID %v to delete", imageId)
		return nil, jujutxn.ErrNoOperations
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// find all metadata docs with given image id
		imageMetadata, err := s.metadataForImageId(imageId)
		if err != nil {
			if err == mgo.ErrNotFound {
				return noOp()
			}
			return nil, err
		}
		if len(imageMetadata) == 0 {
			return noOp()
		}

		allTxn := make([]txn.Op, len(imageMetadata))
		for i, doc := range imageMetadata {
			allTxn[i] = deleteOperation(doc.Id)
		}
		return allTxn, nil
	}

	err := s.store.RunTransaction(buildTxn)
	if err != nil {
		return errors.Annotatef(err, "cannot delete metadata for cloud image %v", imageId)
	}
	return nil
}

func (s *storage) metadataForImageId(imageId string) ([]imagesMetadataDoc, error) {
	coll, closer := s.store.GetCollection(s.collection)
	defer closer()

	var docs []imagesMetadataDoc
	query := bson.D{{"image_id", imageId}}
	if err := coll.Find(query).All(&docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (s *storage) getMetadata(id string) (Metadata, error) {
	coll, closer := s.store.GetCollection(s.collection)
	defer closer()

	var old imagesMetadataDoc
	if err := coll.Find(bson.D{{"_id", id}}).One(&old); err != nil {
		if err == mgo.ErrNotFound {
			return Metadata{}, errors.NotFoundf("image metadata with ID %q", id)
		}
		return emptyMetadata, errors.Trace(err)
	}
	return old.metadata(), nil
}

// AllCloudImageMetadata returns all cloud image metadata in the model.
func (s *storage) AllCloudImageMetadata() ([]Metadata, error) {
	coll, closer := s.store.GetCollection(s.collection)
	defer closer()

	results := []Metadata{}
	docs := []imagesMetadataDoc{}
	err := coll.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all image metadata")
	}
	for _, doc := range docs {
		results = append(results, doc.metadata())
	}
	return results, nil
}

// imagesMetadataDoc results in immutable records. Updates are effectively
// a delate and an insert.
type imagesMetadataDoc struct {
	// Id contains unique key for cloud image metadata.
	// This is an amalgamation of all deterministic metadata attributes to ensure
	// that there can be a public and custom image for the same attributes set.
	Id string `bson:"_id"`

	// ExpireAt is optional and records a time used in conjunction with the
	// TTL index in order to expire the record.
	ExpireAt time.Time `bson:"expire-at,omitempty"`

	// ImageId is an image identifier.
	ImageId string `bson:"image_id"`

	// Stream contains reference to a particular stream,
	// for e.g. "daily" or "released"
	Stream string `bson:"stream"`

	// Region is the name of cloud region associated with the image.
	Region string `bson:"region"`

	// Version is OS version, for e.g. "12.04".
	Version string `bson:"version"`

	// Series is OS series, for e.g. "trusty".
	Series string `bson:"series"`

	// Arch is the architecture for this cloud image, for e.g. "amd64"
	Arch string `bson:"arch"`

	// VirtType contains virtualisation type of the cloud image, for e.g. "pv", "hvm". "kvm".
	VirtType string `bson:"virt_type,omitempty"`

	// RootStorageType contains type of root storage, for e.g. "ebs", "instance".
	RootStorageType string `bson:"root_storage_type,omitempty"`

	// RootStorageSize contains size of root storage in gigabytes (GB).
	RootStorageSize uint64 `bson:"root_storage_size"`

	// DateCreated is the date/time when this doc was created.
	DateCreated int64 `bson:"date_created"`

	// Source describes where this image is coming from: is it public? custom?
	Source string `bson:"source"`

	// Priority is an importance factor for image metadata.
	// Higher number means higher priority.
	// This will allow to sort metadata by importance.
	Priority int `bson:"priority"`
}

func (m imagesMetadataDoc) metadata() Metadata {
	r := Metadata{
		MetadataAttributes: MetadataAttributes{
			Source:          m.Source,
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			Series:          m.Series,
			Arch:            m.Arch,
			RootStorageType: m.RootStorageType,
			VirtType:        m.VirtType,
		},
		Priority:    m.Priority,
		ImageId:     m.ImageId,
		DateCreated: m.DateCreated,
	}
	if m.RootStorageSize != 0 {
		r.RootStorageSize = &m.RootStorageSize
	}
	return r
}

func (s *storage) mongoDoc(m Metadata) imagesMetadataDoc {
	now := time.Now()
	dateCreated := m.DateCreated
	if dateCreated == 0 {
		// TODO(fwereade): 2016-03-17 lp:1558657
		dateCreated = now.UnixNano()
	}
	r := imagesMetadataDoc{
		Id:              buildKey(m),
		Stream:          m.Stream,
		Region:          m.Region,
		Version:         m.Version,
		Series:          m.Series,
		Arch:            m.Arch,
		VirtType:        m.VirtType,
		RootStorageType: m.RootStorageType,
		ImageId:         m.ImageId,
		DateCreated:     dateCreated,
		Source:          m.Source,
		Priority:        m.Priority,
	}
	if r.Source != "custom" {
		r.ExpireAt = now
	}
	if m.RootStorageSize != nil {
		r.RootStorageSize = *m.RootStorageSize
	}
	return r
}

func buildKey(m Metadata) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s",
		m.Stream,
		m.Region,
		m.Series,
		m.Arch,
		m.VirtType,
		m.RootStorageType,
		m.Source)
}

func validateMetadata(m *imagesMetadataDoc) error {
	// series must be supplied.
	if m.Series == "" {
		return errors.NotValidf("missing series: metadata for image %v", m.ImageId)
	}
	v, err := series.SeriesVersion(m.Series)
	if err != nil {
		return err
	}
	m.Version = v

	if m.Stream == "" {
		return errors.NotValidf("missing stream: metadata for image %v", m.ImageId)
	}
	if m.Source == "" {
		return errors.NotValidf("missing source: metadata for image %v", m.ImageId)
	}
	if m.Arch == "" {
		return errors.NotValidf("missing architecture: metadata for image %v", m.ImageId)
	}
	if m.Region == "" {
		return errors.NotValidf("missing region: metadata for image %v", m.ImageId)
	}
	return nil
}

// FindMetadata implements Storage.FindMetadata.
// Results are sorted by date created and grouped by source.
func (s *storage) FindMetadata(criteria MetadataFilter) (map[string][]Metadata, error) {
	coll, closer := s.store.GetCollection(s.collection)
	defer closer()

	logger.Debugf("searching for image metadata %#v", criteria)
	searchCriteria := buildSearchClauses(criteria)
	var docs []imagesMetadataDoc
	if err := coll.Find(searchCriteria).Sort("date_created").All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, errors.NotFoundf("matching cloud image metadata")
	}

	metadata := make(map[string][]Metadata)
	for _, doc := range docs {
		one := doc.metadata()
		metadata[one.Source] = append(metadata[one.Source], one)
	}
	return metadata, nil
}

func buildSearchClauses(criteria MetadataFilter) bson.D {
	all := bson.D{}

	if criteria.Stream != "" {
		all = append(all, bson.DocElem{"stream", criteria.Stream})
	}

	if criteria.Region != "" {
		all = append(all, bson.DocElem{"region", criteria.Region})
	}

	if len(criteria.Series) != 0 {
		all = append(all, bson.DocElem{"series", bson.D{{"$in", criteria.Series}}})
	}

	if len(criteria.Arches) != 0 {
		all = append(all, bson.DocElem{"arch", bson.D{{"$in", criteria.Arches}}})
	}

	if criteria.VirtType != "" {
		all = append(all, bson.DocElem{"virt_type", criteria.VirtType})
	}

	if criteria.RootStorageType != "" {
		all = append(all, bson.DocElem{"root_storage_type", criteria.RootStorageType})
	}

	if len(all.Map()) == 0 {
		return nil
	}
	return all
}

// MetadataFilter contains all metadata attributes that alow to find a particular
// cloud image metadata. Since size and source are not discriminating attributes
// for cloud image metadata, they are not included in search criteria.
type MetadataFilter struct {
	// Region stores metadata region.
	Region string `json:"region,omitempty"`

	// Series stores all desired series.
	Series []string `json:"series,omitempty"`

	// Arches stores all desired architectures.
	Arches []string `json:"arches,omitempty"`

	// Stream can be "" or "released" for the default "released" stream,
	// or "daily" for daily images, or any other stream that the available
	// simplestreams metadata supports.
	Stream string `json:"stream,omitempty"`

	// VirtType stores virtualisation type.
	VirtType string `json:"virt_type,omitempty"`

	// RootStorageType stores storage type.
	RootStorageType string `json:"root-storage-type,omitempty"`
}

// SupportedArchitectures implements Storage.SupportedArchitectures.
func (s *storage) SupportedArchitectures(criteria MetadataFilter) ([]string, error) {
	coll, closer := s.store.GetCollection(s.collection)
	defer closer()

	var arches []string
	if err := coll.Find(buildSearchClauses(criteria)).Distinct("arch", &arches); err != nil {
		return nil, errors.Trace(err)
	}
	return arches, nil
}
