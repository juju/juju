package httpbakery

import (
	"net"
	"net/http"

	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"

	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
)

type httpRequestKey struct{}

// ContextWithRequest returns the context with information from the
// given request attached as context.  This is used by the httpbakery
// checkers (see RegisterCheckers for details).
func ContextWithRequest(ctx context.Context, req *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestKey{}, req)
}

func requestFromContext(ctx context.Context) *http.Request {
	req, _ := ctx.Value(httpRequestKey{}).(*http.Request)
	return req
}

const (
	// CondClientIPAddr holds the first party caveat condition
	// that checks a client's IP address.
	CondClientIPAddr = "client-ip-addr"

	// CondClientOrigin holds the first party caveat condition that
	// checks a client's origin header.
	CondClientOrigin = "origin"
)

// CheckersNamespace holds the URI of the HTTP checkers schema.
const CheckersNamespace = "http"

var allCheckers = map[string]checkers.Func{
	CondClientIPAddr: ipAddrCheck,
	CondClientOrigin: clientOriginCheck,
}

// RegisterCheckers registers all the HTTP checkers with the given checker.
// Current checkers include:
//
//	client-ip-addr <ip-address>
//
// The client-ip-addr caveat checks that the HTTP request has
// the given remote IP address.
//
//    origin <name>
//
// The origin caveat checks that the HTTP Origin header has
// the given value.
func RegisterCheckers(c *checkers.Checker) {
	c.Namespace().Register(CheckersNamespace, "http")
	for cond, check := range allCheckers {
		c.Register(cond, CheckersNamespace, check)
	}
}

// NewChecker returns a new checker with the standard
// and HTTP checkers registered in it.
func NewChecker() *checkers.Checker {
	c := checkers.New(nil)
	RegisterCheckers(c)
	return c
}

// ipAddrCheck implements the IP client address checker
// for an HTTP request.
func ipAddrCheck(ctx context.Context, cond, args string) error {
	req := requestFromContext(ctx)
	if req == nil {
		return errgo.Newf("no IP address found in context")
	}
	ip := net.ParseIP(args)
	if ip == nil {
		return errgo.Newf("cannot parse IP address in caveat")
	}
	if req.RemoteAddr == "" {
		return errgo.Newf("client has no remote address")
	}
	reqIP, err := requestIPAddr(req)
	if err != nil {
		return errgo.Mask(err)
	}
	if !reqIP.Equal(ip) {
		return errgo.Newf("client IP address mismatch, got %s", reqIP)
	}
	return nil
}

// clientOriginCheck implements the Origin header checker
// for an HTTP request.
func clientOriginCheck(ctx context.Context, cond, args string) error {
	req := requestFromContext(ctx)
	if req == nil {
		return errgo.Newf("no origin found in context")
	}
	// Note that web browsers may not provide the origin header when it's
	// not a cross-site request with a GET method. There's nothing we
	// can do about that, so just allow all requests with an empty origin.
	if reqOrigin := req.Header.Get("Origin"); reqOrigin != "" && reqOrigin != args {
		return errgo.Newf("request has invalid Origin header; got %q", reqOrigin)
	}
	return nil
}

// SameClientIPAddrCaveat returns a caveat that will check that
// the remote IP address is the same as that in the given HTTP request.
func SameClientIPAddrCaveat(req *http.Request) checkers.Caveat {
	if req.RemoteAddr == "" {
		return checkers.ErrorCaveatf("client has no remote IP address")
	}
	ip, err := requestIPAddr(req)
	if err != nil {
		return checkers.ErrorCaveatf("%v", err)
	}
	return ClientIPAddrCaveat(ip)
}

// ClientIPAddrCaveat returns a caveat that will check whether the
// client's IP address is as provided.
func ClientIPAddrCaveat(addr net.IP) checkers.Caveat {
	if len(addr) != net.IPv4len && len(addr) != net.IPv6len {
		return checkers.ErrorCaveatf("bad IP address %d", []byte(addr))
	}
	return httpCaveat(CondClientIPAddr, addr.String())
}

// ClientOriginCaveat returns a caveat that will check whether the
// client's Origin header in its HTTP request is as provided.
func ClientOriginCaveat(origin string) checkers.Caveat {
	return httpCaveat(CondClientOrigin, origin)
}

func httpCaveat(cond, arg string) checkers.Caveat {
	return checkers.Caveat{
		Condition: checkers.Condition(cond, arg),
		Namespace: CheckersNamespace,
	}
}

func requestIPAddr(req *http.Request) (net.IP, error) {
	reqHost, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return nil, errgo.Newf("cannot parse host port in remote address: %v", err)
	}
	ip := net.ParseIP(reqHost)
	if ip == nil {
		return nil, errgo.Newf("invalid IP address in remote address %q", req.RemoteAddr)
	}
	return ip, nil
}
