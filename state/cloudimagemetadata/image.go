// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
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

// SaveMetadata implements Storage.SaveMetadata and behaves as save-or-update.
// If desired metadata
//     * does not exist - we insert it;
//     * exists         - we delete the old record and insert a new one.
func (s *storage) SaveMetadata(metadata Metadata) error {
	newDoc := s.mongoDoc(metadata)

	insertOp := txn.Op{
		C:      s.collection,
		Id:     newDoc.ImageId,
		Assert: txn.DocMissing,
		Insert: &newDoc,
	}

	removeOp := func(imageId string) txn.Op {
		return txn.Op{
			C:      s.collection,
			Id:     imageId,
			Assert: txn.DocExists,
			Remove: true,
		}
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Check if this image metadata is already known.
		all, err := s.FindMetadata(metadata.MetadataAttributes)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
		}

		if len(all) > 0 {
			logger.Debugf("updating metadata for cloud image %v", newDoc.ImageId)
			// More than one metadata is not expected, but is not lethal:
			// let's delete them all.
			txns := make([]txn.Op, len(all)+1)
			for i, one := range all {
				txns[i] = removeOp(one.ImageId)
			}
			txns[len(all)] = insertOp
			return txns, nil
		}
		logger.Debugf("inserting metadata for cloud image %v", newDoc.ImageId)
		return []txn.Op{insertOp}, nil
	}

	err := s.runTransaction(buildTxn)
	if err != nil {
		return errors.Annotatef(err, "cannot save metadata for cloud image %v", newDoc.ImageId)
	}
	return nil
}

// FindMetadata implements Storage.FindMetadata.
// Results are sorted by date created.
func (s *storage) FindMetadata(criteria MetadataAttributes) ([]Metadata, error) {
	coll, closer := s.getCollection(s.collection)
	defer closer()

	searchCriteria := buildSearchClauses(criteria)
	var docs []imagesMetadataDoc
	if err := coll.Find(searchCriteria).Sort("date_created").All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
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

	// Size is not a discriminating attribute for cloud image metadata.
	// It is not included in search criteria.

	if len(all.Map()) == 0 {
		return nil
	}
	return all
}

// imagesMetadataDoc results in immutable records. Updates are effectively
// a delate and an insert.
type imagesMetadataDoc struct {
	// EnvUUID is the environment identifier.
	EnvUUID string `bson:"env-uuid"`

	// ImageId contains unique natural key for cloud image metadata - an image id.
	ImageId string `bson:"_id"`

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

	// RootStorageSize contains size of root storage.
	RootStorageSize uint64 `bson:"root_storage_size"`

	// DateCreated is the date/time when this doc was created
	DateCreated time.Time `bson:"date_created"`
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

func (s *storage) mongoDoc(m Metadata) imagesMetadataDoc {
	return imagesMetadataDoc{
		EnvUUID:         s.envuuid,
		Stream:          m.Stream,
		Region:          m.Region,
		Series:          m.Series,
		Arch:            m.Arch,
		VirtualType:     m.VirtualType,
		RootStorageType: m.RootStorageType,
		RootStorageSize: m.RootStorageSize,
		ImageId:         m.ImageId,
		DateCreated:     time.Now(),
	}
}
