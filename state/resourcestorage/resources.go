// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourcestorage

import (
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"time"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/charmresources"
	"github.com/juju/juju/mongo"
)

var logger = loggo.GetLogger("juju.state.resourcestorage")

const (
	// resourcemetadataC is the collection used to store resource metadata.
	resourcemetadataC = "resourcemetadata"

	// ResourcesDB is the database used to store resource blobs.
	ResourcesDB = "resources"
)

type resourceStorage struct {
	envUUID               string
	metadataCollection    *mgo.Collection
	blobDb                *mgo.Database
	getManagedStorageFunc func(*mgo.Database, blobstore.ResourceStorage) blobstore.ManagedStorage
	getTxnRunnerFunc      func(*mgo.Database) jujutxn.Runner
	getTimeFunc           func() time.Time
}

var _ charmresources.ResourceManager = (*resourceStorage)(nil)

// NewResourceManager constructs a new ResourceManager that stores resource
// blobs in a "resources" database. Metadata is also stored in this database
// in the "resourcemetadata" collection.
func NewResourceManager(session *mgo.Session, envUUID string) charmresources.ResourceManager {
	return newResourceManagerInternal(session, envUUID, nil, nil, nil)
}

func newResourceManagerInternal(
	session *mgo.Session,
	envUUID string,
	getManagedStorage func(*mgo.Database, blobstore.ResourceStorage) blobstore.ManagedStorage,
	getTxnRunner func(*mgo.Database) jujutxn.Runner,
	getTime func() time.Time,
) charmresources.ResourceManager {
	blobDb := session.DB(ResourcesDB)
	metadataCollection := blobDb.C(resourcemetadataC)
	if getManagedStorage == nil {
		getManagedStorage = func(db *mgo.Database, rs blobstore.ResourceStorage) blobstore.ManagedStorage {
			return blobstore.NewManagedStorage(db, rs)
		}
	}
	if getTxnRunner == nil {
		getTxnRunner = func(db *mgo.Database) jujutxn.Runner {
			return jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
		}
	}
	if getTime == nil {
		getTime = time.Now
	}
	return &resourceStorage{
		envUUID, metadataCollection, blobDb,
		getManagedStorage, getTxnRunner, getTime,
	}
}

func (s *resourceStorage) getManagedStorage(session *mgo.Session) blobstore.ManagedStorage {
	rs := blobstore.NewGridFS(ResourcesDB, ResourcesDB, session)
	db := session.DB(ResourcesDB)
	metadataDb := db.With(session)
	return s.getManagedStorageFunc(metadataDb, rs)
}

func (s *resourceStorage) txnRunner(session *mgo.Session) jujutxn.Runner {
	db := s.metadataCollection.Database
	runnerDb := db.With(session)
	return s.getTxnRunnerFunc(runnerDb)
}

// ResourcePut is defined on the ResourceManager interface.
func (s *resourceStorage) ResourcePut(meta charmresources.Resource, r io.Reader) (
	result charmresources.Resource, resultErr error,
) {
	attrs, err := charmresources.ParseResourcePath(meta.Path)
	if err != nil {
		return result, errors.Trace(err)
	}
	blobPath, err := newResourceBlobPath(meta.Path)
	if err != nil {
		return result, errors.Annotate(err, "cannot create resource blob path")
	}

	result = meta
	result.Size = 0 // init Size to 0, as we'll count
	result.Created = s.getTimeFunc()
	r = countingReader{r, &result.Size}

	// If size is unspecified, instruct PutForEnvironmentAndCheckHash
	// to read until EOF and count the bytes read.
	if meta.Size <= 0 {
		meta.Size = -1
	}

	// If the hash is specified, then we'll check it in
	// PutForEnvironmentAndCheckHash. Otherwise, compute the hash for
	// the result.
	var hash hash.Hash
	if meta.SHA384Hash == "" {
		hash = sha512.New384()
		r = io.TeeReader(r, hash)
	}

	session := s.blobDb.Session.Copy()
	defer session.Close()
	managedStorage := s.getManagedStorage(session)
	if err := managedStorage.PutForEnvironmentAndCheckHash(s.envUUID, blobPath, r, meta.Size, meta.SHA384Hash); err != nil {
		return result, errors.Annotate(err, "cannot store resource")
	}
	defer func() {
		if resultErr == nil {
			return
		}
		err := managedStorage.RemoveForEnvironment(s.envUUID, blobPath)
		if err != nil {
			logger.Errorf("failed to remove resource blob: %v", err)
		}
	}()
	if hash != nil {
		result.SHA384Hash = fmt.Sprintf("%x", hash.Sum(nil))
	}

	newDoc := resourceMetadataDoc{
		Id:       docId(s.envUUID, meta.Path),
		EnvUUID:  s.envUUID,
		SHA384:   result.SHA384Hash,
		Size:     result.Size,
		Created:  result.Created,
		BlobPath: blobPath,
		Type:     attrs.Type,
		User:     attrs.User,
		Org:      attrs.Org,
		Stream:   attrs.Stream,
		Series:   attrs.Series,
		PathName: attrs.PathName,
		Revision: attrs.Revision,
	}

	// Add or replace metadata. If replacing, record the
	// existing path so we can remove the blob later.
	var oldBlobPath string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:  resourcemetadataC,
			Id: newDoc.Id,
		}

		// On the first attempt we assume we're adding a new resource blob.
		// Subsequent attempts to add resource will fetch the existing
		// doc, record the old path, and attempt to update fields.
		if attempt == 0 {
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
		} else {
			oldDoc, err := s.resourceMetadataDoc(meta.Path)
			if err != nil {
				return nil, err
			}
			oldBlobPath = oldDoc.BlobPath
			op.Assert = bson.D{{"blobpath", oldBlobPath}}
			if oldBlobPath != blobPath {
				op.Update = bson.D{{
					"$set", bson.D{
						{"sha384", newDoc.SHA384},
						{"size", newDoc.Size},
						{"created", newDoc.Created},
						{"blobpath", newDoc.BlobPath},
						// The ResourceAttributes fields
						// cannot have changed, as they
						// are used to determine the path.
					},
				}}
			}
		}
		return []txn.Op{op}, nil
	}
	txnRunner := s.txnRunner(session)
	if err := txnRunner.Run(buildTxn); err != nil {
		return result, errors.Annotate(err, "cannot store resource metadata")
	}

	if oldBlobPath != "" && oldBlobPath != blobPath {
		// Attempt to remove the old path. Failure is non-fatal.
		err := managedStorage.RemoveForEnvironment(s.envUUID, oldBlobPath)
		if err != nil {
			logger.Errorf("failed to remove old resource blob: %v", err)
		} else {
			logger.Debugf("removed old resource blob")
		}
	}
	return result, nil
}

// ResourceList is defined on the ResourceManager interface.
func (s *resourceStorage) ResourceList(filter charmresources.ResourceAttributes) ([]charmresources.Resource, error) {
	metadataDocs, err := s.listResourceMetadataDocs(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]charmresources.Resource, len(metadataDocs))
	for i, doc := range metadataDocs {
		resourcePath, err := charmresources.ResourcePath(charmresources.ResourceAttributes{
			doc.Type,
			doc.User,
			doc.Org,
			doc.Stream,
			doc.Series,
			doc.PathName,
			doc.Revision,
		})
		if err != nil {
			return nil, errors.Annotate(err, "getting resource path")
		}
		result[i] = charmresources.Resource{
			resourcePath,
			doc.SHA384,
			doc.Size,
			doc.Created,
		}
	}
	return result, nil
}

// ResourceDelete is defined on the ResourceManager interface.
func (s *resourceStorage) ResourceDelete(resourcePath string) (resultErr error) {
	metadataDoc, err := s.resourceMetadataDoc(resourcePath)
	if err != nil {
		return errors.Trace(err)
	}
	blobPath := metadataDoc.BlobPath

	session := s.blobDb.Session.Copy()
	defer session.Close()
	managedStorage := s.getManagedStorage(session)
	if err := managedStorage.RemoveForEnvironment(s.envUUID, blobPath); err != nil {
		return errors.Annotate(err, "cannot remove resource blob")
	}
	// Remove the metadata.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:      resourcemetadataC,
			Id:     metadataDoc.Id,
			Remove: true,
		}
		return []txn.Op{op}, nil
	}
	txnRunner := s.txnRunner(session)
	if err := txnRunner.Run(buildTxn); err == mgo.ErrNotFound {
		// Metadata already removed, we don't care.
		return nil
	}
	return errors.Annotate(err, "cannot remove resource metadata")
}

// resourceCloser encapsulates a resource reader and session
// so that both are closed together.
type resourceCloser struct {
	io.ReadCloser
	session *mgo.Session
}

func (c *resourceCloser) Close() error {
	c.session.Close()
	return c.ReadCloser.Close()
}

// ResourceGet is defined on the ResourceManager interface.
func (s *resourceStorage) ResourceGet(resourcePath string) ([]charmresources.ResourceReader, error) {
	metadataDoc, err := s.resourceMetadataDoc(resourcePath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	blobPath := metadataDoc.BlobPath
	session := s.blobDb.Session.Copy()
	managedStorage := s.getManagedStorage(session)
	rc, err := s.resourceBlobReader(managedStorage, blobPath)
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	return []charmresources.ResourceReader{{
		&resourceCloser{rc, session},
		charmresources.Resource{
			resourcePath,
			metadataDoc.SHA384,
			metadataDoc.Size,
			metadataDoc.Created,
		},
	}}, nil
}

type resourceMetadataDoc struct {
	Id       string    `bson:"_id"`
	EnvUUID  string    `bson:"envuuid"`
	SHA384   string    `bson:"sha384"`
	Size     int64     `bson:"size"`
	Created  time.Time `bson:"created"`
	BlobPath string    `bson:"blobpath"`

	// Fields below map 1:1 to charmresources.ResourceAttributes.

	Type     string `bson:"type"`
	User     string `bson:"user"`
	Org      string `bson:"org"`
	Stream   string `bson:"stream"`
	Series   string `bson:"series"`
	PathName string `bson:"pathname"`
	Revision string `bson:"revision"`
}

func (s *resourceStorage) resourceMetadataDoc(resourcePath string) (*resourceMetadataDoc, error) {
	var doc resourceMetadataDoc
	id := docId(s.envUUID, resourcePath)
	coll, closer := mongo.CollectionFromName(s.metadataCollection.Database, resourcemetadataC)
	defer closer()
	err := coll.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("%v resource metadata", id)
	} else if err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *resourceStorage) listResourceMetadataDocs(filter charmresources.ResourceAttributes) ([]resourceMetadataDoc, error) {
	coll, closer := mongo.CollectionFromName(s.metadataCollection.Database, resourcemetadataC)
	defer closer()
	resourceDocs := []resourceMetadataDoc{}

	query := bson.D{{"envuuid", s.envUUID}}
	filterOn := func(k string, v string) {
		if v != "" {
			query = append(query, bson.DocElem{k, v})
		}
	}
	filterOn("type", filter.Type)
	filterOn("user", filter.User)
	filterOn("org", filter.Org)
	filterOn("stream", filter.Stream)
	filterOn("series", filter.Series)
	filterOn("pathname", filter.PathName)
	filterOn("revision", filter.Revision)

	err := coll.Find(query).All(&resourceDocs)
	if err != nil {
		return nil, errors.Annotate(err, "listing resource metadata")
	}
	return resourceDocs, nil
}

func (s *resourceStorage) resourceBlobReader(managedStorage blobstore.ManagedStorage, path string) (io.ReadCloser, error) {
	r, _, err := managedStorage.GetForEnvironment(s.envUUID, path)
	return r, err
}

// newResourceBlobPath returns a new unique blob storage path, given a
// resource path. The returned path will incorporate the resource path
// as an eye-catcher, and a UUID for its uniqueness.
func newResourceBlobPath(resourcePath string) (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("resources%s:%s", resourcePath, uuid.String()), nil
}

// docId returns an id for the mongo resource metadata document.
func docId(envUUID, resourcePath string) string {
	return fmt.Sprintf("%s-%s", envUUID, resourcePath)
}

type countingReader struct {
	r     io.Reader
	count *int64
}

func (r countingReader) Read(buf []byte) (n int, err error) {
	n, err = r.r.Read(buf)
	*r.count += int64(n)
	return n, err
}
