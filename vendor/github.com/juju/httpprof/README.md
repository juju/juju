net/http/pprof
--------------

This repository is a temporary fork of net/http/pprof
with fixes applied to allow providing the services at a
location other than "/debug/pprof/".

The API is identical to http://golang.org/pkg/net/http/pprof,
with additional exported members to support the use case.
