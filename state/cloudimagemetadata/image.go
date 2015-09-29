// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var logger = loggo.GetLogger("juju.state.cloudimagemetadata")

type storage struct {
	envuuid    string
	collection string
	store      DataStore
}

var _ Storage = (*storage)(nil)

// NewStorage constructs a new Storage that stores image metadata
// in the provided data store.
func NewStorage(envuuid, collectionName string, store DataStore) Storage {
	return &storage{envuuid, collectionName, store}
}

var emptyMetadata = Metadata{}

// SaveMetadata implements Storage.SaveMetadata and behaves as save-or-update.
func (s *storage) SaveMetadata(metadata Metadata) error {
	newDoc := s.mongoDoc(metadata)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:  s.collection,
			Id: newDoc.Id,
		}

		// Check if this image metadata is already known.
		existing, err := s.getMetadata(newDoc.Id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if existing.MetadataAttributes == metadata.MetadataAttributes {
			// may need to updated imageId
			if existing.ImageId != metadata.ImageId {
				op.Assert = txn.DocExists
				op.Update = bson.D{{"$set", bson.D{{"image_id", metadata.ImageId}}}}
				logger.Debugf("updating cloud image id for metadata %v", newDoc.Id)
			} else {
				return nil, jujutxn.ErrNoOperations
			}
		} else {
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
			logger.Debugf("inserting cloud image metadata for %v", newDoc.Id)
		}
		return []txn.Op{op}, nil
	}

	err := s.store.RunTransaction(buildTxn)
	if err != nil {
		return errors.Annotatef(err, "cannot save metadata for cloud image %v", newDoc.ImageId)
	}
	return nil
}

func (s *storage) getMetadata(id string) (Metadata, error) {
	coll, closer := s.store.GetCollection(s.collection)
	defer closer()

	var old imagesMetadataDoc
	err := coll.Find(bson.D{{"_id", id}}).One(&old)
	if err != nil {
		if err == mgo.ErrNotFound {
			return emptyMetadata, nil
		}
		return emptyMetadata, errors.Trace(err)
	}
	return old.metadata(), nil
}

// imagesMetadataDoc results in immutable records. Updates are effectively
// a delate and an insert.
type imagesMetadataDoc struct {
	// EnvUUID is the environment identifier.
	EnvUUID string `bson:"env-uuid"`

	// Id contains unique key for cloud image metadata.
	// This is an amalgamation of all deterministic metadata attributes to ensure
	// that there can be a public and custom image for the same attributes set.
	Id string `bson:"_id"`

	// ImageId is an image identifier.
	ImageId string `bson:"image_id"`

	// Stream contains reference to a particular stream,
	// for e.g. "daily" or "released"
	Stream string `bson:"stream"`

	// Region is the name of cloud region associated with the image.
	Region string `bson:"region"`

	// Series is Os version, for e.g. "quantal".
	Series string `bson:"series"`

	// Arch is the architecture for this cloud image, for e.g. "amd64"
	Arch string `bson:"arch"`

	// VirtualType contains the type of the cloud image, for e.g. "pv", "hvm". "kvm".
	VirtualType string `bson:"virtual_type,omitempty"`

	// RootStorageType contains type of root storage, for e.g. "ebs", "instance".
	RootStorageType string `bson:"root_storage_type,omitempty"`

	// RootStorageSize contains size of root storage in gigabytes (GB).
	RootStorageSize uint64 `bson:"root_storage_size"`

	// DateCreated is the date/time when this doc was created.
	DateCreated int64 `bson:"date_created"`

	// Source describes where this image is coming from: is it public? custom?
	Source SourceType `bson:"source"`
}

// SourceType values define source type.
type SourceType string

const (
	// Public type identifies image as public.
	Public SourceType = "public"

	// Custom type identifies image as custom.
	Custom SourceType = "custom"
)

func (m imagesMetadataDoc) metadata() Metadata {
	r := Metadata{
		MetadataAttributes{
			Source:          m.Source,
			Stream:          m.Stream,
			Region:          m.Region,
			Series:          m.Series,
			Arch:            m.Arch,
			RootStorageType: m.RootStorageType,
			VirtualType:     m.VirtualType,
		},
		m.ImageId,
	}
	if m.RootStorageSize != 0 {
		r.RootStorageSize = &m.RootStorageSize
	}
	return r
}

func (s *storage) mongoDoc(m Metadata) imagesMetadataDoc {
	r := imagesMetadataDoc{
		EnvUUID:         s.envuuid,
		Id:              buildKey(m),
		Stream:          m.Stream,
		Region:          m.Region,
		Series:          m.Series,
		Arch:            m.Arch,
		VirtualType:     m.VirtualType,
		RootStorageType: m.RootStorageType,
		ImageId:         m.ImageId,
		DateCreated:     time.Now().UnixNano(),
		Source:          m.Source,
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
		m.VirtualType,
		m.RootStorageType,
		m.Source)
}

// FindMetadata implements Storage.FindMetadata.
// Results are sorted by date created and grouped by source.
func (s *storage) FindMetadata(criteria MetadataFilter) (map[SourceType][]Metadata, error) {
	coll, closer := s.store.GetCollection(s.collection)
	defer closer()

	searchCriteria := buildSearchClauses(criteria)
	var docs []imagesMetadataDoc
	if err := coll.Find(searchCriteria).Sort("date_created").All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, errors.NotFoundf("matching cloud image metadata")
	}

	metadata := make(map[SourceType][]Metadata)
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

	if criteria.VirtualType != "" {
		all = append(all, bson.DocElem{"virtual_type", criteria.VirtualType})
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

	// VirtualType stores virtual type.
	VirtualType string `json:"virtual_type,omitempty"`

	// RootStorageType stores storage type.
	RootStorageType string `json:"root-storage-type,omitempty"`
}
