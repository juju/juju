package ec2

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goamz/s3"
	"launchpad.net/goyaml"
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
	// TODO delete the bucket contents and the bucket itself.
	err := e.RemoveFile(stateFile)
	if err != nil {
		return fmt.Errorf("cannot delete provider state: %v", err)
	}
	return nil
}

// makeBucket makes the environent's control bucket, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (e *environ) makeBucket() error {
	e.checkBucket.Do(func() {
		// try to make the bucket - PutBucket will succeed if the
		// bucket already exists.
		e.checkBucketError = e.bucket().PutBucket(s3.Private)
	})
	return e.checkBucketError
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
		if err, _ := err.(*s3.Error); err != nil && err.StatusCode == 404 {
			continue
		}
		return
	}
	return
}

func (e *environ) RemoveFile(file string) error {
	err := e.bucket().Del(file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if err, _ := err.(*s3.Error); err != nil && err.StatusCode == 404 {
		return nil
	}
	return err
}

func (e *environ) bucket() *s3.Bucket {
	return e.s3.Bucket(e.config.bucket)
}

var toolFilePat = regexp.MustCompile("^juju-([0-9]+\.[0-9]+\.[0-9]+)-([^-]+)-([^-]+)$")

// findTools returns a URL from which the juju tools can
// be downloaded. If exact is true, only a version which exactly
// matches version.Current will be used.
func (e *environs) findTools(spec *ImageConstraint, exact string) (url string, err error) {
	resp, err := e.bucket().List("tools/", "/", "", 0)
	if err != nil {
		// TODO look in public bucket
		return err
	}
	bestVersion := version.Version{Major: -1}
	bestKey := ""
	for _, k := range resp.Contents {
		m := toolFilePat.FindStringSubmatch(k.Key)
		if m == nil {
			continue
		}
		vers, err := version.Parse(m[1])
		if err != nil {
			log.Printf("failed to parse version %q: %v", err)
			continue
		}
		if m[2] != 
		if vers.Major != version.Current.Major {
			continue
		}
		if bestVersion.Less(vers) {
			bestVersion = vers
			bestKey = 
	}
