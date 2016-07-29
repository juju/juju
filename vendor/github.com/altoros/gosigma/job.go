// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"
	"time"

	"github.com/altoros/gosigma/data"
)

const (
	// JobStateStarted defines constant for started job state
	JobStateStarted = "started"
	// JobStateSuccess defines constant for success job state
	JobStateSuccess = "success"
)

// A Job interface represents job instance in CloudSigma account
type Job interface {
	// CloudSigma resource
	Resource

	// Children of this job instance
	Children() []string

	// Created time of this job instance
	Created() time.Time

	// LastModified time of this job instance
	LastModified() time.Time

	// Operation of this job instance
	Operation() string

	// Progress of this job instance
	Progress() int

	// Refresh information about job instance
	Refresh() error

	// Resources of this job instance
	Resources() []string

	// State of this job instance
	State() string

	// Wait job is finished
	Wait() error
}

// A job implements job instance in CloudSigma account
type job struct {
	client *Client
	obj    *data.Job
}

var _ Job = (*job)(nil)

// String method is used to print values passed as an operand to any format that
// accepts a string or to an unformatted printer such as Print.
func (j job) String() string {
	return fmt.Sprintf(`{UUID: %q, Operation: %s, State: %s, Progress: %d, Resources: %v}`,
		j.UUID(),
		j.Operation(),
		j.State(),
		j.Progress(),
		j.Resources())
}

// URI of job instance
func (j job) URI() string { return j.obj.URI }

// UUID of job instance
func (j job) UUID() string { return j.obj.UUID }

// Children of this job instance
func (j job) Children() []string {
	r := make([]string, len(j.obj.Children))
	copy(r, j.obj.Children)
	return r
}

// Created time of this job instance
func (j job) Created() time.Time { return j.obj.Created }

// LastModified time of this job instance
func (j job) LastModified() time.Time { return j.obj.LastModified }

// Operation of this job instance
func (j job) Operation() string { return j.obj.Operation }

// Progress of this job instance
func (j job) Progress() int { return j.obj.Data.Progress }

// Refresh information about job instance
func (j *job) Refresh() error {
	obj, err := j.client.getJob(j.UUID())
	if err != nil {
		return err
	}
	j.obj = obj
	return nil
}

// Resources of this job instance
func (j job) Resources() []string {
	r := make([]string, len(j.obj.Resources))
	copy(r, j.obj.Resources)
	return r
}

// State of this job instance
func (j job) State() string { return j.obj.State }

// Wait job is finished
func (j *job) Wait() error {
	var stop = false

	timeout := j.client.GetOperationTimeout()
	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() { stop = true })
		defer timer.Stop()
	}

	for j.Progress() < 100 {
		if err := j.Refresh(); err != nil {
			return err
		}
		if stop {
			return ErrOperationTimeout
		}
	}

	return nil
}
