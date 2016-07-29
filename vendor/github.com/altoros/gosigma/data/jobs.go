// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import (
	"io"
	"time"
)

// JobData contains metainformation of job instance
type JobData struct {
	Progress int `json:"progress"`
}

// Job contains properties of job instance
type Job struct {
	Resource
	Children     []string  `json:"children"`
	Created      time.Time `json:"created"`
	Data         JobData   `json:"data"`
	LastModified time.Time `json:"last_modified"`
	Operation    string    `json:"operation"`
	Resources    []string  `json:"resources"`
	State        string    `json:"state"`
}

// Jobs holds collection of Job objects
type Jobs struct {
	Meta    Meta  `json:"meta"`
	Objects []Job `json:"objects"`
}

// ReadJob reads and unmarshalls information about job instance from JSON stream
func ReadJob(r io.Reader) (*Job, error) {
	var job Job
	if err := ReadJSON(r, &job); err != nil {
		return nil, err
	}
	return &job, nil
}
