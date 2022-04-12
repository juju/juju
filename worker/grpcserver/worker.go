package grpcserver

import (
	"net"
	"net/http"
	"sync"

	"google.golang.org/grpc"
)

type grpcWorker struct {
	grpcServer *grpc.Server
	gwServer   *http.Server
	grpcErr    error
	gwErr      error
	wg         sync.WaitGroup
}

func (w *grpcWorker) serveGrpc(lis net.Listener) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		err := w.grpcServer.Serve(lis)
		if err != nil {
			logger.Infof("gRPC server stopped with error: %s", err)
			w.grpcErr = err
		}
	}()
}

func (w *grpcWorker) serveGw() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		err := w.gwServer.ListenAndServeTLS("", "")
		if err != http.ErrServerClosed {
			logger.Infof("gateway server stopped with error: %s", err)
			w.gwErr = err
		}
	}()
}

func (w *grpcWorker) Kill() {
	w.grpcServer.Stop()
	w.gwServer.Close()
}

func (w *grpcWorker) Wait() error {
	w.wg.Wait()
	if w.grpcErr != nil {
		return w.grpcErr
	}
	if w.gwErr != nil {
		return w.gwErr
	}
	return nil
}
