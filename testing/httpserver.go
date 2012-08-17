package testing

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// Server is an HTTP server that is convenient to use in tests.
type Server struct {
	l        net.Listener
	delays   map[string]time.Duration
	contents map[string][]byte
}

func NewServer() *Server {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Errorf("cannot start server: %v", err))
	}
	srv := &Server{l, make(map[string]time.Duration), make(map[string][]byte)}
	go http.Serve(l, srv)
	return srv
}

func (srv *Server) Close() {
	srv.l.Close()
}

// AddContent makes the given data available from the server
// at the given path. It returns a URL that will access that path.
func (srv *Server) AddContent(path string, data []byte) string {
	srv.contents[path] = data
	return fmt.Sprintf("http://%v%s", srv.l.Addr(), path)
}

// RemoveContent makes the given URL path return a 404.
func (srv *Server) RemoveContent(path string) {
	delete(srv.contents, path)
}

// AddDelay causes the server to pause for the supplied duration before
// writing its response to requests for the given path.
func (srv *Server) AddDelay(path string, delay time.Duration) {
	srv.delays[path] = delay
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	<-time.After(srv.delays[req.URL.Path])
	data, found := srv.contents[req.URL.Path]
	if !found {
		http.NotFound(w, req)
		return
	}
	w.Write(data)
}
