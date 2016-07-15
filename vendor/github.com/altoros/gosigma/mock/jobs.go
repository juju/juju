// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package mock

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/altoros/gosigma/data"
)

// JobLibrary type to store all jobs in the mock
type JobLibrary struct {
	s sync.Mutex
	m map[string]*data.Job
	p string
}

// Jobs defines library of all jobs in the mock
var Jobs = &JobLibrary{
	m: make(map[string]*data.Job),
	p: "/api/2.0/jobs",
}

// InitJob initializes the job
func InitJob(j *data.Job) (*data.Job, error) {
	if j.UUID == "" {
		uuid, err := GenerateUUID()
		if err != nil {
			return nil, err
		}
		j.UUID = uuid
	}
	if j.State == "" {
		j.State = "started"
	}

	return j, nil
}

// Add job to the library
func (j *JobLibrary) Add(job *data.Job) error {
	job, err := InitJob(job)
	if err != nil {
		return err
	}

	j.s.Lock()
	defer j.s.Unlock()

	j.m[job.UUID] = job

	return nil
}

// AddJobs adds job collection to the libraryh
func (j *JobLibrary) AddJobs(jj []data.Job) []string {
	j.s.Lock()
	defer j.s.Unlock()

	var result []string
	for _, job := range jj {
		job, err := InitJob(&job)
		if err != nil {
			j.m[job.UUID] = job
			result = append(result, job.UUID)
		}
	}
	return result
}

// Remove job from the library
func (j *JobLibrary) Remove(uuid string) bool {
	j.s.Lock()
	defer j.s.Unlock()

	_, ok := j.m[uuid]
	delete(j.m, uuid)

	return ok
}

// Reset the library
func (j *JobLibrary) Reset() {
	j.s.Lock()
	defer j.s.Unlock()
	j.m = make(map[string]*data.Job)
}

// SetState for the job in the library
func (j *JobLibrary) SetState(uuid, state string) {
	j.s.Lock()
	defer j.s.Unlock()

	job, ok := j.m[uuid]
	if ok {
		job.State = state
	}
}

func (j *JobLibrary) handleRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimPrefix(path, j.p)
	path = strings.TrimPrefix(path, "/")

	switch r.Method {
	case "GET":
		if path == "" {
			j.handleList(w, r)
		} else {
			j.handleGet(w, r, path)
		}
	default:
		w.WriteHeader(405)
	}
}

func (j *JobLibrary) handleList(w http.ResponseWriter, r *http.Request) {
	j.s.Lock()
	defer j.s.Unlock()

	var jj data.Jobs
	jj.Meta.TotalCount = len(j.m)
	jj.Objects = make([]data.Job, 0, len(j.m))
	for _, job := range j.m {
		jj.Objects = append(jj.Objects, *job)
	}

	data, err := json.Marshal(&jj)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("500 " + err.Error()))
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func (j *JobLibrary) handleGet(w http.ResponseWriter, r *http.Request, uuid string) {
	j.s.Lock()
	defer j.s.Unlock()

	h := w.Header()

	job, ok := j.m[uuid]
	if !ok {
		h.Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(404)
		w.Write([]byte(jsonNotFound))
		return
	}

	data, err := json.Marshal(&job)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("500 " + err.Error()))
		return
	}

	h.Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	w.Write(data)
}
