# Example Test Suite

The following is a very quick overview of what each file does and why.

```console
 - task.sh           -> suite registration, bootstraps
 - example.sh        -> test runner and tests
 - other.sh          -> test runner and other tests
```

## task.sh

`task.sh` file is the entry point to the suite. You can bootstrap a juju
controller for use inside the suite. Each controller is namespaced, so running
multiple controllers should be fine.

Registration of tests are also done in this file.

## example.sh

`example.sh` describes a test in the test suite. Generally the grouping of tests
inside of a file should be of the same ilk.

## other.sh

`other.sh` describes another set of tests for the test suite.
