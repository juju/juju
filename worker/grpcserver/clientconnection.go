package grpcserver

import (
	"context"
	"regexp"

	"github.com/juju/juju/api"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/connector"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Get a client facade client from the context.  A convenience function!
func (s *server) getClient(ctx context.Context) (*apiclient.Client, error) {
	conn, err := s.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	return apiclient.NewClient(conn), nil
}

// Get an api.Connection from the context (see getConnector)
func (s *server) getConnection(ctx context.Context) (api.Connection, error) {
	connr, err := s.getConnector(ctx)
	if err != nil {
		return nil, err
	}
	return connr.Connect()
}

// Get a connector from the context.  It has to have the headers:
//
//    authorization: basic <username>:<password>
//    model-uuid: <model UUID>
//
// We can imaging authenticating with a macaroon with a different authorization
// header.
func (s *server) getConnector(ctx context.Context) (connector.Connector, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}
	matches := authPtn.FindStringSubmatch(authHeaders[0])
	if matches == nil {
		return nil, status.Error(codes.InvalidArgument, "bad auth header")
	}
	username := matches[1]
	password := matches[2]
	modelHeaders := md.Get("model-uuid")
	if len(modelHeaders) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing model-uuid header")
	}
	modelUUID := modelHeaders[0]
	return connector.NewSimple(connector.SimpleConfig{
		ControllerAddresses: s.controllerAddresses,
		ModelUUID:           modelUUID,
		Username:            username,
		Password:            password,
	}, api.WithSkipVerifyCA())
}

var authPtn = regexp.MustCompile(`^ *basic *([^ :]+):([^ ]+) *$`)
