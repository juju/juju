package ec2

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goamz/s3"
	"launchpad.net/goyaml"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/version"
	"regexp"
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
	return e.PutFile(stateFile, bytes.NewBuffer(data), int64(len(data)))
}

func (e *environ) loadState() (*bootstrapState, error) {
	r, err := e.GetFile(stateFile)
	if err != nil {
		return nil, err
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
	e.bucketMutex.Lock()
	defer e.bucketMutex.Unlock()
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
		return fmt.Errorf("cannot delete all provider state: %v", err)
	default:
	}
	return b.DelBucket()
}

// makeBucket makes the environent's control bucket, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (e *environ) makeBucket() error {
	e.bucketMutex.Lock()
	defer e.bucketMutex.Unlock()
	// try to make the bucket - PutBucket will succeed if the
	// bucket already exists.
	err := e.bucket().PutBucket(s3.Private)
	if err == nil {
		e.madeBucket = true
	}
	return err
}

func (e *environ) PutFile(file string, r io.Reader, length int64) error {
	if err := e.makeBucket(); err != nil {
		return fmt.Errorf("cannot make S3 control bucket: %v", err)
	}
	err := e.bucket().PutReader(file, r, length, "binary/octet-stream", s3.Private)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control bucket: %v", file, err)
	}
	return nil
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

var toolFilePat = regexp.MustCompile(`^tools/juju([0-9]+\.[0-9]+\.[0-9]+)-([^-]+)-([^-]+)\.tgz$`)

var errToolsNotFound = errors.New("no compatible tools found")

// findTools returns a URL from which the juju tools can
// be downloaded appropriate to be run on an image with the
// given specifications.
func (e *environ) findTools(spec *InstanceSpec) (url string, err error) {
	return e.findToolsInBucket(spec, e.bucket())
	// TODO look in public bucket on error
}

// This is a variable so that we can alter it for testing purposes.
var versionCurrentMajor = version.Current.Major

func (e *environ) findToolsInBucket(spec *InstanceSpec, bucket *s3.Bucket) (url string, err error) {
	resp, err := bucket.List("tools/", "/", "", 0)
	if err != nil {
		return "", err
	}
	bestVersion := version.Version{Major: -1}
	bestKey := ""
	for _, k := range resp.Contents {
		m := toolFilePat.FindStringSubmatch(k.Key)
		if m == nil {
			log.Printf("unexpected tools file found %q", k.Key)
			continue
		}
		vers, err := version.Parse(m[1])
		if err != nil {
			log.Printf("failed to parse version %q: %v", k.Key, err)
			continue
		}
		if m[2] != spec.OS {
			continue
		}
		if m[3] != spec.Arch {
			continue
		}
		if vers.Major != versionCurrentMajor {
			continue
		}
		if bestVersion.Less(vers) {
			bestVersion = vers
			bestKey = k.Key
		}
	}
	if bestVersion.Major < 0 {
		return "", errToolsNotFound
	}
	return bucket.URL(bestKey), nil
}
