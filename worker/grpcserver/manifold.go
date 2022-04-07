package grpcserver

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	simpleapi "github.com/juju/juju/grpc/gen/proto/go/juju/client/simple/v1alpha1"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

// Manifold returns a grpcserver manifold.  This starts a gRPC server that
// presents a simple API to users (see simpleapi/simple.proto).
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Start: config.start,
	}
}

type ManifoldConfig struct {
}

func (c ManifoldConfig) start(dep dependency.Context) (worker.Worker, error) {
	ctx := context.Background()
	lis, err := net.Listen("tcp", ":18888")
	if err != nil {
		return nil, err
	}
	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)
	apiServer := &server{}

	simpleapi.RegisterSimpleServiceServer(grpcServer, apiServer)
	done := make(chan error)
	go func() {
		done <- grpcServer.Serve(lis)
	}()
	conn, err := grpc.DialContext(ctx, "127.0.0.1:18888", grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		return nil, err
	}
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(headerMatcher),
	)
	err = simpleapi.RegisterSimpleServiceHandler(ctx, mux, conn)
	if err != nil {
		log.Fatalf("Failed to register gateway: %s", err)
	}
	gwServer := &http.Server{
		Addr:    ":18889",
		Handler: mux,
	}
	go func() {
		gwServer.ListenAndServe()
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
