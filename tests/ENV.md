The following is a list of environment variables that are used to control
behaviour in the Bash tests. This list may not be complete, and as always, the
definitive source is the code.

| Variable name                 | Description                                                                        |
|-------------------------------|------------------------------------------------------------------------------------|
| `BOOTSTRAP_ADDITIONAL_ARGS`   | Additional arguments passed to `juju bootstrap`.                                   |
| `BOOTSTRAP_ARCH`              | Architecture to bootstrap on - passed to `--bootstrap-constraints`.                |
| `BOOTSTRAP_CLOUD`             | The cloud to use when bootstrapping (i.e. the first argument to `juju bootstrap`). |
| `BOOTSTRAP_PROVIDER`          | The provider to use when bootstrapping (see the `provider` package).               |
| `BOOTSTRAP_REUSE`             | Reuse an existing controller when asked to bootstrap (true/false).                 |
| `BOOTSTRAP_REUSE_LOCAL`       | The name of a local controller to reuse for testing. Set using the `-l` flag.      |
| `BOOTSTRAP_SERIES`            | Series to use for the controller. Set using the `-S` flag.                         |
| `CONTROLLER_CHARM_CHANNEL`    | The channel to pull the controller charm from (CaaS only).                         |
| `CONTROLLER_CHARM_PATH_CAAS`  | The Charmhub charm name to pull the controller charm from (CaaS only).             |
| `CONTROLLER_CHARM_PATH_IAAS`  | Path to a locally built controller charm to use (IaaS only).                       |
| `KILL_CONTROLLER`             | If `'true'`, controllers will be forcibly killed during teardown.                  |
| `MODEL_ARCH`                  | Will be set as a model constraint on newly added models.                           |
| `JUJU_MIGRATION_36_BIN`       | Path to a custom juju 3.6 client for the `migration` suite's source controllers. When unset, the suite pins the regular `juju` snap to `3.6/edge` and uses `/snap/bin/juju`; afterwards the snap is refreshed back to its previously tracked channel, or removed again when it was not installed before the run. The `jujud-operator:3.6.30` image is not published on any registry yet, so k8s source-controller bootstraps need it built and imported into the cluster first (`make microk8s-operator-update`). |
| `OPERATOR_IMAGE_ACCOUNT`      | Passed as the value of `--config caas-image-repo` when bootstrapping.              |
| `TEST_INSPECT`                | If set, pause before teardown to allow inspection of the controller.               |
| `CONTAINER_NETWORKING_METHOD` | If set, the default container networking method (`local`, `provider`, or `fan`).   |

The migration suite saves the original snap installation state and channel in
`${TEST_DIR}` before changing the snap. Parent-shell cleanup restores it only
after controller teardown and all registered cleanup callbacks have finished,
since those callbacks still need the 3.6 client. Setup refuses to change the
snap if the installed-snap inventory or an installed snap's channel cannot be
recorded. Cleanup uses `snap refresh --amend` to restore the previous channel,
including an older revision, and warns if restoration fails. Missing or
unreadable cleanup state never authorises snap removal.

Run `bash tests/tools/test-migration-snap.sh` from the repository root to check
this lifecycle without touching snapd or deploying controllers. It exercises
the real test runner with isolated snap and controller fixtures.
