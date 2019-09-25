Static Analysis
===============

Static Analysis checking for the juju code base.

Runs the following static analysis:

 - Copyright
 - API Facade Schema
 - Shell
 - Go
    - dependency checking
    - go vet
    - go lint
    - go deadcode
    - misspell
    - ineffassign
    - go fmt
