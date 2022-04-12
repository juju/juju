package grpcserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	simpleapi "github.com/juju/juju/grpc/gen/proto/go/juju/client/simple/v1alpha1"
	"github.com/juju/juju/pki"
	pkitls "github.com/juju/juju/pki/tls"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/httpserver"
	workerstate "github.com/juju/juju/worker/state"
)

var logger = loggo.GetLogger("juju.worker.grpcserver")

// Manifold returns a grpcserver manifold.  This starts a gRPC server that
// presents a simple API to users (see simpleapi/simple.proto).
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Start: config.start,
		Inputs: []string{
			config.AgentName,
			config.AuthorityName,
			config.StateName,
		},
	}
}

type ManifoldConfig struct {
	AgentName     string
	AuthorityName string
	StateName     string

	GetControllerConfig func(*state.State) (controller.Config, error)
	NewTLSConfig        func(*state.State, httpserver.SNIGetterFunc, httpserver.Logger) (*tls.Config, error)
}

func (c ManifoldConfig) start(dep dependency.Context) (worker.Worker, error) {
	var agent agent.Agent
	if err := dep.Get(c.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	apiInfo, ok := agent.CurrentConfig().APIInfo()
	if !ok {
		return nil, errors.New("API Info not available")
	}

	// This block of code is copied from httpserver to get a *tls.Config and
	// access to the controller config.
	var authority pki.Authority
	if err := dep.Get(c.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}
	var stTracker workerstate.StateTracker
	if err := dep.Get(c.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			_ = stTracker.Done()
		}
	}()
	systemState := statePool.SystemState()
	tlsConfig, err := c.NewTLSConfig(
		systemState,
		pkitls.AuthoritySNITLSGetter(authority, logger),
		logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfig, err := c.GetControllerConfig(systemState)
	if err != nil {
		return nil, errors.Annotate(err, "unable to get controller config")
	}
	// End of copied block of code.

	grpcPort := controllerConfig.GrpcAPIPort()
	gwPort := controllerConfig.GrpcGatewayAPIPort()

	// Optionally get client credentials for the gRPC client connection required
	// by the gateway.  Using api.NewTLSConfig to make sure the correct server
	// name is set in the tls Config.
	var clientCertPool *x509.CertPool
	if apiInfo.CACert != "" {
		clientCertPool = x509.NewCertPool()
		if !clientCertPool.AppendCertsFromPEM([]byte(apiInfo.CACert)) {
			return nil, errors.New("failed to creat cert pool")
		}
	}
	clientCreds := credentials.NewTLS(api.NewTLSConfig(clientCertPool))

	// Set up gRPC server
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	reflection.Register(grpcServer)
	simpleapi.RegisterSimpleServiceServer(grpcServer, &server{
		controllerAddresses: []string{fmt.Sprintf("127.0.0.1:%d", controllerConfig.APIPort())},
	})

	// Set up gateway server
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(headerMatcher),
	)
	gwServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", gwPort),
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// Get a worker that will manage the lifetime of both servers.
	worker := &grpcWorker{
		grpcServer: grpcServer,
		gwServer:   gwServer,
	}

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return nil, err
	}
	logger.Infof("Serving gRPC on port %d", grpcPort)
	worker.serveGrpc(lis)

	// Start gateway server
	ctx := context.Background()
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", grpcPort), grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		grpcServer.Stop()
		return nil, err
	}
	err = simpleapi.RegisterSimpleServiceHandler(ctx, mux, conn)
	if err != nil {
		grpcServer.Stop()
		return nil, err
	}
	logger.Infof("Serving gateway on port %d", gwPort)
	worker.serveGw()

	return worker, nil
}

func headerMatcher(key string) (string, bool) {
	switch key {
	case "X-Juju-Model-Uuid":
		return "model-uuid", true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}
