(integration-testing)=
# Integration testing

```{toctree}
:titlesonly:
:glob:

*
```

> See also: [How to write a integration test](/doc/dev/how-to/write-an-integration-test.md)

Integration testing `juju` currently relies on a series of custom-made `bash` scripts. All these scripts live
in [test folder](/tests). This directory includes two subdirectories, one containing
integration [test suites](/tests/suites) and the
other [test includes](/tests/includes). Both are tools that can help you create
integration tests.

A typical integration testing package consists of:

- A `<suite name>` directory in the [test suites](/tests/suites) directory.
- Inside this directory, a main script for the integration test suite, `task.sh`. This is the entrypoint to your
  integration test suite.
- In the same directory, a separate `<test name>.sh` file for every test.
- The [main test script](/tests/main.sh), which is the entrypoint to your integration testing.
  This file contains a [`TEST_NAMES` variable](https://github.com/juju/juju/blob/main/tests/main.sh#L42),
  which contains the names of all your integration test suites. Whenever you develop a new integration test suites, you
  need to add its name to this variable.

> See more:
> 
> - [Integration test suite](integration-test-suite.md)
>   - [Test includes](test-includes.md)