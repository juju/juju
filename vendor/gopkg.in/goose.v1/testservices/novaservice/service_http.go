// Nova double testing service - HTTP API implementation

package novaservice

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"gopkg.in/goose.v1/errors"
	"gopkg.in/goose.v1/nova"
	"gopkg.in/goose.v1/testservices"
	"gopkg.in/goose.v1/testservices/identityservice"
)

const authToken = "X-Auth-Token"

// errorResponse defines a single HTTP error response.
type errorResponse struct {
	code        int
	body        string
	contentType string
	errorText   string
	headers     map[string]string
	nova        *Nova
}

// verbatim real Nova responses (as errors).
var (
	errUnauthorized = &errorResponse{
		http.StatusUnauthorized,
		`401 Unauthorized

This server could not verify that you are authorized to access the ` +
			`document you requested. Either you supplied the wrong ` +
			`credentials (e.g., bad password), or your browser does ` +
			`not understand how to supply the credentials required.

 Authentication required
`,
		"text/plain; charset=UTF-8",
		"unauthorized request",
		nil,
		nil,
	}
	errForbidden = &errorResponse{
		http.StatusForbidden,
		`{"forbidden": {"message": "Policy doesn't allow compute_extension:` +
			`flavormanage to be performed.", "code": 403}}`,
		"application/json; charset=UTF-8",
		"forbidden flavors request",
		nil,
		nil,
	}
	errBadRequest = &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "Malformed request url", "code": 400}}`,
		"application/json; charset=UTF-8",
		"bad request base path or URL",
		nil,
		nil,
	}
	errBadRequest2 = &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "The server could not comply with the ` +
			`request since it is either malformed or otherwise incorrect.", "code": 400}}`,
		"application/json; charset=UTF-8",
		"bad request URL",
		nil,
		nil,
	}
	errBadRequest3 = &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "Malformed request body", "code": 400}}`,
		"application/json; charset=UTF-8",
		"bad request body",
		nil,
		nil,
	}
	errBadRequestDuplicateValue = &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "entity already exists", "code": 400}}`,
		"application/json; charset=UTF-8",
		"duplicate value",
		nil,
		nil,
	}
	errBadRequestSrvName = &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "Server name is not defined", "code": 400}}`,
		"application/json; charset=UTF-8",
		"bad request - missing server name",
		nil,
		nil,
	}
	errBadRequestSrvFlavor = &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "Missing flavorRef attribute", "code": 400}}`,
		"application/json; charset=UTF-8",
		"bad request - missing flavorRef",
		nil,
		nil,
	}
	errBadRequestSrvImage = &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "Missing imageRef attribute", "code": 400}}`,
		"application/json; charset=UTF-8",
		"bad request - missing imageRef",
		nil,
		nil,
	}
	errNotFound = &errorResponse{
		http.StatusNotFound,
		`404 Not Found

The resource could not be found.


`,
		"text/plain; charset=UTF-8",
		"resource not found",
		nil,
		nil,
	}
	errNotFoundJSON = &errorResponse{
		http.StatusNotFound,
		`{"itemNotFound": {"message": "The resource could not be found.", "code": 404}}`,
		"application/json; charset=UTF-8",
		"resource not found",
		nil,
		nil,
	}
	errNotFoundJSONSG = &errorResponse{
		http.StatusNotFound,
		`{"itemNotFound": {"message": "Security group $ID$ not found.", "code": 404}}`,
		"application/json; charset=UTF-8",
		"",
		nil,
		nil,
	}
	errNotFoundJSONSGR = &errorResponse{
		http.StatusNotFound,
		`{"itemNotFound": {"message": "Rule ($ID$) not found.", "code": 404}}`,
		"application/json; charset=UTF-8",
		"security rule not found",
		nil,
		nil,
	}
	errMultipleChoices = &errorResponse{
		http.StatusMultipleChoices,
		`{"choices": [{"status": "CURRENT", "media-types": [{"base": ` +
			`"application/xml", "type": "application/vnd.openstack.compute+` +
			`xml;version=2"}, {"base": "application/json", "type": "application/` +
			`vnd.openstack.compute+json;version=2"}], "id": "v2.0", "links": ` +
			`[{"href": "$ENDPOINT$$URL$", "rel": "self"}]}]}`,
		"application/json",
		"multiple URL redirection choices",
		nil,
		nil,
	}
	errNoVersion = &errorResponse{
		http.StatusOK,
		`{"versions": [{"status": "CURRENT", "updated": "2011-01-21` +
			`T11:33:21Z", "id": "v2.0", "links": [{"href": "$ENDPOINT$", "rel": "self"}]}]}`,
		"application/json",
		"no version specified in URL",
		nil,
		nil,
	}
	errVersionsLinks = &errorResponse{
		http.StatusOK,
		`{"version": {"status": "CURRENT", "updated": "2011-01-21T11` +
			`:33:21Z", "media-types": [{"base": "application/xml", "type": ` +
			`"application/vnd.openstack.compute+xml;version=2"}, {"base": ` +
			`"application/json", "type": "application/vnd.openstack.compute` +
			`+json;version=2"}], "id": "v2.0", "links": [{"href": "$ENDPOINT$"` +
			`, "rel": "self"}, {"href": "http://docs.openstack.org/api/openstack` +
			`-compute/1.1/os-compute-devguide-1.1.pdf", "type": "application/pdf` +
			`", "rel": "describedby"}, {"href": "http://docs.openstack.org/api/` +
			`openstack-compute/1.1/wadl/os-compute-1.1.wadl", "type": ` +
			`"application/vnd.sun.wadl+xml", "rel": "describedby"}]}}`,
		"application/json",
		"version missing from URL",
		nil,
		nil,
	}
	errNotImplemented = &errorResponse{
		http.StatusNotImplemented,
		"501 Not Implemented",
		"text/plain; charset=UTF-8",
		"not implemented",
		nil,
		nil,
	}
	errNoGroupId = &errorResponse{
		errorText: "no security group id given",
	}
	errRateLimitExceeded = &errorResponse{
		http.StatusRequestEntityTooLarge,
		"",
		"text/plain; charset=UTF-8",
		"too many requests",
		// RFC says that Retry-After should be an int, but we don't want to wait an entire second during the test suite.
		map[string]string{"Retry-After": "0.001"},
		nil,
	}
	errNoMoreFloatingIPs = &errorResponse{
		http.StatusNotFound,
		"Zero floating ips available.",
		"text/plain; charset=UTF-8",
		"zero floating ips available",
		nil,
		nil,
	}
	errIPLimitExceeded = &errorResponse{
		http.StatusRequestEntityTooLarge,
		"Maximum number of floating ips exceeded.",
		"text/plain; charset=UTF-8",
		"maximum number of floating ips exceeded",
		nil,
		nil,
	}
)

func (e *errorResponse) Error() string {
	return e.errorText
}

// requestBody returns the body for the error response, replacing
// $ENDPOINT$, $URL$, $ID$, and $ERROR$ in e.body with the values from
// the request.
func (e *errorResponse) requestBody(r *http.Request) []byte {
	url := strings.TrimLeft(r.URL.Path, "/")
	body := e.body
	if body != "" {
		if e.nova != nil {
			body = strings.Replace(body, "$ENDPOINT$", e.nova.endpointURL(true, "/"), -1)
		}
		body = strings.Replace(body, "$URL$", url, -1)
		body = strings.Replace(body, "$ERROR$", e.Error(), -1)
		if slash := strings.LastIndex(url, "/"); slash != -1 {
			body = strings.Replace(body, "$ID$", url[slash+1:], -1)
		}
	}
	return []byte(body)
}

func (e *errorResponse) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if e.contentType != "" {
		w.Header().Set("Content-Type", e.contentType)
	}
	body := e.requestBody(r)
	if e.headers != nil {
		for h, v := range e.headers {
			w.Header().Set(h, v)
		}
	}
	// workaround for https://code.google.com/p/go/issues/detail?id=4454
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if e.code != 0 {
		w.WriteHeader(e.code)
	}
	if len(body) > 0 {
		w.Write(body)
	}
}

type novaHandler struct {
	n      *Nova
	method func(n *Nova, w http.ResponseWriter, r *http.Request) error
}

func userInfo(i identityservice.IdentityService, r *http.Request) (*identityservice.UserInfo, error) {
	return i.FindUser(r.Header.Get(authToken))
}

func (h *novaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// handle invalid X-Auth-Token header
	_, err := userInfo(h.n.IdentityService, r)
	if err != nil {
		errUnauthorized.ServeHTTP(w, r)
		return
	}
	// handle trailing slash in the path
	if strings.HasSuffix(path, "/") && path != "/" {
		errNotFound.ServeHTTP(w, r)
		return
	}
	err = h.method(h.n, w, r)
	if err == nil {
		return
	}
	var resp http.Handler

	if err == testservices.RateLimitExceededError {
		resp = errRateLimitExceeded
	} else if err == testservices.NoMoreFloatingIPs {
		resp = errNoMoreFloatingIPs
	} else if err == testservices.IPLimitExceeded {
		resp = errIPLimitExceeded
	} else {
		resp, _ = err.(http.Handler)
		if resp == nil {
			code, encodedErr := errorJSONEncode(err)
			resp = &errorResponse{
				code,
				encodedErr,
				"application/json",
				err.Error(),
				nil,
				h.n,
			}
		}
	}
	resp.ServeHTTP(w, r)
}

func writeResponse(w http.ResponseWriter, code int, body []byte) {
	// workaround for https://code.google.com/p/go/issues/detail?id=4454
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(code)
	w.Write(body)
}

// sendJSON sends the specified response serialized as JSON.
func sendJSON(code int, resp interface{}, w http.ResponseWriter, r *http.Request) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	writeResponse(w, code, data)
	return nil
}

func (n *Nova) handler(method func(n *Nova, w http.ResponseWriter, r *http.Request) error) http.Handler {
	return &novaHandler{n, method}
}

func (n *Nova) handleRoot(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/" {
		return errNoVersion
	}
	return errMultipleChoices
}

func (n *Nova) HandleRoot(w http.ResponseWriter, r *http.Request) {
	n.handler((*Nova).handleRoot).ServeHTTP(w, r)
}

// handleFlavors handles the flavors HTTP API.
func (n *Nova) handleFlavors(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		if flavorId := path.Base(r.URL.Path); flavorId != "flavors" {
			flavor, err := n.flavor(flavorId)
			if err != nil {
				return errNotFound
			}
			resp := struct {
				Flavor nova.FlavorDetail `json:"flavor"`
			}{*flavor}
			return sendJSON(http.StatusOK, resp, w, r)
		}
		entities := n.allFlavorsAsEntities()
		if len(entities) == 0 {
			entities = []nova.Entity{}
		}
		resp := struct {
			Flavors []nova.Entity `json:"flavors"`
		}{entities}
		return sendJSON(http.StatusOK, resp, w, r)
	case "POST":
		if flavorId := path.Base(r.URL.Path); flavorId != "flavors" {
			return errNotFound
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return errBadRequest2
		}
		return errNotImplemented
	case "PUT":
		if flavorId := path.Base(r.URL.Path); flavorId != "flavors" {
			return errNotFoundJSON
		}
		return errNotFound
	case "DELETE":
		if flavorId := path.Base(r.URL.Path); flavorId != "flavors" {
			return errForbidden
		}
		return errNotFound
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleFlavorsDetail handles the flavors/detail HTTP API.
func (n *Nova) handleFlavorsDetail(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		if flavorId := path.Base(r.URL.Path); flavorId != "detail" {
			return errNotFound
		}
		flavors := n.allFlavors()
		if len(flavors) == 0 {
			flavors = []nova.FlavorDetail{}
		}
		resp := struct {
			Flavors []nova.FlavorDetail `json:"flavors"`
		}{flavors}
		return sendJSON(http.StatusOK, resp, w, r)
	case "POST":
		return errNotFound
	case "PUT":
		if flavorId := path.Base(r.URL.Path); flavorId != "detail" {
			return errNotFound
		}
		return errNotFoundJSON
	case "DELETE":
		if flavorId := path.Base(r.URL.Path); flavorId != "detail" {
			return errNotFound
		}
		return errForbidden
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleServerActions handles the servers/<id>/action HTTP API.
func (n *Nova) handleServerActions(server *nova.ServerDetail, w http.ResponseWriter, r *http.Request) error {
	if server == nil {
		return errNotFound
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		return errNotFound
	}
	var action struct {
		AddSecurityGroup *struct {
			Name string
		}
		RemoveSecurityGroup *struct {
			Name string
		}
		AddFloatingIP *struct {
			Address string
		}
		RemoveFloatingIP *struct {
			Address string
		}
	}
	if err := json.Unmarshal(body, &action); err != nil {
		return err
	}
	switch {
	case action.AddSecurityGroup != nil:
		name := action.AddSecurityGroup.Name
		group, err := n.securityGroupByName(name)
		if err != nil || n.hasServerSecurityGroup(server.Id, group.Id) {
			return errNotFound
		}
		if err := n.addServerSecurityGroup(server.Id, group.Id); err != nil {
			return err
		}
		writeResponse(w, http.StatusAccepted, nil)
		return nil
	case action.RemoveSecurityGroup != nil:
		name := action.RemoveSecurityGroup.Name
		group, err := n.securityGroupByName(name)
		if err != nil || !n.hasServerSecurityGroup(server.Id, group.Id) {
			return errNotFound
		}
		if err := n.removeServerSecurityGroup(server.Id, group.Id); err != nil {
			return err
		}
		writeResponse(w, http.StatusAccepted, nil)
		return nil
	case action.AddFloatingIP != nil:
		addr := action.AddFloatingIP.Address
		if n.hasServerFloatingIP(server.Id, addr) {
			return errNotFound
		}
		fip, err := n.floatingIPByAddr(addr)
		if err != nil {
			return errNotFound
		}
		if err := n.addServerFloatingIP(server.Id, fip.Id); err != nil {
			return err
		}
		writeResponse(w, http.StatusAccepted, nil)
		return nil
	case action.RemoveFloatingIP != nil:
		addr := action.RemoveFloatingIP.Address
		if !n.hasServerFloatingIP(server.Id, addr) {
			return errNotFound
		}
		fip, err := n.floatingIPByAddr(addr)
		if err != nil {
			return errNotFound
		}
		if err := n.removeServerFloatingIP(server.Id, fip.Id); err != nil {
			return err
		}
		writeResponse(w, http.StatusAccepted, nil)
		return nil
	}
	return fmt.Errorf("unknown server action: %q", string(body))
}

// handleServerMetadata handles the servers/<id>/action HTTP API.
func (n *Nova) handleServerMetadata(server *nova.ServerDetail, w http.ResponseWriter, r *http.Request) error {
	if server == nil {
		return errNotFound
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		return errNotFound
	}
	var req struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	if err := n.setServerMetadata(server.Id, req.Metadata); err != nil {
		return err
	}
	writeResponse(w, http.StatusOK, nil)
	return nil
}

// newUUID generates a random UUID conforming to RFC 4122.
func newUUID() (string, error) {
	uuid := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, uuid); err != nil {
		return "", err
	}
	uuid[8] = uuid[8]&^0xc0 | 0x80 // variant bits; see section 4.1.1.
	uuid[6] = uuid[6]&^0xf0 | 0x40 // version 4; see section 4.1.3.
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

// noGroupError constructs a bad request response for an invalid group.
func noGroupError(groupName, tenantId string) error {
	return &errorResponse{
		http.StatusBadRequest,
		`{"badRequest": {"message": "Security group ` + groupName + ` not found for project ` + tenantId + `.", "code": 400}}`,
		"application/json; charset=UTF-8",
		"bad request URL",
		nil,
		nil,
	}
}

// handleRunServer handles creating and running a server.
func (n *Nova) handleRunServer(body []byte, w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Server struct {
			FlavorRef        string
			ImageRef         string
			Name             string
			Metadata         map[string]string
			SecurityGroups   []map[string]string `json:"security_groups"`
			Networks         []map[string]string
			AvailabilityZone string `json:"availability_zone"`
		}
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return errBadRequest3
	}
	if req.Server.Name == "" {
		return errBadRequestSrvName
	}
	if req.Server.ImageRef == "" {
		return errBadRequestSrvImage
	}
	if req.Server.FlavorRef == "" {
		return errBadRequestSrvFlavor
	}
	if az := req.Server.AvailabilityZone; az != "" {
		if !n.availabilityZones[az].State.Available {
			return testservices.AvailabilityZoneIsNotAvailable
		}
	}
	n.nextServerId++
	id := strconv.Itoa(n.nextServerId)
	uuid, err := newUUID()
	if err != nil {
		return err
	}
	var groups []string
	if len(req.Server.SecurityGroups) > 0 {
		for _, group := range req.Server.SecurityGroups {
			groupName := group["name"]
			if sg, err := n.securityGroupByName(groupName); err != nil {
				return noGroupError(groupName, n.TenantId)
			} else {
				groups = append(groups, sg.Id)
			}
		}
	}
	// TODO(gz) some kind of sane handling of networks
	for _, network := range req.Server.Networks {
		networkId := network["uuid"]
		_, ok := n.networks[networkId]
		if !ok {
			return errNotFoundJSON
		}
	}
	// TODO(dimitern) - 2013-02-11 bug=1121684
	// make sure flavor/image exist (if needed)
	flavor := nova.FlavorDetail{Id: req.Server.FlavorRef}
	n.buildFlavorLinks(&flavor)
	flavorEnt := nova.Entity{Id: flavor.Id, Links: flavor.Links}
	image := nova.Entity{Id: req.Server.ImageRef}
	timestr := time.Now().Format(time.RFC3339)
	userInfo, _ := userInfo(n.IdentityService, r)
	server := nova.ServerDetail{
		Id:               id,
		UUID:             uuid,
		Name:             req.Server.Name,
		TenantId:         n.TenantId,
		UserId:           userInfo.Id,
		HostId:           "1",
		Image:            image,
		Flavor:           flavorEnt,
		Status:           nova.StatusActive,
		Created:          timestr,
		Updated:          timestr,
		Addresses:        make(map[string][]nova.IPAddress),
		AvailabilityZone: req.Server.AvailabilityZone,
		Metadata:         req.Server.Metadata,
	}
	servers, err := n.allServers(nil)
	if err != nil {
		return err
	}
	nextServer := len(servers) + 1
	n.buildServerLinks(&server)
	// set some IP addresses
	addr := fmt.Sprintf("127.10.0.%d", nextServer)
	server.Addresses["public"] = []nova.IPAddress{{4, addr}, {6, "::dead:beef:f00d"}}
	addr = fmt.Sprintf("127.0.0.%d", nextServer)
	server.Addresses["private"] = []nova.IPAddress{{4, addr}, {6, "::face::000f"}}
	if err := n.addServer(server); err != nil {
		return err
	}
	var resp struct {
		Server struct {
			SecurityGroups []map[string]string `json:"security_groups"`
			Id             string              `json:"id"`
			Links          []nova.Link         `json:"links"`
			AdminPass      string              `json:"adminPass"`
		} `json:"server"`
	}
	if len(req.Server.SecurityGroups) > 0 {
		for _, gid := range groups {
			if err := n.addServerSecurityGroup(id, gid); err != nil {
				return err
			}
		}
		resp.Server.SecurityGroups = req.Server.SecurityGroups
	} else {
		resp.Server.SecurityGroups = []map[string]string{{"name": "default"}}
	}
	resp.Server.Id = id
	resp.Server.Links = server.Links
	resp.Server.AdminPass = "secret"
	return sendJSON(http.StatusAccepted, resp, w, r)
}

// handleServers handles the servers HTTP API.
func (n *Nova) handleServers(w http.ResponseWriter, r *http.Request) error {

	if strings.Contains(r.URL.Path, "os-volume_attachments") {
		switch r.Method {
		case "GET":
			return n.handleListVolumes(w, r)
		case "POST":
			return n.handleAttachVolumes(w, r)
		case "DELETE":
			return n.handleDetachVolumes(w, r)
		}
	}

	switch r.Method {
	case "GET":
		if suffix := path.Base(r.URL.Path); suffix != "servers" {
			groups := false
			serverId := ""
			if suffix == "os-security-groups" {
				// handle GET /servers/<id>/os-security-groups
				serverId = path.Base(strings.Replace(r.URL.Path, "/os-security-groups", "", 1))
				groups = true
			} else {
				serverId = suffix
			}
			server, err := n.server(serverId)
			if err != nil {
				return err
			}
			if groups {
				srvGroups := n.allServerSecurityGroups(serverId)
				if len(srvGroups) == 0 {
					srvGroups = []nova.SecurityGroup{}
				}
				resp := struct {
					Groups []nova.SecurityGroup `json:"security_groups"`
				}{srvGroups}
				return sendJSON(http.StatusOK, resp, w, r)
			}

			resp := struct {
				Server nova.ServerDetail `json:"server"`
			}{*server}
			return sendJSON(http.StatusOK, resp, w, r)
		}
		f := make(filter)
		if err := r.ParseForm(); err == nil && len(r.Form) > 0 {
			for filterKey, filterValues := range r.Form {
				for _, value := range filterValues {
					f[filterKey] = value
				}
			}
		}
		entities, err := n.allServersAsEntities(f)
		if err != nil {
			return err
		}
		if len(entities) == 0 {
			entities = []nova.Entity{}
		}
		resp := struct {
			Servers []nova.Entity `json:"servers"`
		}{entities}
		return sendJSON(http.StatusOK, resp, w, r)
	case "POST":
		if suffix := path.Base(r.URL.Path); suffix != "servers" {
			serverId := ""
			if suffix == "action" {
				// handle POST /servers/<id>/action
				serverId = path.Base(strings.Replace(r.URL.Path, "/action", "", 1))
				server, _ := n.server(serverId)
				return n.handleServerActions(server, w, r)
			} else if suffix == "metadata" {
				// handle POST /servers/<id>/metadata
				serverId = path.Base(strings.Replace(r.URL.Path, "/metadata", "", 1))
				server, _ := n.server(serverId)
				return n.handleServerMetadata(server, w, r)
			} else {
				serverId = suffix
			}
			return errNotFound
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return errBadRequest2
		}
		return n.handleRunServer(body, w, r)
	case "PUT":
		serverId := path.Base(r.URL.Path)
		if serverId == "servers" {
			return errNotFound
		}

		var req struct {
			Server struct {
				Name string `json:"name"`
			} `json:"server"`
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			return errBadRequest2
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}

		err = n.updateServerName(serverId, req.Server.Name)
		if err != nil {
			return err
		}

		server, err := n.server(serverId)
		if err != nil {
			return err
		}
		var resp struct {
			Server nova.ServerDetail `json:"server"`
		}
		resp.Server = *server
		return sendJSON(http.StatusOK, resp, w, r)
	case "DELETE":
		if serverId := path.Base(r.URL.Path); serverId != "servers" {
			if _, err := n.server(serverId); err != nil {
				return errNotFoundJSON
			}
			if err := n.removeServer(serverId); err != nil {
				return err
			}
			writeResponse(w, http.StatusNoContent, nil)
			return nil
		}
		return errNotFound
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleServersDetail handles the servers/detail HTTP API.
func (n *Nova) handleServersDetail(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		if serverId := path.Base(r.URL.Path); serverId != "detail" {
			return errNotFound
		}
		f := make(filter)
		if err := r.ParseForm(); err == nil && len(r.Form) > 0 {
			for filterKey, filterValues := range r.Form {
				for _, value := range filterValues {
					f[filterKey] = value
				}
			}
		}
		servers, err := n.allServers(f)
		if err != nil {
			return err
		}
		if len(servers) == 0 {
			servers = []nova.ServerDetail{}
		}
		resp := struct {
			Servers []nova.ServerDetail `json:"servers"`
		}{servers}
		return sendJSON(http.StatusOK, resp, w, r)
	case "POST":
		return errNotFound
	case "PUT":
		if serverId := path.Base(r.URL.Path); serverId != "detail" {
			return errNotFound
		}
		return errBadRequest2
	case "DELETE":
		if serverId := path.Base(r.URL.Path); serverId != "detail" {
			return errNotFound
		}
		return errNotFoundJSON
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// processGroupId returns the group id from the given request.
// If there was no group id specified in the path, it returns errNoGroupId
func (n *Nova) processGroupId(w http.ResponseWriter, r *http.Request) (*nova.SecurityGroup, error) {
	if groupId := path.Base(r.URL.Path); groupId != "os-security-groups" {
		group, err := n.securityGroup(groupId)
		if err != nil {
			return nil, errNotFoundJSONSG
		}
		return group, nil
	}
	return nil, errNoGroupId
}

// handleSecurityGroups handles the os-security-groups HTTP API.
func (n *Nova) handleSecurityGroups(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		group, err := n.processGroupId(w, r)
		if err == errNoGroupId {
			groups := n.allSecurityGroups()
			if len(groups) == 0 {
				groups = []nova.SecurityGroup{}
			}
			resp := struct {
				Groups []nova.SecurityGroup `json:"security_groups"`
			}{groups}
			return sendJSON(http.StatusOK, resp, w, r)
		}
		if err != nil {
			return err
		}
		resp := struct {
			Group nova.SecurityGroup `json:"security_group"`
		}{*group}
		return sendJSON(http.StatusOK, resp, w, r)
	case "POST":
		if groupId := path.Base(r.URL.Path); groupId != "os-security-groups" {
			return errNotFound
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			return errBadRequest2
		}
		var req struct {
			Group struct {
				Name        string
				Description string
			} `json:"security_group"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		} else {
			_, err := n.securityGroupByName(req.Group.Name)
			if err == nil {
				return errBadRequestDuplicateValue
			}
			n.nextGroupId++
			nextId := strconv.Itoa(n.nextGroupId)
			err = n.addSecurityGroup(nova.SecurityGroup{
				Id:          nextId,
				Name:        req.Group.Name,
				Description: req.Group.Description,
				TenantId:    n.TenantId,
			})
			if err != nil {
				return err
			}
			group, err := n.securityGroup(nextId)
			if err != nil {
				return err
			}
			var resp struct {
				Group nova.SecurityGroup `json:"security_group"`
			}
			resp.Group = *group
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "PUT":
		if groupId := path.Base(r.URL.Path); groupId == "os-security-groups" {
			return errNotFound
		}
		group, err := n.processGroupId(w, r)
		if err != nil {
			return err
		}

		var req struct {
			Group struct {
				Name        string
				Description string
			} `json:"security_group"`
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			return errBadRequest2
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}

		err = n.updateSecurityGroup(nova.SecurityGroup{
			Id:          group.Id,
			Name:        req.Group.Name,
			Description: req.Group.Description,
			TenantId:    group.TenantId,
		})
		if err != nil {
			return err
		}
		group, err = n.securityGroup(group.Id)
		if err != nil {
			return err
		}
		var resp struct {
			Group nova.SecurityGroup `json:"security_group"`
		}
		resp.Group = *group
		return sendJSON(http.StatusOK, resp, w, r)

	case "DELETE":
		if group, err := n.processGroupId(w, r); group != nil {
			if err := n.removeSecurityGroup(group.Id); err != nil {
				return err
			}
			writeResponse(w, http.StatusAccepted, nil)
			return nil
		} else if err == errNoGroupId {
			return errNotFound
		} else {
			return err
		}
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleSecurityGroupRules handles the os-security-group-rules HTTP API.
func (n *Nova) handleSecurityGroupRules(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		return errNotFoundJSON
	case "POST":
		if ruleId := path.Base(r.URL.Path); ruleId != "os-security-group-rules" {
			return errNotFound
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			return errBadRequest2
		}
		var req struct {
			Rule nova.RuleInfo `json:"security_group_rule"`
		}
		if err = json.Unmarshal(body, &req); err != nil {
			return err
		}
		inrule := req.Rule
		group, err := n.securityGroup(inrule.ParentGroupId)
		if err != nil {
			return err // TODO: should be a 4XX error with details
		}
		for _, r := range group.Rules {
			// TODO: this logic is actually wrong, not what nova does at all
			// why are we reimplementing half of nova/api/openstack in go again?
			if r.IPProtocol != nil && *r.IPProtocol == inrule.IPProtocol &&
				r.FromPort != nil && *r.FromPort == inrule.FromPort &&
				r.ToPort != nil && *r.ToPort == inrule.ToPort {
				// TODO: Use a proper helper and sane error type
				return &errorResponse{
					http.StatusBadRequest,
					fmt.Sprintf(`{"badRequest": {"message": "This rule already exists in group %s", "code": 400}}`, group.Id),
					"application/json; charset=UTF-8",
					"rule already exists",
					nil,
					nil,
				}
			}
		}
		n.nextRuleId++
		nextId := strconv.Itoa(n.nextRuleId)
		err = n.addSecurityGroupRule(nextId, req.Rule)
		if err != nil {
			return err
		}
		rule, err := n.securityGroupRule(nextId)
		if err != nil {
			return err
		}
		var resp struct {
			Rule nova.SecurityGroupRule `json:"security_group_rule"`
		}
		resp.Rule = *rule
		return sendJSON(http.StatusOK, resp, w, r)
	case "PUT":
		if ruleId := path.Base(r.URL.Path); ruleId != "os-security-group-rules" {
			return errNotFoundJSON
		}
		return errNotFound
	case "DELETE":
		if ruleId := path.Base(r.URL.Path); ruleId != "os-security-group-rules" {
			if _, err := n.securityGroupRule(ruleId); err != nil {
				return errNotFoundJSONSGR
			}
			if err := n.removeSecurityGroupRule(ruleId); err != nil {
				return err
			}
			writeResponse(w, http.StatusAccepted, nil)
			return nil
		}
		return errNotFound
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleFloatingIPs handles the os-floating-ips HTTP API.
func (n *Nova) handleFloatingIPs(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		if ipId := path.Base(r.URL.Path); ipId != "os-floating-ips" {
			fip, err := n.floatingIP(ipId)
			if err != nil {
				return errNotFoundJSON
			}
			resp := struct {
				IP nova.FloatingIP `json:"floating_ip"`
			}{*fip}
			return sendJSON(http.StatusOK, resp, w, r)
		}
		fips := n.allFloatingIPs()
		if len(fips) == 0 {
			fips = []nova.FloatingIP{}
		}
		resp := struct {
			IPs []nova.FloatingIP `json:"floating_ips"`
		}{fips}
		return sendJSON(http.StatusOK, resp, w, r)
	case "POST":
		if ipId := path.Base(r.URL.Path); ipId != "os-floating-ips" {
			return errNotFound
		}
		n.nextIPId++
		addr := fmt.Sprintf("10.0.0.%d", n.nextIPId)
		nextId := strconv.Itoa(n.nextIPId)
		fip := nova.FloatingIP{Id: nextId, IP: addr, Pool: "nova"}
		err := n.addFloatingIP(fip)
		if err != nil {
			return err
		}
		resp := struct {
			IP nova.FloatingIP `json:"floating_ip"`
		}{fip}
		return sendJSON(http.StatusOK, resp, w, r)
	case "PUT":
		if ipId := path.Base(r.URL.Path); ipId != "os-floating-ips" {
			return errNotFoundJSON
		}
		return errNotFound
	case "DELETE":
		if ipId := path.Base(r.URL.Path); ipId != "os-floating-ips" {
			if err := n.removeFloatingIP(ipId); err == nil {
				writeResponse(w, http.StatusAccepted, nil)
				return nil
			}
			return errNotFoundJSON
		}
		return errNotFound
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleNetworks handles the os-networks HTTP API.
func (n *Nova) handleNetworks(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		if ipId := path.Base(r.URL.Path); ipId != "os-networks" {
			// TODO(gz): handle listing a single group
			return errNotFoundJSON
		}
		nets := n.allNetworks()
		if len(nets) == 0 {
			nets = []nova.Network{}
		}
		resp := struct {
			Network []nova.Network `json:"networks"`
		}{nets}
		return sendJSON(http.StatusOK, resp, w, r)
		// TODO(gz): proper handling of other methods
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleAvailabilityZones handles the os-availability-zone HTTP API.
func (n *Nova) handleAvailabilityZones(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		if ipId := path.Base(r.URL.Path); ipId != "os-availability-zone" {
			return errNotFoundJSON
		}
		zones := n.allAvailabilityZones()
		if len(zones) == 0 {
			// If there are no availability zones defined, act as
			// if we don't support the availability zones extension.
			return errNotFoundJSON
		}
		resp := struct {
			Zones []nova.AvailabilityZone `json:"availabilityZoneInfo"`
		}{zones}
		return sendJSON(http.StatusOK, resp, w, r)
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

func (n *Nova) handleAttachVolumes(w http.ResponseWriter, r *http.Request) error {
	serverId := path.Base(strings.Replace(r.URL.Path, "/os-volume_attachments", "", 1))

	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	var attachment struct {
		VolumeAttachment nova.VolumeAttachment `json:"volumeAttachment"`
	}
	if err := json.Unmarshal(bodyBytes, &attachment); err != nil {
		return err
	}
	n.nextAttachmentId++
	attachment.VolumeAttachment.Id = fmt.Sprintf("%d", n.nextAttachmentId)

	serverVols := n.serverIdToAttachedVolumes[serverId]
	serverVols = append(serverVols, attachment.VolumeAttachment)
	n.serverIdToAttachedVolumes[serverId] = serverVols

	// Echo the request back with an attachment ID.
	resp, err := json.Marshal(&attachment)
	if err != nil {
		return err
	}
	_, err = w.Write(resp)
	return err
}

func (n *Nova) handleDetachVolumes(w http.ResponseWriter, r *http.Request) error {
	attachId := path.Base(r.URL.Path)
	serverId := path.Base(strings.Replace(r.URL.Path, "/os-volume_attachments/"+attachId, "", 1))
	serverVols := n.serverIdToAttachedVolumes[serverId]

	for volIdx, vol := range serverVols {
		if vol.Id == attachId {
			serverVols = append(serverVols[:volIdx], serverVols[volIdx+1:]...)
			n.serverIdToAttachedVolumes[serverId] = serverVols
			writeResponse(w, http.StatusAccepted, nil)
			return nil
		}
	}

	return errors.NewNotFoundf(nil, nil, "no such attachment id: %v", attachId)
}

func (n *Nova) handleListVolumes(w http.ResponseWriter, r *http.Request) error {
	serverId := path.Base(strings.Replace(r.URL.Path, "/os-volume_attachments", "", 1))
	serverVols := n.serverIdToAttachedVolumes[serverId]

	resp, err := json.Marshal(struct {
		VolumeAttachments []nova.VolumeAttachment `json:"volumeAttachments"`
	}{serverVols})
	if err != nil {
		return err
	}

	_, err = w.Write(resp)
	return err
}

// SetupHTTP attaches all the needed handlers to provide the HTTP API.
func (n *Nova) SetupHTTP(mux *http.ServeMux) {
	handlers := map[string]http.Handler{
		"/$v/":                           errBadRequest,
		"/$v/$t/":                        errNotFound,
		"/$v/$t/flavors":                 n.handler((*Nova).handleFlavors),
		"/$v/$t/flavors/detail":          n.handler((*Nova).handleFlavorsDetail),
		"/$v/$t/servers":                 n.handler((*Nova).handleServers),
		"/$v/$t/servers/detail":          n.handler((*Nova).handleServersDetail),
		"/$v/$t/os-security-groups":      n.handler((*Nova).handleSecurityGroups),
		"/$v/$t/os-security-group-rules": n.handler((*Nova).handleSecurityGroupRules),
		"/$v/$t/os-floating-ips":         n.handler((*Nova).handleFloatingIPs),
		"/$v/$t/os-networks":             n.handler((*Nova).handleNetworks),
		"/$v/$t/os-availability-zone":    n.handler((*Nova).handleAvailabilityZones),
	}
	for path, h := range handlers {
		path = strings.Replace(path, "$v", n.VersionPath, 1)
		path = strings.Replace(path, "$t", n.TenantId, 1)
		if !strings.HasSuffix(path, "/") {
			mux.Handle(path+"/", h)
		}
		mux.Handle(path, h)
	}
}
