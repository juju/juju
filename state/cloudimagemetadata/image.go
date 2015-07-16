// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
)

var logger = loggo.GetLogger("juju.state.cloudimagemetadata")

type storage struct {
	envuuid        string
	collection     string
	runTransaction func(jujutxn.TransactionSource) error
	getCollection  func(string) (_ mongo.Collection, closer func())
}

var _ Storage = (*storage)(nil)

// NewStorage constructs a new Storage that stores image metadata
// in the provided collection using the provided transaction runner.
func NewStorage(
	envuuid string,
	collectionName string,
	runTransaction func(jujutxn.TransactionSource) error,
	getCollection func(string) (_ mongo.Collection, closer func()),
) Storage {
	return &storage{envuuid, collectionName, runTransaction, getCollection}
}

var emptyMetadata = Metadata{}

// SaveMetadata implements Storage.SaveMetadata and behaves as save-or-update:
// if desired metadata does not exist - we insert it; if it exists - we update.
// It throws an error if metadata did not change.
func (s *storage) SaveMetadata(metadata Metadata) error {
	newDoc := s.mongoDoc(metadata)
	all, closeAll := s.getCollection(s.collection)
	defer closeAll()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		existing, err := s.getMetadata(newDoc.Id)
		if err != nil {
			return nil, errors.Trace(err)
		}

		op := txn.Op{
			C:  all.Name(),
			Id: newDoc.Id,
		}
		if existing == emptyMetadata {
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
			logger.Debugf("inserting cloud image metadata for %v", newDoc.Id)
		} else {
			same := isSameMetadata(existing, metadata)
			if same {
				logger.Debugf("cloud image metadata for %v has not changed", newDoc.Id)
				return []txn.Op{}, errors.Annotate(jujutxn.ErrNoOperations, "no changes were made")
			}
			op.Assert = txn.DocExists
			op.Update = bson.D{{"$set", newDoc.updates()}}
			logger.Debugf("updating cloud image metadata for %v", newDoc.Id)
		}

		return []txn.Op{op}, nil
	}

	err := s.runTransaction(buildTxn)
	if err != nil {
		return errors.Annotatef(err, "cannot add cloud image metadata for %v", newDoc.Id)
	}
	return nil
}

func (s *storage) getMetadata(id string) (Metadata, error) {
	coll, closer := s.getCollection(s.collection)
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

func isSameMetadata(old, new Metadata) bool {
	return old.ImageId == new.ImageId &&
		areSameAttributes(old.MetadataAttributes, new.MetadataAttributes)
}

func areSameAttributes(old, new MetadataAttributes) bool {
	return old.Arch == new.Arch &&
		old.Region == new.Region &&
		old.RootStorageType == new.RootStorageType &&
		old.RootStorageSize == new.RootStorageSize &&
		old.Series == new.Series &&
		old.Stream == new.Stream &&
		old.VirtualType == new.VirtualType
}

// AllMetadata implements Storage.AllMetadata.
func (s *storage) AllMetadata() ([]Metadata, error) {
	return s.FindMetadata(MetadataAttributes{})
}

// FindMetadata implements Storage.FindMetadata.
func (s *storage) FindMetadata(criteria MetadataAttributes) ([]Metadata, error) {
	coll, closer := s.getCollection(s.collection)
	defer closer()

	searchCriteria := buildSearchClauses(criteria)
	var docs []imagesMetadataDoc
	if err := coll.Find(searchCriteria).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	if searchCriteria != nil && len(docs) == 0 {
		// If criteria had values and no metadata was found, err
		return nil, errors.NotFoundf("matching cloud image metadata")
	}
	metadata := make([]Metadata, len(docs))
	for i, doc := range docs {
		metadata[i] = doc.metadata()
	}
	return metadata, nil
}

func buildSearchClauses(criteria MetadataAttributes) bson.D {
	all := bson.D{}

	if criteria.Stream != "" {
		all = append(all, bson.DocElem{"stream", criteria.Stream})
	}

	if criteria.Region != "" {
		all = append(all, bson.DocElem{"region", criteria.Region})
	}

	if criteria.Series != "" {
		all = append(all, bson.DocElem{"series", criteria.Series})
	}

	if criteria.Arch != "" {
		all = append(all, bson.DocElem{"arch", criteria.Arch})
	}

	if criteria.VirtualType != "" {
		all = append(all, bson.DocElem{"virtual_type", criteria.VirtualType})
	}

	if criteria.RootStorageType != "" {
		all = append(all, bson.DocElem{"root_storage_type", criteria.RootStorageType})
	}

	if criteria.RootStorageSize != "" {
		all = append(all, bson.DocElem{"root_storage_size", criteria.RootStorageSize})
	}

	if len(all.Map()) == 0 {
		return nil
	}
	return all
}

type imagesMetadataDoc struct {
	// EnvUUID is the environment identifier.
	EnvUUID string `bson:"env-uuid"`

	// Id contains unique natural key for cloud image metadata
	Id string `bson:"_id"`

	// Stream contains reference to a particular stream,
	// for e.g. "daily" or "released"
	Stream string `bson:"stream"`

	// Region is the name of cloud region associated with the image.
	Region string `bson:"region"`

	// Series is Os version, for e.g. "quantal".
	Series string `bson:"series"`

	// Arch is the architecture for this cloud image, for e.g. "amd64"
	Arch string `bson:"arch"`

	// ImageId is an image identifier.
	ImageId string `bson:"image_id"`

	// VirtualType contains the type of the cloud image, for e.g. "pv", "hvm". "kvm".
	VirtualType string `bson:"virtual_type,omitempty"`

	// RootStorageType contains type of root storage, for e.g. "ebs", "instance".
	RootStorageType string `bson:"root_storage_type,omitempty"`

	// RootStorageSize contains size of root storage, for e.g. "30GB", "8GB".
	RootStorageSize string `bson:"root_storage_size,omitempty"`
}

func (m imagesMetadataDoc) metadata() Metadata {
	return Metadata{
		MetadataAttributes{
			Stream:          m.Stream,
			Region:          m.Region,
			Series:          m.Series,
			Arch:            m.Arch,
			RootStorageType: m.RootStorageType,
			RootStorageSize: m.RootStorageSize,
			VirtualType:     m.VirtualType},
		m.ImageId,
	}
}

func (m imagesMetadataDoc) updates() bson.D {
	return bson.D{
		{"stream", m.Stream},
		{"region", m.Region},
		{"series", m.Series},
		{"arch", m.Arch},
		{"virtual_type", m.VirtualType},
		{"root_storage_type", m.RootStorageType},
		{"root_storage_size", m.RootStorageSize},
		{"image_id", m.ImageId},
	}
}

func (s *storage) mongoDoc(m Metadata) imagesMetadataDoc {
	return imagesMetadataDoc{
		EnvUUID:         s.envuuid,
		Id:              buildKey(m),
		Stream:          m.Stream,
		Region:          m.Region,
		Series:          m.Series,
		Arch:            m.Arch,
		VirtualType:     m.VirtualType,
		RootStorageType: m.RootStorageType,
		RootStorageSize: m.RootStorageSize,
		ImageId:         m.ImageId,
	}
}

func buildKey(m Metadata) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s",
		m.Stream,
		m.Region,
		m.Series,
		m.Arch,
		m.VirtualType,
		m.RootStorageType,
		m.RootStorageSize)
}
