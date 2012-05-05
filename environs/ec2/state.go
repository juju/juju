package ec2

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goamz/s3"
	"launchpad.net/goyaml"
	"os"
	"sync"
)

const stateFile = "provider-state"

type bootstrapState struct {
	ZookeeperInstances []string `yaml:"zookeeper-instances"`
}

func (e *environ) saveState(state *bootstrapState) error {
	data, err := goyaml.Marshal(state)
	if err != nil {
		return err
	}
	return e.PutFile(stateFile, bytes.NewReader(data))
}

func (e *environ) loadState() (*bootstrapState, error) {
	r, err := e.GetFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %v", stateFile, err)
	}
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %v", stateFile, err)
	}
	var state bootstrapState
	err = goyaml.Unmarshal(data, &state)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", stateFile, err)
	}
	return &state, nil
}

func (e *environ) deleteState() error {
	b := e.bucket()
	resp, err := b.List("", "", "", 0)
	if err != nil {
		if s3ErrorStatusCode(err) == 404 {
			return nil
		}
		return err
	}
	// Remove all the objects in parallel so that we incur less round-trips.
	// If we're in danger of having hundreds of objects,
	// we'll want to change this to limit the number
	// of concurrent operations.
	var wg sync.WaitGroup
	wg.Add(len(resp.Contents))
	errc := make(chan error, len(resp.Contents))
	for _, obj := range resp.Contents {
		name := obj.Key
		go func() {
			if err := e.RemoveFile(name); err != nil {
				errc <- err
			}
			wg.Done()
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return fmt.Errorf("cannot delete provider state: %v", err)
	default:
	}
	return b.DelBucket()
}

func (e *environ) PutFile(file string, r io.ReadSeeker) error {
	// Find out the length of the file by seeking to
	// the end and back again.
	curPos, err := r.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	length, err := r.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}
	if _, err := r.Seek(curPos, os.SEEK_SET); err != nil {
		return err
	}
	// To avoid round-tripping on each PutFile, we attempt to put the
	// file and only make the bucket if it fails due to the bucket's
	// non-existence.
	err = e.bucket().PutReader(file, r, length - curPos, "binary/octet-stream", s3.Private)
	if err == nil {
		return nil
	}
	if s3err, _ := err.(*s3.Error); s3err == nil || s3err.Code != "NoSuchBucket" {
		return err
	}
	// Make the bucket and repeat.
	if err := e.bucket().PutBucket(s3.Private); err != nil {
		return err
	}
	if _, err := r.Seek(curPos, os.SEEK_SET); err != nil {
		return err
	}
	return e.bucket().PutReader(file, r, length - curPos, "binary/octet-stream", s3.Private)
}

func (e *environ) GetFile(file string) (r io.ReadCloser, err error) {
	for a := shortAttempt.start(); a.next(); {
		r, err = e.bucket().GetReader(file)
		if s3ErrorStatusCode(err) == 404 {
			continue
		}
		return
	}
	return
}

// s3ErrorStatusCode returns the HTTP status of the S3 request error,
// if it is an error from an S3 operation, or 0 if it was not.
func s3ErrorStatusCode(err error) int {
	if err, _ := err.(*s3.Error); err != nil {
		return err.StatusCode
	}
	return 0
}

func (e *environ) RemoveFile(file string) error {
	err := e.bucket().Del(file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if s3ErrorStatusCode(err) == 404 {
		return nil
	}
	return err
}

func (e *environ) bucket() *s3.Bucket {
	return e.s3.Bucket(e.config.bucket)
}
