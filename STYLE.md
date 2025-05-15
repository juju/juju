### Code formatting

All code **must** be formatted using `go fmt`. 

#### Line length

Line length must not exceed 80 characters.

### Imports

Import statements must be grouped into 3 sections: standard library, 3rd party
libraries, juju imports. The tool "go fmt" can be used to ensure each
group is alphabetically sorted. eg:

```go
import (
    "fmt"
    "time"

    "github.com/juju/loggo/v2"
    "github.com/juju/worker/v4"

    "github.com/juju/juju/core/model"
)
```

### Method documentation

Do not cut and paste swabs of text from the interface method declaration to each
and every implementation method.
Just use doc comment references with a brief comment relevant to the local
use case.

```go
// Bootstrapper provides the way for bootstrapping controller.
type Bootstrapper interface {
    // PrepareForBootstrap will be called very early in the bootstrap
    // procedure to give an Environ a chance to perform interactive
    // operations that are required for bootstrapping.
    PrepareForBootstrap(ctx BootstrapContext, controllerName string) error

    // Bootstrap creates a new environment, and an instance to host the
    // controller for that environment. The instance will have the
    // series and architecture of the Environ's choice, constrained to
    // those of the available tools. Bootstrap will return the instance's
    // architecture, series, and a function that must be called to finalize
    // the bootstrap process by transferring the tools and installing the
    // initial Juju controller.
    //
    // It is possible to direct Bootstrap to use a specific architecture
    // (or fail if it cannot start an instance of that architecture) by
    // using an architecture constraint; this will have the effect of
    // limiting the available tools to just those matching the specified
    // architecture.
    Bootstrap(
        ctx BootstrapContext, params BootstrapParams,
    ) (*BootstrapResult, error)
}
```

The implementation of an interface method on a receiver just needs a reference
to the interface, but it may be useful to include a brief summary of the
behaviour.

```go
// Bootstrap is part of the [environs.Bootstrapper] interface.
// It will create a new environment and a controller instance.
func (e *Environ) Bootstrap(
    ctx BootstrapContext, params BootstrapParams,
) (*BootstrapResult, error) {
    ...
    ...
}
```

The same applies when declaring a subset of interface methods for local use.
```go
// BootstrapService is a subset of the [environs.Bootstrapper] interface
// used to create a new environment and a controller instance.
type BootstrapperService interface {
    Bootstrap(
        ctx BootstrapContext, params BootstrapParams,
    ) (*BootstrapResult, error)
}
```

### Method documentation (errors)

Method returning errors should be documented with the following pattern:

```go
// LastModelLogin will return the last login time of the specified user.
// The following errors may be returned:
// - [accesserrors.UserNameNotValid] when the username is not valid.
// - [accesserrors.UserNotFound] when the user cannot be found.
// - [modelerrors.NotFound] when no model by the given modelUUID exists.
// - [accesserrors.UserNeverAccessedModel] when there is no record of the user
//   accessing the model.
func (s *UserService) LastModelLogin(
    ctx context.Context, name user.Name, modelUUID coremodel.UUID,
) (time.Time, error) {
    ....
}
```

Use the above approach even when there's only one error (for consistency).
