// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/mocks"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type OperationsSuite struct{}

var _ = gc.Suite(&OperationsSuite{})

func (s *OperationsSuite) TestConcurrentGetResource(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	er := mocks.NewMockEntityRepository(ctrl)
	rg := mocks.NewMockResourceGetter(ctrl)

	stateLock := sync.Mutex{}
	fetchMut := &sync.Mutex{}
	fetchMut.Lock()

	er.EXPECT().FetchLock(gomock.Any()).AnyTimes().Return(fetchMut)

	openState := struct {
		res    coreresource.Resource
		buffer []byte
		err    error
	}{
		err: errors.NotFoundf("resource"),
	}
	er.EXPECT().OpenResource(gomock.Any()).AnyTimes().DoAndReturn(func(name string) (coreresource.Resource, io.ReadCloser, error) {
		stateLock.Lock()
		defer stateLock.Unlock()
		reader := io.ReadCloser(nil)
		if openState.err == nil && openState.buffer != nil {
			reader = ioutil.NopCloser(bytes.NewBuffer(openState.buffer))
		}
		return openState.res, reader, openState.err
	})

	getState := struct {
		res coreresource.Resource
	}{
		res: coreresource.Resource{
			ApplicationID: "gitlab",
			Username:      "gitlab-0",
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name: "company-icon",
				},
				Origin: charmresource.OriginStore,
			},
		},
	}
	er.EXPECT().GetResource(gomock.Any()).AnyTimes().DoAndReturn(func(name string) (coreresource.Resource, error) {
		stateLock.Lock()
		defer stateLock.Unlock()
		return getState.res, nil
	})

	gomock.InOrder(
		rg.EXPECT().GetResource(resource.ResourceRequest{
			CharmID: resource.CharmID{URL: charm.MustParseURL("cs:gitlab")},
			Name:    "company-icon",
		}).Times(1).Return(resource.ResourceData{
			ReadCloser: ioutil.NopCloser(bytes.NewBufferString("data")),
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name: "company-icon",
				},
				Origin: charmresource.OriginStore,
			},
		}, nil),
		er.EXPECT().SetResource(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(res charmresource.Resource, reader io.Reader, arg state.IncrementCharmModifiedVersionType) (charmresource.Resource, error) {
			stateLock.Lock()
			defer stateLock.Unlock()
			// Make sure this takes a while.
			time.Sleep(10 * time.Millisecond)
			buf, err := ioutil.ReadAll(reader)
			if err != nil {
				return charmresource.Resource{}, errors.Trace(err)
			}
			res.Size = int64(len(buf))
			openState.buffer = buf
			openState.err = nil
			getState.res.Resource = res
			openState.res = getState.res
			return res, nil
		}),
	)

	numRequests := 100
	done := sync.WaitGroup{}
	args := resource.GetResourceArgs{
		Client:     rg,
		Repository: er,
		Name:       "company-icon",
		CharmID:    resource.CharmID{URL: charm.MustParseURL("cs:gitlab")},
		Done:       done.Done,
	}

	start := sync.WaitGroup{}
	for i := 0; i < numRequests; i++ {
		start.Add(1)
		done.Add(1)
		go func() {
			start.Done()
			rsc, reader, err := resource.GetResource(args)
			c.Check(err, jc.ErrorIsNil)
			c.Check(reader, gc.NotNil)
			defer func() { _ = reader.Close() }()
			c.Check(rsc, gc.DeepEquals, getState.res)
		}()
	}

	start.Wait()
	// This synchronises all the threads to start at the same time.
	fetchMut.Unlock()

	finished := make(chan bool)
	go func() {
		done.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for resources to be closed")
	}
}

func (s *OperationsSuite) TestGetResourceErrorReleasesLock(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	er := mocks.NewMockEntityRepository(ctrl)
	rg := mocks.NewMockResourceGetter(ctrl)

	fetchMut := &sync.Mutex{}
	er.EXPECT().FetchLock(gomock.Any()).AnyTimes().Return(fetchMut)
	er.EXPECT().OpenResource(gomock.Any()).DoAndReturn(func(name string) (coreresource.Resource, io.ReadCloser, error) {
		return coreresource.Resource{}, io.ReadCloser(nil), errors.NotFoundf("resource")
	})
	er.EXPECT().GetResource(gomock.Any()).AnyTimes().DoAndReturn(func(name string) (coreresource.Resource, error) {
		return coreresource.Resource{}, errors.New("boom")
	})
	called := false
	args := resource.GetResourceArgs{
		Client:     rg,
		Repository: er,
		Name:       "company-icon",
		CharmID:    resource.CharmID{URL: charm.MustParseURL("cs:gitlab")},
		Done: func() {
			called = true
		},
	}

	_, _, err := resource.GetResource(args)
	c.Check(err, gc.ErrorMatches, "boom")
	c.Assert(called, jc.IsTrue)
}
