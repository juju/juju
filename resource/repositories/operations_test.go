// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repositories_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/repositories"
	"github.com/juju/juju/resource/repositories/mocks"
	"github.com/juju/juju/state"
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
		res    resource.Resource
		buffer []byte
		err    error
	}{
		err: errors.NotFoundf("resource"),
	}
	er.EXPECT().OpenResource(gomock.Any()).AnyTimes().DoAndReturn(func(name string) (resource.Resource, io.ReadCloser, error) {
		stateLock.Lock()
		defer stateLock.Unlock()
		reader := io.ReadCloser(nil)
		if openState.err == nil && openState.buffer != nil {
			reader = ioutil.NopCloser(bytes.NewBuffer(openState.buffer))
		}
		return openState.res, reader, openState.err
	})

	getState := struct {
		res resource.Resource
	}{
		res: resource.Resource{
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
	er.EXPECT().GetResource(gomock.Any()).AnyTimes().DoAndReturn(func(name string) (resource.Resource, error) {
		stateLock.Lock()
		defer stateLock.Unlock()
		return getState.res, nil
	})

	gomock.InOrder(
		rg.EXPECT().GetResource(repositories.ResourceRequest{
			CharmID: repositories.CharmID{URL: charm.MustParseURL("cs:gitlab")},
			Name:    "company-icon",
		}).Times(1).Return(charmstore.ResourceData{
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

	args := repositories.GetResourceArgs{
		Client:     rg,
		Repository: er,
		Name:       "company-icon",
		CharmID:    repositories.CharmID{URL: charm.MustParseURL("cs:gitlab")},
	}

	start := sync.WaitGroup{}
	done := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		start.Add(1)
		done.Add(1)
		go func() {
			defer done.Done()
			start.Done()
			rsc, reader, err := repositories.GetResource(args)
			c.Check(err, jc.ErrorIsNil)
			c.Check(reader, gc.NotNil)
			c.Check(rsc, gc.DeepEquals, getState.res)
		}()
	}

	start.Wait()
	// This synchronises all the threads to start at the same time.
	fetchMut.Unlock()
	done.Wait()
}
