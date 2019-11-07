# Test Suite

The following package contains the integration test suite for Juju.

## Getting started

To get started, it's best to quickly look at the help command from the runner.

```sh
./main.sh -h
```

Running a full sweep of the integration tests (which will take a long time), can
be done by running:

```sh
./main.sh
```
<<<<<<< HEAD
=======

Note: running subtests, will also invoke the parent test to ensure that it has
everything setup correctly.

Running `./main.sh deploy run_deploy_bundle` will also run `test_deploy_bundles`,
but no other subtests, just `run_deploy_bundle`.

### Using local controllers

The use of local controllers whilst development is advantagous because you don't
have to rebootstrap a controller, or you can test a particular setup that has
been manually created.

To do so, just specify a controller name and pass it though.

```sh
./main.sh -l <local-controller-name> deploy
```

Note: because you're using a local controller, you don't get the same guarantees
of setup and cleanup that you can with letting the test suite do that for you.
So if you expect everything to be cleaned up and leave no trace, then don't use
this method of bootstrapping.
>>>>>>> c7d38f0830... Update README
