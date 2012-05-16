package ec2

import (
	"fmt"
	"io"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju/go/environs"
	"path"
	"sync"
)

// storage implements environs.StorageReadWriter on
// an ec2.bucket.
type storage struct {
	checkBucket      sync.Once
	checkBucketError error
	b                *s3.Bucket
}

// makeBucket makes the environent's control bucket, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *storage) makeBucket() error {
	s.checkBucket.Do(func() {
		// try to make the bucket - PutBucket will succeed if the
		// bucket already exists.
		s.checkBucketError = s.b.PutBucket(s3.Private)
	})
	return s.checkBucketError
}

func (s *storage) Put(file string, r io.Reader, length int64) error {
	if err := s.makeBucket(); err != nil {
		return fmt.Errorf("cannot make S3 control bucket: %v", err)
	}
	err := s.b.PutReader(file, r, length, "binary/octet-stream", s3.Private)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control bucket: %v", file, err)
	}
	return nil
}

func (s *storage) Get(file string) (r io.ReadCloser, err error) {
	for a := shortAttempt.start(); a.next(); {
		r, err = s.b.GetReader(file)
		if s3ErrorStatusCode(err) == 404 {
			continue
		}
		return
	}
	if s3ErrorStatusCode(err) == 404 {
		err = &environs.NotFoundError{err}
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

func (s *storage) Remove(file string) error {
	err := s.b.Del(file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if s3ErrorStatusCode(err) == 404 {
		return nil
	}
	return err
}

func (s *storage) List(dir string) ([]string, error) {
	dir = path.Clean(dir)
	if dir == "/" {
		dir = ""
	} else {
		dir += "/"
	}

	// TODO cope with more than 1000 objects in the bucket.
	resp, err := s.b.List(dir, "", "", 0)
	if err != nil {
		if s3ErrorStatusCode(err) == 404 {
			return nil, &environs.NotFoundError{err}
		}
		return nil, err
	}
	var names []string
	for _, key := range resp.Contents {
		names = append(names, key.Key)
	}
	return names, nil
}

func (e *environ) Storage() environs.StorageReadWriter {
	return &e.store
}

func (e *environ) PublicStorage() environs.StorageReader {
	// TODO use public storage bucket
	return environs.EmptyStorage
}
