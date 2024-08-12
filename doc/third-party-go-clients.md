# How to create and use Go clients for Juju

This document shows how to create a Go client for Juju.

Contents: 

* Create a `Connector`
* Make a connection
* Get an API client and make requests

## Create a `Connector`

Create a connector instance, telling it how to connect to a Juju controller
and how to provide authentication credentials.

The `github.com/juju/juju/api/connector` package defines a `Connector`
interface.

```golang
type Connector interface {
	Connect(...api.DialOption) (api.Connection, error)
}
```

Its only purpose is to create instances of `api.Connection` that can later be
used to talk to a given Juju controller with a given set of credentials. There
are currently two ways to obtain a `Connector` instance: use the SimpleConnector
or ClientStoreConnector

### Create a `Connector` using`SimpleConnector`

Given controller configuration (controller addresses and ca certificate) and Juju
user credentials (username and password) it is possible to obtain a connector like this:

```go
import "github.com/juju/juju/api/connector"
```
```go
connr, err := connector.NewSimple(connector.SimpleConfig{
    ControllerAddresses: []string{"10.225.205.241:17070"},
    CaCert: "...",
    Username: "jujuuser",
    Password: "password1",
})
```

### Create a `Connector` using `ClientStoreConnector`

If a client store is available on the filesystem, it is possible to use it to
configure the connector.

The NewClientStore will try to find a client store at the location specified by
the JUJU_DATA environment variable which defaults to `~/.local/share/juju`. It is
also possible to pass in a custom value(see `clientstoreconnector.go` for details).

```go
import "github.com/juju/juju/api/connector"
```
```go
connr, err := connector.NewClientStore(connector.ClientStoreConfig{
	ControllerName: "overlord",
})
if err != nil {
	log.Fatalf("Error getting new Juju connector: %s", err)
}
```

## Make a connection

Once we have a connector, make a connection. The connection can be used by multiple
API clients before being closed. It is essential to close the connection when finished
using.

```go
import "log"
```
```go
// Get a connection
conn, err := connr.Connect()
if err != nil {
	log.Fatalf("Error opening connection: %s", err)
}
defer func(){ _ = conn.Close() }()
```

## Get an API client and make requests

Connections can be used to get a Juju API facade client. The client is then used to make
api calls. One connection can be used by multiple API clients.

```go
import (
	"fmt"
	
	"github.com/juju/juju/api/client/modelmanager"
)
```
```go
// Get a model manager facade client.
client := modelmanager.NewClient(conn)

// Find model info for the controller's models.
info, err := client.ListModels("admin")
if err != nil {
	log.Fatalf("Error requesting model info: %s", err)
}

// Print a list of all model names.
names := make([]string, len(info))
for i, model := range info {
	names[i] = model.Name
}
// Print to stdout.
fmt.Printf("%s\n", strings.Join(names, ", "))
```

### Available Juju client APIs

The Juju client APIs are located in `api/client` directory.

- action: Everything to do with actions: run, get output, get status. 
- annotations: Get and set annotations for a Juju entity. Annotations exist for charms, machines,
               units, models, storage and applications.
- application: Everything to do with applications: deploy, refresh, scale.
- applicationoffers: Everything to do with creating and managing cross model relations.
- client: Get status of a Juju model.
- cloud: Everything to do with clouds and cloud credentials.
- highavailability: Enable HA for the Juju controller.
- keymanager: Everything to do with ssh keys for users.
- machinemanager: Add and remove Juju machines
- modelconfig: Manage model config, constraints, and user secrets.
- modelmanager: Everything to do with creating and destroying model.s
- resources: Add new resources to an application, get details of current resources.
- secretbackends: Everything to do with secret backends.
- secrets: Everything to do with user secrets.
- spaces: Manage network spaces for your model.
- storage: Manage and list storage.
- subnets: List subnets
- usermanager: Manage users, permissions are granted/revoke in specific facades.


