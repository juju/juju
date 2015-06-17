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

	"github.com/juju/juju/environs/imagemetadata"
)

var logger = loggo.GetLogger("juju.state.cloudimagemetadata")

type cloudImageStorage struct {
	envUUID            string
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

var _ Storage = (*cloudImageStorage)(nil)

// NewStorage constructs a new Storage that stores  image metadata
// in the provided collection using the provided transaction runner.
func NewStorage(
	envUUID string,
	metadataCollection *mgo.Collection,
	runner jujutxn.Runner,
) Storage {
	return &cloudImageStorage{
		envUUID:            envUUID,
		metadataCollection: metadataCollection,
		txnRunner:          runner,
	}
}

// AddMetadata implements Storage.AddMetadata
func (s *cloudImageStorage) AddMetadata(metadata Metadata) error {
	newDoc := imagesMetadataDoc{
		Id:          metadata.imageId(),
		Version:     metadata.Version,
		Storage:     metadata.Storage,
		VirtType:    metadata.VirtType,
		Arch:        metadata.Arch,
		RegionAlias: metadata.RegionAlias,
		RegionName:  metadata.RegionName,
		Endpoint:    metadata.Endpoint,
		Stream:      metadata.Stream,
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:      s.metadataCollection.Name,
			Id:     newDoc.Id,
			Assert: txn.DocMissing,
			Insert: &newDoc,
		}
		return []txn.Op{op}, nil
	}

	err := s.txnRunner.Run(buildTxn)
	if err != nil {
		return errors.Annotate(err, "cannot store cloud images metadata")
	}
	return nil
}

// Metadata implements Storage.Metadata
func (s *cloudImageStorage) Metadata(stream, version, arch string) (Metadata, error) {
	metadataDoc, err := s.imagesMetadata(stream, version, arch)
	if err != nil {
		return Metadata{}, errors.Trace(err)
	}
	return metadataDoc.public(), nil
}

// AllMetadata implements Storage.AllMetadata
func (s *cloudImageStorage) AllMetadata() ([]Metadata, error) {
	var docs []imagesMetadataDoc
	if err := s.metadataCollection.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	metadata := make([]Metadata, len(docs))
	for i, doc := range docs {
		metadata[i] = doc.public()
	}
	return metadata, nil
}

func (c *cloudImageStorage) imagesMetadata(
	stream, version, arch string,
) (imagesMetadataDoc, error) {
	// Construct metadata search id based on input
	desiredID := createId(stream, version, arch)
	var doc imagesMetadataDoc
	err := c.metadataCollection.Find(bson.D{{"_id", desiredID}}).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf("%v cloud images metadata", desiredID)
	}
	if err != nil {
		return doc, err
	}
	return doc, nil
}

type imagesMetadataDoc struct {
	Id          string `bson:"_id"`
	Storage     string `bson:"root_store,omitempty"`
	VirtType    string `bson:"virt,omitempty"`
	Arch        string `bson:"arch,omitempty"`
	Version     string `bson:"version"`
	RegionAlias string `bson:"crsn,omitempty"`
	RegionName  string `bson:"region,omitempty"`
	Endpoint    string `bson:"endpoint,omitempty"`
	Stream      string `bson:"stream"`
}

func (m imagesMetadataDoc) public() Metadata {
	return Metadata{
		Version:     m.Version,
		Storage:     m.Storage,
		VirtType:    m.VirtType,
		Arch:        m.Arch,
		RegionAlias: m.RegionAlias,
		RegionName:  m.RegionName,
		Endpoint:    m.Endpoint,
		Stream:      m.Stream,
	}
}

func idStream(stream string) string {
	idstream := ""
	if stream != "" && stream != imagemetadata.ReleasedStream {
		idstream = "." + stream
	}
	return idstream
}

var createId = func(stream, version, arch string) string {
	return fmt.Sprintf("com.ubuntu.cloud%s:server:%s:%s", stream, version, arch)
}

func (im *Metadata) imageId() string {
	return createId(idStream(im.Stream), im.Version, im.Arch)
}

func (im *Metadata) String() string {
	return fmt.Sprintf("%#v", im)
}
