This document demonstrates how to write an integration test for `juju`.

First, navigate to https://github.com/juju/juju/tree/main/tests/suites.

In this directory, create a subdirectory named after the integration test suite
you want to use. Let's call ours `example_integration_test_suite`.

In your test suite directory, create a file called `task.sh`. This file
includes the various setup and teardown functions that are required for your
integration test.

 - Allows for skipping tests
 - Set log verbosity
 - Ensure that dependencies are installed
 - Ensuring that a controller exists, otherwise bootstrap one
 - Run the tests
 - Destroy the controller

An example is given below:

```bash
test_examples() {
	if [ "$(skip 'test_examples')" ]; then
		echo "==> TEST SKIPPED: example tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-example.log"

	bootstrap "test-example" "${file}"

	# Test that need to be run are added here!
	test_example
	test_other

	destroy_controller "test-example"
}
```

Also in your test suite directory, create a `<test name>.sh` file for every
integration test you want to write. For example, we'll create one called
`example_integration_test`, with contents as below. This file consists of a
series of subtests (below, `run_example1` and `run_example2`) and a main
function (below, `test_example`), which is the entrypoint to your integration
test and which contains some standard logic and also runs the subtests.

```bash
run_example1() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists
	file="${TEST_DIR}/test-example1.log"
	ensure "example1" "${file}"

	# Run your checks here
	echo "Hello example 1!" | check "Hello example 1!"

	# Clean up!
	destroy_model "example1"
}

run_example2() {
	echo

	file="${TEST_DIR}/test-example2.log"
	ensure "example2" "${file}"

	echo "Hello example 2!" | check "Hello example 2!"

	destroy_model "example2"
}

test_example() {
	if [ -n "$(skip 'test_example')" ]; then
		echo "==> SKIP: Asked to skip example tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_example1"
		run "run_example2"
	)
}
```

When you are done with your test file, navigate to [test folder](/tests), open
the `main.sh` file (which is the entrypoint to your integration testing overall)
and add your test suite name to the [`TEST_NAMES`
variable](https://github.com/juju/juju/blob/main/tests/main.sh#L42).

Finally, run your integration test, following the instructions in the [test
folder](/tests) . Essentially, what you need to do is as below:

```bash
./main.sh [<suite_name> [<test_name>]]
```