// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp_test

import (
	"io"
	"net/http"
	"sync"
	stdtesting "testing"

	"github.com/bmizerany/pat"

	"github.com/juju/juju/apiserver/apiserverhttp"
)

func BenchmarkMux(b *stdtesting.B) {
	mux := apiserverhttp.NewMux()
	mux.AddHandler("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	benchmarkMux(b, mux)
}

func BenchmarkPatMux(b *stdtesting.B) {
	mux := pat.New()
	mux.Add("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	benchmarkMux(b, mux)
}

func benchmarkMux(b *stdtesting.B, mux http.Handler) {
	req := newRequest("GET", "/hello/blake", nil)
	b.ResetTimer()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < b.N; n++ {
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
