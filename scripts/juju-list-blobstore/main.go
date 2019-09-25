// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/dustin/go-humanize"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.listblobstore")

const (
	defaultLoggingConfig = "<root>=WARNING"
	dataDir              = "/var/lib/juju"
	managedResourceC     = "managedStoredResources"
	resourceCatalogC     = "storedResources"
)

var loggingConfig = gnuflag.String("logging-config", defaultLoggingConfig, "specify log levels for modules")
var human = gnuflag.Bool("h", false, "print human readable values")

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

func shortModelUUID(modelUUID string) string {
	if len(modelUUID) > 6 {
		return modelUUID[:6]
	}
	return modelUUID
}

// managedResourceDoc is the persistent representation of a ManagedResource.
// copied from gopkg.in/juju/blobstore.v2/managedstorage.go
type managedResourceDoc struct {
	Id         string `bson:"_id"`
	BucketUUID string `bson:"bucketuuid"`
	User       string `bson:"user"`
	Path       string `bson:"path"`
	ResourceId string `bson:"resourceid"`
}

// resourceDoc is the persistent representation of a Resource.
// copied from gopkg.in/juju/blobstore.v2/resourcecatalog.go
type resourceDoc struct {
	Id string `bson:"_id"`
	// Path is the storage path of the resource, which will be
	// the empty string until the upload has been completed.
	Path       string `bson:"path"`
	SHA384Hash string `bson:"sha384hash"`
	Length     int64  `bson:"length"`
	RefCount   int64  `bson:"refcount"`
}

func inspectAgentBinaries(st *state.State, session *mgo.Session) {
	toolsStorage, err := st.ToolsStorage()
	checkErr("tools storage", err)
	defer toolsStorage.Close()
	var totalBytes uint64

	fmt.Fprintf(os.Stdout, "Agent Binaries\n")
	modelUUID := st.ModelUUID()
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	resources := jujuDB.C(resourceCatalogC)
	// managedStore := blobstore.NewManagedStorage(jujuDB, blobstore.NewGridFS("blobstore", "blobstore", session))
	toolsMetadata, err := toolsStorage.AllMetadata()
	checkErr("tools metadata", err)
	logger.Debugf("found %d tools", len(toolsMetadata))
	seen := make(map[version.Number]int, 0)
	seenResources := make(map[string]int64)
	for _, metadata := range toolsMetadata {
		// internally we store a metadata.Path for each object, we really need that here.. :(
		binary, err := version.ParseBinary(metadata.Version)
		if err != nil {
			logger.Warningf("Unknown Binary Version: %q", metadata.Version)
			continue
		}
		count := seen[binary.Number]
		seen[binary.Number] = count + 1
		// This assumes we aren't reading from a fallback storage
		// These queries aren't exposed via the ToolsStorage interface nor via the ManagedStorage interface
		toolPath := path.Join("tools", fmt.Sprintf("%s-%s", metadata.Version, metadata.SHA256))
		bucketPath := path.Join("buckets", modelUUID, toolPath)
		resource := lookupResource(managedResources, resources, bucketPath, metadata.Version)
		_, found := seenResources[resource.Id]
		seenResources[resource.Id] = resource.Length
		if !found {
			totalBytes += uint64(resource.Length)
			size := fmt.Sprint(resource.Length)
			if *human {
				size = humanize.Bytes(uint64(resource.Length))
			}
			fmt.Fprintf(os.Stdout, "%v: %s refcount %d %s...\n",
				binary.Number, size, resource.RefCount, resource.SHA384Hash[:8])
		}
	}
	size := fmt.Sprintf("%d", totalBytes)
	if *human {
		size = humanize.Bytes(totalBytes)
	}
	fmt.Fprintf(os.Stdout, "total: %s\n\n", size)
}

func lookupResource(managedResources, resources *mgo.Collection, bucketPath, description string) resourceDoc {
	var manageDoc managedResourceDoc
	var resource resourceDoc
	err := managedResources.Find(bson.M{"path": bucketPath}).One(&manageDoc)
	if err == mgo.ErrNotFound {
		logger.Warningf("could not find managed resource doc for %q", description)
		return resource
	}
	checkErr("managed resource doc", err)
	err = resources.FindId(manageDoc.ResourceId).One(&resource)
	if err == mgo.ErrNotFound {
		logger.Warningf("could not find resource doc for %q", description)
		return resource
	}
	checkErr("resource doc", err)
	if resource.Id != resource.SHA384Hash {
		logger.Warningf("resource with id != sha384: %q != %q", resource.Id, resource.SHA384Hash)
	}
	return resource
}

func inspectModel(pool *state.StatePool, session *mgo.Session, modelUUID string) {
	model, helper, err := pool.GetModel(modelUUID)
	defer helper.Release()
	shortUID := shortModelUUID(modelUUID)
	checkErr(fmt.Sprintf("reading model %s", shortUID), err)
	charms, err := model.State().AllCharms()
	checkErr("AllCharms", err)
	logger.Debugf("[%s] checking model", shortUID)
	name := model.Name()
	fmt.Fprintf(os.Stdout, "Model: %q\n", name)
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	resources := jujuDB.C(resourceCatalogC)
	var totalBytes uint64
	seen := make(map[string]int64)
	for _, charm := range charms {
		bucketPath := path.Join("buckets", modelUUID, charm.StoragePath())
		resource := lookupResource(managedResources, resources, bucketPath, charm.String())
		_, found := seen[resource.Id]
		seen[resource.Id] = resource.Length
		if !found {
			totalBytes += uint64(resource.Length)
			size := fmt.Sprint(resource.Length)
			if *human {
				size = humanize.Bytes(uint64(resource.Length))
			}
			fmt.Fprintf(os.Stdout, "%v: %s refcount %d %s...\n",
				charm.String(), size, resource.RefCount, resource.Id[:8])
		}
	}
	size := fmt.Sprintf("%d", totalBytes)
	if *human {
		size = humanize.Bytes(totalBytes)
	}
	fmt.Fprintf(os.Stdout, "total: %s\n\n", size)
}

func main() {
	loggo.GetLogger("").SetLogLevel(loggo.TRACE)
	gnuflag.Usage = func() {
		fmt.Printf("Usage: %s\n", os.Args[0])
		os.Exit(1)
	}

	gnuflag.Parse(true)

	args := gnuflag.Args()
	if len(args) < 0 {
		gnuflag.Usage()
	}
	checkErr("logging config", loggo.ConfigureLoggers(*loggingConfig))

	statePool, session, err := getState()
	checkErr("getting state connection", err)
	defer statePool.Close()
	defer session.Close()
	system := statePool.SystemState()
	inspectAgentBinaries(system, session)
	modelUUIDs, err := system.AllModelUUIDs()
	checkErr("listing model UUIDs", err)
	logger.Debugf("Found models: %s", modelUUIDs)
	for _, modelUUID := range modelUUIDs {
		inspectModel(statePool, session, modelUUID)
	}
}
