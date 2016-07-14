package auditing

import (
	"net/http"

	"github.com/juju/errors"
	"golang.org/x/net/websocket"
)

// NewHTTPHandler returns a new HTTP handler that uses a websocket to
// field HTTP requests.
func NewHTTPHandler(handler websocket.Handler) http.Handler {
	return websocket.Server{
		Handler: handler,
	}
}

// NewWebsocketHandler returns a websocket.Handler which utilizes the
// generic auditing connection handler.
func NewWebsocketHandler(ctx ConnHandlerContext) (websocket.Handler, error) {
	handler, err := NewConnHandler(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return func(conn *websocket.Conn) {
		handler(&connAdapter{
			Conn:  conn,
			Codec: websocket.JSON,
		})
	}, nil
}

type connAdapter struct {
	*websocket.Conn
	websocket.Codec
}

func (a *connAdapter) Send(data interface{}) error {
	return a.Codec.Send(a.Conn, data)
}
