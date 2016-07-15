package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"net/http"
	runtimepprof "runtime/pprof"
	"strings"
	"text/template"

	"github.com/juju/httpprof"

	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

type pprofHandler struct {
	mux  *http.ServeMux
	auth adminAuthenticator
}

type adminAuthenticator interface {
	authenticateAdmin(*http.Request) error
}

func newPprofHandler(auth adminAuthenticator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/cmdline", pprof.Cmdline)
	mux.HandleFunc("/profile", pprof.Profile)
	mux.HandleFunc("/symbol", pprof.Symbol)
	mux.HandleFunc("/", pprofIndex)
	return &pprofHandler{
		mux:  mux,
		auth: auth,
	}
}

func (h *pprofHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := h.auth.authenticateAdmin(req); err != nil {
		router.WriteError(w, err)
		return
	}
	h.mux.ServeHTTP(w, req)
}

// pprofIndex is copied from pprof.Index with minor modifications
// to make it work using a relative path.
func pprofIndex(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/" {
		profiles := runtimepprof.Profiles()
		if err := indexTmpl.Execute(w, profiles); err != nil {
			logger.Errorf("cannot execute pprof template: %v", err)
		}
		return
	}
	name := strings.TrimPrefix(req.URL.Path, "/")
	pprof.Handler(name).ServeHTTP(w, req)
}

var indexTmpl = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html>
<head>
  <title>pprof</title>
</head>
<body>
  <h1>pprof</h1>
  <h2>profiles:</h2>
  <table>
    {{range .}}
    <tr>
      <td style="text-align: right;">{{.Count}}</td>
      <td><a href="{{.Name}}?debug=1">{{.Name}}</a></td>
    </tr>
    {{end}}
  </table>
  <p><a href="goroutine?debug=2">full goroutine stack dump</a></p>
</body>
</html>
`))
