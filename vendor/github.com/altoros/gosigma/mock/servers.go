// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package mock

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/altoros/gosigma/data"
)

var syncServers sync.Mutex
var servers = make(map[string]*data.Server)

// GenerateUUID generated new UUID for server
func GenerateUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}

	uuid[8] = 0x80 // variant bits see page 5
	uuid[4] = 0x40 // version 4 Pseudo Random, see page 7

	return fmt.Sprintf("%0x%0x%0x%0x-%0x%0x-%0x%0x-%0x%0x-%0x%0x%0x%0x%0x%0x",
		uuid[0], uuid[1], uuid[2], uuid[3],
		uuid[4], uuid[5],
		uuid[6], uuid[7],
		uuid[8], uuid[9],
		uuid[10], uuid[11], uuid[12], uuid[13], uuid[14], uuid[15]), nil
}

func initServer(s *data.Server) (*data.Server, error) {
	if s.UUID == "" {
		uuid, err := GenerateUUID()
		if err != nil {
			return nil, err
		}
		s.UUID = uuid
	}
	if s.Status == "" {
		s.Status = "stopped"
	}

	return s, nil
}

// AddServer adds server instance record under the mock
func AddServer(s *data.Server) error {
	s, err := initServer(s)
	if err != nil {
		return err
	}

	syncServers.Lock()
	defer syncServers.Unlock()

	servers[s.UUID] = s

	return nil
}

// AddServers adds server instance records under the mock
func AddServers(ss []data.Server) []string {
	syncServers.Lock()
	defer syncServers.Unlock()

	var result []string
	for _, s := range ss {
		s, err := initServer(&s)
		if err != nil {
			servers[s.UUID] = s
			result = append(result, s.UUID)
		}
	}
	return result
}

// RemoveServer removes server instance record from the mock
func RemoveServer(uuid string) bool {
	syncServers.Lock()
	defer syncServers.Unlock()

	_, ok := servers[uuid]
	delete(servers, uuid)

	return ok
}

// ResetServers removes all server instance records from the mock
func ResetServers() {
	syncServers.Lock()
	defer syncServers.Unlock()
	servers = make(map[string]*data.Server)
}

// SetServerStatus changes status of server instance in the mock
func SetServerStatus(uuid, status string) {
	syncServers.Lock()
	defer syncServers.Unlock()

	s, ok := servers[uuid]
	if ok {
		s.Status = status
	}
}

const jsonNotFound = `[{
		"error_point": null,
	 	"error_type": "notexist",
	 	"error_message": "notfound"
}]`

const jsonStartFailed = `[{
		"error_point": null,
		"error_type": "permission",
		"error_message": "Cannot start guest in state \"started\". Guest should be in state \"stopped\""
}]`

const jsonStopFailed = `[{
		"error_point": null,
		"error_type": "permission",
		"error_message": "Cannot stop guest in state \"stopped\". Guest should be in state \"['started', 'running_legacy']\""
}]`

const jsonActionSuccess = `{
		"action": "%s",
		"result": "success",
		"uuid": "%s"
}`

// URLs:
// /api/2.0/servers
// /api/2.0/servers/detail/
// /api/2.0/servers/{uuid}/
func serversHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		serversHandlerGet(w, r)
	case "POST":
		serversHandlerPost(w, r)
	case "DELETE":
		serversHandlerDelete(w, r)
	}
}

func serversHandlerGet(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch path {
	case "/api/2.0/servers":
		handleServers(w, r)
	case "/api/2.0/servers/detail":
		handleServersDetail(w, r, 200, nil)
	default:
		uuid := strings.TrimPrefix(path, "/api/2.0/servers/")
		handleServer(w, r, 200, uuid)
	}
}

func serversHandlerPost(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/action/")
	uuid := strings.TrimPrefix(path, "/api/2.0/servers/")
	handleServerAction(w, r, uuid)
}

func serversHandlerDelete(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	uuid := strings.TrimPrefix(path, "/api/2.0/servers/")
	if RemoveServer(uuid) {
		w.WriteHeader(204)
	} else {
		h := w.Header()
		h.Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(404)
		w.Write([]byte(jsonNotFound))
	}
}

func handleServers(w http.ResponseWriter, r *http.Request) {
	syncServers.Lock()
	defer syncServers.Unlock()

	var ss data.Servers
	ss.Meta.TotalCount = len(servers)
	ss.Objects = make([]data.Server, 0, len(servers))
	for _, s := range servers {
		var srv data.Server
		srv.Resource = s.Resource
		srv.Name = s.Name
		srv.Status = s.Status
		ss.Objects = append(ss.Objects, srv)
	}

	data, err := json.Marshal(&ss)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("500 " + err.Error()))
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func handleServersDetail(w http.ResponseWriter, r *http.Request, okcode int, filter []string) {
	syncServers.Lock()
	defer syncServers.Unlock()

	var ss data.Servers

	if len(filter) == 0 {
		ss.Meta.TotalCount = len(servers)
		ss.Objects = make([]data.Server, 0, len(servers))
		for _, s := range servers {
			ss.Objects = append(ss.Objects, *s)
		}
	} else {
		ss.Meta.TotalCount = len(filter)
		ss.Objects = make([]data.Server, 0, len(filter))
		for _, uuid := range filter {
			if s, ok := servers[uuid]; ok {
				ss.Objects = append(ss.Objects, *s)
			}
		}
	}

	data, err := json.Marshal(&ss)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("500 " + err.Error()))
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(okcode)
	w.Write(data)
}

func handleServer(w http.ResponseWriter, r *http.Request, okcode int, uuid string) {
	syncServers.Lock()
	defer syncServers.Unlock()

	h := w.Header()

	s, ok := servers[uuid]
	if !ok {
		h.Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(404)
		w.Write([]byte(jsonNotFound))
		return
	}

	data, err := json.Marshal(&s)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("500 " + err.Error()))
		return
	}

	h.Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(okcode)
	w.Write(data)
}

func handleServerAction(w http.ResponseWriter, r *http.Request, uuid string) {
	vv := r.URL.Query()

	v, ok := vv["do"]
	if !ok || len(v) < 1 {
		handleServerCreate(w, r)
		return
	}

	action := v[0]
	switch action {
	case "start":
		handleServerStart(w, r, uuid)
	case "stop":
		handleServerStop(w, r, uuid)
	default:
		handleServerCreate(w, r)
	}
}

func handleServerStart(w http.ResponseWriter, r *http.Request, uuid string) {
	syncServers.Lock()
	defer syncServers.Unlock()

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")

	s, ok := servers[uuid]
	if !ok {
		w.WriteHeader(404)
		w.Write([]byte(jsonNotFound))
		return
	}

	if !strings.HasPrefix(s.Status, "stopped") {
		w.WriteHeader(403)
		w.Write([]byte(jsonStartFailed))
		return
	}

	s.Status = "starting"
	go func() {
		syncServers.Lock()
		defer syncServers.Unlock()
		<-time.After(300 * time.Millisecond)
		s.Status = "running"
		for i, n := range s.NICs {
			if n.IPv4 != nil && n.IPv4.Conf == "dhcp" {
				s.NICs[i].Runtime = &data.RuntimeNetwork{
					InterfaceType: "public",
					IPv4:          data.MakeIPResource("0.1.2.3"),
				}
			}
		}
	}()

	w.WriteHeader(202)
	w.Write([]byte(fmt.Sprintf(string(jsonActionSuccess), "start", s.UUID)))
}

func handleServerStop(w http.ResponseWriter, r *http.Request, uuid string) {
	syncServers.Lock()
	defer syncServers.Unlock()

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")

	s, ok := servers[uuid]
	if !ok {
		w.WriteHeader(404)
		w.Write([]byte(jsonNotFound))
		return
	}

	if !strings.HasPrefix(s.Status, "running") {
		w.WriteHeader(403)
		w.Write([]byte(jsonStopFailed))
		return
	}

	s.Status = "stopping"
	go func() {
		syncServers.Lock()
		defer syncServers.Unlock()
		<-time.After(300 * time.Millisecond)
		s.Status = "stopped"
		for i := range s.NICs {
			s.NICs[i].Runtime = nil
		}
	}()

	w.WriteHeader(202)
	w.Write([]byte(fmt.Sprintf(jsonActionSuccess, "stop", s.UUID)))
}

func handleServerCreate(w http.ResponseWriter, r *http.Request) {
	bb, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	s, err := data.ReadServer(bytes.NewReader(bb))
	if err == nil {
		if err = AddServer(s); err != nil {
			w.WriteHeader(400)
		} else {
			handleServersDetail(w, r, 201, []string{s.UUID})
		}
		return
	}

	ss, err := data.ReadServers(bytes.NewReader(bb))
	if err == nil {
		uuids := AddServers(ss)
		handleServersDetail(w, r, 201, uuids)
		return
	}

	w.WriteHeader(400)
}
