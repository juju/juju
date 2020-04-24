// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/blobstore.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.blobstorecleanup")

const (
	defaultLoggingConfig = "<root>=WARNING"
	dataDir              = "/var/lib/juju"
	managedResourceC     = "managedStoredResources"
	resourceCatalogC     = "storedResources"
	//resourceC            = "resources"
	blobstoreFilesC  = "blobstore.files"
	blobstoreChunksC = "blobstore.chunks"
)

type ContentType string

var loggingConfig = gnuflag.String("logging-config", defaultLoggingConfig, "specify log levels for modules")
var human = gnuflag.Bool("h", false, "print human readable values")
var verbose = gnuflag.Bool("v", false, "print more detailed information about found references")
var dryRun = gnuflag.Bool("dry-run", true, "use --dry-run=false to actually cleanup references")
var noChunks = gnuflag.Bool("no-chunks", false, "disable looking for chunks that have no corresponding file")

func main() {
	gnuflag.Usage = func() {
		fmt.Printf("Usage: %s\n", os.Args[0])
		gnuflag.PrintDefaults()
		os.Exit(1)
	}

	gnuflag.Parse(true)

	args := gnuflag.Args()
	if len(args) < 0 {
		gnuflag.Usage()
	}
	checkErr(loggo.ConfigureLoggers(*loggingConfig), "logging config")

	cleaner := NewBlobstoreCleaner()
	cleaner.findModellessManagedResources()
	cleaner.cleanupManagedResources()
	cleaner.findUnmanagedResources()
	cleaner.cleanupUnmanagedResources()
	cleaner.findResourcelessFiles()
	cleaner.cleanupFiles()
	cleaner.findFilelessChunks()
	cleaner.cleanupChunks()
	fmt.Printf("total bytes: %s\n", lengthToSize(cleaner.totalBytes()))
}

func checkErr(err error, label string) {
	if err != nil {
		logger.Errorf("%s: %s", label, err)
		os.Exit(1)
	}
}

func out(format string, a ...interface{}) {
	_, err := fmt.Fprintf(os.Stdout, format, a...)
	checkErr(err, "writing output message")
}

func tick() {
	_, err := fmt.Fprint(os.Stderr, ".")
	checkErr(err, "writing output message")
}

func tickDone() {
	_, err := fmt.Fprint(os.Stderr, "\n")
	checkErr(err, "writing output message")
}

// getState returns a StatePool and the underlying Session.
// callers are responsible for calling session.Close() if there is no error
func getState() (*state.StatePool, *mgo.Session, error) {
	tag, err := getCurrentMachineTag(dataDir)
	if err != nil {
		return nil, nil, errors.Annotate(err, "finding machine tag")
	}

	logger.Infof("current machine tag: %s", tag)

	config, err := getConfig(tag)
	if err != nil {
		return nil, nil, errors.Annotate(err, "loading agent config")
	}

	mongoInfo, available := config.MongoInfo()
	if !available {
		return nil, nil, errors.New("mongo info not available from agent config")
	}
	session, err := mongo.DialWithInfo(*mongoInfo, mongo.DefaultDialOpts())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      config.Controller(),
		ControllerModelTag: config.Model(),
		MongoSession:       session,
	})
	if err != nil {
		session.Close()
		return nil, nil, errors.Annotate(err, "opening state connection")
	}
	return pool, session, nil
}

func getCurrentMachineTag(datadir string) (names.MachineTag, error) {
	var empty names.MachineTag
	values, err := filepath.Glob(filepath.Join(datadir, "agents", "machine-*"))
	if err != nil {
		return empty, errors.Annotate(err, "problem globbing")
	}
	switch len(values) {
	case 0:
		return empty, errors.Errorf("no machines found")
	case 1:
		return names.ParseMachineTag(filepath.Base(values[0]))
	default:
		return empty, errors.Errorf("too many possible machine agents: %v", values)
	}
}

func getConfig(tag names.MachineTag) (agent.ConfigSetterWriter, error) {
	diskPath := agent.ConfigPath(dataDir, tag)
	return agent.ReadConfig(diskPath)
}

func lengthToSize(length uint64) string {
	if *human {
		return humanize.Bytes(length)
	}
	return fmt.Sprintf("%d", length)
}

func NewBlobstoreCleaner() *BlobstoreCleaner {
	statePool, session, err := getState()
	// Some of the chunks queries take a long time
	session.SetSocketTimeout(10 * time.Minute)
	checkErr(err, "getting state connection")
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	storedResources := jujuDB.C(resourceCatalogC)
	blobstoreDB := session.DB("blobstore")
	blobstoreFiles := blobstoreDB.C(blobstoreFilesC)
	blobstoreChunks := blobstoreDB.C(blobstoreChunksC)
	return &BlobstoreCleaner{
		pool:    statePool,
		session: session,
		system:  statePool.SystemState(),

		managedResources: managedResources,
		storedResources:  storedResources,
		blobstoreChunks:  blobstoreChunks,
		blobstoreFiles:   blobstoreFiles,

		foundResourceIDs:    set.NewStrings(),
		foundBlobstorePaths: set.NewStrings(),
		foundBlobstoreIDs:   set.NewStrings(),
	}
}

type ManagedResource struct {
	BucketUUID string
	Path       string
}
type UnmanagedResource struct {
	ID         string
	GridFSPath string
}

type BlobstoreCleaner struct {
	pool    *state.StatePool
	session *mgo.Session
	system  *state.State

	managedResources *mgo.Collection
	storedResources  *mgo.Collection
	blobstoreFiles   *mgo.Collection
	blobstoreChunks  *mgo.Collection

	foundResourceIDs    set.Strings
	foundBlobstorePaths set.Strings
	foundBlobstoreIDs   set.Strings

	modellessResources      []ManagedResource
	modellessResourceBytes  uint64
	unmanagedResources      []UnmanagedResource
	unmanagedResourceBytes  uint64
	unreferencedFiles       []string
	unreferencedFilesBytes  uint64
	unreferencedChunks      []string
	unreferencedChunksBytes uint64
}

func (b *BlobstoreCleaner) Close() {
	b.pool.Close()
	b.session.Close()
}

func (b *BlobstoreCleaner) totalBytes() uint64 {
	return b.modellessResourceBytes + b.unmanagedResourceBytes +
		b.unreferencedFilesBytes + b.unreferencedChunksBytes
}

// managedResourceDoc is the persistent representation of a ManagedResource.
// copied from gopkg.in/juju/blobstore.v2/managedstorage.go
type managedResourceDoc struct {
	ID         string `bson:"_id"`
	BucketUUID string `bson:"bucketuuid"`
	User       string `bson:"user"`
	Path       string `bson:"path"`
	ResourceId string `bson:"resourceid"`
}

// storedResourceDoc is the persistent representation of a Resource.
// copied from gopkg.in/juju/blobstore.v2/resourcecatalog.go
type storedResourceDoc struct {
	ID string `bson:"_id"`
	// Path is the storage path of the resource, which will be
	// the empty string until the upload has been completed.
	Path       string `bson:"path"`
	SHA384Hash string `bson:"sha384hash"`
	Length     int64  `bson:"length"`
	RefCount   int64  `bson:"refcount"`
}

type blobstoreChunk struct {
	ID      bson.ObjectId `bson:"_id"`
	FilesID bson.ObjectId `bson:"files_id"`
	N       int           `bson:"n"`
	Data    []byte        `bson:"data"`
}

var blobstoreChunkNoDataFieldSelector = bson.M{"_id": 1, "files_id": 1}

type blobstoreFile struct {
	ID       bson.ObjectId `bson:"_id"`
	Filename string        `bson:"filename"`
	Length   int64         `bson:"length"`
}

var blobstoreFileFieldSelector = bson.M{"_id": 1, "filename": 1, "length": 1}

func (b *BlobstoreCleaner) findModellessManagedResources() {
	modelUUIDs, err := b.system.AllModelUUIDs()
	checkErr(err, "AllModelUUIDS")
	allModels := set.NewStrings(modelUUIDs...)

	var managedDoc managedResourceDoc
	managedIter := b.managedResources.Find(nil).Iter()
	for managedIter.Next(&managedDoc) {
		alreadyFound := b.foundResourceIDs.Contains(managedDoc.ResourceId)
		b.foundResourceIDs.Add(managedDoc.ResourceId)
		if allModels.Contains(managedDoc.BucketUUID) {
			continue
		}
		// in the Database the Path value is the full path with the bucket
		// prefix (same as the documents ._id). However, in-memory APIs
		// take just the bucket local path and build the longer path.
		// make sure it conforms to our expectation, and then strip off the
		// prefix in preparation for deleting.
		bucketPrefix := fmt.Sprintf("buckets/%s/", managedDoc.BucketUUID)
		if !strings.HasPrefix(managedDoc.Path, bucketPrefix) {
			logger.Warningf("bucket has unexpected prefix, skipping: %q", managedDoc.Path)
			continue
		}
		b.modellessResources = append(b.modellessResources, ManagedResource{
			BucketUUID: managedDoc.BucketUUID,
			Path:       managedDoc.Path[len(bucketPrefix):],
		})

		if alreadyFound {
			if *verbose {
				out("%s: (repeat)\n", managedDoc.Path)
			}
		} else {
			var resourceDoc storedResourceDoc
			err := b.storedResources.FindId(managedDoc.ResourceId).One(&resourceDoc)
			if err == mgo.ErrNotFound {
				logger.Warningf("Managed Resource %q points to missing storedResource: %v", managedDoc.ResourceId, err)
				continue
			} else {
				checkErr(err, "reading stored resource")
			}
			b.modellessResourceBytes += uint64(resourceDoc.Length)
			if *verbose {
				out("%s: %s\n", managedDoc.Path, lengthToSize(uint64(resourceDoc.Length)))
			}
		}
	}
	checkErr(managedIter.Close(), "listing managed stored resources")
	out("Found %d managed resource documents without models totaling %s\n\n",
		len(b.modellessResources), lengthToSize(b.modellessResourceBytes))
}

func (b *BlobstoreCleaner) cleanupManagedResources() {
	if *dryRun {
		out("Not removing %d managed resources\n\n", len(b.modellessResources))
		return
	}
	fmt.Printf("Removing %d managed resources\n", len(b.modellessResources))
	gridfs := blobstore.NewGridFS("blobstore", "blobstore", b.session)
	manager := blobstore.NewManagedStorage(b.session.DB("juju"), gridfs)
	for _, mres := range b.modellessResources {
		logger.Debugf("removing managed resource: buckets/%s/%s", mres.BucketUUID, mres.Path)
		tick()
		// Note, this removes the managed resource document, and if that decrements the refcount to
		// 0, it might remove the underlying resource.
		err := manager.RemoveForBucket(mres.BucketUUID, mres.Path)
		if err != nil {
			logger.Warningf("error trying to delete %q %q: %v", mres.BucketUUID, mres.Path, err)
			continue
		}
	}
	tickDone()
}

func (b *BlobstoreCleaner) findUnmanagedResources() {
	// must be called after findModellessManagedResources because that populates foundResourceIDs
	var resourceDoc storedResourceDoc
	storedResourceIter := b.storedResources.Find(nil).Iter()
	for storedResourceIter.Next(&resourceDoc) {
		b.foundBlobstorePaths.Add(resourceDoc.Path)
		if b.foundResourceIDs.Contains(resourceDoc.ID) {
			continue
		}
		b.unmanagedResources = append(b.unmanagedResources, UnmanagedResource{
			ID:         resourceDoc.ID,
			GridFSPath: resourceDoc.Path,
		})
		b.unmanagedResourceBytes += uint64(resourceDoc.Length)
		if *verbose {
			out("%s: %s refcount: %d\n",
				resourceDoc.ID, lengthToSize(uint64(resourceDoc.Length)), resourceDoc.RefCount)
		}
	}
	checkErr(storedResourceIter.Close(), "listing stored resources")
	out("Found %d unreferenced resource documents totaling %s\n\n",
		len(b.unmanagedResources), lengthToSize(b.unmanagedResourceBytes))
}

func txnRunner(db *mgo.Database) jujutxn.Runner {
	return jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
}

func removeStoredResourceOps(docID string) []txn.Op {
	return []txn.Op{{
		C:      resourceCatalogC,
		Id:     docID,
		Assert: bson.D{{"refcount", 1}},
		Remove: true,
	}}
}

func (b *BlobstoreCleaner) cleanupUnmanagedResources() {
	if *dryRun {
		out("Not removing %d dangling resources\n\n", len(b.unmanagedResources))
		return
	}
	out("Removing %d dangling resources\n", len(b.unmanagedResources))
	// TODO: This duplicates the code in gopkg.in/blobstore.v2/resourcecatalog.go
	// However, that code doesn't expose newResourceCatalog or resourceCatalog.
	// And by their nature, we can't use blobstore.ManagedStorage because the
	// reference to these objects doesn't exist.
	runner := txnRunner(b.session.DB("juju"))
	gridfs := blobstore.NewGridFS("blobstore", "blobstore", b.session)
	for _, unmanagedResource := range b.unmanagedResources {
		logger.Debugf("removing unmanaged resource: %q", unmanagedResource.ID)
		tick()
		// We're explicitly not caring about refcount. Do we care if refcount > 1 ?
		err := runner.Run(func(attempt int) ([]txn.Op, error) {
			if attempt > 0 {
				var resourceDoc storedResourceDoc
				// trying to delete something that doesn't exist anymore
				err := b.storedResources.FindId(unmanagedResource.ID).One(&resourceDoc)
				if err == mgo.ErrNotFound {
					return nil, jujutxn.ErrNoOperations
				}
				checkErr(err, fmt.Sprintf("finding storedResources doc: %q", unmanagedResource.ID))
			}
			return removeStoredResourceOps(unmanagedResource.ID), nil
		})
		if err != nil {
			logger.Warningf("error removing resource: %q %v", unmanagedResource.ID, err)
			continue
		}

		if err := gridfs.Remove(unmanagedResource.GridFSPath); err != nil {
			logger.Warningf("error removing blobstore path: %q %v", unmanagedResource.GridFSPath, err)
			continue
		}
	}
	tickDone()
}

func (b *BlobstoreCleaner) findResourcelessFiles() {
	// must be called after findUnmanagedResources because that populates foundBlobstorePaths
	var fileDoc blobstoreFile
	filesIter := b.blobstoreFiles.Find(nil).Select(blobstoreFileFieldSelector).Iter()
	for filesIter.Next(&fileDoc) {
		b.foundBlobstoreIDs.Add(string(fileDoc.ID))
		if b.foundBlobstorePaths.Contains(fileDoc.Filename) {
			continue
		}
		b.foundBlobstorePaths.Add(fileDoc.Filename)
		b.unreferencedFiles = append(b.unreferencedFiles, fileDoc.Filename)
		b.unreferencedFilesBytes += uint64(fileDoc.Length)
		if *verbose {
			out("%s: %s\n", fileDoc.Filename, lengthToSize(uint64(fileDoc.Length)))
		}
	}
	checkErr(filesIter.Close(), "listing blobstore files")
	out("Found %d unreferenced blobstore files totaling %s\n\n",
		len(b.unreferencedFiles), lengthToSize(b.unreferencedFilesBytes))
}

func (b *BlobstoreCleaner) cleanupFiles() {
	if *dryRun {
		out("Not removing %d dangling files\n\n", len(b.unreferencedFiles))
		return
	}
	out("Removing %d dangling files\n", len(b.unreferencedFiles))
	gridfs := blobstore.NewGridFS("blobstore", "blobstore", b.session)
	for _, path := range b.unreferencedFiles {
		logger.Debugf("removing blobstore file: %q", path)
		tick()
		gridfs.Remove(path)
	}
	tickDone()
}

func (b *BlobstoreCleaner) findFilelessChunks() {
	if *noChunks {
		return
	}
	// must be called after findResourcelessFiles as that populates foundBlobstoreIds
	var chunkDoc blobstoreChunk
	filesIter := b.blobstoreChunks.Find(nil).Select(blobstoreChunkNoDataFieldSelector).Iter()
	for filesIter.Next(&chunkDoc) {
		if b.foundBlobstoreIDs.Contains(string(chunkDoc.FilesID)) {
			continue
		}
		b.unreferencedChunks = append(b.unreferencedChunks, string(chunkDoc.ID))
		var dataDoc blobstoreChunk
		err := b.blobstoreChunks.FindId(chunkDoc.ID).One(&dataDoc)
		if err != nil {
			if err == mgo.ErrNotFound {
				logger.Warningf("chunk doc %s went missing", chunkDoc.ID)
				continue
			}
		}
		b.unreferencedChunksBytes += uint64(len(dataDoc.Data))
		if *verbose {
			out("%s: %s\n", chunkDoc.ID.Hex(), lengthToSize(uint64(len(dataDoc.Data))))
		}
	}
	checkErr(filesIter.Close(), "listing blobstore chunks")
	out("Found %d unreferenced blobstore chunks totaling %s\n\n",
		len(b.unreferencedChunks), lengthToSize(b.unreferencedChunksBytes))
}

func (b *BlobstoreCleaner) cleanupChunks() {
	if *dryRun {
		out("Not removing %d dangling chunks\n\n", len(b.unreferencedChunks))
		return
	}
	out("Removing %d dangling files\n", len(b.unreferencedFiles))
	chunks := b.session.DB("blobstore").C(blobstoreChunksC)
	for _, chunkID := range b.unreferencedChunks {
		objID := bson.ObjectId(chunkID)
		logger.Debugf("removing blobstore file: %q", objID.Hex())
		tick()
		err := chunks.RemoveId(objID)
		if err == mgo.ErrNotFound {
			continue
		}
		if err != nil {
			logger.Warningf("error cleaning up blobstore chunk: %q %v", objID.Hex(), err)
			continue
		}
	}
	tickDone()
}
