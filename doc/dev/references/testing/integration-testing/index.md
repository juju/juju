> <small> [Testing](/t/7203) > Integration testing</small>
>
> See also: [How to write a integration test](/t/7210)

Integration testing `juju` currently relies on a series of custom-made `bash` scripts. All these scripts live
in https://github.com/juju/juju/tree/develop/tests . This directory includes two subdirectories, one containing
integration [test suites](https://github.com/juju/juju/tree/develop/tests/suites) and the
other [test includes](https://github.com/juju/juju/tree/develop/tests/includes). Both are tools that can help you create
integration tests.

A typical integration testing package consists of:

- A `<suite name>` directory in the [/tests/suites](https://github.com/juju/juju/tree/develop) directory.
- Inside this directory, a main script for the integration test suite, `task.sh`. This is the entrypoint to your
  integration test suite.
- In the same directory, a separate `<test name>.sh` file for every test.
- The https://github.com/juju/juju/blob/develop/tests/main.sh , which is the entrypoint to your integration testing.
  This file contains a [
  `TEST_NAMES` variable](https://github.com/juju/juju/blob/6a378dfee8c0b109b5e71b035d5acf1da940f1cd/tests/main.sh#L40),
  which contains the names of all your integration test suites. Whenever you develop a new integration test suites, you
  need to add its name to this variable.

> See more:
> - [Integration test suite](/t/7258)
    >     - [Test includes](/t/7206)