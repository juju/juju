// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424test

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
)

// Handler defines an interface for handling RFC5424 messages.
type Handler interface {
	HandleSyslog(Message Message)
}

// HandlerFunc is a Handler that is implemented as a function.
type HandlerFunc func(Message)

func (f HandlerFunc) HandleSyslog(m Message) {
	f(m)
}

// Message contains the content and origin address of an RFC5424 message.
type Message struct {
	RemoteAddr string
	Message    string
}

// Server is a server for testing the receipt of RFC5424 messages.
type Server struct {
	Listener net.Listener
	TLS      *tls.Config
	handler  Handler

	mu       sync.Mutex
	wg       sync.WaitGroup
	listener net.Listener // Listener, or Listener wrapped with TLS
	closed   bool
	conns    []net.Conn
}

// NewServer creates a new Server which will invoke the given Handler
// for received messages. The server will listen for connections on
// localhost, on an ephemeral port. The listening address can be
// obtained by inspecting the Server's Listener field.
//
// The Server returned will not listen for connections until Start
// is called.
func NewServer(handler Handler) *Server {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("NewServer: %v", err))
	}
	return &Server{Listener: l, handler: handler}
}

// Start starts the server listening for client connections.
func (s *Server) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		panic("Start: server already closed")
	}
	if s.listener != nil {
		panic("Start: server already started")
	}
	s.listener = s.Listener
	s.goServe()
}

// StartTLS starts the server listening for client connections
// using TLS.
func (s *Server) StartTLS() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		panic("StartTLS: server already closed")
	}
	if s.listener != nil {
		panic("StartTLS: server already started")
	}
	if s.TLS == nil || len(s.TLS.Certificates) == 0 {
		panic("no certificates specified")
	}
	s.listener = tls.NewListener(s.Listener, s.TLS)
	s.goServe()
}

// Close closes any client connections, and stops the server from accepting
// any new ones.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.wg.Wait()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		if s.listener != nil {
			s.listener.Close()
			for _, conn := range s.conns {
				conn.Close()
			}
		}
	}
}

func (s *Server) goServe() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.serve()
	}()
}

func (s *Server) serve() {
	defer s.listener.Close()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			s.mu.Lock()
			if !s.closed {
				s.conns = append(s.conns, conn)
				defer s.serveConn(conn)
			}
			s.mu.Unlock()
		}()
	}
}

func (s *Server) serveConn(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		text := scanner.Text()
		message := Message{
			RemoteAddr: remoteAddr,
			Message:    text,
		}
		s.handler.HandleSyslog(message)
	}
}
