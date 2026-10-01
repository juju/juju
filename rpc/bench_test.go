// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/rpc/params"
)

// benchRoot is a minimal RPC root exposing echo methods used to
// exercise the codec with realistic API payloads.
type benchRoot struct{}

func (benchRoot) Bench(id string) (*benchMethods, error) {
	return &benchMethods{}, nil
}

type benchMethods struct{}

func (benchMethods) EchoEntities(e params.Entities) params.Entities {
	return e
}

func (benchMethods) EchoStatus(s params.FullStatus) params.FullStatus {
	return s
}

type benchPayload struct {
	name   string
	action string
	arg    any
	newRes func() any
}

func benchPayloads() []benchPayload {
	return []benchPayload{{
		name:   "small",
		action: "EchoEntities",
		arg: params.Entities{Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
		}},
		newRes: func() any { return new(params.Entities) },
	}, {
		name:   "medium",
		action: "EchoStatus",
		arg:    makeStatus(10, 3, 10),
		newRes: func() any { return new(params.FullStatus) },
	}, {
		name:   "large",
		action: "EchoStatus",
		arg:    makeStatus(100, 5, 100),
		newRes: func() any { return new(params.FullStatus) },
	}}
}

func makeStatus(apps, unitsPerApp, machines int) params.FullStatus {
	since := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	detailed := func(status, info string) params.DetailedStatus {
		return params.DetailedStatus{
			Status:  status,
			Info:    info,
			Data:    map[string]any{"hook": "install", "retries": 3.0},
			Since:   &since,
			Version: "4.0.1",
			Life:    "alive",
		}
	}
	fs := params.FullStatus{
		Model: params.ModelStatusInfo{
			Name:        "bench",
			Type:        "iaas",
			CloudTag:    "cloud-aws",
			CloudRegion: "us-east-1",
			Version:     "4.0.1",
			ModelStatus: detailed("available", ""),
		},
		Machines:            make(map[string]params.MachineStatus, machines),
		Applications:        make(map[string]params.ApplicationStatus, apps),
		ControllerTimestamp: &since,
	}
	for i := 0; i < machines; i++ {
		id := fmt.Sprint(i)
		fs.Machines[id] = params.MachineStatus{
			AgentStatus:    detailed("started", ""),
			InstanceStatus: detailed("running", "running"),
			DNSName:        fmt.Sprintf("10.0.0.%d", i),
			IPAddresses:    []string{fmt.Sprintf("10.0.0.%d", i), fmt.Sprintf("252.0.0.%d", i)},
			InstanceId:     instance.Id("i-" + strings.Repeat("0", 8) + id),
			Base:           params.Base{Name: "ubuntu", Channel: "24.04"},
			Id:             id,
			Containers:     map[string]params.MachineStatus{},
			Constraints:    "arch=amd64 mem=4096M",
			Hardware:       "arch=amd64 cores=2 mem=4096M root-disk=20480M",
			Jobs:           []model.MachineJob{"JobHostUnits"},
		}
	}
	for i := 0; i < apps; i++ {
		name := fmt.Sprintf("app-%d", i)
		units := make(map[string]params.UnitStatus, unitsPerApp)
		for j := 0; j < unitsPerApp; j++ {
			units[fmt.Sprintf("%s/%d", name, j)] = params.UnitStatus{
				AgentStatus:     detailed("idle", ""),
				WorkloadStatus:  detailed("active", "ready"),
				WorkloadVersion: "8.0.32",
				Machine:         fmt.Sprint((i + j) % max(machines, 1)),
				OpenedPorts:     []string{"3306/tcp", "33060/tcp"},
				PublicAddress:   fmt.Sprintf("10.1.%d.%d", i, j),
				Charm:           "ch:amd64/jammy/" + name + "-42",
				Leader:          j == 0,
			}
		}
		fs.Applications[name] = params.ApplicationStatus{
			Charm:            "ch:amd64/jammy/" + name + "-42",
			CharmVersion:     "abc123",
			CharmChannel:     "8.0/stable",
			CharmRev:         42,
			Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
			Life:             "alive",
			Relations:        map[string][]string{"db": {"other-app"}},
			Units:            units,
			Status:           detailed("active", ""),
			WorkloadVersion:  "8.0.32",
			EndpointBindings: map[string]string{"": "alpha", "db": "alpha"},
			Scale:            unitsPerApp,
		}
	}
	return fs
}

// discardConn accepts and drops all writes.
type discardConn struct{}

func (discardConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (discardConn) Write(p []byte) (int, error) { return len(p), nil }
func (discardConn) Close() error                { return nil }

// repeatConn replays the same bytes forever.
type repeatConn struct {
	data []byte
	off  int
}

func (r *repeatConn) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		r.off = 0
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

func (*repeatConn) Write(p []byte) (int, error) { return len(p), nil }
func (*repeatConn) Close() error                { return nil }

type bufferConn struct {
	bytes.Buffer
}

func (*bufferConn) Close() error { return nil }

func BenchmarkCodecWrite(b *testing.B) {
	for _, p := range benchPayloads() {
		b.Run(p.name, func(b *testing.B) {
			var buf bufferConn
			hdr := &rpc.Header{RequestId: 1, Version: 1}
			if err := jsoncodec.NewNet(&buf).WriteMessage(hdr, p.arg); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(buf.Len()))

			codec := jsoncodec.NewNet(discardConn{})
			b.ReportAllocs()
			for b.Loop() {
				if err := codec.WriteMessage(hdr, p.arg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCodecRead(b *testing.B) {
	for _, p := range benchPayloads() {
		b.Run(p.name, func(b *testing.B) {
			var buf bufferConn
			hdr := &rpc.Header{RequestId: 1, Version: 1}
			if err := jsoncodec.NewNet(&buf).WriteMessage(hdr, p.arg); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(buf.Len()))

			codec := jsoncodec.NewNet(&repeatConn{data: buf.Bytes()})
			b.ReportAllocs()
			for b.Loop() {
				var hdr rpc.Header
				if err := codec.ReadHeader(&hdr); err != nil {
					b.Fatal(err)
				}
				if err := codec.ReadBody(p.newRes(), false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRoundTripTCP(b *testing.B) {
	for _, p := range benchPayloads() {
		b.Run(p.name, func(b *testing.B) {
			client, cleanup := newBenchTCPClientServer(b)
			defer cleanup()
			runBenchCalls(b, client, p)
		})
	}
}

func BenchmarkRoundTripWebsocket(b *testing.B) {
	for _, p := range benchPayloads() {
		b.Run(p.name, func(b *testing.B) {
			client, cleanup := newBenchWebsocketClientServer(b)
			defer cleanup()
			runBenchCalls(b, client, p)
		})
	}
}

func runBenchCalls(b *testing.B, client *rpc.Conn, p benchPayload) {
	ctx := context.Background()
	req := rpc.Request{Type: "Bench", Id: "x", Action: p.action}
	// Warm up and validate.
	if err := client.Call(ctx, req, p.arg, p.newRes()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Call(ctx, req, p.arg, p.newRes()); err != nil {
			b.Fatal(err)
		}
	}
}

func serveBench(ctx context.Context, codec rpc.Codec) *rpc.Conn {
	conn := rpc.NewConn(codec, nil)
	conn.Serve(benchRoot{}, nil, nil)
	conn.Start(ctx)
	return conn
}

func newBenchTCPClientServer(b *testing.B) (*rpc.Conn, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		c, err := l.Accept()
		if err != nil {
			return
		}
		server := serveBench(ctx, jsoncodec.NewNet(c))
		<-server.Dead()
		_ = server.Close()
	}()
	c, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	client := rpc.NewConn(jsoncodec.NewNet(c), nil)
	client.Start(ctx)
	return client, func() {
		_ = client.Close()
		<-serverDone
		_ = l.Close()
		cancel()
	}
}

func newBenchWebsocketClientServer(b *testing.B) (*rpc.Conn, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var upgrader websocket.Upgrader
	serverDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(serverDone)
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		server := serveBench(ctx, jsoncodec.NewWebsocket(ws))
		<-server.Dead()
		_ = server.Close()
	}))
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		b.Fatal(err)
	}
	client := rpc.NewConn(jsoncodec.NewWebsocket(ws), nil)
	client.Start(ctx)
	return client, func() {
		_ = client.Close()
		<-serverDone
		srv.Close()
		cancel()
	}
}
