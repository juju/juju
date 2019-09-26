// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/dustin/go-humanize"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

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
	checkUnreferencedResources(session, foundHashes)
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
		if res.Id == "" {
			continue
		}
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
	fmt.Fprintf(os.Stdout, "total: %s\n", size)
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

type mapStringStringSlice map[string][]string

func (m mapStringStringSlice) Add(key, value string) bool {
	cur, found := m[key]
	m[key] = append(cur, value)
	return found
}

func (m mapStringStringSlice) SortValues() {
	for key, values := range m {
		sort.Strings(values)
		m[key] = values
	}
}

func (m mapStringStringSlice) SortedKeys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// KeysBySortedValues returns keys sorted by the order of the values.
// So if you have {a: [1], b: [0]} this will return [b, a]
// This assume you've already called m.SortValues() to put all the
// values in sorted order.
func (m mapStringStringSlice) KeysBySortedValues() []string {
	firstValues := make([]string, 0, len(m))
	valToKey := make(map[string]string)
	for key, values := range m {
		if len(values) == 0 {
			// shouldn't ever happen
			continue
		}
		v := values[0]
		firstValues = append(firstValues, v)
		valToKey[v] = key
	}
	sort.Strings(firstValues)
	// We can reuse firstValues because it is just []string
	for i, v := range firstValues {
		k := valToKey[v]
		firstValues[i] = k
	}
	return firstValues
}

func lengthToSize(length uint64) string {
	if *human {
		return humanize.Bytes(length)
	}
	return fmt.Sprintf("%d", length)
}

// TODO: Refactor this into a small type
func inspectModel(pool *state.StatePool, session *mgo.Session, modelUUID string, foundHashes map[string]struct{}) {
	model, helper, err := pool.GetModel(modelUUID)
	defer helper.Release()
	shortUID := shortModelUUID(modelUUID)
	checkErr(fmt.Sprintf("reading model %s", shortUID), err)
	apps, err := model.State().AllApplications()
	checkErr("AllApplications", err)
	// What Charm URLs are referenced by Applications
	appReferencedCharms := make(mapStringStringSlice)
	// What Charm URLs are referenced by Units that aren't referenced by Apps
	unitReferencedCharms := make(mapStringStringSlice)
	for _, app := range apps {
		charmURL, _ := app.CharmURL()
		appCharmURLStr := charmURL.String()
		appReferencedCharms.Add(appCharmURLStr, app.Name())
		units, err := app.AllUnits()
		checkErr("AllUnits", err)
		for _, unit := range units {
			unitCharmURL, found := unit.CharmURL()
			if !found {
				continue
			}
			unitString := unitCharmURL.String()
			if unitString != appCharmURLStr {
				unitReferencedCharms.Add(unitString, unit.Name())
			}
		}
	}
	appReferencedCharms.SortValues()
	unitReferencedCharms.SortValues()

	charms, err := model.State().AllCharms()
	checkErr("AllCharms", err)
	logger.Debugf("[%s] checking model", shortUID)
	modelName := model.Name()
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	resources := jujuDB.C(resourceCatalogC)
	modelReferencedCharms := make(map[string]bool)
	resourceToCharmURLs := make(mapStringStringSlice)
	sizes := make(map[string]uint64)
	for _, charm := range charms {
		charmURL := charm.URL().String()
		modelReferencedCharms[charmURL] = true
		bucketPath := path.Join("buckets", modelUUID, charm.StoragePath())
		res := lookupResource(managedResources, resources, bucketPath, charm.String())
		if res.Id == "" {
			continue
		}
		foundHashes[res.SHA384Hash] = struct{}{}
		resourceToCharmURLs.Add(res.Id, charmURL)
		sizes[res.SHA384Hash] = uint64(res.Length)
	}
	resourceToCharmURLs.SortValues()
	// Note, there is a small is a small issue where things can't be easily tracked.
	// namely, if 2 models have the same charm, then which one do we assign the
	// storage to? You have to remove it from both to save any space, but you
	// don't want to double count the storage either.
	fmt.Fprintf(os.Stdout, "\nModel: %q\n", modelName)
	var totalBytes uint64
	var referencedBytes uint64
	var unreferencedBytes uint64
	notReferenced := make([]string, 0)
	for _, resourceId := range resourceToCharmURLs.KeysBySortedValues() {
		length := sizes[resourceId]
		totalBytes += length
		var referenced bool
		for _, charmURL := range resourceToCharmURLs[resourceId] {
			if _, found := appReferencedCharms[charmURL]; found {
				referenced = true
				break
			}
		}
		if referenced {
			referencedBytes += length
			charmURL := resourceToCharmURLs[resourceId][0]
			fmt.Fprintf(os.Stdout, "  %v: %s %s...\n",
				charmURL, lengthToSize(length), resourceId[:8])
			if *verbose {
				for _, curl := range resourceToCharmURLs[resourceId] {
					if curl != charmURL {
						fmt.Fprintf(os.Stdout, "    %v:\n", curl)
					}
					for _, app := range appReferencedCharms[curl] {
						fmt.Fprintf(os.Stdout, "    - %v\n", app)
					}
				}
			}
		} else {
			notReferenced = append(notReferenced, resourceId)
			unreferencedBytes += uint64(length)
		}
	}
	if len(notReferenced) > 0 {
		fmt.Fprintf(os.Stdout, "  Not Referenced By Apps\n")
		for _, resourceId := range notReferenced {
			length := sizes[resourceId]
			charmURL := resourceToCharmURLs[resourceId][0]
			fmt.Fprintf(os.Stdout, "  %v: %s %s...\n",
				charmURL, lengthToSize(length), resourceId[:8])
			for _, curl := range resourceToCharmURLs[resourceId] {
				if curl != charmURL {
					fmt.Fprintf(os.Stdout, "    %v:\n", curl)
				}
				for _, unit := range unitReferencedCharms[curl] {
					fmt.Fprintf(os.Stdout, "    - %v\n", unit)
				}
			}
		}

		fmt.Fprintf(os.Stdout, "  referenced charm bytes: %s\n", lengthToSize(referencedBytes))
		fmt.Fprintf(os.Stdout, "  unreferenced charm bytes: %s\n", lengthToSize(unreferencedBytes))
	}
	fmt.Fprintf(os.Stdout, "  total charm bytes: %s\n", lengthToSize(totalBytes))
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
	unknownResourceIdToManaged, sizes := lookForExtraBuckets(managedResources, resources, modelUUID, foundHashes)
	if unknownResourceIdToManaged != nil {
		var unrefResourceBytes uint64
		fmt.Fprintf(os.Stdout, "  Unreferenced Managed Resources\n")
		unknownResourceIdToManaged.SortValues()
		for _, resourceId := range unknownResourceIdToManaged.KeysBySortedValues() {
			length := sizes[resourceId]
			unrefResourceBytes += length
			fmt.Fprintf(os.Stdout, "    %v: %s\n", resourceId, lengthToSize(length))
			for _, path := range unknownResourceIdToManaged[resourceId] {
				fmt.Fprintf(os.Stdout, "      %v\n", path)
			}
		}
		fmt.Fprintf(os.Stdout, "  total unreferenced bytes: %s\n", lengthToSize(unrefResourceBytes))
		totalBytes += unrefResourceBytes
	}
	fmt.Fprintf(os.Stdout, "  total model bytes: %s\n", lengthToSize(totalBytes))
}

func lookForExtraBuckets(managedResources, resources *mgo.Collection, modelUUID string, foundHashes map[string]struct{}) (mapStringStringSlice, map[string]uint64) {
	bucketPrefix := fmt.Sprintf("^buckets/%s/.*", modelUUID)
	// Note: it looks like mongo will properly deal with a ^buckets/* on _id search by doing an index lookup
	// on the prefix. Which is good, though it means we read all the resources for this model again.
	managedBuckets := managedResources.Find(bson.M{"_id": bson.M{"$regex": bucketPrefix}}).Iter()
	var managedDoc managedResourceDoc
	resourceIds := make([]string, 0)
	resourceIdToManaged := make(mapStringStringSlice)
	for managedBuckets.Next(&managedDoc) {
		_, found := foundHashes[managedDoc.ResourceId]
		if found {
			continue
		}
		found = resourceIdToManaged.Add(managedDoc.ResourceId, managedDoc.Id)
		if !found {
			resourceIds = append(resourceIds, managedDoc.ResourceId)
		}
	}
	checkErr("bucket search", managedBuckets.Close())
	if len(resourceIds) == 0 {
		return nil, nil
	}
	var res resourceDoc
	sizes := make(map[string]uint64, len(resourceIds))
	resourceDocs := resources.Find(bson.M{"_id": bson.M{"$in": resourceIds}}).Iter()
	for resourceDocs.Next(&res) {
		sizes[res.SHA384Hash] = uint64(res.Length)
		foundHashes[res.SHA384Hash] = struct{}{}
	}
	checkErr("bucket search", resourceDocs.Close())
	return resourceIdToManaged, sizes
}

// func lookForBucketsMissingModels(session *mgo.Session, modelUUIDs []string, foundHashes map[string]struct{}) {
// 	jujuDB := session.DB("juju")
// 	managedResources := jujuDB.C(managedResourceC)
// 	resources := jujuDB.C(resourceCatalogC)
// }
//
func checkUnreferencedResources(session *mgo.Session, foundHashes map[string]struct{}) {
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	resources := jujuDB.C(resourceCatalogC)
	allResources := resources.Find(nil).Iter()
	var res resourceDoc
	missingResources := make(map[string]resourceDoc)
	missingIds := make([]string, 0)
	var totalBytes uint64
	for allResources.Next(&res) {
		if _, found := foundHashes[res.Id]; found {
			continue
		}
		missingResources[res.SHA384Hash] = res
		missingIds = append(missingIds, res.Id)
		totalBytes += uint64(res.Length)
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
	fmt.Fprint(os.Stdout, "Unknown Resources\n")
	for _, key := range missingIds {
		res := missingResources[key]
		size := fmt.Sprintf("%d", res.Length)
		if *human {
			size = humanize.Bytes(uint64(res.Length))
		}
		fmt.Fprintf(os.Stdout, "%v: %s\n", key, size)
		for _, doc := range resourceRefs[key] {
			fmt.Fprintf(os.Stdout, "  %v\n", doc.Path)
		}
	}
	size := fmt.Sprintf("%d", totalBytes)
	if *human {
		size = humanize.Bytes(uint64(totalBytes))
	}
	fmt.Fprintf(os.Stdout, "total: %s\n\n", size)
}
