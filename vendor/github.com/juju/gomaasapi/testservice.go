// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"gopkg.in/mgo.v2/bson"
)

// TestMAASObject is a fake MAAS server MAASObject.
type TestMAASObject struct {
	MAASObject
	TestServer *TestServer
}

// checkError is a shorthand helper that panics if err is not nil.
func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

// NewTestMAAS returns a TestMAASObject that implements the MAASObject
// interface and thus can be used as a test object instead of the one returned
// by gomaasapi.NewMAAS().
func NewTestMAAS(version string) *TestMAASObject {
	server := NewTestServer(version)
	authClient, err := NewAnonymousClient(server.URL, version)
	checkError(err)
	maas := NewMAAS(*authClient)
	return &TestMAASObject{*maas, server}
}

// Close shuts down the test server.
func (testMAASObject *TestMAASObject) Close() {
	testMAASObject.TestServer.Close()
}

// A TestServer is an HTTP server listening on a system-chosen port on the
// local loopback interface, which simulates the behavior of a MAAS server.
// It is intendend for use in end-to-end HTTP tests using the gomaasapi
// library.
type TestServer struct {
	*httptest.Server
	serveMux   *http.ServeMux
	client     Client
	nodes      map[string]MAASObject
	ownedNodes map[string]bool
	// mapping system_id -> list of operations performed.
	nodeOperations map[string][]string
	// list of operations performed at the /nodes/ level.
	nodesOperations []string
	// mapping system_id -> list of Values passed when performing
	// operations
	nodeOperationRequestValues map[string][]url.Values
	// list of Values passed when performing operations at the
	// /nodes/ level.
	nodesOperationRequestValues []url.Values
	nodeMetadata                map[string]Node
	files                       map[string]MAASObject
	networks                    map[string]MAASObject
	networksPerNode             map[string][]string
	ipAddressesPerNetwork       map[string][]string
	version                     string
	macAddressesPerNetwork      map[string]map[string]JSONObject
	tagsPerNode                 map[string][]string
	nodeDetails                 map[string]string
	zones                       map[string]JSONObject
	tags                        map[string]JSONObject
	// bootImages is a map of nodegroup UUIDs to boot-image objects.
	bootImages map[string][]JSONObject
	// nodegroupsInterfaces is a map of nodegroup UUIDs to interface
	// objects.
	nodegroupsInterfaces map[string][]JSONObject

	// versionJSON is the response to the /version/ endpoint listing the
	// capabilities of the MAAS server.
	versionJSON string

	// devices is a map of device UUIDs to devices.
	devices map[string]*TestDevice

	subnets         map[uint]TestSubnet
	subnetNameToID  map[string]uint
	nextSubnet      uint
	spaces          map[uint]*TestSpace
	spaceNameToID   map[string]uint
	nextSpace       uint
	vlans           map[int]TestVLAN
	nextVLAN        int
	staticRoutes    map[uint]*TestStaticRoute
	nextStaticRoute uint
}

type TestDevice struct {
	IPAddresses  []string
	SystemId     string
	MACAddresses []string
	Parent       string
	Hostname     string

	// Not part of the device definition but used by the template.
	APIVersion string
}

func getNodesEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/nodes/", version)
}

func getNodeURL(version, systemId string) string {
	return fmt.Sprintf("/api/%s/nodes/%s/", version, systemId)
}

func getNodeURLRE(version string) *regexp.Regexp {
	reString := fmt.Sprintf("^/api/%s/nodes/([^/]*)/$", regexp.QuoteMeta(version))
	return regexp.MustCompile(reString)
}

func getDevicesEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/devices/", version)
}

func getDeviceURL(version, systemId string) string {
	return fmt.Sprintf("/api/%s/devices/%s/", version, systemId)
}

func getDeviceURLRE(version string) *regexp.Regexp {
	reString := fmt.Sprintf("^/api/%s/devices/([^/]*)/$", regexp.QuoteMeta(version))
	return regexp.MustCompile(reString)
}

func getFilesEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/files/", version)
}

func getFileURL(version, filename string) string {
	// Uses URL object so filename is correctly percent-escaped
	url := url.URL{}
	url.Path = fmt.Sprintf("/api/%s/files/%s/", version, filename)
	return url.String()
}

func getFileURLRE(version string) *regexp.Regexp {
	reString := fmt.Sprintf("^/api/%s/files/(.*)/$", regexp.QuoteMeta(version))
	return regexp.MustCompile(reString)
}

func getNetworksEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/networks/", version)
}

func getNetworkURL(version, name string) string {
	return fmt.Sprintf("/api/%s/networks/%s/", version, name)
}

func getNetworkURLRE(version string) *regexp.Regexp {
	reString := fmt.Sprintf("^/api/%s/networks/(.*)/$", regexp.QuoteMeta(version))
	return regexp.MustCompile(reString)
}

func getIPAddressesEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/ipaddresses/", version)
}

func getMACAddressURL(version, systemId, macAddress string) string {
	return fmt.Sprintf("/api/%s/nodes/%s/macs/%s/", version, systemId, url.QueryEscape(macAddress))
}

func getVersionURL(version string) string {
	return fmt.Sprintf("/api/%s/version/", version)
}

func getNodegroupsEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/nodegroups/", version)
}

func getNodegroupURL(version, uuid string) string {
	return fmt.Sprintf("/api/%s/nodegroups/%s/", version, uuid)
}

func getNodegroupsInterfacesURLRE(version string) *regexp.Regexp {
	reString := fmt.Sprintf("^/api/%s/nodegroups/([^/]*)/interfaces/$", regexp.QuoteMeta(version))
	return regexp.MustCompile(reString)
}

func getBootimagesURLRE(version string) *regexp.Regexp {
	reString := fmt.Sprintf("^/api/%s/nodegroups/([^/]*)/boot-images/$", regexp.QuoteMeta(version))
	return regexp.MustCompile(reString)
}

func getZonesEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/zones/", version)
}

func getTagsEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/tags/", version)
}

func getTagURL(version, tag_name string) string {
	return fmt.Sprintf("/api/%s/tags/%s/", version, tag_name)
}

func getTagURLRE(version string) *regexp.Regexp {
	reString := fmt.Sprintf("^/api/%s/tags/([^/]*)/$", regexp.QuoteMeta(version))
	return regexp.MustCompile(reString)
}

// Clear clears all the fake data stored and recorded by the test server
// (nodes, recorded operations, etc.).
func (server *TestServer) Clear() {
	server.nodes = make(map[string]MAASObject)
	server.ownedNodes = make(map[string]bool)
	server.nodesOperations = make([]string, 0)
	server.nodeOperations = make(map[string][]string)
	server.nodesOperationRequestValues = make([]url.Values, 0)
	server.nodeOperationRequestValues = make(map[string][]url.Values)
	server.nodeMetadata = make(map[string]Node)
	server.files = make(map[string]MAASObject)
	server.networks = make(map[string]MAASObject)
	server.networksPerNode = make(map[string][]string)
	server.ipAddressesPerNetwork = make(map[string][]string)
	server.tagsPerNode = make(map[string][]string)
	server.macAddressesPerNetwork = make(map[string]map[string]JSONObject)
	server.nodeDetails = make(map[string]string)
	server.bootImages = make(map[string][]JSONObject)
	server.nodegroupsInterfaces = make(map[string][]JSONObject)
	server.zones = make(map[string]JSONObject)
	server.tags = make(map[string]JSONObject)
	server.versionJSON = `{"capabilities": ["networks-management","static-ipaddresses","devices-management","network-deployment-ubuntu"]}`
	server.devices = make(map[string]*TestDevice)
	server.subnets = make(map[uint]TestSubnet)
	server.subnetNameToID = make(map[string]uint)
	server.nextSubnet = 1
	server.spaces = make(map[uint]*TestSpace)
	server.spaceNameToID = make(map[string]uint)
	server.nextSpace = 1
	server.vlans = make(map[int]TestVLAN)
	server.nextVLAN = 1
	server.staticRoutes = make(map[uint]*TestStaticRoute)
	server.nextStaticRoute = 1
}

// SetVersionJSON sets the JSON response (capabilities) returned from the
// /version/ endpoint.
func (server *TestServer) SetVersionJSON(json string) {
	server.versionJSON = json
}

// NodesOperations returns the list of operations performed at the /nodes/
// level.
func (server *TestServer) NodesOperations() []string {
	return server.nodesOperations
}

// NodeOperations returns the map containing the list of the operations
// performed for each node.
func (server *TestServer) NodeOperations() map[string][]string {
	return server.nodeOperations
}

// NodesOperationRequestValues returns the list of url.Values extracted
// from the request used when performing operations at the /nodes/ level.
func (server *TestServer) NodesOperationRequestValues() []url.Values {
	return server.nodesOperationRequestValues
}

// NodeOperationRequestValues returns the map containing the list of the
// url.Values extracted from the request used when performing operations
// on nodes.
func (server *TestServer) NodeOperationRequestValues() map[string][]url.Values {
	return server.nodeOperationRequestValues
}

func parseRequestValues(request *http.Request) url.Values {
	var requestValues url.Values
	if request.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		if request.PostForm == nil {
			if err := request.ParseForm(); err != nil {
				panic(err)
			}
		}
		requestValues = request.PostForm
	}
	return requestValues
}

func (server *TestServer) addNodesOperation(operation string, request *http.Request) url.Values {
	requestValues := parseRequestValues(request)
	server.nodesOperations = append(server.nodesOperations, operation)
	server.nodesOperationRequestValues = append(server.nodesOperationRequestValues, requestValues)
	return requestValues
}

func (server *TestServer) addNodeOperation(systemId, operation string, request *http.Request) url.Values {
	operations, present := server.nodeOperations[systemId]
	operationRequestValues, present2 := server.nodeOperationRequestValues[systemId]
	if present != present2 {
		panic("inconsistent state: nodeOperations and nodeOperationRequestValues don't have the same keys.")
	}
	requestValues := parseRequestValues(request)
	if !present {
		operations = []string{operation}
		operationRequestValues = []url.Values{requestValues}
	} else {
		operations = append(operations, operation)
		operationRequestValues = append(operationRequestValues, requestValues)
	}
	server.nodeOperations[systemId] = operations
	server.nodeOperationRequestValues[systemId] = operationRequestValues
	return requestValues
}

// NewNode creates a MAAS node.  The provided string should be a valid json
// string representing a map and contain a string value for the key
// 'system_id'.  e.g. `{"system_id": "mysystemid"}`.
// If one of these conditions is not met, NewNode panics.
func (server *TestServer) NewNode(jsonText string) MAASObject {
	var attrs map[string]interface{}
	err := json.Unmarshal([]byte(jsonText), &attrs)
	checkError(err)
	systemIdEntry, hasSystemId := attrs["system_id"]
	if !hasSystemId {
		panic("The given map json string does not contain a 'system_id' value.")
	}
	systemId := systemIdEntry.(string)
	attrs[resourceURI] = getNodeURL(server.version, systemId)
	if _, hasStatus := attrs["status"]; !hasStatus {
		attrs["status"] = NodeStatusDeployed
	}
	obj := newJSONMAASObject(attrs, server.client)
	server.nodes[systemId] = obj
	return obj
}

// Nodes returns a map associating all the nodes' system ids with the nodes'
// objects.
func (server *TestServer) Nodes() map[string]MAASObject {
	return server.nodes
}

// OwnedNodes returns a map whose keys represent the nodes that are currently
// allocated.
func (server *TestServer) OwnedNodes() map[string]bool {
	return server.ownedNodes
}

// NewFile creates a file in the test MAAS server.
func (server *TestServer) NewFile(filename string, filecontent []byte) MAASObject {
	attrs := make(map[string]interface{})
	attrs[resourceURI] = getFileURL(server.version, filename)
	base64Content := base64.StdEncoding.EncodeToString(filecontent)
	attrs["content"] = base64Content
	attrs["filename"] = filename

	// Allocate an arbitrary URL here.  It would be nice if the caller
	// could do this, but that would change the API and require many
	// changes.
	escapedName := url.QueryEscape(filename)
	attrs["anon_resource_uri"] = "/maas/1.0/files/?op=get_by_key&key=" + escapedName + "_key"

	obj := newJSONMAASObject(attrs, server.client)
	server.files[filename] = obj
	return obj
}

func (server *TestServer) Files() map[string]MAASObject {
	return server.files
}

// ChangeNode updates a node with the given key/value.
func (server *TestServer) ChangeNode(systemId, key, value string) {
	node, found := server.nodes[systemId]
	if !found {
		panic("No node with such 'system_id'.")
	}
	node.GetMap()[key] = maasify(server.client, value)
}

// NewIPAddress creates a new static IP address reservation for the
// given network/subnet and ipAddress. While networks is being deprecated
// try the given name as both a netowrk and a subnet.
func (server *TestServer) NewIPAddress(ipAddress, networkOrSubnet string) {
	_, foundNetwork := server.networks[networkOrSubnet]
	subnetID, foundSubnet := server.subnetNameToID[networkOrSubnet]

	if (foundNetwork || foundSubnet) == false {
		panic("No such network or subnet: " + networkOrSubnet)
	}
	if foundNetwork {
		ips, found := server.ipAddressesPerNetwork[networkOrSubnet]
		if found {
			ips = append(ips, ipAddress)
		} else {
			ips = []string{ipAddress}
		}
		server.ipAddressesPerNetwork[networkOrSubnet] = ips
	} else {
		subnet := server.subnets[subnetID]
		netIp := net.ParseIP(ipAddress)
		if netIp == nil {
			panic(ipAddress + " is invalid")
		}
		ip := IPFromNetIP(netIp)
		ip.Purpose = []string{"assigned-ip"}
		subnet.InUseIPAddresses = append(subnet.InUseIPAddresses, ip)
		server.subnets[subnetID] = subnet
	}
}

// RemoveIPAddress removes the given existing ipAddress and returns
// whether it was actually removed.
func (server *TestServer) RemoveIPAddress(ipAddress string) bool {
	for network, ips := range server.ipAddressesPerNetwork {
		for i, ip := range ips {
			if ip == ipAddress {
				ips = append(ips[:i], ips[i+1:]...)
				server.ipAddressesPerNetwork[network] = ips
				return true
			}
		}
	}
	for _, device := range server.devices {
		for i, addr := range device.IPAddresses {
			if addr == ipAddress {
				device.IPAddresses = append(device.IPAddresses[:i], device.IPAddresses[i+1:]...)
				return true
			}
		}
	}
	return false
}

// IPAddresses returns the map with network names as keys and slices
// of IP addresses belonging to each network as values.
func (server *TestServer) IPAddresses() map[string][]string {
	return server.ipAddressesPerNetwork
}

// NewNetwork creates a network in the test MAAS server
func (server *TestServer) NewNetwork(jsonText string) MAASObject {
	var attrs map[string]interface{}
	err := json.Unmarshal([]byte(jsonText), &attrs)
	checkError(err)
	nameEntry, hasName := attrs["name"]
	_, hasIP := attrs["ip"]
	_, hasNetmask := attrs["netmask"]
	if !hasName || !hasIP || !hasNetmask {
		panic("The given map json string does not contain a 'name', 'ip', or 'netmask' value.")
	}
	// TODO(gz): Sanity checking done on other fields
	name := nameEntry.(string)
	attrs[resourceURI] = getNetworkURL(server.version, name)
	obj := newJSONMAASObject(attrs, server.client)
	server.networks[name] = obj
	return obj
}

// NewNodegroupInterface adds a nodegroup-interface, for the specified
// nodegroup,  in the test MAAS server.
func (server *TestServer) NewNodegroupInterface(uuid, jsonText string) JSONObject {
	_, ok := server.bootImages[uuid]
	if !ok {
		panic("no nodegroup with the given UUID")
	}
	var attrs map[string]interface{}
	err := json.Unmarshal([]byte(jsonText), &attrs)
	checkError(err)
	requiredMembers := []string{"ip_range_high", "ip_range_low", "broadcast_ip", "static_ip_range_low", "static_ip_range_high", "name", "ip", "subnet_mask", "management", "interface"}
	for _, member := range requiredMembers {
		_, hasMember := attrs[member]
		if !hasMember {
			panic(fmt.Sprintf("The given map json string does not contain a required %q", member))
		}
	}
	obj := maasify(server.client, attrs)
	server.nodegroupsInterfaces[uuid] = append(server.nodegroupsInterfaces[uuid], obj)
	return obj
}

func (server *TestServer) ConnectNodeToNetwork(systemId, name string) {
	_, hasNode := server.nodes[systemId]
	if !hasNode {
		panic("no node with the given system id")
	}
	_, hasNetwork := server.networks[name]
	if !hasNetwork {
		panic("no network with the given name")
	}
	networkNames, _ := server.networksPerNode[systemId]
	server.networksPerNode[systemId] = append(networkNames, name)
}

func (server *TestServer) ConnectNodeToNetworkWithMACAddress(systemId, networkName, macAddress string) {
	node, hasNode := server.nodes[systemId]
	if !hasNode {
		panic("no node with the given system id")
	}
	if _, hasNetwork := server.networks[networkName]; !hasNetwork {
		panic("no network with the given name")
	}
	networkNames, _ := server.networksPerNode[systemId]
	server.networksPerNode[systemId] = append(networkNames, networkName)
	attrs := make(map[string]interface{})
	attrs[resourceURI] = getMACAddressURL(server.version, systemId, macAddress)
	attrs["mac_address"] = macAddress
	array := []JSONObject{}
	if set, ok := node.GetMap()["macaddress_set"]; ok {
		var err error
		array, err = set.GetArray()
		if err != nil {
			panic(err)
		}
	}
	array = append(array, maasify(server.client, attrs))
	node.GetMap()["macaddress_set"] = JSONObject{value: array, client: server.client}
	if _, ok := server.macAddressesPerNetwork[networkName]; !ok {
		server.macAddressesPerNetwork[networkName] = map[string]JSONObject{}
	}
	server.macAddressesPerNetwork[networkName][systemId] = maasify(server.client, attrs)
}

// AddBootImage adds a boot-image object to the specified nodegroup.
func (server *TestServer) AddBootImage(nodegroupUUID string, jsonText string) {
	var attrs map[string]interface{}
	err := json.Unmarshal([]byte(jsonText), &attrs)
	checkError(err)
	if _, ok := attrs["architecture"]; !ok {
		panic("The boot-image json string does not contain an 'architecture' value.")
	}
	if _, ok := attrs["release"]; !ok {
		panic("The boot-image json string does not contain a 'release' value.")
	}
	obj := maasify(server.client, attrs)
	server.bootImages[nodegroupUUID] = append(server.bootImages[nodegroupUUID], obj)
}

// AddZone adds a physical zone to the server.
func (server *TestServer) AddZone(name, description string) {
	attrs := map[string]interface{}{
		"name":        name,
		"description": description,
	}
	obj := maasify(server.client, attrs)
	server.zones[name] = obj
}

// AddTah adds a tag to the server.
func (server *TestServer) AddTag(name, comment string) {
	attrs := map[string]interface{}{
		"name":      name,
		"comment":   comment,
		resourceURI: getTagURL(server.version, name),
	}
	obj := maasify(server.client, attrs)
	server.tags[name] = obj
}

func (server *TestServer) AddDevice(device *TestDevice) {
	server.devices[device.SystemId] = device
}

func (server *TestServer) Devices() map[string]*TestDevice {
	return server.devices
}

// NewTestServer starts and returns a new MAAS test server. The caller should call Close when finished, to shut it down.
func NewTestServer(version string) *TestServer {
	server := &TestServer{version: version}

	serveMux := http.NewServeMux()
	devicesURL := getDevicesEndpoint(server.version)
	// Register handler for '/api/<version>/devices/*'.
	serveMux.HandleFunc(devicesURL, func(w http.ResponseWriter, r *http.Request) {
		devicesHandler(server, w, r)
	})
	nodesURL := getNodesEndpoint(server.version)
	// Register handler for '/api/<version>/nodes/*'.
	serveMux.HandleFunc(nodesURL, func(w http.ResponseWriter, r *http.Request) {
		nodesHandler(server, w, r)
	})
	filesURL := getFilesEndpoint(server.version)
	// Register handler for '/api/<version>/files/*'.
	serveMux.HandleFunc(filesURL, func(w http.ResponseWriter, r *http.Request) {
		filesHandler(server, w, r)
	})
	networksURL := getNetworksEndpoint(server.version)
	// Register handler for '/api/<version>/networks/'.
	serveMux.HandleFunc(networksURL, func(w http.ResponseWriter, r *http.Request) {
		networksHandler(server, w, r)
	})
	ipAddressesURL := getIPAddressesEndpoint(server.version)
	// Register handler for '/api/<version>/ipaddresses/'.
	serveMux.HandleFunc(ipAddressesURL, func(w http.ResponseWriter, r *http.Request) {
		ipAddressesHandler(server, w, r)
	})
	versionURL := getVersionURL(server.version)
	// Register handler for '/api/<version>/version/'.
	serveMux.HandleFunc(versionURL, func(w http.ResponseWriter, r *http.Request) {
		versionHandler(server, w, r)
	})
	// Register handler for '/api/<version>/nodegroups/*'.
	nodegroupsURL := getNodegroupsEndpoint(server.version)
	serveMux.HandleFunc(nodegroupsURL, func(w http.ResponseWriter, r *http.Request) {
		nodegroupsHandler(server, w, r)
	})

	// Register handler for '/api/<version>/zones/*'.
	zonesURL := getZonesEndpoint(server.version)
	serveMux.HandleFunc(zonesURL, func(w http.ResponseWriter, r *http.Request) {
		zonesHandler(server, w, r)
	})

	// Register handler for '/api/<version>/zones/*'.
	tagsURL := getTagsEndpoint(server.version)
	serveMux.HandleFunc(tagsURL, func(w http.ResponseWriter, r *http.Request) {
		tagsHandler(server, w, r)
	})

	subnetsURL := getSubnetsEndpoint(server.version)
	serveMux.HandleFunc(subnetsURL, func(w http.ResponseWriter, r *http.Request) {
		subnetsHandler(server, w, r)
	})

	spacesURL := getSpacesEndpoint(server.version)
	serveMux.HandleFunc(spacesURL, func(w http.ResponseWriter, r *http.Request) {
		spacesHandler(server, w, r)
	})

	staticRoutesURL := getStaticRoutesEndpoint(server.version)
	serveMux.HandleFunc(staticRoutesURL, func(w http.ResponseWriter, r *http.Request) {
		staticRoutesHandler(server, w, r)
	})

	vlansURL := getVLANsEndpoint(server.version)
	serveMux.HandleFunc(vlansURL, func(w http.ResponseWriter, r *http.Request) {
		vlansHandler(server, w, r)
	})

	var mu sync.Mutex
	singleFile := func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		serveMux.ServeHTTP(w, req)
	}

	newServer := httptest.NewServer(http.HandlerFunc(singleFile))
	client, err := NewAnonymousClient(newServer.URL, "1.0")
	checkError(err)
	server.Server = newServer
	server.serveMux = serveMux
	server.client = *client
	server.Clear()
	return server
}

// devicesHandler handles requests for '/api/<version>/devices/*'.
func devicesHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	op := values.Get("op")
	deviceURLRE := getDeviceURLRE(server.version)
	deviceURLMatch := deviceURLRE.FindStringSubmatch(r.URL.Path)
	devicesURL := getDevicesEndpoint(server.version)
	switch {
	case r.URL.Path == devicesURL:
		devicesTopLevelHandler(server, w, r, op)
	case deviceURLMatch != nil:
		// Request for a single device.
		deviceHandler(server, w, r, deviceURLMatch[1], op)
	default:
		// Default handler: not found.
		http.NotFoundHandler().ServeHTTP(w, r)
	}
}

// devicesTopLevelHandler handles a request for /api/<version>/devices/
// (with no device id following as part of the path).
func devicesTopLevelHandler(server *TestServer, w http.ResponseWriter, r *http.Request, op string) {
	switch {
	case r.Method == "GET" && op == "list":
		// Device listing operation.
		deviceListingHandler(server, w, r)
	case r.Method == "POST" && op == "new":
		newDeviceHandler(server, w, r)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func macMatches(mac string, device *TestDevice) bool {
	return contains(device.MACAddresses, mac)
}

// deviceListingHandler handles requests for '/devices/'.
func deviceListingHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	// TODO(mfoord): support filtering by hostname and id
	macs, hasMac := values["mac_address"]
	var matchedDevices []*TestDevice
	if !hasMac {
		for _, device := range server.devices {
			matchedDevices = append(matchedDevices, device)
		}
	} else {
		for _, mac := range macs {
			for _, device := range server.devices {
				if macMatches(mac, device) {
					matchedDevices = append(matchedDevices, device)
				}
			}
		}
	}
	deviceChunks := make([]string, len(matchedDevices))
	for i := range matchedDevices {
		deviceChunks[i] = renderDevice(matchedDevices[i])
	}
	json := fmt.Sprintf("[%v]", strings.Join(deviceChunks, ", "))

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, json)
}

var templateFuncs = template.FuncMap{
	"quotedList": func(items []string) string {
		var pieces []string
		for _, item := range items {
			pieces = append(pieces, fmt.Sprintf("%q", item))
		}
		return strings.Join(pieces, ", ")
	},
	"last": func(items []string) []string {
		if len(items) == 0 {
			return []string{}
		}
		return items[len(items)-1:]
	},
	"allButLast": func(items []string) []string {
		if len(items) < 2 {
			return []string{}
		}
		return items[0 : len(items)-1]
	},
}

const (
	// The json template for generating new devices.
	// TODO(mfoord): set resource_uri in MAC addresses
	deviceTemplate = `{
	"macaddress_set": [{{range .MACAddresses | allButLast}}
	    {
		"mac_address": "{{.}}"
	    },{{end}}{{range .MACAddresses | last}}
	    {
		"mac_address": "{{.}}"
	    }{{end}}
	],
	"zone": {
	    "resource_uri": "/MAAS/api/{{.APIVersion}}/zones/default/",
	    "name": "default",
	    "description": ""
	},
	"parent": "{{.Parent}}",
	"ip_addresses": [{{.IPAddresses | quotedList }}],
	"hostname": "{{.Hostname}}",
	"tag_names": [],
	"owner": "maas-admin",
	"system_id": "{{.SystemId}}",
	"resource_uri": "/MAAS/api/{{.APIVersion}}/devices/{{.SystemId}}/"
}`
)

func renderDevice(device *TestDevice) string {
	t := template.New("Device template")
	t = t.Funcs(templateFuncs)
	t, err := t.Parse(deviceTemplate)
	checkError(err)
	var buf bytes.Buffer
	err = t.Execute(&buf, device)
	checkError(err)
	return buf.String()
}

func getValue(values url.Values, value string) (string, bool) {
	result, hasResult := values[value]
	if !hasResult || len(result) != 1 || result[0] == "" {
		return "", false
	}
	return result[0], true
}

func getValues(values url.Values, key string) ([]string, bool) {
	result, hasResult := values[key]
	if !hasResult {
		return nil, false
	}
	var output []string
	for _, val := range result {
		if val != "" {
			output = append(output, val)
		}
	}
	if len(output) == 0 {
		return nil, false
	}
	return output, true
}

// newDeviceHandler creates, stores and returns new devices.
func newDeviceHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	checkError(err)
	values := r.PostForm

	// TODO(mfood): generate a "proper" uuid for the system Id.
	uuid, err := generateNonce()
	checkError(err)
	systemId := fmt.Sprintf("node-%v", uuid)
	// At least one MAC address must be specified.
	// TODO(mfoord) we only support a single MAC in the test server.
	macs, hasMacs := getValues(values, "mac_addresses")

	// hostname and parent are optional.
	// TODO(mfoord): we require both to be set in the test server.
	hostname, hasHostname := getValue(values, "hostname")
	parent, hasParent := getValue(values, "parent")
	if !hasHostname || !hasMacs || !hasParent {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	device := &TestDevice{
		MACAddresses: macs,
		APIVersion:   server.version,
		Parent:       parent,
		Hostname:     hostname,
		SystemId:     systemId,
	}

	deviceJSON := renderDevice(device)
	server.devices[systemId] = device

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, deviceJSON)
	return
}

// deviceHandler handles requests for '/api/<version>/devices/<system_id>/'.
func deviceHandler(server *TestServer, w http.ResponseWriter, r *http.Request, systemId string, operation string) {
	device, ok := server.devices[systemId]
	if !ok {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}
	if r.Method == "GET" {
		deviceJSON := renderDevice(device)
		if operation == "" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, deviceJSON)
			return
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	if r.Method == "POST" {
		if operation == "claim_sticky_ip_address" {
			err := r.ParseForm()
			checkError(err)
			values := r.PostForm
			// TODO(mfoord): support optional mac_address parameter
			// TODO(mfoord): requested_address should be optional
			// and we should generate one if it isn't provided.
			address, hasAddress := getValue(values, "requested_address")
			if !hasAddress {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			checkError(err)
			device.IPAddresses = append(device.IPAddresses, address)
			deviceJSON := renderDevice(device)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, deviceJSON)
			return
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else if r.Method == "DELETE" {
		delete(server.devices, systemId)
		w.WriteHeader(http.StatusNoContent)
		return

	}

	// TODO(mfoord): support PUT method for updating device
	http.NotFoundHandler().ServeHTTP(w, r)
}

// nodesHandler handles requests for '/api/<version>/nodes/*'.
func nodesHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	op := values.Get("op")
	nodeURLRE := getNodeURLRE(server.version)
	nodeURLMatch := nodeURLRE.FindStringSubmatch(r.URL.Path)
	nodesURL := getNodesEndpoint(server.version)
	switch {
	case r.URL.Path == nodesURL:
		nodesTopLevelHandler(server, w, r, op)
	case nodeURLMatch != nil:
		// Request for a single node.
		nodeHandler(server, w, r, nodeURLMatch[1], op)
	default:
		// Default handler: not found.
		http.NotFoundHandler().ServeHTTP(w, r)
	}
}

// nodeHandler handles requests for '/api/<version>/nodes/<system_id>/'.
func nodeHandler(server *TestServer, w http.ResponseWriter, r *http.Request, systemId string, operation string) {
	node, ok := server.nodes[systemId]
	if !ok {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}
	UUID, UUIDError := node.values["system_id"].GetString()
	if UUIDError == nil {
		i, err := JSONObjectFromStruct(server.client, server.nodeMetadata[UUID].Interfaces)
		checkError(err)
		node.values["interface_set"] = i
	}

	if r.Method == "GET" {
		if operation == "" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, marshalNode(node))
			return
		} else if operation == "details" {
			nodeDetailsHandler(server, w, r, systemId)
			return
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	if r.Method == "POST" {
		// The only operations supported are "start", "stop" and "release".
		if operation == "start" || operation == "stop" || operation == "release" {
			// Record operation on node.
			server.addNodeOperation(systemId, operation, r)

			if operation == "release" {
				delete(server.OwnedNodes(), systemId)
			}

			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, marshalNode(node))
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if r.Method == "DELETE" {
		delete(server.nodes, systemId)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.NotFoundHandler().ServeHTTP(w, r)
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// nodeListingHandler handles requests for '/nodes/'.
func nodeListingHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	ids, hasId := values["id"]
	var convertedNodes = []map[string]JSONObject{}
	for systemId, node := range server.nodes {
		if !hasId || contains(ids, systemId) {
			convertedNodes = append(convertedNodes, node.GetMap())
		}
	}
	res, err := json.MarshalIndent(convertedNodes, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// nodeDeploymentStatusHandler handles requests for '/nodes/?op=deployment_status'.
func nodeDeploymentStatusHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	nodes, _ := values["nodes"]
	var nodeStatus = make(map[string]interface{})
	for _, systemId := range nodes {
		node := server.nodes[systemId]
		field, err := node.GetField("status")
		if err != nil {
			continue
		}
		switch field {
		case NodeStatusDeployed:
			nodeStatus[systemId] = "Deployed"
		case NodeStatusFailedDeployment:
			nodeStatus[systemId] = "Failed deployment"
		default:
			nodeStatus[systemId] = "Not in Deployment"
		}
	}
	obj := maasify(server.client, nodeStatus)
	res, err := json.MarshalIndent(obj, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// findFreeNode looks for a node that is currently available, and
// matches the specified filter.
func findFreeNode(server *TestServer, filter url.Values) *MAASObject {
	for systemID, node := range server.Nodes() {
		_, present := server.OwnedNodes()[systemID]
		if !present {
			var agentName, nodeName, zoneName, tagName, mem, cpuCores, arch string
			for k := range filter {
				switch k {
				case "agent_name":
					agentName = filter.Get(k)
				case "name":
					nodeName = filter.Get(k)
				case "zone":
					zoneName = filter.Get(k)
				case "tags":
					tagName = filter.Get(k)
				case "mem":
					mem = filter.Get(k)
				case "arch":
					arch = filter.Get(k)
				case "cpu-cores":
					cpuCores = filter.Get(k)
				}
			}
			if nodeName != "" && !matchField(node, "hostname", nodeName) {
				continue
			}
			if zoneName != "" && !matchField(node, "zone", zoneName) {
				continue
			}
			if tagName != "" && !matchField(node, "tag_names", tagName) {
				continue
			}
			if mem != "" && !matchNumericField(node, "memory", mem) {
				continue
			}
			if arch != "" && !matchArchitecture(node, "architecture", arch) {
				continue
			}
			if cpuCores != "" && !matchNumericField(node, "cpu_count", cpuCores) {
				continue
			}
			if agentName != "" {
				agentNameObj := maasify(server.client, agentName)
				node.GetMap()["agent_name"] = agentNameObj
			} else {
				delete(node.GetMap(), "agent_name")
			}
			return &node
		}
	}
	return nil
}

func matchArchitecture(node MAASObject, k, v string) bool {
	field, err := node.GetField(k)
	if err != nil {
		return false
	}
	baseArch := strings.Split(field, "/")
	return v == baseArch[0]
}

func matchNumericField(node MAASObject, k, v string) bool {
	field, ok := node.GetMap()[k]
	if !ok {
		return false
	}
	nodeVal, err := field.GetFloat64()
	if err != nil {
		return false
	}
	constraintVal, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return false
	}
	return constraintVal <= nodeVal
}

func matchField(node MAASObject, k, v string) bool {
	field, err := node.GetField(k)
	if err != nil {
		return false
	}
	return field == v
}

// nodesAcquireHandler simulates acquiring a node.
func nodesAcquireHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	requestValues := server.addNodesOperation("acquire", r)
	node := findFreeNode(server, requestValues)
	if node == nil {
		w.WriteHeader(http.StatusConflict)
	} else {
		systemId, err := node.GetField("system_id")
		checkError(err)
		server.OwnedNodes()[systemId] = true
		res, err := json.MarshalIndent(node, "", "  ")
		checkError(err)
		// Record operation.
		server.addNodeOperation(systemId, "acquire", r)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(res))
	}
}

// nodesReleaseHandler simulates releasing multiple nodes.
func nodesReleaseHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	server.addNodesOperation("release", r)
	values := server.NodesOperationRequestValues()
	systemIds := values[len(values)-1]["nodes"]
	var unknown []string
	for _, systemId := range systemIds {
		if _, ok := server.Nodes()[systemId]; !ok {
			unknown = append(unknown, systemId)
		}
	}
	if len(unknown) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Unknown node(s): %s.", strings.Join(unknown, ", "))
		return
	}
	var releasedNodes = []map[string]JSONObject{}
	for _, systemId := range systemIds {
		if _, ok := server.OwnedNodes()[systemId]; !ok {
			continue
		}
		delete(server.OwnedNodes(), systemId)
		node := server.Nodes()[systemId]
		releasedNodes = append(releasedNodes, node.GetMap())
	}
	res, err := json.MarshalIndent(releasedNodes, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// nodesTopLevelHandler handles a request for /api/<version>/nodes/
// (with no node id following as part of the path).
func nodesTopLevelHandler(server *TestServer, w http.ResponseWriter, r *http.Request, op string) {
	switch {
	case r.Method == "GET" && op == "list":
		// Node listing operation.
		nodeListingHandler(server, w, r)
	case r.Method == "GET" && op == "deployment_status":
		// Node deployment_status operation.
		nodeDeploymentStatusHandler(server, w, r)
	case r.Method == "POST" && op == "acquire":
		nodesAcquireHandler(server, w, r)
	case r.Method == "POST" && op == "release":
		nodesReleaseHandler(server, w, r)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// AddNodeDetails stores node details, expected in XML format.
func (server *TestServer) AddNodeDetails(systemId, xmlText string) {
	_, hasNode := server.nodes[systemId]
	if !hasNode {
		panic("no node with the given system id")
	}
	server.nodeDetails[systemId] = xmlText
}

const lldpXML = `
<?xml version="1.0" encoding="UTF-8"?>
<lldp label="LLDP neighbors"/>`

// nodeDetailesHandler handles requests for '/api/<version>/nodes/<system_id>/?op=details'.
func nodeDetailsHandler(server *TestServer, w http.ResponseWriter, r *http.Request, systemId string) {
	attrs := make(map[string]interface{})
	attrs["lldp"] = lldpXML
	xmlText, _ := server.nodeDetails[systemId]
	attrs["lshw"] = []byte(xmlText)
	res, err := bson.Marshal(attrs)
	checkError(err)
	w.Header().Set("Content-Type", "application/bson")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// filesHandler handles requests for '/api/<version>/files/*'.
func filesHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	op := values.Get("op")
	fileURLRE := getFileURLRE(server.version)
	fileURLMatch := fileURLRE.FindStringSubmatch(r.URL.Path)
	fileListingURL := getFilesEndpoint(server.version)
	switch {
	case r.Method == "GET" && op == "list" && r.URL.Path == fileListingURL:
		// File listing operation.
		fileListingHandler(server, w, r)
	case op == "get" && r.Method == "GET" && r.URL.Path == fileListingURL:
		getFileHandler(server, w, r)
	case op == "add" && r.Method == "POST" && r.URL.Path == fileListingURL:
		addFileHandler(server, w, r)
	case fileURLMatch != nil:
		// Request for a single file.
		fileHandler(server, w, r, fileURLMatch[1], op)
	default:
		// Default handler: not found.
		http.NotFoundHandler().ServeHTTP(w, r)
	}

}

// listFilenames returns the names of those uploaded files whose names start
// with the given prefix, sorted lexicographically.
func listFilenames(server *TestServer, prefix string) []string {
	var filenames = make([]string, 0)
	for filename := range server.files {
		if strings.HasPrefix(filename, prefix) {
			filenames = append(filenames, filename)
		}
	}
	sort.Strings(filenames)
	return filenames
}

// stripFileContent copies a map of attributes representing an uploaded file,
// but with the "content" attribute removed.
func stripContent(original map[string]JSONObject) map[string]JSONObject {
	newMap := make(map[string]JSONObject, len(original)-1)
	for key, value := range original {
		if key != "content" {
			newMap[key] = value
		}
	}
	return newMap
}

// fileListingHandler handles requests for '/api/<version>/files/?op=list'.
func fileListingHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	prefix := values.Get("prefix")
	filenames := listFilenames(server, prefix)

	// Build a sorted list of the files as map[string]JSONObject objects.
	convertedFiles := make([]map[string]JSONObject, 0)
	for _, filename := range filenames {
		// The "content" attribute is not in the listing.
		fileMap := stripContent(server.files[filename].GetMap())
		convertedFiles = append(convertedFiles, fileMap)
	}
	res, err := json.MarshalIndent(convertedFiles, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// fileHandler handles requests for '/api/<version>/files/<filename>/'.
func fileHandler(server *TestServer, w http.ResponseWriter, r *http.Request, filename string, operation string) {
	switch {
	case r.Method == "DELETE":
		delete(server.files, filename)
		w.WriteHeader(http.StatusOK)
	case r.Method == "GET":
		// Retrieve a file's information (including content) as a JSON
		// object.
		file, ok := server.files[filename]
		if !ok {
			http.NotFoundHandler().ServeHTTP(w, r)
			return
		}
		jsonText, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			panic(err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jsonText)
	default:
		// Default handler: not found.
		http.NotFoundHandler().ServeHTTP(w, r)
	}
}

// InternalError replies to the request with an HTTP 500 internal error.
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// getFileHandler handles requests for
// '/api/<version>/files/?op=get&filename=filename'.
func getFileHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	filename := values.Get("filename")
	file, found := server.files[filename]
	if !found {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}
	base64Content, err := file.GetField("content")
	if err != nil {
		InternalError(w, r, err)
		return
	}
	content, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		InternalError(w, r, err)
		return
	}
	w.Write(content)
}

func readMultipart(upload *multipart.FileHeader) ([]byte, error) {
	file, err := upload.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	return ioutil.ReadAll(reader)
}

// filesHandler handles requests for '/api/<version>/files/?op=add&filename=filename'.
func addFileHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10000000)
	checkError(err)

	filename := r.Form.Get("filename")
	if filename == "" {
		panic("upload has no filename")
	}

	uploads := r.MultipartForm.File
	if len(uploads) != 1 {
		panic("the payload should contain one file and one file only")
	}
	var upload *multipart.FileHeader
	for _, uploadContent := range uploads {
		upload = uploadContent[0]
	}
	content, err := readMultipart(upload)
	checkError(err)
	server.NewFile(filename, content)
	w.WriteHeader(http.StatusOK)
}

// networkListConnectedMACSHandler handles requests for '/api/<version>/networks/<network>/?op=list_connected_macs'
func networkListConnectedMACSHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	networkURLRE := getNetworkURLRE(server.version)
	networkURLREMatch := networkURLRE.FindStringSubmatch(r.URL.Path)
	if networkURLREMatch == nil {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}
	networkName := networkURLREMatch[1]
	convertedMacAddresses := []map[string]JSONObject{}
	if macAddresses, ok := server.macAddressesPerNetwork[networkName]; ok {
		for _, macAddress := range macAddresses {
			m, err := macAddress.GetMap()
			checkError(err)
			convertedMacAddresses = append(convertedMacAddresses, m)
		}
	}
	res, err := json.MarshalIndent(convertedMacAddresses, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// networksHandler handles requests for '/api/<version>/networks/?node=system_id'.
func networksHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		panic("only networks GET operation implemented")
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	op := values.Get("op")
	systemId := values.Get("node")
	if op == "list_connected_macs" {
		networkListConnectedMACSHandler(server, w, r)
		return
	}
	if op != "" {
		panic("only list_connected_macs and default operations implemented")
	}
	if systemId == "" {
		panic("network missing associated node system id")
	}
	networks := []MAASObject{}
	if networkNames, hasNetworks := server.networksPerNode[systemId]; hasNetworks {
		networks = make([]MAASObject, len(networkNames))
		for i, networkName := range networkNames {
			networks[i] = server.networks[networkName]
		}
	}
	res, err := json.MarshalIndent(networks, "", "  ")
	checkError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// ipAddressesHandler handles requests for '/api/<version>/ipaddresses/'.
func ipAddressesHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	checkError(err)
	values := r.Form
	op := values.Get("op")

	switch r.Method {
	case "GET":
		if op != "" {
			panic("expected empty op for GET, got " + op)
		}
		listIPAddressesHandler(server, w, r)
		return
	case "POST":
		switch op {
		case "reserve":
			reserveIPAddressHandler(server, w, r, values.Get("network"), values.Get("requested_address"))
			return
		case "release":
			releaseIPAddressHandler(server, w, r, values.Get("ip"))
			return
		default:
			panic("expected op=release|reserve for POST, got " + op)
		}
	}
	http.NotFoundHandler().ServeHTTP(w, r)
}

func marshalIPAddress(server *TestServer, ipAddress string) (JSONObject, error) {
	jsonTemplate := `{"alloc_type": 4, "ip": %q, "resource_uri": %q, "created": %q}`
	uri := getIPAddressesEndpoint(server.version)
	now := time.Now().UTC().Format(time.RFC3339)
	bytes := []byte(fmt.Sprintf(jsonTemplate, ipAddress, uri, now))
	return Parse(server.client, bytes)
}

func badRequestError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprint(w, err.Error())
}

func listIPAddressesHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	results := []MAASObject{}
	for _, ips := range server.IPAddresses() {
		for _, ip := range ips {
			jsonObj, err := marshalIPAddress(server, ip)
			if err != nil {
				badRequestError(w, err)
				return
			}
			maasObj, err := jsonObj.GetMAASObject()
			if err != nil {
				badRequestError(w, err)
				return
			}
			results = append(results, maasObj)
		}
	}
	res, err := json.MarshalIndent(results, "", "  ")
	checkError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

func reserveIPAddressHandler(server *TestServer, w http.ResponseWriter, r *http.Request, network, reqAddress string) {
	_, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		badRequestError(w, fmt.Errorf("Invalid network parameter %s", network))
		return
	}
	if reqAddress != "" {
		// Validate "requested_address" parameter.
		reqIP := net.ParseIP(reqAddress)
		if reqIP == nil {
			badRequestError(w, fmt.Errorf("failed to detect a valid IP address from u'%s'", reqAddress))
			return
		}
		if !ipNet.Contains(reqIP) {
			badRequestError(w, fmt.Errorf("%s is not inside the range %s", reqAddress, ipNet.String()))
			return
		}
	}
	// Find the network name matching the parsed CIDR.
	foundNetworkName := ""
	for netName, netObj := range server.networks {
		// Get the "ip" and "netmask" attributes of the network.
		netIP, err := netObj.GetField("ip")
		checkError(err)
		netMask, err := netObj.GetField("netmask")
		checkError(err)

		// Convert the netmask string to net.IPMask.
		parts := strings.Split(netMask, ".")
		ipMask := make(net.IPMask, len(parts))
		for i, part := range parts {
			intPart, err := strconv.Atoi(part)
			checkError(err)
			ipMask[i] = byte(intPart)
		}
		netNet := &net.IPNet{IP: net.ParseIP(netIP), Mask: ipMask}
		if netNet.String() == network {
			// Exact match found.
			foundNetworkName = netName
			break
		}
	}
	if foundNetworkName == "" {
		badRequestError(w, fmt.Errorf("No network found matching %s", network))
		return
	}
	ips, found := server.ipAddressesPerNetwork[foundNetworkName]
	if !found {
		// This will be the first address.
		ips = []string{}
	}
	reservedIP := ""
	if reqAddress != "" {
		// Use what the user provided. NOTE: Because this is testing
		// code, no duplicates check is done.
		reservedIP = reqAddress
	} else {
		// Generate an IP in the network range by incrementing the
		// last byte of the network's IP.
		firstIP := ipNet.IP
		firstIP[len(firstIP)-1] += byte(len(ips) + 1)
		reservedIP = firstIP.String()
	}
	ips = append(ips, reservedIP)
	server.ipAddressesPerNetwork[foundNetworkName] = ips
	jsonObj, err := marshalIPAddress(server, reservedIP)
	checkError(err)
	maasObj, err := jsonObj.GetMAASObject()
	checkError(err)
	res, err := json.MarshalIndent(maasObj, "", "  ")
	checkError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

func releaseIPAddressHandler(server *TestServer, w http.ResponseWriter, r *http.Request, ip string) {
	if netIP := net.ParseIP(ip); netIP == nil {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}
	if server.RemoveIPAddress(ip) {
		w.WriteHeader(http.StatusOK)
		return
	}
	http.NotFoundHandler().ServeHTTP(w, r)
}

// versionHandler handles requests for '/api/<version>/version/'.
func versionHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		panic("only version GET operation implemented")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, server.versionJSON)
}

// nodegroupsHandler handles requests for '/api/<version>/nodegroups/*'.
func nodegroupsHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	op := values.Get("op")
	bootimagesURLRE := getBootimagesURLRE(server.version)
	bootimagesURLMatch := bootimagesURLRE.FindStringSubmatch(r.URL.Path)
	nodegroupsInterfacesURLRE := getNodegroupsInterfacesURLRE(server.version)
	nodegroupsInterfacesURLMatch := nodegroupsInterfacesURLRE.FindStringSubmatch(r.URL.Path)
	nodegroupsURL := getNodegroupsEndpoint(server.version)
	switch {
	case r.URL.Path == nodegroupsURL:
		nodegroupsTopLevelHandler(server, w, r, op)
	case bootimagesURLMatch != nil:
		bootimagesHandler(server, w, r, bootimagesURLMatch[1], op)
	case nodegroupsInterfacesURLMatch != nil:
		nodegroupsInterfacesHandler(server, w, r, nodegroupsInterfacesURLMatch[1], op)
	default:
		// Default handler: not found.
		http.NotFoundHandler().ServeHTTP(w, r)
	}
}

// nodegroupsTopLevelHandler handles requests for '/api/<version>/nodegroups/'.
func nodegroupsTopLevelHandler(server *TestServer, w http.ResponseWriter, r *http.Request, op string) {
	if r.Method != "GET" || op != "list" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodegroups := []JSONObject{}
	for uuid := range server.bootImages {
		attrs := map[string]interface{}{
			"uuid":      uuid,
			resourceURI: getNodegroupURL(server.version, uuid),
		}
		obj := maasify(server.client, attrs)
		nodegroups = append(nodegroups, obj)
	}

	res, err := json.MarshalIndent(nodegroups, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// bootimagesHandler handles requests for '/api/<version>/nodegroups/<uuid>/boot-images/'.
func bootimagesHandler(server *TestServer, w http.ResponseWriter, r *http.Request, nodegroupUUID, op string) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	bootImages, ok := server.bootImages[nodegroupUUID]
	if !ok {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}

	res, err := json.MarshalIndent(bootImages, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// nodegroupsInterfacesHandler handles requests for '/api/<version>/nodegroups/<uuid>/interfaces/'
func nodegroupsInterfacesHandler(server *TestServer, w http.ResponseWriter, r *http.Request, nodegroupUUID, op string) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_, ok := server.bootImages[nodegroupUUID]
	if !ok {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}

	interfaces, ok := server.nodegroupsInterfaces[nodegroupUUID]
	if !ok {
		// we already checked the nodegroup exists, so return an empty list
		interfaces = []JSONObject{}
	}
	res, err := json.MarshalIndent(interfaces, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// zonesHandler handles requests for '/api/<version>/zones/'.
func zonesHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(server.zones) == 0 {
		// Until a zone is registered, behave as if the endpoint
		// does not exist. This way we can simulate older MAAS
		// servers that do not support zones.
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}

	zones := make([]JSONObject, 0, len(server.zones))
	for _, zone := range server.zones {
		zones = append(zones, zone)
	}
	res, err := json.MarshalIndent(zones, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(res))
}

// tagsHandler handles requests for '/api/<version>/tags/'.
func tagsHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	tagURLRE := getTagURLRE(server.version)
	tagURLMatch := tagURLRE.FindStringSubmatch(r.URL.Path)
	tagsURL := getTagsEndpoint(server.version)
	err := r.ParseForm()
	checkError(err)
	values := r.PostForm
	names, hasName := getValues(values, "name")
	quary, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	op := quary.Get("op")
	if r.URL.Path == tagsURL {
		if r.Method == "GET" {
			tags := make([]JSONObject, 0, len(server.zones))
			for _, tag := range server.tags {
				tags = append(tags, tag)
			}
			res, err := json.MarshalIndent(tags, "", "  ")
			checkError(err)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, string(res))
		} else if r.Method == "POST" && hasName {
			if op == "" || op == "new" {
				for _, name := range names {
					newTagHandler(server, w, r, name, values)
				}
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	} else if tagURLMatch != nil {
		// Request for a single tag
		tagHandler(server, w, r, tagURLMatch[1], op, values)
	} else {
		http.NotFoundHandler().ServeHTTP(w, r)
	}
}

// newTagHandler creates, stores and returns new tag.
func newTagHandler(server *TestServer, w http.ResponseWriter, r *http.Request, name string, values url.Values) {
	comment, hascomment := getValue(values, "comment")
	var attrs map[string]interface{}
	if hascomment {
		attrs = map[string]interface{}{
			"name":      name,
			"comment":   comment,
			resourceURI: getTagURL(server.version, name),
		}
	} else {
		attrs = map[string]interface{}{
			"name":      name,
			resourceURI: getTagURL(server.version, name),
		}
	}
	obj := maasify(server.client, attrs)
	server.tags[name] = obj
	res, err := json.MarshalIndent(obj, "", "  ")
	checkError(err)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, res)
}

// tagHandler handles requests for '/api/<version>/tag/<name>/'.
func tagHandler(server *TestServer, w http.ResponseWriter, r *http.Request, name string, operation string, values url.Values) {
	switch r.Method {
	case "GET":
		switch operation {
		case "node":
			var convertedNodes = []map[string]JSONObject{}
			for systemID, node := range server.nodes {
				for _, nodetag := range server.tagsPerNode[systemID] {
					if name == nodetag {
						convertedNodes = append(convertedNodes, node.GetMap())
					}
				}
			}
			res, err := json.MarshalIndent(convertedNodes, "", "  ")
			checkError(err)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, string(res))
		default:
			res, err := json.MarshalIndent(server.tags[name], "", "  ")
			checkError(err)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, string(res))
		}
	case "POST":
		if operation == "update_nodes" {
			addNodes, hasAdd := getValues(values, "add")
			delNodes, hasRemove := getValues(values, "remove")
			addremovecount := map[string]int{"add": len(addNodes), "remove": len(delNodes)}
			if !hasAdd && !hasRemove {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, systemID := range addNodes {
				_, ok := server.nodes[systemID]
				if !ok {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var newTags []string
				for _, tag := range server.tagsPerNode[systemID] {
					if tag != name {
						newTags = append(newTags, tag)
					}
				}
				server.tagsPerNode[systemID] = append(newTags, name)
				newTagsObj := make([]JSONObject, len(server.tagsPerNode[systemID]))
				for i, tagsofnode := range server.tagsPerNode[systemID] {
					newTagsObj[i] = server.tags[tagsofnode]
				}
				tagNamesObj := JSONObject{
					value: newTagsObj,
				}
				server.nodes[systemID].values["tag_names"] = tagNamesObj
			}
			for _, systemID := range delNodes {
				_, ok := server.nodes[systemID]
				if !ok {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var newTags []string
				for _, tag := range server.tagsPerNode[systemID] {
					if tag != name {
						newTags = append(newTags, tag)
					}
				}
				server.tagsPerNode[systemID] = newTags
				newTagsObj := make([]JSONObject, len(server.tagsPerNode[systemID]))
				for i, tagsofnode := range server.tagsPerNode[systemID] {
					newTagsObj[i] = server.tags[tagsofnode]
				}
				tagNamesObj := JSONObject{
					value: newTagsObj,
				}
				server.nodes[systemID].values["tag_names"] = tagNamesObj
			}
			res, err := json.MarshalIndent(addremovecount, "", "  ")
			checkError(err)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, string(res))
		}
	case "PUT":
		newTagHandler(server, w, r, name, values)
	case "DELETE":
		delete(server.tags, name)
		w.WriteHeader(http.StatusOK)
	}
}
