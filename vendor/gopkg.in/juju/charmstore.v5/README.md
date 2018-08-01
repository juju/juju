# juju/charmstore

Store and publish Juju charms and bundles.

## Installation

To start using the charm store, first ensure you have a valid
Go environment, then run the following:

    go get -d gopkg.in/juju/charmstore.v5-unstable
    cd $GOPATH/gopkg.in/juju/charmstore.v5-unstable

## Go dependencies

The project uses godeps (https://launchpad.net/godeps) to manage Go
dependencies. To install this, run:

    go get launchpad.net/godeps

After installing it, you can update the dependencies
to the revision specified in the `dependencies.tsv` file with the following:

    make deps

Use `make create-deps` to update the dependencies file.

## Development environment

A couple of system packages are required in order to set up a charm store
development environment. To install them, run the following:

    make sysdeps

To run the elasticsearch tests you must run an elasticsearch server. If the
elasticsearch server is running at an address other than localhost:9200 then
set `JUJU_TEST_ELASTICSEARCH=<host>:<port>` where host and port provide
the address of the elasticsearch server. If you do not wish to run the
elasticsearh tests, set `JUJU_TEST_ELASTICSEARCH=none`.

At this point, from the root of this branch, run the command::

    make install

The command above builds and installs the charm store binaries, and places them
in `$GOPATH/bin`. This is the list of the installed commands:

- charmd: start the charm store server;
- essync: synchronize the contents of the Elastic Search database with the charm store.

A description of each command can be found below.

## Testing

Run `make check` to test the application.
Run `make help` to display help about all the available make targets.

## Charmstore server

Once the charms database is fully populated, it is possible to interact with
charm data using the charm store server. It can be started with the following
command:

    charmd -logging-config INFO cmd/charmd/config.yaml

The same result can be achieved more easily by running `make server`.
Note that this configuration *should not* be used when running
a production server, as it uses a known password for authentication.

At this point the server starts listening on port 8080 (as specified in the
config YAML file).
