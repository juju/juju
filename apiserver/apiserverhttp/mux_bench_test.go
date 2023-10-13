// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp_test

/*
type MuxBenchSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&MuxBenchSuite{})

func (s *MuxBenchSuite) BenchmarkMux(c *gc.C) {
	mux := apiserverhttp.NewMux()
	mux.AddHandler("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.benchmarkMux(c, mux)
}

func (s *MuxBenchSuite) BenchmarkPatMux(c *gc.C) {
	mux := pat.New()
	mux.Add("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.benchmarkMux(c, mux)
}

func (s *MuxBenchSuite) benchmarkMux(c *gc.C, mux http.Handler) {
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
*/
