// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/amz.v2/s3"

	"github.com/juju/juju/environs/storage"
)

func init() {
	// We will decide when to retry and under what circumstances, not s3.
	// Sometimes it is expected a file may not exist and we don't want s3
	// to hold things up by unilaterally deciding to retry for no good reason.
	s3.RetryAttempts(false)
}

func NewStorage(bucket *s3.Bucket) storage.Storage {
	return &ec2storage{bucket: bucket}
}

// ec2storage implements storage.Storage on
// an ec2.bucket.
type ec2storage struct {
	sync.Mutex
	madeBucket bool
	bucket     *s3.Bucket
}

// makeBucket makes the environent's control bucket, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *ec2storage) makeBucket() error {
	s.Lock()
	defer s.Unlock()
	if s.madeBucket {
		return nil
	}
	// PutBucket always return a 200 if we recreate an existing bucket for the
	// original s3.amazonaws.com endpoint. For all other endpoints PutBucket
	// returns 409 with a known subcode.
	if err := s.bucket.PutBucket(s3.Private); err != nil && s3ErrCode(err) != "BucketAlreadyOwnedByYou" {
		return err
	}

	s.madeBucket = true
	return nil
}

func (s *ec2storage) Put(file string, r io.Reader, length int64) error {
	if err := s.makeBucket(); err != nil {
		return fmt.Errorf("cannot make S3 control bucket: %v", err)
	}
	err := s.bucket.PutReader(file, r, length, "binary/octet-stream", s3.Private)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control bucket: %v", file, err)
	}
	return nil
}

func (s *ec2storage) Get(file string) (r io.ReadCloser, err error) {
	r, err = s.bucket.GetReader(file)
	return r, maybeNotFound(err)
}

func (s *ec2storage) URL(name string) (string, error) {
	// 10 years should be good enough.
	return s.bucket.SignedURL(name, time.Now().AddDate(10, 0, 0)), nil
}

var storageAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// DefaultConsistencyStrategy is specified in the StorageReader interface.
func (s *ec2storage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return storageAttempt
}

// ShouldRetry is specified in the StorageReader interface.
func (s *ec2storage) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	switch err {
	case io.ErrUnexpectedEOF, io.EOF:
		return true
	}
	if s3ErrorStatusCode(err) == 404 {
		return true
	}
	switch e := err.(type) {
	case *net.DNSError:
		return true
	case *net.OpError:
		switch e.Op {
		case "read", "write":
			return true
		}
	case *s3.Error:
		switch e.Code {
		case "InternalError":
			return true
		}
	}
	return false
}

// s3ErrorStatusCode returns the HTTP status of the S3 request error,
// if it is an error from an S3 operation, or 0 if it was not.
func s3ErrorStatusCode(err error) int {
	if err, _ := err.(*s3.Error); err != nil {
		return err.StatusCode
	}
	return 0
}

// s3ErrCode returns the text status code of the S3 error code.
func s3ErrCode(err error) string {
	if err, ok := err.(*s3.Error); ok {
		return err.Code
	}
	return ""
}

func (s *ec2storage) Remove(file string) error {
	err := s.bucket.Del(file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if s3ErrorStatusCode(err) == 404 {
		return nil
	}
	return err
}

func (s *ec2storage) List(prefix string) ([]string, error) {
	// TODO cope with more than 1000 objects in the bucket.
	resp, err := s.bucket.List(prefix, "", "", 0)
	if err != nil {
		// If the bucket is not found, it's not an error
		// because it's only created when the first
		// file is put.
		if s3ErrorStatusCode(err) == 404 {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, key := range resp.Contents {
		names = append(names, key.Key)
	}
	return names, nil
}

func (s *ec2storage) RemoveAll() error {
	names, err := storage.List(s, "")
	if err != nil {
		return err
	}
	// Remove all the objects in parallel to minimize round-trips.
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

	s.Lock()
	defer s.Unlock()
	// Even DelBucket fails, it won't harm if we try again - the operation
	// might have succeeded even if we get an error.
	s.madeBucket = false
	err = deleteBucket(s)
	err = s.bucket.DelBucket()
	if s3ErrorStatusCode(err) == 404 {
		return nil
	}
	return err
}

func deleteBucket(s *ec2storage) (err error) {
	for a := s.DefaultConsistencyStrategy().Start(); a.Next(); {
		err = s.bucket.DelBucket()
		if err == nil || !s.ShouldRetry(err) {
			break
		}
	}
	return err
}

func maybeNotFound(err error) error {
	if err != nil && s3ErrorStatusCode(err) == 404 {
		return errors.NewNotFound(err, "")
	}
	return err
}
