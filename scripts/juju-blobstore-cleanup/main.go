// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

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
	checkErr("logging config", loggo.ConfigureLoggers(*loggingConfig))

	cleaner := NewBlobstoreCleaner()
	cleaner.findModellessManagedResources()
	cleaner.findUnmanagedResources()
	cleaner.findResourcelessFiles()
	cleaner.findFilelessChunks()
}

func checkErr(label string, err error) {
	if err != nil {
		logger.Errorf("%s: %s", label, err)
		os.Exit(1)
	}
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
	checkErr("getting state connection", err)
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

	modellessResources []string
	unmanagedResources []string
	unreferencedFiles  []string
	unreferencedChunks []string
}

func (b *BlobstoreCleaner) Close() {
	b.pool.Close()
	b.session.Close()
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
	checkErr("AllModelUUIDS", err)
	allModels := set.NewStrings(modelUUIDs...)

	var managedDoc managedResourceDoc
	var managedBytes uint64
	managedIter := b.managedResources.Find(nil).Iter()
	for managedIter.Next(&managedDoc) {
		alreadyFound := b.foundResourceIDs.Contains(managedDoc.ResourceId)
		b.foundResourceIDs.Add(managedDoc.ResourceId)
		if allModels.Contains(managedDoc.BucketUUID) {
			continue
		}
		b.modellessResources = append(b.modellessResources, managedDoc.ID)

		if alreadyFound {
			if *verbose {
				fmt.Printf("%s: (repeat)\n", managedDoc.Path)
			}
		} else {
			var resourceDoc storedResourceDoc
			err := b.storedResources.FindId(managedDoc.ResourceId).One(&resourceDoc)
			if err == mgo.ErrNotFound {
				logger.Warningf("Managed Resource %q points to missing storedResource: %q", managedDoc.ResourceId)
				continue
			} else {
				checkErr("reading stored resource", err)
			}
			managedBytes += uint64(resourceDoc.Length)
			if *verbose {
				fmt.Printf("%s: %d\n", managedDoc.Path, lengthToSize(uint64(resourceDoc.Length)))
			}
		}
	}
	checkErr("listing managed stored resources", managedIter.Close())
	fmt.Printf("Found %d managed resource documents without models totaling %s bytes\n", len(b.modellessResources), lengthToSize(managedBytes))
}

func (b *BlobstoreCleaner) findUnmanagedResources() {
	// must be called after findModellessManagedResources because that populates foundResourceIDs
	var resourceDoc storedResourceDoc
	storedResourceIter := b.storedResources.Find(nil).Iter()
	var unmanagedBytes uint64
	for storedResourceIter.Next(&resourceDoc) {
		b.foundBlobstorePaths.Add(resourceDoc.Path)
		if b.foundResourceIDs.Contains(resourceDoc.ID) {
			continue
		}
		b.unmanagedResources = append(b.unmanagedResources, resourceDoc.ID)
		unmanagedBytes += uint64(resourceDoc.Length)
		if *verbose {
			fmt.Printf("%s: %d\n", resourceDoc.ID, lengthToSize(uint64(resourceDoc.Length)))
		}
	}
	checkErr("listing stored resources", storedResourceIter.Close())
	fmt.Printf("Found %d unreferenced resource documents totaling %s bytes\n", len(b.unmanagedResources), lengthToSize(unmanagedBytes))
}

func (b *BlobstoreCleaner) findResourcelessFiles() {
	// must be called after findUnmanagedResources because that populates foundBlobstorePaths
	var fileDoc blobstoreFile
	var unreferencedFileBytes uint64
	filesIter := b.blobstoreFiles.Find(nil).Select(blobstoreFileFieldSelector).Iter()
	for filesIter.Next(&fileDoc) {
		b.foundBlobstoreIDs.Add(string(fileDoc.ID))
		if b.foundBlobstorePaths.Contains(fileDoc.Filename) {
			continue
		}
		b.foundBlobstorePaths.Add(fileDoc.Filename)
		b.unreferencedFiles = append(b.unreferencedFiles, fileDoc.Filename)
		unreferencedFileBytes += uint64(fileDoc.Length)
		if *verbose {
			fmt.Printf("%s: %d\n", fileDoc.Filename, lengthToSize(uint64(fileDoc.Length)))
		}
	}
	checkErr("listing blobstore files", filesIter.Close())
	fmt.Printf("Found %d unreferenced blobstore files totaling %s bytes\n", len(b.unreferencedFiles), lengthToSize(unreferencedFileBytes))
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
		if *verbose {
			fmt.Printf("%s: ?\n", chunkDoc.ID)
		}
	}
	checkErr("listing blobstore chunks", filesIter.Close())
	fmt.Printf("Found %d unreferenced blobstore chunks totaling ? bytes\n", len(b.unreferencedChunks))
}
