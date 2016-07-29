/*
The gosdc/cloudapi package interacts with the Cloud API (http://apidocs.joyent.com/cloudapi/).

Licensed under LGPL v3.

Copyright (c) 2013 Joyent Inc.
Written by Daniele Stroppa <daniele.stroppa@joyent.com>

*/
package cloudapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/joyent/gocommon/client"
	"github.com/joyent/gocommon/errors"
	jh "github.com/joyent/gocommon/http"
	"github.com/juju/loggo"
)

const (
	// The default version of the Cloud API to use
	DefaultAPIVersion = "~7.0"

	// CloudAPI URL parts
	apiKeys                    = "keys"
	apiPackages                = "packages"
	apiImages                  = "images"
	apiDatacenters             = "datacenters"
	apiMachines                = "machines"
	apiMetadata                = "metadata"
	apiSnapshots               = "snapshots"
	apiTags                    = "tags"
	apiAnalytics               = "analytics"
	apiInstrumentations        = "instrumentations"
	apiInstrumentationsValue   = "value"
	apiInstrumentationsRaw     = "raw"
	apiInstrumentationsHeatmap = "heatmap"
	apiInstrumentationsImage   = "image"
	apiInstrumentationsDetails = "details"
	apiUsage                   = "usage"
	apiAudit                   = "audit"
	apiFirewallRules           = "fwrules"
	apiFirewallRulesEnable     = "enable"
	apiFirewallRulesDisable    = "disable"
	apiNetworks                = "networks"

	// CloudAPI actions
	actionExport    = "export"
	actionStop      = "stop"
	actionStart     = "start"
	actionReboot    = "reboot"
	actionResize    = "resize"
	actionRename    = "rename"
	actionEnableFw  = "enable_firewall"
	actionDisableFw = "disable_firewall"
)

// Logger for this package
var Logger = loggo.GetLogger("gosdc.cloudapi")

// Client provides a means to access the Joyent CloudAPI
type Client struct {
	client client.Client
}

// New creates a new Client.
func New(client client.Client) *Client {
	return &Client{client}
}

// Filter represents a filter that can be applied to an API request.
type Filter struct {
	v url.Values
}

// NewFilter creates a new Filter.
func NewFilter() *Filter {
	return &Filter{make(url.Values)}
}

// Set a value for the specified filter.
func (f *Filter) Set(filter, value string) {
	f.v.Set(filter, value)
}

// Add a value for the specified filter.
func (f *Filter) Add(filter, value string) {
	f.v.Add(filter, value)
}

// request represents an API request
type request struct {
	method         string
	url            string
	filter         *Filter
	reqValue       interface{}
	reqHeader      http.Header
	resp           interface{}
	respHeader     *http.Header
	expectedStatus int
}

// Helper method to send an API request
func (c *Client) sendRequest(req request) (*jh.ResponseData, error) {
	request := jh.RequestData{
		ReqValue:   req.reqValue,
		ReqHeaders: req.reqHeader,
	}
	if req.filter != nil {
		request.Params = &req.filter.v
	}
	if req.expectedStatus == 0 {
		req.expectedStatus = http.StatusOK
	}
	respData := jh.ResponseData{
		RespValue:      req.resp,
		RespHeaders:    req.respHeader,
		ExpectedStatus: []int{req.expectedStatus},
	}
	err := c.client.SendRequest(req.method, req.url, "", &request, &respData)
	return &respData, err
}

// Helper method to create the API URL
func makeURL(parts ...string) string {
	return path.Join(parts...)
}

// Key represent a public key
type Key struct {
	Name        string // Name for the key
	Fingerprint string // Key Fingerprint
	Key         string // OpenSSH formatted public key
}

/*func (k Key) Equals(other Key) bool {
	if k.Name == other.Name && k.Fingerprint == other.Fingerprint && k.Key == other.Key {
		return true
	}
	return false
}*/

// CreateKeyOpts represent the option that can be specified
// when creating a new key.
type CreateKeyOpts struct {
	Name string `json:"name"` // Name for the key, optional
	Key  string `json:"key"`  // OpenSSH formatted public key
}

// Returns a list of public keys registered with a specific account.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListKeys
func (c *Client) ListKeys() ([]Key, error) {
	var resp []Key
	req := request{
		method: client.GET,
		url:    apiKeys,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of keys")
	}
	return resp, nil
}

// Returns the key identified by keyName.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetKey
func (c *Client) GetKey(keyName string) (*Key, error) {
	var resp Key
	req := request{
		method: client.GET,
		url:    makeURL(apiKeys, keyName),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get key with name: %s", keyName)
	}
	return &resp, nil
}

// Creates a new key with the specified options.
// See API docs: http://apidocs.joyent.com/cloudapi/#CreateKey
func (c *Client) CreateKey(opts CreateKeyOpts) (*Key, error) {
	var resp Key
	req := request{
		method:         client.POST,
		url:            apiKeys,
		reqValue:       opts,
		resp:           &resp,
		expectedStatus: http.StatusCreated,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to create key with name: %s", opts.Name)
	}
	return &resp, nil
}

// Deletes the key identified by keyName.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteKey
func (c *Client) DeleteKey(keyName string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiKeys, keyName),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete key with name: %s", keyName)
	}
	return nil
}

// A Package represent a named collections of resources that are used to describe the ‘sizes’
// of either a smart machine or a virtual machine.
type Package struct {
	Name        string // Name for the package
	Memory      int    // Memory available (in Mb)
	Disk        int    // Disk space available (in Gb)
	Swap        int    // Swap memory available (in Mb)
	VCPUs       int    // Number of VCPUs for the package
	Default     bool   // Indicates whether this is the default package in the datacenter
	Id          string // Unique identifier for the package
	Version     string // Version for the package
	Group       string // Group this package belongs to
	Description string // Human friendly description for the package
}

// Provides a list of packages available in the datacenter.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListPackages
func (c *Client) ListPackages(filter *Filter) ([]Package, error) {
	var resp []Package
	req := request{
		method: client.GET,
		url:    apiPackages,
		filter: filter,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of packages")
	}
	return resp, nil
}

// Returns the package specified by packageName. NOTE: packageName can specify
// either the package name or package Id.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetPackage
func (c *Client) GetPackage(packageName string) (*Package, error) {
	var resp Package
	req := request{
		method: client.GET,
		url:    makeURL(apiPackages, packageName),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get package with name: %s", packageName)
	}
	return &resp, nil
}

// Image represent the software packages that will be available on newly provisioned machines
type Image struct {
	Id           string                 // Unique identifier for the image
	Name         string                 // Image friendly name
	OS           string                 // Underlying operating system
	Version      string                 // Image version
	Type         string                 // Image type, one of 'smartmachine' or 'virtualmachine'
	Description  string                 // Image description
	Requirements map[string]interface{} // Minimum requirements for provisioning a machine with this image, e.g. 'password' indicates that a password must be provided
	Homepage     string                 // URL for a web page including detailed information for this image (new in API version 7.0)
	PublishedAt  string                 `json:"published_at"` // Time this image has been made publicly available (new in API version 7.0)
	Public       string                 // Indicates if the image is publicly available (new in API version 7.1)
	State        string                 // Current image state. One of 'active', 'unactivated', 'disabled', 'creating', 'failed' (new in API version 7.1)
	Tags         map[string]string      // A map of key/value pairs that allows clients to categorize images by any given criteria (new in API version 7.1)
	EULA         string                 // URL of the End User License Agreement (EULA) for the image (new in API version 7.1)
	ACL          []string               // An array of account UUIDs given access to a private image. The field is only relevant to private images (new in API version 7.1)
	Owner        string                 // The UUID of the user owning the image
}

// ExportImageOpts represent the option that can be specified
// when exporting an image.
type ExportImageOpts struct {
	MantaPath string `json:"manta_path"` // The Manta path prefix to use when exporting the image
}

// MantaLocation represent the properties that allow a user
// to retrieve the image file and manifest from Manta
type MantaLocation struct {
	MantaURL     string `json:"manta_url"`     // Manta datacenter URL
	ImagePath    string `json:"image_path"`    // Path to the image
	ManifestPath string `json:"manifest_path"` // Path to the image manifest
}

// CreateImageFromMachineOpts represent the option that can be specified
// when creating a new image from an existing machine.
type CreateImageFromMachineOpts struct {
	Machine     string            `json:"machine"`     // The machine UUID from which the image is to be created
	Name        string            `json:"name"`        // Image name
	Version     string            `json:"version"`     // Image version
	Description string            `json:"description"` // Image description
	Homepage    string            `json:"homepage"`    // URL for a web page including detailed information for this image
	EULA        string            `json:"eula"`        // URL of the End User License Agreement (EULA) for the image
	ACL         []string          `json:"acl"`         // An array of account UUIDs given access to a private image. The field is only relevant to private images
	Tags        map[string]string `json:"tags"`        // A map of key/value pairs that allows clients to categorize images by any given criteria
}

// Provides a list of images available in the datacenter.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListImages
func (c *Client) ListImages(filter *Filter) ([]Image, error) {
	var resp []Image
	req := request{
		method: client.GET,
		url:    apiImages,
		filter: filter,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of images")
	}
	return resp, nil
}

// Returns the image specified by imageId.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetImage
func (c *Client) GetImage(imageId string) (*Image, error) {
	var resp Image
	req := request{
		method: client.GET,
		url:    makeURL(apiImages, imageId),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get image with id: %s", imageId)
	}
	return &resp, nil
}

// (Beta) Delete the image specified by imageId. Must be image owner to do so.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteImage
func (c *Client) DeleteImage(imageId string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiImages, imageId),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete image with id: %s", imageId)
	}
	return nil
}

// (Beta) Exports an image to the specified Manta path.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListImages
func (c *Client) ExportImage(imageId string, opts ExportImageOpts) (*MantaLocation, error) {
	var resp MantaLocation
	req := request{
		method:   client.POST,
		url:      fmt.Sprintf("%s/%s?action=%s", apiImages, imageId, actionExport),
		reqValue: opts,
		resp:     &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to export image %s to %s", imageId, opts.MantaPath)
	}
	return &resp, nil
}

// (Beta) Create a new custom image from a machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListImages
func (c *Client) CreateImageFromMachine(opts CreateImageFromMachineOpts) (*Image, error) {
	var resp Image
	req := request{
		method:         client.POST,
		url:            apiImages,
		reqValue:       opts,
		resp:           &resp,
		expectedStatus: http.StatusCreated,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to create image from machine %s", opts.Machine)
	}
	return &resp, nil
}

// Provides a list of all datacenters this cloud is aware of.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListDatacenters
func (c *Client) ListDatacenters() (map[string]interface{}, error) {
	var resp map[string]interface{}
	req := request{
		method: client.GET,
		url:    apiDatacenters,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of datcenters")
	}
	return resp, nil
}

// Gets an individual datacenter by name. Returns an HTTP redirect to your client,
// the datacenter URL is in the Location header.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetDatacenter
func (c *Client) GetDatacenter(datacenterName string) (string, error) {
	var respHeader http.Header
	req := request{
		method:         client.GET,
		url:            makeURL(apiDatacenters, datacenterName),
		respHeader:     &respHeader,
		expectedStatus: http.StatusFound,
	}
	respData, err := c.sendRequest(req)
	if err != nil {
		return "", errors.Newf(err, "failed to get datacenter with name: %s", datacenterName)
	}
	return respData.RespHeaders.Get("Location"), nil
}

// Machine represent a provisioned virtual machines
type Machine struct {
	Id        string            // Unique identifier for the image
	Name      string            // Machine friendly name
	Type      string            // Machine type, one of 'smartmachine' or 'virtualmachine'
	State     string            // Current state of the machine
	Dataset   string            // The dataset URN the machine was provisioned with. For new images/datasets this value will be the dataset id, i.e, same value than the image attribute
	Memory    int               // The amount of memory the machine has (in Mb)
	Disk      int               // The amount of disk the machine has (in Gb)
	IPs       []string          // The IP addresses the machine has
	Metadata  map[string]string // Map of the machine metadata, e.g. authorized-keys
	Tags      map[string]string // Map of the machine tags
	Created   string            // When the machine was created
	Updated   string            // When the machine was updated
	Package   string            // The name of the package used to create the machine
	Image     string            // The image id the machine was provisioned with
	PrimaryIP string            // The primary (public) IP address for the machine
	Networks  []string          // The network IDs for the machine
}

// Helper method to compare two machines. Ignores state and timestamps.
func (m Machine) Equals(other Machine) bool {
	if m.Id == other.Id && m.Name == other.Name && m.Type == other.Type && m.Dataset == other.Dataset &&
		m.Memory == other.Memory && m.Disk == other.Disk && m.Package == other.Package && m.Image == other.Image &&
		m.compareIPs(other) && m.compareMetadata(other) {
		return true
	}
	return false
}

// Helper method to compare two machines IPs
func (m Machine) compareIPs(other Machine) bool {
	if len(m.IPs) != len(other.IPs) {
		return false
	}
	for i, v := range m.IPs {
		if v != other.IPs[i] {
			return false
		}
	}
	return true
}

// Helper method to compare two machines metadata
func (m Machine) compareMetadata(other Machine) bool {
	if len(m.Metadata) != len(other.Metadata) {
		return false
	}
	for k, v := range m.Metadata {
		if v != other.Metadata[k] {
			return false
		}
	}
	return true
}

// CreateMachineOpts represent the option that can be specified
// when creating a new machine.
type CreateMachineOpts struct {
	Name            string            `json:"name"`             // Machine friendly name, default is a randomly generated name
	Package         string            `json:"package"`          // Name of the package to use on provisioning
	Image           string            `json:"image"`            // The image UUID
	Networks        []string          `json:"networks"`         // Desired networks IDs
	Metadata        map[string]string `json:"-"`                // An arbitrary set of metadata key/value pairs can be set at provision time
	Tags            map[string]string `json:"-"`                // An arbitrary set of tags can be set at provision time
	FirewallEnabled bool              `json:"firewall_enabled"` // Completely enable or disable firewall for this machine (new in API version 7.0)
}

// Snapshot represent a point in time state of a machine.
type Snapshot struct {
	Name  string // Snapshot name
	State string // Snapshot state
}

// SnapshotOpts represent the option that can be specified
// when creating a new machine snapshot.
type SnapshotOpts struct {
	Name string `json:"name"` // Snapshot name
}

// AuditAction represents an action/event accomplished by a machine.
type AuditAction struct {
	Action     string                 // Action name
	Parameters map[string]interface{} // Original set of parameters sent when the action was requested
	Time       string                 // When the action finished
	Success    string                 // Either 'yes' or 'no', depending on the action successfulness
	Caller     Caller                 // Account requesting the action
}

// Caller represents an account requesting an action.
type Caller struct {
	Type  string // Authentication type for the action request. One of 'basic', 'operator', 'signature' or 'token'
	User  string // When the authentication type is 'basic', this member will be present and include user login
	IP    string // The IP addresses this from which the action was requested. Not present if type is 'operator'
	KeyId string // When authentication type is either 'signature' or 'token', SSH key identifier
}

// appendJSON marshals the given attribute value and appends it as an encoded value to the given json data.
// The newly encode (attr, value) is inserted just before the closing "}" in the json data.
func appendJSON(data []byte, attr string, value interface{}) ([]byte, error) {
	newData, err := json.Marshal(&value)
	if err != nil {
		return nil, err
	}
	strData := string(data)
	result := fmt.Sprintf(`%s, "%s":%s}`, strData[:len(strData)-1], attr, string(newData))
	return []byte(result), nil
}

type jsonOpts CreateMachineOpts

func (opts CreateMachineOpts) MarshalJSON() ([]byte, error) {
	var jo jsonOpts = jsonOpts(opts)
	data, err := json.Marshal(&jo)
	if err != nil {
		return nil, err
	}
	for k, v := range opts.Tags {
		data, err = appendJSON(data, k, v)
		if err != nil {
			return nil, err
		}
	}
	for k, v := range opts.Metadata {
		data, err = appendJSON(data, k, v)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// Lists all machines on record for an account.
// You can paginate this API by passing in offset, and limit
// See API docs: http://apidocs.joyent.com/cloudapi/#ListMachines
func (c *Client) ListMachines(filter *Filter) ([]Machine, error) {
	var resp []Machine
	req := request{
		method: client.GET,
		url:    apiMachines,
		filter: filter,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of machines")
	}
	return resp, nil
}

// Returns the number of machines on record for an account.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListMachines
func (c *Client) CountMachines() (int, error) {
	var resp int
	req := request{
		method: client.HEAD,
		url:    apiMachines,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return -1, errors.Newf(err, "failed to get count of machines")
	}
	return resp, nil
}

// Returns the machine specified by machineId.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetMachine
func (c *Client) GetMachine(machineId string) (*Machine, error) {
	var resp Machine
	req := request{
		method: client.GET,
		url:    makeURL(apiMachines, machineId),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get machine with id: %s", machineId)
	}
	return &resp, nil
}

// Creates a new machine with the options specified.
// See API docs: http://apidocs.joyent.com/cloudapi/#CreateMachine
func (c *Client) CreateMachine(opts CreateMachineOpts) (*Machine, error) {
	var resp Machine
	req := request{
		method:         client.POST,
		url:            apiMachines,
		reqValue:       opts,
		resp:           &resp,
		expectedStatus: http.StatusCreated,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to create machine with name: %s", opts.Name)
	}
	return &resp, nil
}

// Stops a running machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#StopMachine
func (c *Client) StopMachine(machineId string) error {
	req := request{
		method:         client.POST,
		url:            fmt.Sprintf("%s/%s?action=%s", apiMachines, machineId, actionStop),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to stop machine with id: %s", machineId)
	}
	return nil
}

// Starts a stopped machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#StartMachine
func (c *Client) StartMachine(machineId string) error {
	req := request{
		method:         client.POST,
		url:            fmt.Sprintf("%s/%s?action=%s", apiMachines, machineId, actionStart),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to start machine with id: %s", machineId)
	}
	return nil
}

// Reboots (stop followed by a start) a machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#RebootMachine
func (c *Client) RebootMachine(machineId string) error {
	req := request{
		method:         client.POST,
		url:            fmt.Sprintf("%s/%s?action=%s", apiMachines, machineId, actionReboot),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to reboot machine with id: %s", machineId)
	}
	return nil
}

// Allows you to resize a SmartMachine. Virtual machines can also be resized,
// but only resizing virtual machines to a higher capacity package is supported.
// See API docs: http://apidocs.joyent.com/cloudapi/#ResizeMachine
func (c *Client) ResizeMachine(machineId, packageName string) error {
	req := request{
		method:         client.POST,
		url:            fmt.Sprintf("%s/%s?action=%s&package=%s", apiMachines, machineId, actionResize, packageName),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to resize machine with id: %s", machineId)
	}
	return nil
}

// Renames an existing machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#RenameMachine
func (c *Client) RenameMachine(machineId, machineName string) error {
	req := request{
		method:         client.POST,
		url:            fmt.Sprintf("%s/%s?action=%s&name=%s", apiMachines, machineId, actionRename, machineName),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to rename machine with id: %s", machineId)
	}
	return nil
}

// List all the firewall rules for the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListMachineFirewallRules
func (c *Client) ListMachineFirewallRules(machineId string) ([]FirewallRule, error) {
	var resp []FirewallRule
	req := request{
		method: client.GET,
		url:    makeURL(apiMachines, machineId, apiFirewallRules),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of firewall rules for machine with id %s", machineId)
	}
	return resp, nil
}

// Enable the firewall for the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#EnableMachineFirewall
func (c *Client) EnableFirewallMachine(machineId string) error {
	req := request{
		method:         client.POST,
		url:            fmt.Sprintf("%s/%s?action=%s", apiMachines, machineId, actionEnableFw),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to enable firewall on machine with id: %s", machineId)
	}
	return nil
}

// Disable the firewall for the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#DisableMachineFirewall
func (c *Client) DisableFirewallMachine(machineId string) error {
	req := request{
		method:         client.POST,
		url:            fmt.Sprintf("%s/%s?action=%s", apiMachines, machineId, actionDisableFw),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to disable firewall on machine with id: %s", machineId)
	}
	return nil
}

// Creates a new snapshot for the machine with the options specified.
// See API docs: http://apidocs.joyent.com/cloudapi/#CreateMachineSnapshot
func (c *Client) CreateMachineSnapshot(machineId string, opts SnapshotOpts) (*Snapshot, error) {
	var resp Snapshot
	req := request{
		method:         client.POST,
		url:            makeURL(apiMachines, machineId, apiSnapshots),
		reqValue:       opts,
		resp:           &resp,
		expectedStatus: http.StatusCreated,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to create snapshot %s from machine with id %s", opts.Name, machineId)
	}
	return &resp, nil
}

// Start the machine from the specified snapshot. Machine must be in 'stopped' state.
// See API docs: http://apidocs.joyent.com/cloudapi/#StartMachineFromSnapshot
func (c *Client) StartMachineFromSnapshot(machineId, snapshotName string) error {
	req := request{
		method:         client.POST,
		url:            makeURL(apiMachines, machineId, apiSnapshots, snapshotName),
		expectedStatus: http.StatusAccepted,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to start machine with id %s from snapshot %s", machineId, snapshotName)
	}
	return nil
}

// List all snapshots for the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListMachineSnapshots
func (c *Client) ListMachineSnapshots(machineId string) ([]Snapshot, error) {
	var resp []Snapshot
	req := request{
		method: client.GET,
		url:    makeURL(apiMachines, machineId, apiSnapshots),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of snapshots for machine with id %s", machineId)
	}
	return resp, nil
}

// Returns the state of the specified snapshot.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetMachineSnapshot
func (c *Client) GetMachineSnapshot(machineId, snapshotName string) (*Snapshot, error) {
	var resp Snapshot
	req := request{
		method: client.GET,
		url:    makeURL(apiMachines, machineId, apiSnapshots, snapshotName),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get snapshot %s for machine with id %s", snapshotName, machineId)
	}
	return &resp, nil
}

// Deletes the specified snapshot.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteMachineSnapshot
func (c *Client) DeleteMachineSnapshot(machineId, snapshotName string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiMachines, machineId, apiSnapshots, snapshotName),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete snapshot %s for machine with id %s", snapshotName, machineId)
	}
	return nil
}

// Updates the metadata for a given machine.
// Any metadata keys passed in here are created if they do not exist, and overwritten if they do.
// See API docs: http://apidocs.joyent.com/cloudapi/#UpdateMachineMetadata
func (c *Client) UpdateMachineMetadata(machineId string, metadata map[string]string) (map[string]interface{}, error) {
	var resp map[string]interface{}
	req := request{
		method:   client.POST,
		url:      makeURL(apiMachines, machineId, apiMetadata),
		reqValue: metadata,
		resp:     &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to update metadata for machine with id %s", machineId)
	}
	return resp, nil
}

// Returns the complete set of metadata associated with the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetMachineMetadata
func (c *Client) GetMachineMetadata(machineId string) (map[string]interface{}, error) {
	var resp map[string]interface{}
	req := request{
		method: client.GET,
		url:    makeURL(apiMachines, machineId, apiMetadata),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of metadata for machine with id %s", machineId)
	}
	return resp, nil
}

// Deletes a single metadata key from the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteMachineMetadata
func (c *Client) DeleteMachineMetadata(machineId, metadataKey string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiMachines, machineId, apiMetadata, metadataKey),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete metadata with key %s for machine with id %s", metadataKey, machineId)
	}
	return nil
}

// Deletes all metadata keys from the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteAllMachineMetadata
func (c *Client) DeleteAllMachineMetadata(machineId string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiMachines, machineId, apiMetadata),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete metadata for machine with id %s", machineId)
	}
	return nil
}

// Adds additional tags to the specified machine.
// This API lets you append new tags, not overwrite existing tags.
// See API docs: http://apidocs.joyent.com/cloudapi/#AddMachineTags
func (c *Client) AddMachineTags(machineId string, tags map[string]string) (map[string]string, error) {
	var resp map[string]string
	req := request{
		method:   client.POST,
		url:      makeURL(apiMachines, machineId, apiTags),
		reqValue: tags,
		resp:     &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to add tags for machine with id %s", machineId)
	}
	return resp, nil
}

// Replaces existing tags for the specified machine.
// This API lets you overwrite existing tags, not append to existing tags.
// See API docs: http://apidocs.joyent.com/cloudapi/#ReplaceMachineTags
func (c *Client) ReplaceMachineTags(machineId string, tags map[string]string) (map[string]string, error) {
	var resp map[string]string
	req := request{
		method:   client.PUT,
		url:      makeURL(apiMachines, machineId, apiTags),
		reqValue: tags,
		resp:     &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to replace tags for machine with id %s", machineId)
	}
	return resp, nil
}

// Returns the complete set of tags associated with the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListMachineTags
func (c *Client) ListMachineTags(machineId string) (map[string]string, error) {
	var resp map[string]string
	req := request{
		method: client.GET,
		url:    makeURL(apiMachines, machineId, apiTags),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of tags for machine with id %s", machineId)
	}
	return resp, nil
}

// Returns the value for a single tag on the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetMachineTag
func (c *Client) GetMachineTag(machineId, tagKey string) (string, error) {
	var resp string
	requestHeaders := make(http.Header)
	requestHeaders.Set("Accept", "text/plain")
	req := request{
		method:    client.GET,
		url:       makeURL(apiMachines, machineId, apiTags, tagKey),
		resp:      &resp,
		reqHeader: requestHeaders,
	}
	if _, err := c.sendRequest(req); err != nil {
		return "", errors.Newf(err, "failed to get tag %s for machine with id %s", tagKey, machineId)
	}
	return resp, nil
}

// Deletes a single tag from the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteMachineTag
func (c *Client) DeleteMachineTag(machineId, tagKey string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiMachines, machineId, apiTags, tagKey),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete tag with key %s for machine with id %s", tagKey, machineId)
	}
	return nil
}

// Deletes all tags from the specified machine.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteMachineTags
func (c *Client) DeleteMachineTags(machineId string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiMachines, machineId, apiTags),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete tags for machine with id %s", machineId)
	}
	return nil
}

// Allows you to completely destroy a machine. Machine must be in the 'stopped' state.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteMachine
func (c *Client) DeleteMachine(machineId string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiMachines, machineId),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete machine with id %s", machineId)
	}
	return nil
}

// Provides a list of machine's accomplished actions, (sorted from latest to older one).
// See API docs: http://apidocs.joyent.com/cloudapi/#MachineAudit
func (c *Client) MachineAudit(machineId string) ([]AuditAction, error) {
	var resp []AuditAction
	req := request{
		method: client.GET,
		url:    makeURL(apiMachines, machineId, apiAudit),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get actions for machine with id %s", machineId)
	}
	return resp, nil
}

// Analytics represents the available analytics
type Analytics struct {
	Modules         map[string]interface{} // Namespace to organize metrics
	Fields          map[string]interface{} // Fields represent metadata by which data points can be filtered or decomposed
	Types           map[string]interface{} // Types are used with both metrics and fields for two purposes: to hint to clients at how to best label values, and to distinguish between numeric and discrete quantities.
	Metrics         map[string]interface{} // Metrics describe quantities which can be measured by the system
	Transformations map[string]interface{} // Transformations are post-processing functions that can be applied to data when it's retrieved.
}

// Instrumentation specify which metric to collect, how frequently to aggregate data (e.g., every second, every hour, etc.)
// how much data to keep (e.g., 10 minutes' worth, 6 months' worth, etc.) and other configuration options
type Instrumentation struct {
	Module          string   `json:"module"`
	Stat            string   `json:"stat"`
	Predicate       string   `json:"predicate"`
	Decomposition   []string `json:"decomposition"`
	ValueDimension  int      `json:"value-dimenstion"`
	ValueArity      string   `json:"value-arity"`
	RetentionTime   int      `json:"retention-time"`
	Granularity     int      `json:"granularitiy"`
	IdleMax         int      `json:"idle-max"`
	Transformations []string `json:"transformations"`
	PersistData     bool     `json:"persist-data"`
	Crtime          int      `json:"crtime"`
	ValueScope      string   `json:"value-scope"`
	Id              string   `json:"id"`
	Uris            []Uri    `json:"uris"`
}

// Uri represents a Universal Resource Identifier
type Uri struct {
	Uri  string // Resource identifier
	Name string // URI name
}

// InstrumentationValue represents the data associated to an instrumentation for a point in time
type InstrumentationValue struct {
	Value           interface{}
	Transformations map[string]interface{}
	StartTime       int
	Duration        int
}

// HeatmapOpts represent the option that can be specified
// when retrieving an instrumentation.'s heatmap
type HeatmapOpts struct {
	Height       int      `json:"height"`        // Height of the image in pixels
	Width        int      `json:"width"`         // Width of the image in pixels
	Ymin         int      `json:"ymin"`          // Y-Axis value for the bottom of the image (default: 0)
	Ymax         int      `json:"ymax"`          // Y-Axis value for the top of the image (default: auto)
	Nbuckets     int      `json:"nbuckets"`      // Number of buckets in the vertical dimension
	Selected     []string `json:"selected"`      // Array of field values to highlight, isolate or exclude
	Isolate      bool     `json:"isolate"`       // If true, only draw selected values
	Exclude      bool     `json:"exclude"`       // If true, don't draw selected values at all
	Hues         []string `json:"hues"`          // Array of colors for highlighting selected field values
	DecomposeAll bool     `json:"decompose_all"` // Highlight all field values
	X            int      `json:"x"`
	Y            int      `json:"y"`
}

// Heatmap represents an instrumentation's heatmap
type Heatmap struct {
	BucketTime int                    `json:"bucket_time"` // Time corresponding to the bucket (Unix seconds)
	BucketYmin int                    `json:"bucket_ymin"` // Minimum y-axis value for the bucket
	BucketYmax int                    `json:"bucket_ymax"` // Maximum y-axis value for the bucket
	Present    map[string]interface{} `json:"present"`     // If the instrumentation defines a discrete decomposition, this property's value is an object whose keys are values of that field and whose values are the number of data points in that bucket for that key
	Total      int                    `json:"total"`       // The total number of data points in the bucket
}

// CreateInstrumentationOpts represent the option that can be specified
// when creating a new instrumentation.
type CreateInstrumentationOpts struct {
	Clone         int    `json:"clone"`     // An existing instrumentation ID to be cloned
	Module        string `json:"module"`    // Analytics module
	Stat          string `json:"stat"`      // Analytics stat
	Predicate     string `json:"predicate"` // Instrumentation predicate, must be JSON string
	Decomposition string `json:"decomposition"`
	Granularity   int    `json:"granularity"`    // Number of seconds between data points (default is 1)
	RetentionTime int    `json:"retention-time"` // How long to keep this instrumentation data for
	PersistData   bool   `json:"persist-data"`   // Whether or not to store this for historical analysis
	IdleMax       int    `json:"idle-max"`       // Number of seconds after which if the instrumentation or its data has not been accessed via the API the service may delete the instrumentation and its data
}

// Retrieves the "schema" for instrumentations that can be created.
// See API docs: http://apidocs.joyent.com/cloudapi/#DescribeAnalytics
func (c *Client) DescribeAnalytics() (*Analytics, error) {
	var resp Analytics
	req := request{
		method: client.GET,
		url:    apiAnalytics,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get analytics")
	}
	return &resp, nil
}

// Retrieves all currently created instrumentations.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListInstrumentations
func (c *Client) ListInstrumentations() ([]Instrumentation, error) {
	var resp []Instrumentation
	req := request{
		method: client.GET,
		url:    makeURL(apiAnalytics, apiInstrumentations),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get instrumentations")
	}
	return resp, nil
}

// Retrieves the configuration for the specified instrumentation.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetInstrumentation
func (c *Client) GetInstrumentation(instrumentationId string) (*Instrumentation, error) {
	var resp Instrumentation
	req := request{
		method: client.GET,
		url:    makeURL(apiAnalytics, apiInstrumentations, instrumentationId),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get instrumentation with id %s", instrumentationId)
	}
	return &resp, nil
}

// Retrieves the data associated to an instrumentation for a point in time.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetInstrumentationValue
func (c *Client) GetInstrumentationValue(instrumentationId string) (*InstrumentationValue, error) {
	var resp InstrumentationValue
	req := request{
		method: client.GET,
		url:    makeURL(apiAnalytics, apiInstrumentations, instrumentationId, apiInstrumentationsValue, apiInstrumentationsRaw),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get value for instrumentation with id %s", instrumentationId)
	}
	return &resp, nil
}

// Retrieves the specified instrumentation's heatmap.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetInstrumentationHeatmap
func (c *Client) GetInstrumentationHeatmap(instrumentationId string) (*Heatmap, error) {
	var resp Heatmap
	req := request{
		method: client.GET,
		url:    makeURL(apiAnalytics, apiInstrumentations, instrumentationId, apiInstrumentationsValue, apiInstrumentationsHeatmap, apiInstrumentationsImage),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get heatmap image for instrumentation with id %s", instrumentationId)
	}
	return &resp, nil
}

// Allows you to retrieve the bucket details for a heatmap.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetInstrumentationHeatmapDetails
func (c *Client) GetInstrumentationHeatmapDetails(instrumentationId string) (*Heatmap, error) {
	var resp Heatmap
	req := request{
		method: client.GET,
		url:    makeURL(apiAnalytics, apiInstrumentations, instrumentationId, apiInstrumentationsValue, apiInstrumentationsHeatmap, apiInstrumentationsDetails),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get heatmap details for instrumentation with id %s", instrumentationId)
	}
	return &resp, nil
}

// Creates an instrumentation.
// You can clone an existing instrumentation by passing in the parameter clone, which should be a numeric id of an existing instrumentation.
// See API docs: http://apidocs.joyent.com/cloudapi/#CreateInstrumentation
func (c *Client) CreateInstrumentation(opts CreateInstrumentationOpts) (*Instrumentation, error) {
	var resp Instrumentation
	req := request{
		method:         client.POST,
		url:            makeURL(apiAnalytics, apiInstrumentations),
		reqValue:       opts,
		resp:           &resp,
		expectedStatus: http.StatusCreated,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to create instrumentation")
	}
	return &resp, nil
}

// Destroys an instrumentation.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteInstrumentation
func (c *Client) DeleteInstrumentation(instrumentationId string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiAnalytics, apiInstrumentations, instrumentationId),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete instrumentation with id %s", instrumentationId)
	}
	return nil
}

// FirewallRule represent a firewall rule that can be specifed for a machine.
type FirewallRule struct {
	Id      string // Unique identifier for the rule
	Enabled bool   // Whether the rule is enabled or not
	Rule    string // Firewall rule in the form 'FROM <target a> TO <target b> <action> <protocol> <port>'
}

// CreateFwRuleOpts represent the option that can be specified
// when creating a new firewall rule.
type CreateFwRuleOpts struct {
	Enabled bool   `json:"enabled"` // Whether to enable the rule or not
	Rule    string `json:"rule"`    // Firewall rule in the form 'FROM <target a> TO <target b> <action> <protocol> <port>'
}

// Lists all the firewall rules on record for a specified account.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListFirewallRules
func (c *Client) ListFirewallRules() ([]FirewallRule, error) {
	var resp []FirewallRule
	req := request{
		method: client.GET,
		url:    apiFirewallRules,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of firewall rules")
	}
	return resp, nil
}

// Returns the specified firewall rule.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetFirewallRule
func (c *Client) GetFirewallRule(fwRuleId string) (*FirewallRule, error) {
	var resp FirewallRule
	req := request{
		method: client.GET,
		url:    makeURL(apiFirewallRules, fwRuleId),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get firewall rule with id %s", fwRuleId)
	}
	return &resp, nil
}

// Creates the firewall rule with the specified options.
// See API docs: http://apidocs.joyent.com/cloudapi/#CreateFirewallRule
func (c *Client) CreateFirewallRule(opts CreateFwRuleOpts) (*FirewallRule, error) {
	var resp FirewallRule
	req := request{
		method:         client.POST,
		url:            apiFirewallRules,
		reqValue:       opts,
		resp:           &resp,
		expectedStatus: http.StatusCreated,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to create firewall rule: %s", opts.Rule)
	}
	return &resp, nil
}

// Updates the specified firewall rule.
// See API docs: http://apidocs.joyent.com/cloudapi/#UpdateFirewallRule
func (c *Client) UpdateFirewallRule(fwRuleId string, opts CreateFwRuleOpts) (*FirewallRule, error) {
	var resp FirewallRule
	req := request{
		method:   client.POST,
		url:      makeURL(apiFirewallRules, fwRuleId),
		reqValue: opts,
		resp:     &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to update firewall rule with id %s to %s", fwRuleId, opts.Rule)
	}
	return &resp, nil
}

// Enables the given firewall rule record if it is disabled.
// See API docs: http://apidocs.joyent.com/cloudapi/#EnableFirewallRule
func (c *Client) EnableFirewallRule(fwRuleId string) (*FirewallRule, error) {
	var resp FirewallRule
	req := request{
		method: client.POST,
		url:    makeURL(apiFirewallRules, fwRuleId, apiFirewallRulesEnable),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to enable firewall rule with id %s", fwRuleId)
	}
	return &resp, nil
}

// Disables the given firewall rule record if it is enabled.
// See API docs: http://apidocs.joyent.com/cloudapi/#DisableFirewallRule
func (c *Client) DisableFirewallRule(fwRuleId string) (*FirewallRule, error) {
	var resp FirewallRule
	req := request{
		method: client.POST,
		url:    makeURL(apiFirewallRules, fwRuleId, apiFirewallRulesDisable),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to disable firewall rule with id %s", fwRuleId)
	}
	return &resp, nil
}

// Removes the given firewall rule record from all the required account machines.
// See API docs: http://apidocs.joyent.com/cloudapi/#DeleteFirewallRule
func (c *Client) DeleteFirewallRule(fwRuleId string) error {
	req := request{
		method:         client.DELETE,
		url:            makeURL(apiFirewallRules, fwRuleId),
		expectedStatus: http.StatusNoContent,
	}
	if _, err := c.sendRequest(req); err != nil {
		return errors.Newf(err, "failed to delete firewall rule with id %s", fwRuleId)
	}
	return nil
}

// Return the list of machines affected by the given firewall rule.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListFirewallRuleMachines
func (c *Client) ListFirewallRuleMachines(fwRuleId string) ([]Machine, error) {
	var resp []Machine
	req := request{
		method: client.GET,
		url:    makeURL(apiFirewallRules, fwRuleId, apiMachines),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of machines affected by firewall rule wit id %s", fwRuleId)
	}
	return resp, nil
}

// Network represents a network available to a given account
type Network struct {
	Id          string // Unique identifier for the network
	Name        string // Network name
	Public      bool   // Whether this a public or private (rfc1918) network
	Description string // Optional description for this network, when name is not enough
}

// List all the networks which can be used by the given account.
// See API docs: http://apidocs.joyent.com/cloudapi/#ListNetworks
func (c *Client) ListNetworks() ([]Network, error) {
	var resp []Network
	req := request{
		method: client.GET,
		url:    apiNetworks,
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get list of networks")
	}
	return resp, nil
}

// Retrieves an individual network record.
// See API docs: http://apidocs.joyent.com/cloudapi/#GetNetwork
func (c *Client) GetNetwork(networkId string) (*Network, error) {
	var resp Network
	req := request{
		method: client.GET,
		url:    makeURL(apiNetworks, networkId),
		resp:   &resp,
	}
	if _, err := c.sendRequest(req); err != nil {
		return nil, errors.Newf(err, "failed to get network with id %s", networkId)
	}
	return &resp, nil
}
