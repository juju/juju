(integration-testing)=
# Integration testing

```{toctree}
:titlesonly:
:glob:

*
```

> See also: {ref}`write-an-integration-test`

Integration testing `juju` currently relies on a series of custom-made `bash` scripts. All these scripts live
in the `tests` directory. This directory includes two subdirectories, one containing
integration {ref}`test suites <integration-test-suite>` and the other integration {ref}`test includes <test-include>`. Both are tools that can help you create
integration tests.

A typical integration testing package consists of:

- A `<suite name>` directory in the `tests/suites` directory.
- Inside this directory, a main script for the integration test suite, `task.sh`. This is the entrypoint to your
  integration test suite.
- In the same directory, a separate `<test name>.sh` file for every test.
- The `tests/main.sh` test script, which is the entrypoint to your integration testing.
  This file contains a `TEST_NAMES` variable which contains the names of all your integration test suites. Whenever you develop a new integration test suites, you
  need to add its name to this variable.
