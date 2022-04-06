package grpcserver

import (
	"net"

	"github.com/juju/juju/worker/grpcserver/simpleapi"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"google.golang.org/grpc"

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
	return &grpcWorker{
		server: grpcServer,
		done:   done,
	}, nil
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
