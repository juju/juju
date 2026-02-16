---
applyTo: "**/*.go"
---

# Go Code Style Instructions

## Docstrings

All exported types, interfaces, functions and methods in non-test code MUST have a docstring comment that starts with
the name of the symbol being documented and is made up of complete sentences. This also applies to interface methods.

For example:

```go
// ApplicationState describes retrieval and persistence methods for
// applications.
type ApplicationState interface {
	// GetApplicationName returns the name of the specified application.
	GetApplicationName(context.Context, coreapplication.UUID) (string, error)
}
```
```go
// NodeManager is responsible for interrogating a single Dqlite node,
// and emitting configuration for starting its Dqlite `App` based on
// operational requirements and controller agent config.
type NodeManager struct {
	...
}
```
```go
// IsRemoteApplication returns true if the application name indicates
// that it is a remote application. This is determined by checking if the
// application name is of the form remote-<uuid> (where <uuid> is a valid UUID
// without dashes).
func IsRemoteApplication(appName string) bool {
	...
}
```
```go
// GetApplicationEndpointBindings returns the mapping for each endpoint name and
// the space ID it is bound to (or empty if unspecified). When no bindings are
// stored for the application, defaults are returned.
func (s *Service) GetApplicationEndpointBindings(ctx context.Context, appName string) (map[string]network.SpaceUUID, error) {
	...
}
```
