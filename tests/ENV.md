The following is a list of environment variables that are used to control
behaviour in the Bash tests. This list may not be complete, and as always, the
definitive source is the code.

| Variable name               | Description                                                                   |
|-----------------------------|-------------------------------------------------------------------------------|
| `BOOTSTRAP_ADDITIONAL_ARGS` | Additional arguments passed to `juju bootstrap`.                              |
| `BOOTSTRAP_ARCH`            | Architecture to bootstrap on - passed to `--bootstrap-constraints`.           |
| `BOOTSTRAP_REUSE`           | Reuse an existing controller when asked to bootstrap (true/false).            |
| `BOOTSTRAP_REUSE_LOCAL`     | The name of a local controller to reuse for testing. Set using the `-l` flag. |
| `BOOTSTRAP_SERIES`          | Series to use for the controller. Set using the `-S` flag.                    |
| `KILL_CONTROLLER`           | If `'true'`, controllers will be forcibly killed during teardown.             |
| `MODEL_ARCH`                | Will be set as a model constraint on newly added models.                      |
| `OPERATOR_IMAGE_ACCOUNT`    | Passed as the value of `--config caas-image-repo` when bootstrapping.         |
| `TEST_INSPECT`              | If set, pause before teardown to allow inspection of the controller.          |