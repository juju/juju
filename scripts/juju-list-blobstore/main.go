// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6/resource"
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
var verbose = gnuflag.Bool("v", false, "print more detailed information about found references")

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
	foundHashes := make(map[string]struct{})
	inspectAgentBinaries(system, session, foundHashes)
	modelUUIDs, err := system.AllModelUUIDs()
	checkErr("listing model UUIDs", err)
	logger.Debugf("Found models: %s", modelUUIDs)
	for _, modelUUID := range modelUUIDs {
		inspectModel(statePool, session, modelUUID, foundHashes)
	}
	checkMissing(system, session, foundHashes)
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

type sortableBinaries []version.Binary

func (s sortableBinaries) Len() int      { return len(s) }
func (s sortableBinaries) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortableBinaries) Less(i, j int) bool {
	cmp := s[i].Compare(s[j].Number)
	switch {
	case cmp < 0:
		return true
	case cmp > 0:
		return false
	default: // cmp == 0
		// Technically we could compare Series and then Arch as strings
		// individually, but this still gives the right answer.
		return s[i].String() < s[j].String()
	}
}

type binaryInfo struct {
	Version version.Binary
	Hash    string
}
type binariesInfo []binaryInfo

func (bi binariesInfo) Len() int      { return len(bi) }
func (bi binariesInfo) Swap(i, j int) { bi[i], bi[j] = bi[j], bi[i] }
func (bi binariesInfo) Less(i, j int) bool {
	cmp := bi[i].Version.Compare(bi[j].Version.Number)
	switch {
	case cmp < 0:
		return true
	case cmp > 0:
		return false
	default: // cmp == 0
		return bi[i].Version.String() < bi[j].Version.String()
	}
}

func inspectAgentBinaries(st *state.State, session *mgo.Session, foundHashes map[string]struct{}) {
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
	// map from SHA384 to the list of agent versions that match it
	seen := make(map[string][]version.Binary, 0)
	sizes := make(map[string]int64)
	for _, metadata := range toolsMetadata {
		// internally we store a metadata.Path for each object, we really need that here.. :(
		binary, err := version.ParseBinary(metadata.Version)
		if err != nil {
			logger.Warningf("Unknown Binary Version: %q", metadata.Version)
			continue
		}
		// This assumes we aren't reading from a fallback storage
		// These queries aren't exposed via the ToolsStorage interface nor via the ManagedStorage interface
		toolPath := path.Join("tools", fmt.Sprintf("%s-%s", metadata.Version, metadata.SHA256))
		bucketPath := path.Join("buckets", modelUUID, toolPath)
		res := lookupResource(managedResources, resources, bucketPath, metadata.Version)
		soFar, found := seen[res.SHA384Hash]
		seen[res.SHA384Hash] = append(soFar, binary)
		sizes[res.SHA384Hash] = res.Length
		foundHashes[res.SHA384Hash] = struct{}{}
		if !found {
			totalBytes += uint64(res.Length)
		}
	}
	firstKeys := make(binariesInfo, 0, len(seen))
	for key := range seen {
		binaries := seen[key]
		if len(binaries) == 0 {
			// How could it be seen but have no entries?
			continue
		}
		sort.Sort(sortableBinaries(binaries))
		seen[key] = binaries
		firstKeys = append(firstKeys, binaryInfo{
			Version: binaries[0],
			Hash:    key,
		})
	}
	sort.Sort(firstKeys)
	for _, info := range firstKeys {
		length := sizes[info.Hash]
		size := fmt.Sprint(length)
		if *human {
			size = humanize.Bytes(uint64(length))
		}
		fmt.Fprintf(os.Stdout, "%v: %s %d %s...\n",
			info.Version.Number, size, len(seen[info.Hash]), info.Hash[:8])
		if *verbose {
			binaries := seen[info.Hash]
			for _, binary := range binaries {
				fmt.Fprintf(os.Stdout, "  %s\n", binary)
			}
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
	var res resourceDoc
	err := managedResources.Find(bson.M{"path": bucketPath}).One(&manageDoc)
	if err == mgo.ErrNotFound {
		logger.Warningf("could not find managed resource doc for %q", description)
		return res
	}
	checkErr("managed resource doc", err)
	err = resources.FindId(manageDoc.ResourceId).One(&res)
	if err == mgo.ErrNotFound {
		logger.Warningf("could not find resource doc for %q", description)
		return res
	}
	checkErr("resource doc", err)
	if res.Id != res.SHA384Hash {
		logger.Warningf("resource with id != sha384: %q != %q", res.Id, res.SHA384Hash)
	}
	return res
}

func inspectModel(pool *state.StatePool, session *mgo.Session, modelUUID string, foundHashes map[string]struct{}) {
	model, helper, err := pool.GetModel(modelUUID)
	defer helper.Release()
	shortUID := shortModelUUID(modelUUID)
	checkErr(fmt.Sprintf("reading model %s", shortUID), err)
	charms, err := model.State().AllCharms()
	checkErr("AllCharms", err)
	logger.Debugf("[%s] checking model", shortUID)
	modelName := model.Name()
	fmt.Fprintf(os.Stdout, "Model: %q\n", modelName)
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	resources := jujuDB.C(resourceCatalogC)
	var totalBytes uint64
	seen := make(map[string]int64)
	for _, charm := range charms {
		bucketPath := path.Join("buckets", modelUUID, charm.StoragePath())
		res := lookupResource(managedResources, resources, bucketPath, charm.String())
		foundHashes[res.SHA384Hash] = struct{}{}
		_, found := seen[res.Id]
		seen[res.Id] = res.Length
		if !found {
			totalBytes += uint64(res.Length)
			size := fmt.Sprint(res.Length)
			if *human {
				size = humanize.Bytes(uint64(res.Length))
			}
			fmt.Fprintf(os.Stdout, "%v: %s %d %s...\n",
				charm.String(), size, res.RefCount, res.Id[:8])
		}
	}
	charmResources, err := model.State().Resources()
	checkErr("resources", err)
	applications, err := model.State().AllApplications()
	checkErr("applications", err)
	for _, app := range applications {
		resources, err := charmResources.ListResources(app.Name())
		if err != nil {
			logger.Warningf("%v: error listing resources for app %q", modelName, app.Name())
		}
		for _, res := range resources.Resources {
			if res.Type == resource.TypeFile {
			}
		}
	}
	size := fmt.Sprintf("%d", totalBytes)
	if *human {
		size = humanize.Bytes(totalBytes)
	}
	fmt.Fprintf(os.Stdout, "total: %s\n\n", size)
}

func checkMissing(st *state.State, session *mgo.Session, foundHashes map[string]struct{}) {
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	resources := jujuDB.C(resourceCatalogC)
	allResources := resources.Find(nil).Iter()
	var res resourceDoc
	missingResources := make(map[string]resourceDoc)
	missingIds := make([]string, 0)
	for allResources.Next(&res) {
		if _, found := foundHashes[res.Id]; found {
			continue
		}
		missingResources[res.SHA384Hash] = res
		missingIds = append(missingIds, res.Id)
	}
	checkErr("missingResources", allResources.Close())
	if len(missingResources) == 0 {
		return
	}
	resourceRefs := make(map[string][]managedResourceDoc, len(missingResources))
	// Note, there isn't an index on resourceid, otherwise we'd just do a reverse lookup
	var manageDoc managedResourceDoc
	managedRefs := managedResources.Find(bson.M{"resourceid": bson.M{"$in": missingIds}}).Iter()
	for managedRefs.Next(&manageDoc) {
		// if _, missing := missingResources[manageDoc.ResourceId]; !missing {
		// 	continue
		// }
		resourceRefs[manageDoc.ResourceId] = append(resourceRefs[manageDoc.ResourceId], manageDoc)
	}
	fmt.Fprint(os.Stderr, "Unknown Resources\n")
	for _, key := range missingIds {
		fmt.Fprintf(os.Stderr, "%v:\n", key)
		for _, doc := range resourceRefs[key] {
			fmt.Fprintf(os.Stdout, "  %v\n", doc.Path)
		}
	}
	fmt.Fprint(os.Stdout, "\n")
}
