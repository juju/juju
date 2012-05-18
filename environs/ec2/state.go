package ec2

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju/go/environs"
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
	return e.Storage().Put(stateFile, bytes.NewBuffer(data), int64(len(data)))
}

func (e *environ) loadState() (*bootstrapState, error) {
	r, err := e.Storage().Get(stateFile)
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

func maybeNotFound(err error) error {
	if s3ErrorStatusCode(err) == 404 {
		return &environs.NotFoundError{err}
	}
	return err
}

func (e *environ) deleteState() error {
	s := e.Storage().(*storage)

	names, err := s.List("")
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
	wg.Add(len(names))
	errc := make(chan error, len(names))
	for _, name := range names {
		name := name
		go func() {
			if err := s.Remove(name); err != nil {
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
	return s.bucket.DelBucket()
}

var toolFilePat = regexp.MustCompile(`^tools/juju([0-9]+\.[0-9]+\.[0-9]+)-([^-]+)-([^-]+)\.tgz$`)

var errToolsNotFound = errors.New("no compatible tools found")

// findTools returns a URL from which the juju tools can
// be downloaded. If exact is true, only a version which exactly
// matches version.Current will be used.
func (e *environ) findTools() (url string, err error) {
	return e.findToolsInBucket(e.bucket())
	// TODO look in public bucket on error
}

// This is a variable so that we can alter it for testing purposes.
var versionCurrentMajor = version.Current.Major

func (e *environ) findToolsInBucket(bucket *s3.Bucket) (url string, err error) {
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
		if m[2] != version.CurrentOS {
			continue
		}
		// TODO allow different architectures.
		if m[3] != version.CurrentArch {
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
