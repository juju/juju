(test-include)=
# Test include
> Source: https://github.com/juju/juju/tree/main/tests/includes

In Juju, test **includes** are special `bash` util functions designed to help in creating effective integration test
suites for `juju`. This document gives the complete list (STILL UNDER CONSTRUCTION) along with quick descriptions.

|               |                 |                                                                                                                                                                                                                   |
|---------------|-----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `check`       |                 | Various check functions that help to analyse the output of cli commands and define if test passed or not. Most commonly used: `check_contains`, `check`, `check_gt`.                                              |
| `cleanup`     |                 | Functions that clean up after test is complete.                                                                                                                                                                   |
| `colors`      |                 | Colorize the output if the terminal supports doing this.                                                                                                                                                          |
| `date`        |                 | Add date to test output.                                                                                                                                                                                          |
| `expect-that` |                 | The ability to work with interactive commands with expect tool.                                                                                                                                                   |
| `juju`        |                 | Functions that allow to operate with juju bootstrap, models, controllers. E.g., `ensure`, `bootstrap`, ...                                                                                                        |
|               | `destroy-model` | Takes a model name and destroys the model.                                                                                                                                                                        ||
|               | `ensure`        | Ensures that there is a bootstrapped controller with model `<model name>`.                                                                                                                                        |
| `random`      |                 | Generated random string.                                                                                                                                                                                          |
| `run`         |                 | Run a command and immediately terminate the script when any error occurs.                                                                                                                                         |
| `storage`     |                 | Functions which help to test the storages.                                                                                                                                                                        |
| `verbose`     |                 | Define the level of verbosity. There are three levels of verbosity. Both 1 and 2 will fail on any error -- the difference is that 2 will also turn on `juju debug` statements, though not shell debug statements. |