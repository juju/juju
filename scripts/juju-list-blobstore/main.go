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
	"github.com/juju/collections/set"
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

	checker := NewBlobStoreChecker()
	defer checker.Close()
	checker.readAgentBinaries()
	modelUUIDs, err := checker.system.AllModelUUIDs()
	checkErr("listing model UUIDs", err)
	logger.Debugf("Found models: %s", modelUUIDs)
	for _, modelUUID := range modelUUIDs {
		mchecker := inspectModel(checker.pool, checker.session, modelUUID, checker.foundHashes, checker.foundBlobPaths)
		checker.modelCheckers = append(checker.modelCheckers, mchecker)
	}
	checker.reportAgentBinaries()
	checker.checkUnreferencedResources()
	checker.checkUnreferencedFiles()
}

// BlobStoreChecker tracks references to the blobstore from multiple locations
type BlobStoreChecker struct {
	pool    *state.StatePool
	session *mgo.Session
	system  *state.State

	foundHashes    set.Strings
	foundBlobPaths set.Strings

	managedResources *mgo.Collection
	resources        *mgo.Collection

	// agentKeys is a sorted way to lookup information in agentReferencedBinaries
	// or agentBinarySizes
	agentKeys binariesInfo
	// agentReferencedBinaries tracks the resource ids and the agent
	// version-series-arch that map to them
	agentReferencedBinaries map[string][]version.Binary
	agentBinarySizes        map[string]uint64

	modelCheckers []*ModelChecker
}

func NewBlobStoreChecker() *BlobStoreChecker {
	statePool, session, err := getState()
	checkErr("getting state connection", err)
	jujuDB := session.DB("juju")
	managedResources := jujuDB.C(managedResourceC)
	resources := jujuDB.C(resourceCatalogC)
	return &BlobStoreChecker{
		pool:    statePool,
		session: session,
		system:  statePool.SystemState(),

		managedResources: managedResources,
		resources:        resources,

		foundHashes:    set.NewStrings(),
		foundBlobPaths: set.NewStrings(),
	}
}

func (b *BlobStoreChecker) Close() {
	b.session.Close()
	b.pool.Close()
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

func (b *BlobStoreChecker) readAgentBinaries() {
	toolsStorage, err := b.system.ToolsStorage()
	checkErr("tools storage", err)
	defer toolsStorage.Close()

	modelUUID := b.system.ModelUUID()
	// managedStore := blobstore.NewManagedStorage(jujuDB, blobstore.NewGridFS("blobstore", "blobstore", session))
	toolsMetadata, err := toolsStorage.AllMetadata()
	checkErr("tools metadata", err)
	logger.Debugf("found %d tools", len(toolsMetadata))
	// map from SHA384 to the list of agent versions that match it
	seen := make(map[string][]version.Binary, 0)
	sizes := make(map[string]uint64)
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
		res := lookupResource(b.managedResources, b.resources, bucketPath, metadata.Version)
		if res.Id == "" {
			continue
		}
		seen[res.SHA384Hash] = append(seen[res.SHA384Hash], binary)
		sizes[res.SHA384Hash] = uint64(res.Length)
		b.foundHashes.Add(res.SHA384Hash)
		b.foundBlobPaths.Add(res.Path)
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
	b.agentKeys = firstKeys
	b.agentBinarySizes = sizes
	b.agentReferencedBinaries = seen
}

type agentReferences struct {
	version string
	models  map[string]modelAgents
}
type modelAgents struct {
	modelName string
	agents    []string
}
type hashToModelAgents map[string][]agentReferences

func (b *BlobStoreChecker) findReferencedAgentVersions() hashToModelAgents {
	versionToModelAgents := make(map[string]agentReferences, 0)
	for _, mchecker := range b.modelCheckers {
		for agentVersion, agents := range mchecker.agentVersions {
			modelName := mchecker.model.Name()
			info, found := versionToModelAgents[agentVersion]
			if !found {
				info.version = agentVersion
				info.models = make(map[string]modelAgents, 0)
			}
			info.models[modelName] = modelAgents{
				modelName: modelName,
				agents:    agents[:],
			}
			versionToModelAgents[agentVersion] = info
		}
	}
	hashToModelAgents := make(hashToModelAgents)
	for hash, binaries := range b.agentReferencedBinaries {
		for _, binary := range binaries {
			s := binary.String()
			info, found := versionToModelAgents[s]
			if found {
				hashToModelAgents[hash] = append(hashToModelAgents[hash], info)
			}
		}
	}
	return hashToModelAgents
}

func (b *BlobStoreChecker) reportAgentBinaries() {
	referencedAgentHashes := b.findReferencedAgentVersions()
	fmt.Fprintf(os.Stdout, "\nAgent Binaries\n")
	var totalAgentBytes uint64
	var referencedAgentBytes uint64
	var unreferencedAgentBytes uint64
	unreferencedHashes := make(binariesInfo, 0)
	for _, info := range b.agentKeys {
		agentReferences, referenced := referencedAgentHashes[info.Hash]
		length, found := b.agentBinarySizes[info.Hash]
		if !found {
			panic(fmt.Sprintf("couldn't find agentBinarySizes for: %#v", info))
		}
		totalAgentBytes += length
		if !referenced {
			unreferencedAgentBytes += length
			unreferencedHashes = append(unreferencedHashes, info)
			continue
		}
		referencedAgentBytes += length
		fmt.Fprintf(os.Stdout, "  %v: %s %s...\n",
			info.Version.Number, lengthToSize(length), info.Hash[:8])
		if *verbose {
			for _, agentRef := range agentReferences {
				fmt.Fprintf(os.Stdout, "    %v:\n", agentRef.version)
				for _, model := range agentRef.models {
					fmt.Fprintf(os.Stdout, "      %v:\n", model.modelName)
					for _, agent := range model.agents {
						fmt.Fprintf(os.Stdout, "        %v\n", agent)
					}
				}
			}
		}
	}
	fmt.Fprintf(os.Stdout, "  referenced agent bytes: %s\n", lengthToSize(referencedAgentBytes))

	if len(unreferencedHashes) > 0 {
		fmt.Fprintf(os.Stdout, "\nUnreferenced Agent Binaries\n")
		for _, info := range unreferencedHashes {
			length := b.agentBinarySizes[info.Hash]
			binaries := b.agentReferencedBinaries[info.Hash]
			// TODO: Use the Model information to determine what agent versions are referenced.
			fmt.Fprintf(os.Stdout, "  %v: %s %d %s...\n",
				info.Version, lengthToSize(length), len(binaries), info.Hash[:8])
			if *verbose {
				for _, binary := range binaries {
					fmt.Fprintf(os.Stdout, "    %v\n", binary.String())
				}
			}
		}
		fmt.Fprintf(os.Stdout, "  unreferenced agent bytes: %s\n", lengthToSize(unreferencedAgentBytes))
	} else {
		fmt.Fprintf(os.Stdout, "\nNo Unreferenced Agent Binaries\n")
	}
	fmt.Fprintf(os.Stdout, "total agent bytes: %s\n", lengthToSize(totalAgentBytes))
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

type ModelChecker struct {
	// foundHashes is the set of resourceIds that we have seen
	foundHashes set.Strings
	// foundBlobPaths is the set of blobstore.Path uuids that we have seen
	foundBlobPaths set.Strings
	session        *mgo.Session
	model          *state.Model
	system         *state.State

	managedResources *mgo.Collection
	resources        *mgo.Collection

	// What Charm URLs are referenced by Applications
	appReferencedCharms mapStringStringSlice
	// What Charm URLs are referenced by Units that aren't referenced by Apps
	unitReferencedCharms mapStringStringSlice

	// agentVersions is the version.Binary.String() mapping to the agents using it
	agentVersions mapStringStringSlice

	// For each resource ID, what CharmURLs point to it
	resourceIdToCharmURLs mapStringStringSlice
	// For each resource ID, what size does it have
	resourceSizes map[string]uint64

	// totalBytes that seem related to this model (whether directly referenced or indirectly)
	totalBytes uint64
	// referencedBytes are bytes that are 'in use'
	referencedBytes uint64
	// unreferencedBytes seem to belong to the Model, but aren't referenced by live objects
	// (eg charm bytes that aren't used by any applications)
	unreferencedBytes uint64

	// referencedCharmBytes are bytes that are 'in use' by applications referencing charms
	referencedCharmBytes uint64
	// unreferencedCharmBytes are charm archives that are recorded in the model, but
	// not referenced by other applications
	unreferencedCharmBytes uint64

	// referencedResourceBytes are bytes that are 'in use' by applications using that
	// specific version of the resource.
	referencedResourceBytes uint64
	// unreferencedResourceBytes are charm resources that are recorded in the model, but
	// not referenced by other applications
	unreferencedResourceBytes uint64

	// unreferencedMiscBytes are bytes that are in the model's buckets, but don't
	// appear to be referenced from anywhere
	unreferencedMiscBytes uint64
}

func NewModelChecker(model *state.Model, session *mgo.Session, foundHashes, foundBlobPaths set.Strings) *ModelChecker {
	jujuDB := session.DB("juju")
	checker := &ModelChecker{
		foundHashes:    foundHashes,
		foundBlobPaths: foundBlobPaths,
		session:        session,
		model:          model,

		managedResources: jujuDB.C(managedResourceC),
		resources:        jujuDB.C(resourceCatalogC),

		agentVersions: make(mapStringStringSlice),

		appReferencedCharms:   make(mapStringStringSlice),
		unitReferencedCharms:  make(mapStringStringSlice),
		resourceIdToCharmURLs: make(mapStringStringSlice),
		resourceSizes:         make(map[string]uint64),
	}
	return checker
}

// readApplicationsAndUnits figures out what CharmURLs are referenced by apps and units
func (checker *ModelChecker) readApplicationsAndUnits() {
	version, err := checker.model.AgentVersion()
	checkErr("model AgentVersion", err)
	// Models track the desired version.Number, but Units track version.Binary
	// because they run a specific Series+Arch
	checker.agentVersions.Add(version.String(), checker.model.Tag().String())
	apps, err := checker.model.State().AllApplications()
	checkErr("AllApplications", err)
	for _, app := range apps {
		charmURL, _ := app.CharmURL()
		appCharmURLStr := charmURL.String()
		checker.appReferencedCharms.Add(appCharmURLStr, app.Name())
		units, err := app.AllUnits()
		checkErr("AllUnits", err)
		for _, unit := range units {
			unitCharmURL, found := unit.CharmURL()
			if !found {
				continue
			}
			unitString := unitCharmURL.String()
			if unitString != appCharmURLStr {
				checker.unitReferencedCharms.Add(unitString, unit.Name())
			}
			tools, err := unit.AgentTools()
			checkErr("unit AgentTools", err)
			checker.agentVersions.Add(tools.Version.String(), unit.Name())
		}
	}
	checker.appReferencedCharms.SortValues()
	checker.unitReferencedCharms.SortValues()
	checker.agentVersions.SortValues()
}

// readModelCharms loads model.AllCharms to determine what charms the model
// itself is tracking.
// This ends up populating resourceIdToCharmURLs
func (checker *ModelChecker) readModelCharms() {
	modelUUID := checker.model.UUID()
	charms, err := checker.model.State().AllCharms()
	checkErr("AllCharms", err)
	for _, charm := range charms {
		charmURL := charm.URL().String()
		bucketPath := path.Join("buckets", modelUUID, charm.StoragePath())
		res := checker.lookupResource(bucketPath, charm.String())
		if res.Id == "" {
			continue
		}
		checker.foundHashes.Add(res.SHA384Hash)
		checker.foundBlobPaths.Add(res.Path)
		checker.resourceIdToCharmURLs.Add(res.Id, charmURL)
		checker.resourceSizes[res.SHA384Hash] = uint64(res.Length)
	}
	checker.resourceIdToCharmURLs.SortValues()
}

func (checker *ModelChecker) lookupResource(bucketPath, description string) resourceDoc {
	return lookupResource(checker.managedResources, checker.resources, bucketPath, description)
}

func (checker *ModelChecker) reportStart() {
	fmt.Fprintf(os.Stdout, "\nModel: %q\n", checker.model.Name())
}

// TODO: this should probably build up a reporting Struct that we then
// write out as YAML, rather than reporting straight to stdout.
func (checker *ModelChecker) reportCharms() {
	// Note, there is a small is a small issue where things can't be easily tracked.
	// namely, if 2 models have the same charm/resource, then which one do we assign the
	// storage to? You have to remove it from both to save any space, but you
	// don't want to double count the storage either.
	notReferenced := make([]string, 0)
	for _, resourceId := range checker.resourceIdToCharmURLs.KeysBySortedValues() {
		length := checker.resourceSizes[resourceId]
		checker.totalBytes += length
		var referenced bool
		for _, charmURL := range checker.resourceIdToCharmURLs[resourceId] {
			if _, found := checker.appReferencedCharms[charmURL]; found {
				referenced = true
				break
			}
		}
		if referenced {
			checker.referencedBytes += length
			checker.referencedCharmBytes += length
			charmURL := checker.resourceIdToCharmURLs[resourceId][0]
			fmt.Fprintf(os.Stdout, "  %v: %s %s...\n",
				charmURL, lengthToSize(length), resourceId[:8])
			if *verbose {
				for _, curl := range checker.resourceIdToCharmURLs[resourceId] {
					if curl != charmURL {
						fmt.Fprintf(os.Stdout, "    %v:\n", curl)
					}
					for _, app := range checker.appReferencedCharms[curl] {
						fmt.Fprintf(os.Stdout, "    - %v\n", app)
					}
				}
			}
		} else {
			notReferenced = append(notReferenced, resourceId)
			checker.unreferencedBytes += length
			checker.unreferencedCharmBytes += length
		}
	}
	if len(notReferenced) > 0 {
		fmt.Fprintf(os.Stdout, "  Not Referenced By Apps\n")
		for _, resourceId := range notReferenced {
			length := checker.resourceSizes[resourceId]
			charmURL := checker.resourceIdToCharmURLs[resourceId][0]
			fmt.Fprintf(os.Stdout, "   %v: %s %s...\n",
				charmURL, lengthToSize(length), resourceId[:8])
			for _, curl := range checker.resourceIdToCharmURLs[resourceId] {
				if curl != charmURL {
					fmt.Fprintf(os.Stdout, "     %v:\n", curl)
				}
				for _, unit := range checker.unitReferencedCharms[curl] {
					fmt.Fprintf(os.Stdout, "     - %v\n", unit)
				}
			}
		}

		fmt.Fprintf(os.Stdout, "  referenced charm bytes: %s\n", lengthToSize(checker.referencedCharmBytes))
		fmt.Fprintf(os.Stdout, "  unreferenced charm bytes: %s\n", lengthToSize(checker.unreferencedCharmBytes))
	}
	fmt.Fprintf(os.Stdout, "  total charm bytes: %s\n", lengthToSize(checker.referencedCharmBytes+checker.unreferencedCharmBytes))
}

func (checker *ModelChecker) reportResources() {
	charmResources, err := checker.model.State().Resources()
	checkErr("resources", err)
	applications, err := checker.model.State().AllApplications()
	checkErr("applications", err)
	for _, app := range applications {
		resources, err := charmResources.ListResources(app.Name())
		if err != nil {
			logger.Warningf("%v: error listing resources for app %q", checker.model.Name(), app.Name())
		}
		for _, res := range resources.Resources {
			if res.Type == resource.TypeFile {
			}
		}
	}
}

func (checker *ModelChecker) reportUnreferencedBuckets() {
	bucketPrefix := fmt.Sprintf("^buckets/%s/.*", checker.model.UUID())
	// Note: it looks like mongo will properly deal with a ^buckets/* on _id search by doing an index lookup
	// on the prefix. Which is good, though it means we read all the resources for this model again.
	managedBuckets := checker.managedResources.Find(bson.M{"_id": bson.M{"$regex": bucketPrefix}}).Iter()
	var managedDoc managedResourceDoc
	resourceIds := make([]string, 0)
	resourceIdToManaged := make(mapStringStringSlice)
	for managedBuckets.Next(&managedDoc) {
		if checker.foundHashes.Contains(managedDoc.ResourceId) {
			continue
		}
		found := resourceIdToManaged.Add(managedDoc.ResourceId, managedDoc.Id)
		if !found {
			resourceIds = append(resourceIds, managedDoc.ResourceId)
		}
	}
	checkErr("bucket search", managedBuckets.Close())
	if len(resourceIds) == 0 {
		return
	}
	var res resourceDoc
	sizes := make(map[string]uint64, len(resourceIds))
	resourceDocs := checker.resources.Find(bson.M{"_id": bson.M{"$in": resourceIds}}).Iter()
	for resourceDocs.Next(&res) {
		sizes[res.SHA384Hash] = uint64(res.Length)
		checker.foundHashes.Add(res.SHA384Hash)
		checker.foundBlobPaths.Add(res.Path)
	}
	checkErr("bucket search", resourceDocs.Close())
	if len(resourceIdToManaged) > 0 {
		fmt.Fprintf(os.Stdout, "  Unreferenced Managed Resources\n")
		resourceIdToManaged.SortValues()
		for _, resourceId := range resourceIdToManaged.KeysBySortedValues() {
			length := sizes[resourceId]
			checker.unreferencedMiscBytes += length
			fmt.Fprintf(os.Stdout, "    %v...: %s\n", resourceId[:8], lengthToSize(length))
			for _, path := range resourceIdToManaged[resourceId] {
				fmt.Fprintf(os.Stdout, "      %v\n", path)
			}
		}
		fmt.Fprintf(os.Stdout, "  total unreferenced misc bytes: %s\n", lengthToSize(checker.unreferencedMiscBytes))
		checker.totalBytes += checker.unreferencedMiscBytes
		checker.unreferencedBytes += checker.unreferencedMiscBytes
	}
}

func (checker *ModelChecker) reportEnd() {
	fmt.Fprintf(os.Stdout, "\n  total unreferenced model bytes: %s\n", lengthToSize(checker.unreferencedBytes))
	fmt.Fprintf(os.Stdout, "  total model bytes: %s\n", lengthToSize(checker.totalBytes))
}

func inspectModel(pool *state.StatePool, session *mgo.Session, modelUUID string, foundHashes, foundBlobPaths set.Strings) *ModelChecker {
	model, helper, err := pool.GetModel(modelUUID)
	defer helper.Release()
	checkErr("lookup model", err)
	checker := NewModelChecker(model, session, foundHashes, foundBlobPaths)
	checker.readApplicationsAndUnits()
	checker.readModelCharms()
	checker.reportStart()
	checker.reportCharms()
	checker.reportResources()
	checker.reportUnreferencedBuckets()
	checker.reportEnd()
	return checker
}

// TODO: read all buckets looking for ones that reference modelUUIDs that we don't know about.
// func lookForBucketsMissingModels(session *mgo.Session, modelUUIDs []string, foundHashes map[string]struct{}) {
// 	jujuDB := session.DB("juju")
// 	managedResources := jujuDB.C(managedResourceC)
// 	resources := jujuDB.C(resourceCatalogC)
// }
//

type blobstoreChunk struct {
	ID      bson.ObjectId `bson:"_id"`
	FilesID bson.ObjectId `bson:"files_id"`
	N       int           `bson:"n"`
	// we intentionally omit Data
}

var blobstoreChunkFieldSelector = bson.M{"_id": 1, "files_id": 1, "n": 1}

type blobstoreFile struct {
	ID       bson.ObjectId `bson:"_id"`
	Filename string        `bson:"filename"`
	Length   int64         `bson:"length"`
}

var blobstoreFileFieldSelector = bson.M{"_id": 1, "filename": 1, "length": 1}

// TODO: read all blobstore files looking for paths that we haven't seen yet
func (b *BlobStoreChecker) checkUnreferencedFiles() {
	blobstoreDB := b.session.DB("blobstore")
	blobFiles := blobstoreDB.C("blobstore.files")
	fileIter := blobFiles.Find(nil).Select(blobstoreFileFieldSelector).Iter()
	var doc blobstoreFile
	wroteHeader := false
	var unreferencedBytes uint64
	for fileIter.Next(&doc) {
		if b.foundBlobPaths.Contains(doc.Filename) {
			continue
		}
		b.foundBlobPaths.Add(doc.Filename)
		if !wroteHeader {
			wroteHeader = true
			fmt.Fprint(os.Stdout, "\nUnknown Blobstore Files\n")
		}
		length := uint64(doc.Length)
		fmt.Fprintf(os.Stdout, "  %v: %s\n", doc.Filename, lengthToSize(length))
		unreferencedBytes += length
	}
	if wroteHeader {
		fmt.Fprint(os.Stdout, "\n  total unreferenced blobstore file bytes: %s\n", lengthToSize(unreferencedBytes))
	} else {
		fmt.Fprint(os.Stdout, "\nNo Unknown Blobstore Files\n")
	}

	checkErr("iterating blob files", fileIter.Close())
}

// TODO: read all blobstore chunks looking for chunks that don't reference a file

func (b *BlobStoreChecker) checkUnreferencedResources() {
	allResources := b.resources.Find(nil).Iter()
	var res resourceDoc
	missingResources := make(map[string]resourceDoc)
	missingIds := make([]string, 0)
	var totalBytes uint64
	for allResources.Next(&res) {
		if b.foundHashes.Contains(res.Id) {
			continue
		}
		b.foundHashes.Add(res.Id)
		b.foundBlobPaths.Add(res.Path)
		missingResources[res.SHA384Hash] = res
		missingIds = append(missingIds, res.Id)
		totalBytes += uint64(res.Length)
	}
	checkErr("missingResources", allResources.Close())
	if len(missingResources) == 0 {
		fmt.Fprint(os.Stdout, "\nNo Unknown Resources\n")
		return
	}
	resourceRefs := make(map[string][]managedResourceDoc, len(missingResources))
	// Note, there isn't an index on resourceid, otherwise we'd just do a reverse lookup
	var manageDoc managedResourceDoc
	managedRefs := b.managedResources.Find(bson.M{"resourceid": bson.M{"$in": missingIds}}).Iter()
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
