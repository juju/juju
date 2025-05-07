// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp_test

import (
	"io"
	"net/http"
	"sync"

	"github.com/bmizerany/pat"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/internal/testhelpers"
)

type MuxBenchSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&MuxBenchSuite{})

func (s *MuxBenchSuite) BenchmarkMux(c *tc.C) {
	mux := apiserverhttp.NewMux()
	mux.AddHandler("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.benchmarkMux(c, mux)
}

func (s *MuxBenchSuite) BenchmarkPatMux(c *tc.C) {
	mux := pat.New()
	mux.Add("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.benchmarkMux(c, mux)
}

func (s *MuxBenchSuite) benchmarkMux(c *tc.C, mux http.Handler) {
	req := newRequest("GET", "/hello/blake", nil)
	c.ResetTimer()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < c.N; n++ {
				mux.ServeHTTP(nil, req)
			}
		}()
	}
	wg.Wait()
}

func newRequest(method, urlStr string, body io.Reader) *http.Request {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		panic(err)
	}
	return req
}
