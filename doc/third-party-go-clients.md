# Third party Go clients

This describes an approach to allow third party Go codebases to drive Juju.  At
a high level it works like this.

1. Create a connector instance, telling it how to connect to a Juju controller
   and how to provide authentication credentials.
2. Use this connector instance to obtain a connection instance.
3. Get a facade client from the connection.
4. Use the facade client to make one or more requests.

## The `Connector` interface

The `github.com/juju/juju/api/connector` package defines a `Connector`
interface.

```golang
type Connector interface {
	Connect(...api.DialOption) (api.Connection, error)
}
```

Its only purpose is to create instances of `api.Connection` that can later be
used to talk to a given Juju controller with a given set of credentials.  There
are currently two ways to obtain a `Connector` instance.

### `SimpleConnector`

If one knows the how to contact a controller, and some juju user credentials
(e.g. username and password) it is possible to obtain a connector like this.

```golang
import "github.com/juju/juju/api/connector"
```
```golang
connr, err := connector.NewSimple(connector.SimpleConfig{
    ControllerAddresses: []string{"10.225.205.241:17070"},
    CaCert: "...",
    Username: "jujuuser",
    Password: "password1",
})
```

There can be an error if the config is not valid (e.g. no controller address
specified).

### `ClientStoreConnector`

If a client store is available on the filesystem, it is possible to use it to
configure the connector.

```golang
import "github.com/juju/juju/api/connector"
```
```golang
connr, err := connector.NewClientStore(connector.ClientStoreConfig{
    ControllerName: "overlord",
})
```

The above will try to find a client store in the default location.  It is also
possible to pass in a custom value (see `clientstoreconnector.go` for details).

## Making requests

Once we have a connector, it's easy to make requests.

```golang
import (
    "encoding/json"
	"github.com/juju/juju/api/connector"
	"github.com/juju/juju/api/client/client"
)
```
```golang
    // Get a connection
	conn, err := connr.Connect()
	if err != nil {
		log.Fatalf("Error opening connection: %s", err)
	}
    defer conn.Close()

    // Get a Client facade client
	client := apiclient.NewClient(conn)

    // Call the Status endpoint of the client facade
    status, err := client.Status(nil)
    if err != nil {
        log.Fatalf("Error requesting status: %s", err)
    }

    // Print to stdout.
    b, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		log.Fatalf("Error marshalling response: %s", err)
	}
	fmt.Printf("%s\n", b)
```

## Points to address

- When to reuse a connection, when to create a new one?
- How to find the useful endpoints?

