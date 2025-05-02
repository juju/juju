(integration-testing)=
# Integration testing

```{toctree}
:titlesonly:
:glob:

*
```

> See also: {ref}`write-an-integration-test`

Integration testing `juju` currently relies on a series of custom-made `bash` scripts. All these scripts live
in the [tests folder](https://github.com/juju/juju/tree/main/tests). This directory includes two subdirectories, one containing
integration [test suites](https://github.com/juju/juju/tree/main/tests/suites) and the
other [test includes](https://github.com/juju/juju/tree/main/tests/includes). Both are tools that can help you create
integration tests.

A typical integration testing package consists of:

- A `<suite name>` directory in the [test suites](https://github.com/juju/juju/tree/main/tests/suites) directory.
- Inside this directory, a main script for the integration test suite, `task.sh`. This is the entrypoint to your
  integration test suite.
- In the same directory, a separate `<test name>.sh` file for every test.
- The [main test script](https://github.com/juju/juju/blob/main/tests/main.sh), which is the entrypoint to your integration testing.
  This file contains a [`TEST_NAMES` variable](https://github.com/juju/juju/blob/main/tests/main.sh#L42),
  which contains the names of all your integration test suites. Whenever you develop a new integration test suites, you
  need to add its name to this variable.

> See more:
> 
> - {ref}`integration-test-suite`
>   - {ref}`test-include`