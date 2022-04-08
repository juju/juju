package grpcserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	simpleapi "github.com/juju/juju/grpc/gen/proto/go/juju/client/simple/v1alpha1"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

var logger = loggo.GetLogger("juju.worker.grpcserver")

// Manifold returns a grpcserver manifold.  This starts a gRPC server that
// presents a simple API to users (see simpleapi/simple.proto).
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Start: config.start,
		Inputs: []string{
			config.AgentName,
		},
	}
}

type ManifoldConfig struct {
	AgentName string
}

func (c ManifoldConfig) start(dep dependency.Context) (worker.Worker, error) {
	var agent agent.Agent
	if err := dep.Get(c.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	info, ok := agent.CurrentConfig().StateServingInfo()
	if !ok {
		return nil, errors.New("State serving info not available")
	}
	apiInfo, ok := agent.CurrentConfig().APIInfo()
	if !ok {
		return nil, errors.New("API Info not available")
	}
	serverCert, err := tls.X509KeyPair([]byte(info.Cert), []byte(info.PrivateKey))
	if err != nil {
		return nil, err
	}
	serverCreds := credentials.NewServerTLSFromCert(&serverCert)

	clientCertPool := x509.NewCertPool()
	if !clientCertPool.AppendCertsFromPEM([]byte(apiInfo.CACert)) {
		return nil, errors.New("failed to creat cert pool")
	}
	clientCreds := credentials.NewClientTLSFromCert(clientCertPool, "juju-apiserver")

	grpcServer := grpc.NewServer(grpc.Creds(serverCreds))
	reflection.Register(grpcServer)
	apiServer := &server{}

	simpleapi.RegisterSimpleServiceServer(grpcServer, apiServer)

	lis, err := net.Listen("tcp", ":18888")
	if err != nil {
		return nil, err
	}

	done := make(chan error)
	go func() {
		done <- grpcServer.Serve(lis)
	}()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "localhost:18888", grpc.WithBlock(), grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		grpcServer.Stop()
		return nil, err
	}
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(headerMatcher),
	)
	err = simpleapi.RegisterSimpleServiceHandler(ctx, mux, conn)
	if err != nil {
		grpcServer.Stop()
		return nil, err
	}
	gwServer := &http.Server{
		Addr:    ":18889",
		Handler: mux,
	}
	go func() {
		err := gwServer.ListenAndServe()
		if err != nil {
			logger.Errorf("Error serving gateway: %s", err)
		}
	}()

	return &grpcWorker{
		server: grpcServer,
		done:   done,
	}, nil
}

func headerMatcher(key string) (string, bool) {
	switch key {
	case "X-Juju-Model-Uuid":
		return "model-uuid", true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}

type grpcWorker struct {
	server *grpc.Server
	done   chan error
}

func (w *grpcWorker) Kill() {
	w.server.Stop()
}

func (w *grpcWorker) Wait() error {
	return <-w.done
}
