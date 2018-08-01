// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync/atomic"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	"github.com/juju/version"
)

var (
	logger = loggo.GetLogger("maas")

	// The supported versions should be ordered from most desirable version to
	// least as they will be tried in order.
	supportedAPIVersions = []string{"2.0"}

	// Each of the api versions that change the request or response structure
	// for any given call should have a value defined for easy definition of
	// the deserialization functions.
	twoDotOh = version.Number{Major: 2, Minor: 0}

	// Current request number. Informational only for logging.
	requestNumber int64
)

// ControllerArgs is an argument struct for passing the required parameters
// to the NewController method.
type ControllerArgs struct {
	BaseURL string
	APIKey  string
}

// NewController creates an authenticated client to the MAAS API, and
// checks the capabilities of the server. If the BaseURL specified
// includes the API version, that version of the API will be used,
// otherwise the controller will use the highest supported version
// available.
//
// If the APIKey is not valid, a NotValid error is returned.
// If the credentials are incorrect, a PermissionError is returned.
func NewController(args ControllerArgs) (Controller, error) {
	base, apiVersion, includesVersion := SplitVersionedURL(args.BaseURL)
	if includesVersion {
		if !supportedVersion(apiVersion) {
			return nil, NewUnsupportedVersionError("version %s", apiVersion)
		}
		return newControllerWithVersion(base, apiVersion, args.APIKey)
	}
	return newControllerUnknownVersion(args)
}

func supportedVersion(value string) bool {
	for _, version := range supportedAPIVersions {
		if value == version {
			return true
		}
	}
	return false
}

func newControllerWithVersion(baseURL, apiVersion, apiKey string) (Controller, error) {
	major, minor, err := version.ParseMajorMinor(apiVersion)
	// We should not get an error here. See the test.
	if err != nil {
		return nil, errors.Errorf("bad version defined in supported versions: %q", apiVersion)
	}
	client, err := NewAuthenticatedClient(AddAPIVersionToURL(baseURL, apiVersion), apiKey)
	if err != nil {
		// If the credentials aren't valid, return now.
		if errors.IsNotValid(err) {
			return nil, errors.Trace(err)
		}
		// Any other error attempting to create the authenticated client
		// is an unexpected error and return now.
		return nil, NewUnexpectedError(err)
	}
	controllerVersion := version.Number{
		Major: major,
		Minor: minor,
	}
	controller := &controller{client: client, apiVersion: controllerVersion}
	controller.capabilities, err = controller.readAPIVersionInfo()
	if err != nil {
		logger.Debugf("read version failed: %#v", err)
		return nil, errors.Trace(err)
	}

	if err := controller.checkCreds(); err != nil {
		return nil, errors.Trace(err)
	}
	return controller, nil
}

func newControllerUnknownVersion(args ControllerArgs) (Controller, error) {
	// For now we don't need to test multiple versions. It is expected that at
	// some time in the future, we will try the most up to date version and then
	// work our way backwards.
	for _, apiVersion := range supportedAPIVersions {
		controller, err := newControllerWithVersion(args.BaseURL, apiVersion, args.APIKey)
		switch {
		case err == nil:
			return controller, nil
		case IsUnsupportedVersionError(err):
			// This will only come back from readAPIVersionInfo for 410/404.
			continue
		default:
			return nil, errors.Trace(err)
		}
	}

	return nil, NewUnsupportedVersionError("controller at %s does not support any of %s", args.BaseURL, supportedAPIVersions)
}

type controller struct {
	client       *Client
	apiVersion   version.Number
	capabilities set.Strings
}

// Capabilities implements Controller.
func (c *controller) Capabilities() set.Strings {
	return c.capabilities
}

// BootResources implements Controller.
func (c *controller) BootResources() ([]BootResource, error) {
	source, err := c.get("boot-resources")
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	resources, err := readBootResources(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []BootResource
	for _, r := range resources {
		result = append(result, r)
	}
	return result, nil
}

// Fabrics implements Controller.
func (c *controller) Fabrics() ([]Fabric, error) {
	source, err := c.get("fabrics")
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	fabrics, err := readFabrics(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []Fabric
	for _, f := range fabrics {
		result = append(result, f)
	}
	return result, nil
}

// Spaces implements Controller.
func (c *controller) Spaces() ([]Space, error) {
	source, err := c.get("spaces")
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	spaces, err := readSpaces(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []Space
	for _, space := range spaces {
		result = append(result, space)
	}
	return result, nil
}

// StaticRoutes implements Controller.
func (c *controller) StaticRoutes() ([]StaticRoute, error) {
	source, err := c.get("static-routes")
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	staticRoutes, err := readStaticRoutes(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []StaticRoute
	for _, staticRoute := range staticRoutes {
		result = append(result, staticRoute)
	}
	return result, nil
}

// Zones implements Controller.
func (c *controller) Zones() ([]Zone, error) {
	source, err := c.get("zones")
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	zones, err := readZones(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []Zone
	for _, z := range zones {
		result = append(result, z)
	}
	return result, nil
}

// Domains implements Controller
func (c *controller) Domains() ([]Domain, error) {
	source, err := c.get("domains")
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	domains, err := readDomains(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []Domain
	for _, domain := range domains {
		result = append(result, domain)
	}
	return result, nil
}

// DevicesArgs is a argument struct for selecting Devices.
// Only devices that match the specified criteria are returned.
type DevicesArgs struct {
	Hostname     []string
	MACAddresses []string
	SystemIDs    []string
	Domain       string
	Zone         string
	AgentName    string
}

// Devices implements Controller.
func (c *controller) Devices(args DevicesArgs) ([]Device, error) {
	params := NewURLParams()
	params.MaybeAddMany("hostname", args.Hostname)
	params.MaybeAddMany("mac_address", args.MACAddresses)
	params.MaybeAddMany("id", args.SystemIDs)
	params.MaybeAdd("domain", args.Domain)
	params.MaybeAdd("zone", args.Zone)
	params.MaybeAdd("agent_name", args.AgentName)
	source, err := c.getQuery("devices", params.Values)
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	devices, err := readDevices(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []Device
	for _, d := range devices {
		d.controller = c
		result = append(result, d)
	}
	return result, nil
}

// CreateDeviceArgs is a argument struct for passing information into CreateDevice.
type CreateDeviceArgs struct {
	Hostname     string
	MACAddresses []string
	Domain       string
	Parent       string
}

// Devices implements Controller.
func (c *controller) CreateDevice(args CreateDeviceArgs) (Device, error) {
	// There must be at least one mac address.
	if len(args.MACAddresses) == 0 {
		return nil, NewBadRequestError("at least one MAC address must be specified")
	}
	params := NewURLParams()
	params.MaybeAdd("hostname", args.Hostname)
	params.MaybeAdd("domain", args.Domain)
	params.MaybeAddMany("mac_addresses", args.MACAddresses)
	params.MaybeAdd("parent", args.Parent)
	result, err := c.post("devices", "", params.Values)
	if err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			if svrErr.StatusCode == http.StatusBadRequest {
				return nil, errors.Wrap(err, NewBadRequestError(svrErr.BodyMessage))
			}
		}
		// Translate http errors.
		return nil, NewUnexpectedError(err)
	}

	device, err := readDevice(c.apiVersion, result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	device.controller = c
	return device, nil
}

// MachinesArgs is a argument struct for selecting Machines.
// Only machines that match the specified criteria are returned.
type MachinesArgs struct {
	Hostnames    []string
	MACAddresses []string
	SystemIDs    []string
	Domain       string
	Zone         string
	AgentName    string
	OwnerData    map[string]string
}

// Machines implements Controller.
func (c *controller) Machines(args MachinesArgs) ([]Machine, error) {
	params := NewURLParams()
	params.MaybeAddMany("hostname", args.Hostnames)
	params.MaybeAddMany("mac_address", args.MACAddresses)
	params.MaybeAddMany("id", args.SystemIDs)
	params.MaybeAdd("domain", args.Domain)
	params.MaybeAdd("zone", args.Zone)
	params.MaybeAdd("agent_name", args.AgentName)
	// At the moment the MAAS API doesn't support filtering by owner
	// data so we do that ourselves below.
	source, err := c.getQuery("machines", params.Values)
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	machines, err := readMachines(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []Machine
	for _, m := range machines {
		m.controller = c
		if ownerDataMatches(m.ownerData, args.OwnerData) {
			result = append(result, m)
		}
	}
	return result, nil
}

func ownerDataMatches(ownerData, filter map[string]string) bool {
	for key, value := range filter {
		if ownerData[key] != value {
			return false
		}
	}
	return true
}

// StorageSpec represents one element of storage constraints necessary
// to be satisfied to allocate a machine.
type StorageSpec struct {
	// Label is optional and an arbitrary string. Labels need to be unique
	// across the StorageSpec elements specified in the AllocateMachineArgs.
	Label string
	// Size is required and refers to the required minimum size in GB.
	Size int
	// Zero or more tags assocated to with the disks.
	Tags []string
}

// Validate ensures that there is a positive size and that there are no Empty
// tag values.
func (s *StorageSpec) Validate() error {
	if s.Size <= 0 {
		return errors.NotValidf("Size value %d", s.Size)
	}
	for _, v := range s.Tags {
		if v == "" {
			return errors.NotValidf("empty tag")
		}
	}
	return nil
}

// String returns the string representation of the storage spec.
func (s *StorageSpec) String() string {
	label := s.Label
	if label != "" {
		label += ":"
	}
	tags := strings.Join(s.Tags, ",")
	if tags != "" {
		tags = "(" + tags + ")"
	}
	return fmt.Sprintf("%s%d%s", label, s.Size, tags)
}

// InterfaceSpec represents one elemenet of network related constraints.
type InterfaceSpec struct {
	// Label is required and an arbitrary string. Labels need to be unique
	// across the InterfaceSpec elements specified in the AllocateMachineArgs.
	// The label is returned in the ConstraintMatches response from
	// AllocateMachine.
	Label string
	Space string

	// NOTE: there are other interface spec values that we are not exposing at
	// this stage that can be added on an as needed basis. Other possible values are:
	//     'fabric_class', 'not_fabric_class',
	//     'subnet_cidr', 'not_subnet_cidr',
	//     'vid', 'not_vid',
	//     'fabric', 'not_fabric',
	//     'subnet', 'not_subnet',
	//     'mode'
}

// Validate ensures that a Label is specified and that there is at least one
// Space or NotSpace value set.
func (a *InterfaceSpec) Validate() error {
	if a.Label == "" {
		return errors.NotValidf("missing Label")
	}
	// Perhaps at some stage in the future there will be other possible specs
	// supported (like vid, subnet, etc), but until then, just space to check.
	if a.Space == "" {
		return errors.NotValidf("empty Space constraint")
	}
	return nil
}

// String returns the interface spec as MaaS requires it.
func (a *InterfaceSpec) String() string {
	return fmt.Sprintf("%s:space=%s", a.Label, a.Space)
}

// AllocateMachineArgs is an argument struct for passing args into Machine.Allocate.
type AllocateMachineArgs struct {
	Hostname     string
	SystemId     string
	Architecture string
	MinCPUCount  int
	// MinMemory represented in MB.
	MinMemory int
	Tags      []string
	NotTags   []string
	Zone      string
	NotInZone []string
	// Storage represents the required disks on the Machine. If any are specified
	// the first value is used for the root disk.
	Storage []StorageSpec
	// Interfaces represents a number of required interfaces on the machine.
	// Each InterfaceSpec relates to an individual network interface.
	Interfaces []InterfaceSpec
	// NotSpace is a machine level constraint, and applies to the entire machine
	// rather than specific interfaces.
	NotSpace  []string
	AgentName string
	Comment   string
	DryRun    bool
}

// Validate makes sure that any labels specifed in Storage or Interfaces
// are unique, and that the required specifications are valid.
func (a *AllocateMachineArgs) Validate() error {
	storageLabels := set.NewStrings()
	for _, spec := range a.Storage {
		if err := spec.Validate(); err != nil {
			return errors.Annotate(err, "Storage")
		}
		if spec.Label != "" {
			if storageLabels.Contains(spec.Label) {
				return errors.NotValidf("reusing storage label %q", spec.Label)
			}
			storageLabels.Add(spec.Label)
		}
	}
	interfaceLabels := set.NewStrings()
	for _, spec := range a.Interfaces {
		if err := spec.Validate(); err != nil {
			return errors.Annotate(err, "Interfaces")
		}
		if interfaceLabels.Contains(spec.Label) {
			return errors.NotValidf("reusing interface label %q", spec.Label)
		}
		interfaceLabels.Add(spec.Label)
	}
	for _, v := range a.NotSpace {
		if v == "" {
			return errors.NotValidf("empty NotSpace constraint")
		}
	}
	return nil
}

func (a *AllocateMachineArgs) storage() string {
	var values []string
	for _, spec := range a.Storage {
		values = append(values, spec.String())
	}
	return strings.Join(values, ",")
}

func (a *AllocateMachineArgs) interfaces() string {
	var values []string
	for _, spec := range a.Interfaces {
		values = append(values, spec.String())
	}
	return strings.Join(values, ";")
}

func (a *AllocateMachineArgs) notSubnets() []string {
	var values []string
	for _, v := range a.NotSpace {
		values = append(values, "space:"+v)
	}
	return values
}

// ConstraintMatches provides a way for the caller of AllocateMachine to determine
//.how the allocated machine matched the storage and interfaces constraints specified.
// The labels that were used in the constraints are the keys in the maps.
type ConstraintMatches struct {
	// Interface is a mapping of the constraint label specified to the Interfaces
	// that match that constraint.
	Interfaces map[string][]Interface

	// Storage is a mapping of the constraint label specified to the BlockDevices
	// that match that constraint.
	Storage map[string][]BlockDevice
}

// AllocateMachine implements Controller.
//
// Returns an error that satisfies IsNoMatchError if the requested
// constraints cannot be met.
func (c *controller) AllocateMachine(args AllocateMachineArgs) (Machine, ConstraintMatches, error) {
	var matches ConstraintMatches
	params := NewURLParams()
	params.MaybeAdd("name", args.Hostname)
	params.MaybeAdd("system_id", args.SystemId)
	params.MaybeAdd("arch", args.Architecture)
	params.MaybeAddInt("cpu_count", args.MinCPUCount)
	params.MaybeAddInt("mem", args.MinMemory)
	params.MaybeAddMany("tags", args.Tags)
	params.MaybeAddMany("not_tags", args.NotTags)
	params.MaybeAdd("storage", args.storage())
	params.MaybeAdd("interfaces", args.interfaces())
	params.MaybeAddMany("not_subnets", args.notSubnets())
	params.MaybeAdd("zone", args.Zone)
	params.MaybeAddMany("not_in_zone", args.NotInZone)
	params.MaybeAdd("agent_name", args.AgentName)
	params.MaybeAdd("comment", args.Comment)
	params.MaybeAddBool("dry_run", args.DryRun)
	result, err := c.post("machines", "allocate", params.Values)
	if err != nil {
		// A 409 Status code is "No Matching Machines"
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			if svrErr.StatusCode == http.StatusConflict {
				return nil, matches, errors.Wrap(err, NewNoMatchError(svrErr.BodyMessage))
			}
		}
		// Translate http errors.
		return nil, matches, NewUnexpectedError(err)
	}

	machine, err := readMachine(c.apiVersion, result)
	if err != nil {
		return nil, matches, errors.Trace(err)
	}
	machine.controller = c

	// Parse the constraint matches.
	matches, err = parseAllocateConstraintsResponse(result, machine)
	if err != nil {
		return nil, matches, errors.Trace(err)
	}

	return machine, matches, nil
}

// ReleaseMachinesArgs is an argument struct for passing the machine system IDs
// and an optional comment into the ReleaseMachines method.
type ReleaseMachinesArgs struct {
	SystemIDs []string
	Comment   string
}

// ReleaseMachines implements Controller.
//
// Release multiple machines at once. Returns
//  - BadRequestError if any of the machines cannot be found
//  - PermissionError if the user does not have permission to release any of the machines
//  - CannotCompleteError if any of the machines could not be released due to their current state
func (c *controller) ReleaseMachines(args ReleaseMachinesArgs) error {
	params := NewURLParams()
	params.MaybeAddMany("machines", args.SystemIDs)
	params.MaybeAdd("comment", args.Comment)
	_, err := c.post("machines", "release", params.Values)
	if err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			switch svrErr.StatusCode {
			case http.StatusBadRequest:
				return errors.Wrap(err, NewBadRequestError(svrErr.BodyMessage))
			case http.StatusForbidden:
				return errors.Wrap(err, NewPermissionError(svrErr.BodyMessage))
			case http.StatusConflict:
				return errors.Wrap(err, NewCannotCompleteError(svrErr.BodyMessage))
			}
		}
		return NewUnexpectedError(err)
	}

	return nil
}

// Files implements Controller.
func (c *controller) Files(prefix string) ([]File, error) {
	params := NewURLParams()
	params.MaybeAdd("prefix", prefix)
	source, err := c.getQuery("files", params.Values)
	if err != nil {
		return nil, NewUnexpectedError(err)
	}
	files, err := readFiles(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []File
	for _, f := range files {
		f.controller = c
		result = append(result, f)
	}
	return result, nil
}

// GetFile implements Controller.
func (c *controller) GetFile(filename string) (File, error) {
	if filename == "" {
		return nil, errors.NotValidf("missing filename")
	}
	source, err := c.get("files/" + filename)
	if err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			if svrErr.StatusCode == http.StatusNotFound {
				return nil, errors.Wrap(err, NewNoMatchError(svrErr.BodyMessage))
			}
		}
		return nil, NewUnexpectedError(err)
	}
	file, err := readFile(c.apiVersion, source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	file.controller = c
	return file, nil
}

// AddFileArgs is a argument struct for passing information into AddFile.
// One of Content or (Reader, Length) must be specified.
type AddFileArgs struct {
	Filename string
	Content  []byte
	Reader   io.Reader
	Length   int64
}

// Validate checks to make sure the filename has no slashes, and that one of
// Content or (Reader, Length) is specified.
func (a *AddFileArgs) Validate() error {
	dir, _ := path.Split(a.Filename)
	if dir != "" {
		return errors.NotValidf("paths in Filename %q", a.Filename)
	}
	if a.Filename == "" {
		return errors.NotValidf("missing Filename")
	}
	if a.Content == nil {
		if a.Reader == nil {
			return errors.NotValidf("missing Content or Reader")
		}
		if a.Length == 0 {
			return errors.NotValidf("missing Length")
		}
	} else {
		if a.Reader != nil {
			return errors.NotValidf("specifying Content and Reader")
		}
		if a.Length != 0 {
			return errors.NotValidf("specifying Length and Content")
		}
	}
	return nil
}

// AddFile implements Controller.
func (c *controller) AddFile(args AddFileArgs) error {
	if err := args.Validate(); err != nil {
		return errors.Trace(err)
	}
	fileContent := args.Content
	if fileContent == nil {
		content, err := ioutil.ReadAll(io.LimitReader(args.Reader, args.Length))
		if err != nil {
			return errors.Annotatef(err, "cannot read file content")
		}
		fileContent = content
	}
	params := url.Values{"filename": {args.Filename}}
	_, err := c.postFile("files", "", params, fileContent)
	if err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			if svrErr.StatusCode == http.StatusBadRequest {
				return errors.Wrap(err, NewBadRequestError(svrErr.BodyMessage))
			}
		}
		return NewUnexpectedError(err)
	}
	return nil
}

func (c *controller) checkCreds() error {
	if _, err := c.getOp("users", "whoami"); err != nil {
		if svrErr, ok := errors.Cause(err).(ServerError); ok {
			if svrErr.StatusCode == http.StatusUnauthorized {
				return errors.Wrap(err, NewPermissionError(svrErr.BodyMessage))
			}
		}
		return NewUnexpectedError(err)
	}
	return nil
}

func (c *controller) put(path string, params url.Values) (interface{}, error) {
	path = EnsureTrailingSlash(path)
	requestID := nextRequestID()
	logger.Tracef("request %x: PUT %s%s, params: %s", requestID, c.client.APIURL, path, params.Encode())
	bytes, err := c.client.Put(&url.URL{Path: path}, params)
	if err != nil {
		logger.Tracef("response %x: error: %q", requestID, err.Error())
		logger.Tracef("error detail: %#v", err)
		return nil, errors.Trace(err)
	}
	logger.Tracef("response %x: %s", requestID, string(bytes))

	var parsed interface{}
	err = json.Unmarshal(bytes, &parsed)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return parsed, nil
}

func (c *controller) post(path, op string, params url.Values) (interface{}, error) {
	bytes, err := c._postRaw(path, op, params, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var parsed interface{}
	err = json.Unmarshal(bytes, &parsed)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return parsed, nil
}

func (c *controller) postFile(path, op string, params url.Values, fileContent []byte) (interface{}, error) {
	// Only one file is ever sent at a time.
	files := map[string][]byte{"file": fileContent}
	return c._postRaw(path, op, params, files)
}

func (c *controller) _postRaw(path, op string, params url.Values, files map[string][]byte) ([]byte, error) {
	path = EnsureTrailingSlash(path)
	requestID := nextRequestID()
	if logger.IsTraceEnabled() {
		opArg := ""
		if op != "" {
			opArg = "?op=" + op
		}
		logger.Tracef("request %x: POST %s%s%s, params=%s", requestID, c.client.APIURL, path, opArg, params.Encode())
	}
	bytes, err := c.client.Post(&url.URL{Path: path}, op, params, files)
	if err != nil {
		logger.Tracef("response %x: error: %q", requestID, err.Error())
		logger.Tracef("error detail: %#v", err)
		return nil, errors.Trace(err)
	}
	logger.Tracef("response %x: %s", requestID, string(bytes))
	return bytes, nil
}

func (c *controller) delete(path string) error {
	path = EnsureTrailingSlash(path)
	requestID := nextRequestID()
	logger.Tracef("request %x: DELETE %s%s", requestID, c.client.APIURL, path)
	err := c.client.Delete(&url.URL{Path: path})
	if err != nil {
		logger.Tracef("response %x: error: %q", requestID, err.Error())
		logger.Tracef("error detail: %#v", err)
		return errors.Trace(err)
	}
	logger.Tracef("response %x: complete", requestID)
	return nil
}

func (c *controller) getQuery(path string, params url.Values) (interface{}, error) {
	return c._get(path, "", params)
}

func (c *controller) get(path string) (interface{}, error) {
	return c._get(path, "", nil)
}

func (c *controller) getOp(path, op string) (interface{}, error) {
	return c._get(path, op, nil)
}

func (c *controller) _get(path, op string, params url.Values) (interface{}, error) {
	bytes, err := c._getRaw(path, op, params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var parsed interface{}
	err = json.Unmarshal(bytes, &parsed)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return parsed, nil
}

func (c *controller) _getRaw(path, op string, params url.Values) ([]byte, error) {
	path = EnsureTrailingSlash(path)
	requestID := nextRequestID()
	if logger.IsTraceEnabled() {
		var query string
		if params != nil {
			query = "?" + params.Encode()
		}
		logger.Tracef("request %x: GET %s%s%s", requestID, c.client.APIURL, path, query)
	}
	bytes, err := c.client.Get(&url.URL{Path: path}, op, params)
	if err != nil {
		logger.Tracef("response %x: error: %q", requestID, err.Error())
		logger.Tracef("error detail: %#v", err)
		return nil, errors.Trace(err)
	}
	logger.Tracef("response %x: %s", requestID, string(bytes))
	return bytes, nil
}

func nextRequestID() int64 {
	return atomic.AddInt64(&requestNumber, 1)
}

func indicatesUnsupportedVersion(err error) bool {
	if err == nil {
		return false
	}
	if serverErr, ok := errors.Cause(err).(ServerError); ok {
		code := serverErr.StatusCode
		return code == http.StatusNotFound || code == http.StatusGone
	}
	// Workaround for bug in MAAS 1.9.4 - instead of a 404 we get a
	// redirect to the HTML login page, which doesn't parse as JSON.
	// https://bugs.launchpad.net/maas/+bug/1583715
	if syntaxErr, ok := errors.Cause(err).(*json.SyntaxError); ok {
		message := "invalid character '<' looking for beginning of value"
		return syntaxErr.Offset == 1 && syntaxErr.Error() == message
	}
	return false
}

func (c *controller) readAPIVersionInfo() (set.Strings, error) {
	parsed, err := c.get("version")
	if indicatesUnsupportedVersion(err) {
		return nil, WrapWithUnsupportedVersionError(err)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	// As we care about other fields, add them.
	fields := schema.Fields{
		"capabilities": schema.List(schema.String()),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(parsed, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "version response")
	}
	// For now, we don't append any subversion, but as it becomes used, we
	// should parse and check.

	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	capabilities := set.NewStrings()
	capabilityValues := valid["capabilities"].([]interface{})
	for _, value := range capabilityValues {
		capabilities.Add(value.(string))
	}

	return capabilities, nil
}

func parseAllocateConstraintsResponse(source interface{}, machine *machine) (ConstraintMatches, error) {
	var empty ConstraintMatches
	matchFields := schema.Fields{
		"storage":    schema.StringMap(schema.List(schema.ForceInt())),
		"interfaces": schema.StringMap(schema.List(schema.ForceInt())),
	}
	matchDefaults := schema.Defaults{
		"storage":    schema.Omit,
		"interfaces": schema.Omit,
	}
	fields := schema.Fields{
		"constraints_by_type": schema.FieldMap(matchFields, matchDefaults),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return empty, WrapWithDeserializationError(err, "allocation constraints response schema check failed")
	}
	valid := coerced.(map[string]interface{})
	constraintsMap := valid["constraints_by_type"].(map[string]interface{})
	result := ConstraintMatches{
		Interfaces: make(map[string][]Interface),
		Storage:    make(map[string][]BlockDevice),
	}

	if interfaceMatches, found := constraintsMap["interfaces"]; found {
		matches := convertConstraintMatches(interfaceMatches)
		for label, ids := range matches {
			interfaces := make([]Interface, len(ids))
			for index, id := range ids {
				iface := machine.Interface(id)
				if iface == nil {
					return empty, NewDeserializationError("constraint match interface %q: %d does not match an interface for the machine", label, id)
				}
				interfaces[index] = iface
			}
			result.Interfaces[label] = interfaces
		}
	}

	if storageMatches, found := constraintsMap["storage"]; found {
		matches := convertConstraintMatches(storageMatches)
		for label, ids := range matches {
			blockDevices := make([]BlockDevice, len(ids))
			for index, id := range ids {
				blockDevice := machine.BlockDevice(id)
				if blockDevice == nil {
					return empty, NewDeserializationError("constraint match storage %q: %d does not match a block device for the machine", label, id)
				}
				blockDevices[index] = blockDevice
			}
			result.Storage[label] = blockDevices
		}
	}
	return result, nil
}

func convertConstraintMatches(source interface{}) map[string][]int {
	// These casts are all safe because of the schema check.
	result := make(map[string][]int)
	matchMap := source.(map[string]interface{})
	for label, values := range matchMap {
		items := values.([]interface{})
		result[label] = make([]int, len(items))
		for index, value := range items {
			result[label][index] = value.(int)
		}
	}
	return result
}
