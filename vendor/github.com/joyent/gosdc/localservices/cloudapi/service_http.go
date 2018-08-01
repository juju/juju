//
// gosdc - Go library to interact with the Joyent CloudAPI
//
// CloudAPI double testing service - HTTP API implementation
//
// Copyright (c) 2013 Joyent Inc.
//
// Written by Daniele Stroppa <daniele.stroppa@joyent.com>
//

package cloudapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/joyent/gosdc/cloudapi"
)

// ErrorResponse defines a single HTTP error response.
type ErrorResponse struct {
	Code        int
	Body        string
	contentType string
	errorText   string
	headers     map[string]string
	cloudapi    *CloudAPI
}

var (
	ErrNotAllowed = &ErrorResponse{
		http.StatusMethodNotAllowed,
		"Method is not allowed",
		"text/plain; charset=UTF-8",
		"MethodNotAllowedError",
		nil,
		nil,
	}
	ErrNotFound = &ErrorResponse{
		http.StatusNotFound,
		"Resource Not Found",
		"text/plain; charset=UTF-8",
		"NotFoundError",
		nil,
		nil,
	}
	ErrBadRequest = &ErrorResponse{
		http.StatusBadRequest,
		"Malformed request url",
		"text/plain; charset=UTF-8",
		"BadRequestError",
		nil,
		nil,
	}
)

func (e *ErrorResponse) Error() string {
	return e.errorText
}

func (e *ErrorResponse) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if e.contentType != "" {
		w.Header().Set("Content-Type", e.contentType)
	}
	body := e.Body
	if e.headers != nil {
		for h, v := range e.headers {
			w.Header().Set(h, v)
		}
	}
	// workaround for https://code.google.com/p/go/issues/detail?id=4454
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if e.Code != 0 {
		w.WriteHeader(e.Code)
	}
	if len(body) > 0 {
		w.Write([]byte(body))
	}
}

type cloudapiHandler struct {
	cloudapi *CloudAPI
	method   func(m *CloudAPI, w http.ResponseWriter, r *http.Request) error
}

func (h *cloudapiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// handle trailing slash in the path
	if strings.HasSuffix(path, "/") && path != "/" {
		ErrNotFound.ServeHTTP(w, r)
		return
	}
	err := h.method(h.cloudapi, w, r)
	if err == nil {
		return
	}
	var resp http.Handler
	resp, _ = err.(http.Handler)
	if resp == nil {
		resp = &ErrorResponse{
			http.StatusInternalServerError,
			`{"internalServerError":{"message":"Unkown Error",code:500}}`,
			"application/json",
			err.Error(),
			nil,
			h.cloudapi,
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

func processFilter(rawQuery string) map[string]string {
	var filters map[string]string
	if rawQuery != "" {
		filters = make(map[string]string)
		for _, filter := range strings.Split(rawQuery, "&") {
			filters[filter[:strings.Index(filter, "=")]] = filter[strings.Index(filter, "=")+1:]
		}
	}

	return filters
}

func (cloudapi *CloudAPI) handler(method func(m *CloudAPI, w http.ResponseWriter, r *http.Request) error) http.Handler {
	return &cloudapiHandler{cloudapi, method}
}

// handleKeys handles the keys HTTP API.
func (c *CloudAPI) handleKeys(w http.ResponseWriter, r *http.Request) error {
	prefix := fmt.Sprintf("/%s/keys/", c.ServiceInstance.UserAccount)
	keyName := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case "GET":
		if strings.HasSuffix(r.URL.Path, "keys") {
			// ListKeys
			keys, err := c.ListKeys()
			if err != nil {
				return err
			}
			if keys == nil {
				keys = []cloudapi.Key{}
			}
			resp := keys
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			// GetKey
			key, err := c.GetKey(keyName)
			if err != nil {
				return err
			}
			if key == nil {
				key = &cloudapi.Key{}
			}
			resp := key
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "POST":
		if strings.HasSuffix(r.URL.Path, "keys") {
			// CreateKey
			var (
				name string
				key  string
			)
			opts := &cloudapi.CreateKeyOpts{}
			body, errB := ioutil.ReadAll(r.Body)
			if errB != nil {
				return errB
			}
			if len(body) > 0 {
				if errJ := json.Unmarshal(body, opts); errJ != nil {
					return errJ
				}
				name = opts.Name
				key = opts.Key
			}
			k, err := c.CreateKey(name, key)
			if err != nil {
				return err
			}
			if k == nil {
				k = &cloudapi.Key{}
			}
			resp := k
			return sendJSON(http.StatusCreated, resp, w, r)
		} else {
			return ErrNotAllowed
		}
	case "PUT":
		return ErrNotAllowed
	case "DELETE":
		if strings.HasSuffix(r.URL.Path, "keys") {
			return ErrNotAllowed
		} else {
			// DeleteKey
			err := c.DeleteKey(keyName)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusNoContent, nil, w, r)
		}
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleImages handles the images HTTP API.
func (c *CloudAPI) handleImages(w http.ResponseWriter, r *http.Request) error {
	prefix := fmt.Sprintf("/%s/images/", c.ServiceInstance.UserAccount)
	imageId := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case "GET":
		if strings.HasSuffix(r.URL.Path, "images") {
			// ListImages
			images, err := c.ListImages(processFilter(r.URL.RawQuery))
			if err != nil {
				return err
			}
			if images == nil {
				images = []cloudapi.Image{}
			}
			resp := images
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			// GetImage
			image, err := c.GetImage(imageId)
			if err != nil {
				return err
			}
			if image == nil {
				image = &cloudapi.Image{}
			}
			resp := image
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "POST":
		if strings.HasSuffix(r.URL.Path, "images") {
			// CreateImageFromMachine
			return ErrNotFound
		} else {
			return ErrNotAllowed
		}
	case "PUT":
		return ErrNotAllowed
	case "DELETE":
		/*if strings.HasSuffix(r.URL.Path, "images") {
			return ErrNotAllowed
		} else {
			err := c.DeleteImage(imageId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusNoContent, nil, w, r)
		}*/
		return ErrNotAllowed
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handlePackages handles the packages HTTP API.
func (c *CloudAPI) handlePackages(w http.ResponseWriter, r *http.Request) error {
	prefix := fmt.Sprintf("/%s/packages/", c.ServiceInstance.UserAccount)
	pkgName := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case "GET":
		if strings.HasSuffix(r.URL.Path, "packages") {
			// ListPackages
			pkgs, err := c.ListPackages(processFilter(r.URL.RawQuery))
			if err != nil {
				return err
			}
			if pkgs == nil {
				pkgs = []cloudapi.Package{}
			}
			resp := pkgs
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			// GetPackage
			pkg, err := c.GetPackage(pkgName)
			if err != nil {
				return err
			}
			if pkg == nil {
				pkg = &cloudapi.Package{}
			}
			resp := pkg
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "POST":
		return ErrNotAllowed
	case "PUT":
		return ErrNotAllowed
	case "DELETE":
		return ErrNotAllowed
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleMachines handles the machine HTTP API.
func (c *CloudAPI) handleMachines(w http.ResponseWriter, r *http.Request) error {
	prefix := fmt.Sprintf("/%s/machines/", c.ServiceInstance.UserAccount)
	machineId := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case "GET":
		if strings.HasSuffix(r.URL.Path, "machines") {
			// ListMachines
			machines, err := c.ListMachines(processFilter(r.URL.RawQuery))
			if err != nil {
				return err
			}
			if machines == nil {
				machines = []*cloudapi.Machine{}
			}
			resp := machines
			return sendJSON(http.StatusOK, resp, w, r)
		} else if strings.HasSuffix(r.URL.Path, "fwrules") {
			// ListMachineFirewallRules
			machineId = strings.TrimSuffix(machineId, "/fwrules")
			fwRules, err := c.ListMachineFirewallRules(machineId)
			if err != nil {
				return err
			}
			if fwRules == nil {
				fwRules = []*cloudapi.FirewallRule{}
			}
			resp := fwRules
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			// GetMachine
			machine, err := c.GetMachine(machineId)
			if err != nil {
				return err
			}
			if machine == nil {
				machine = &cloudapi.Machine{}
			}
			resp := machine
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "HEAD":
		if strings.HasSuffix(r.URL.Path, "machines") {
			// CountMachines
			count, err := c.CountMachines()
			if err != nil {
				return err
			}
			resp := count
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			return ErrNotAllowed
		}
	case "POST":
		if strings.HasSuffix(r.URL.Path, "machines") {
			// CreateMachine
			var (
				name     string
				pkg      string
				image    string
				metadata map[string]string
				tags     map[string]string
			)
			opts := &cloudapi.CreateMachineOpts{}
			body, errB := ioutil.ReadAll(r.Body)
			if errB != nil {
				return errB
			}
			if len(body) > 0 {
				if errJ := json.Unmarshal(body, opts); errJ != nil {
					return errJ
				}
				name = opts.Name
				pkg = opts.Package
				image = opts.Image
				metadata = opts.Metadata
				tags = opts.Tags
			}
			machine, err := c.CreateMachine(name, pkg, image, metadata, tags)
			if err != nil {
				return err
			}
			if machine == nil {
				machine = &cloudapi.Machine{}
			}
			resp := machine
			return sendJSON(http.StatusCreated, resp, w, r)
		} else if r.URL.Query().Get("action") == "stop" {
			//StopMachine
			err := c.StopMachine(machineId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusAccepted, nil, w, r)
		} else if r.URL.Query().Get("action") == "start" {
			//StartMachine
			err := c.StartMachine(machineId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusAccepted, nil, w, r)
		} else if r.URL.Query().Get("action") == "reboot" {
			//RebootMachine
			err := c.RebootMachine(machineId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusAccepted, nil, w, r)
		} else if r.URL.Query().Get("action") == "resize" {
			//ResizeMachine
			err := c.ResizeMachine(machineId, r.URL.Query().Get("package"))
			if err != nil {
				return err
			}
			return sendJSON(http.StatusAccepted, nil, w, r)
		} else if r.URL.Query().Get("action") == "rename" {
			//RenameMachine
			err := c.RenameMachine(machineId, r.URL.Query().Get("name"))
			if err != nil {
				return err
			}
			return sendJSON(http.StatusAccepted, nil, w, r)
		} else if r.URL.Query().Get("action") == "enable_firewall" {
			//EnableFirewallMachine
			err := c.EnableFirewallMachine(machineId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusAccepted, nil, w, r)
		} else if r.URL.Query().Get("action") == "disable_firewall" {
			//DisableFirewallMachine
			err := c.DisableFirewallMachine(machineId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusAccepted, nil, w, r)
		} else {
			return ErrNotAllowed
		}
	case "PUT":
		return ErrNotAllowed
	case "DELETE":
		if strings.HasSuffix(r.URL.Path, "machines") {
			return ErrNotAllowed
		} else {
			// DeleteMachine
			err := c.DeleteMachine(machineId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusNoContent, nil, w, r)
		}
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleFwRules handles the firewall rules HTTP API.
func (c *CloudAPI) handleFwRules(w http.ResponseWriter, r *http.Request) error {
	prefix := fmt.Sprintf("/%s/fwrules/", c.ServiceInstance.UserAccount)
	fwRuleId := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case "GET":
		if strings.HasSuffix(r.URL.Path, "fwrules") {
			// ListFirewallRules
			fwRules, err := c.ListFirewallRules()
			if err != nil {
				return err
			}
			if fwRules == nil {
				fwRules = []*cloudapi.FirewallRule{}
			}
			resp := fwRules
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			// GetFirewallRule
			fwRule, err := c.GetFirewallRule(fwRuleId)
			if err != nil {
				return err
			}
			if fwRule == nil {
				fwRule = &cloudapi.FirewallRule{}
			}
			resp := fwRule
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "POST":
		if strings.HasSuffix(r.URL.Path, "fwrules") {
			// CreateFirewallRule
			var (
				rule    string
				enabled bool
			)
			opts := &cloudapi.CreateFwRuleOpts{}
			body, errB := ioutil.ReadAll(r.Body)
			if errB != nil {
				return errB
			}
			if len(body) > 0 {
				if errJ := json.Unmarshal(body, opts); errJ != nil {
					return errJ
				}
				rule = opts.Rule
				enabled = opts.Enabled
			}
			fwRule, err := c.CreateFirewallRule(rule, enabled)
			if err != nil {
				return err
			}
			if fwRule == nil {
				fwRule = &cloudapi.FirewallRule{}
			}
			resp := fwRule
			return sendJSON(http.StatusCreated, resp, w, r)
		} else if strings.HasSuffix(r.URL.Path, "enable") {
			// EnableFirewallRule
			fwRuleId = strings.TrimSuffix(fwRuleId, "/enable")
			fwRule, err := c.EnableFirewallRule(fwRuleId)
			if err != nil {
				return err
			}
			if fwRule == nil {
				fwRule = &cloudapi.FirewallRule{}
			}
			resp := fwRule
			return sendJSON(http.StatusOK, resp, w, r)
		} else if strings.HasSuffix(r.URL.Path, "disable") {
			// DisableFirewallRule
			fwRuleId = strings.TrimSuffix(fwRuleId, "/disable")
			fwRule, err := c.DisableFirewallRule(fwRuleId)
			if err != nil {
				return err
			}
			if fwRule == nil {
				fwRule = &cloudapi.FirewallRule{}
			}
			resp := fwRule
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			// UpdateFirewallRule
			var (
				rule    string
				enabled bool
			)
			opts := &cloudapi.CreateFwRuleOpts{}
			body, errB := ioutil.ReadAll(r.Body)
			if errB != nil {
				return errB
			}
			if len(body) > 0 {
				if errJ := json.Unmarshal(body, opts); errJ != nil {
					return errJ
				}
				rule = opts.Rule
				enabled = opts.Enabled
			}
			fwRule, err := c.UpdateFirewallRule(fwRuleId, rule, enabled)
			if err != nil {
				return err
			}
			if fwRule == nil {
				fwRule = &cloudapi.FirewallRule{}
			}
			resp := fwRule
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "PUT":
		return ErrNotAllowed
	case "DELETE":
		if strings.HasSuffix(r.URL.Path, "fwrules") {
			return ErrNotAllowed
		} else {
			// DeleteFirewallRule
			err := c.DeleteFirewallRule(fwRuleId)
			if err != nil {
				return err
			}
			return sendJSON(http.StatusNoContent, nil, w, r)
		}
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// handleNetworks handles the networks HTTP API.
func (c *CloudAPI) handleNetworks(w http.ResponseWriter, r *http.Request) error {
	prefix := fmt.Sprintf("/%s/networks/", c.ServiceInstance.UserAccount)
	networkId := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case "GET":
		if strings.HasSuffix(r.URL.Path, "networks") {
			// ListNetworks
			networks, err := c.ListNetworks()
			if err != nil {
				return err
			}
			if networks == nil {
				networks = []cloudapi.Network{}
			}
			resp := networks
			return sendJSON(http.StatusOK, resp, w, r)
		} else {
			// GetNetwork
			network, err := c.GetNetwork(networkId)
			if err != nil {
				return err
			}
			if network == nil {
				network = &cloudapi.Network{}
			}
			resp := network
			return sendJSON(http.StatusOK, resp, w, r)
		}
	case "POST":
		return ErrNotAllowed
	case "PUT":
		return ErrNotAllowed
	case "DELETE":
		return ErrNotAllowed
	}
	return fmt.Errorf("unknown request method %q for %s", r.Method, r.URL.Path)
}

// setupHTTP attaches all the needed handlers to provide the HTTP API.
func (c *CloudAPI) SetupHTTP(mux *http.ServeMux) {
	handlers := map[string]http.Handler{
		"/":               ErrNotFound,
		"/$user/":         ErrBadRequest,
		"/$user/keys":     c.handler((*CloudAPI).handleKeys),
		"/$user/images":   c.handler((*CloudAPI).handleImages),
		"/$user/packages": c.handler((*CloudAPI).handlePackages),
		"/$user/machines": c.handler((*CloudAPI).handleMachines),
		//"/$user/datacenters": 	c.handler((*CloudAPI).handleDatacenters),
		"/$user/fwrules":  c.handler((*CloudAPI).handleFwRules),
		"/$user/networks": c.handler((*CloudAPI).handleNetworks),
	}
	for path, h := range handlers {
		path = strings.Replace(path, "$user", c.ServiceInstance.UserAccount, 1)
		if !strings.HasSuffix(path, "/") {
			mux.Handle(path+"/", h)
		}
		mux.Handle(path, h)
	}
}
