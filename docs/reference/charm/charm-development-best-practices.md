(charm-development-best-practices)=
# Charm development best practices

This document describes the current best practices around developing and contributing to a charm.

## Conventions

### Programming languages and frameworks

The primary programming language charms are written in Python, and the primary framework for developing charms is the [Python Operator Framework](https://github.com/canonical/operator), or ops.

The recommended way to import the library is to write `import ops` and use it similar to:

```python
import ops
class MyCharm(ops.CharmBase):
    ...
    def _my_event_handler(self, event: ops.EventBase):  # or a more specific type
        ...
        self.unit.status = ops.ActiveStatus()
...
if __name__ == "__main__":
    ops.main(MyCharm)
```

### Naming

Use clear and consequent naming. For example, prometheus includes multiple charms cover different scenarios:

* Running on bare metal as a machine charm, under the name prometheus
* Running in kubernetes as a k8s charm, under the name prometheus-k8s

When naming configuration items or actions, prefer lowercase alphanumeric names, separated with dashes if required. For example `timeout` or `enable-feature`. For charms that have already standardized on underscores, it is not necessary to change them, and it is better to be consistent within a charm then to have some values be dashes and some be underscores.

### State

Write your charm to be stateless. If a charm needs to track state between invocations it should be done as described in the guide on [uses and limitations of stored state](https://juju.is/docs/sdk/stored-state-uses-limitations).

For sharing state between units of the same application, use [peer relation data bags](https://juju.is/docs/sdk/relations#heading--peer-relation-example).

Do not track the emission of events, or elements relating to the charm’s lifecycle, in a state. Where possible, construct this information by accessing the model, i.e. self.model, and the charm’s relations (peer relations or otherwise).

### Revisions

**If your charm's workload is delivered by a snap:** Pin the snap revision to the charm revision by hard-coding it in the charm. Examples:

- [`mysql-router-operator`](https://github.com/canonical/mysql-router-operator/blob/5d89fb6acd08ec3ea2e65356fad520ad5d9862d9/src/snap.py#L25) (the snap revision is hard-coded directly; note that this charm also keeps it in sync with the [workload version](https://github.com/canonical/mysql-router-operator/blob/5d89fb6acd08ec3ea2e65356fad520ad5d9862d9/workload_version) -- something database charms like to do because, to minimize risk and cost, they favor in-place upgrades, and this helps them check compatibility prior to the upgrade)
- [`postgresql-operator`](https://github.com/canonical/postgresql-operator/blob/37570c761deffd11e0e83a324fc256fc6e174adb/src/constants.py#L38) (the snap revision is hard-coded in a format suitable for when you have multiple workload snaps; because the charm is a multi-architecture charm, the revision is also pinned to a particular architecture, and will be selected depending on [the current architecture](https://github.com/canonical/postgresql-operator/blob/37570c761deffd11e0e83a324fc256fc6e174adb/src/charm.py#L1347))

### Resources

Resources can either be of the type oci-image or file. When providing binary files as resources, provide binaries for all CPU architectures your binary might end up being run on. An example of this can be found [here](https://github.com/canonical/prometheus-operator/blob/9fddf95fe29d3a63f8c131f63d7e93d98257d179/metadata.yaml#L37).

Implement the usage of these resources in such a way that the user may build a binary for their architecture of choice and supply it themselves. An example of this can be found [here](https://github.com/canonical/prometheus-operator/blob/9fddf95fe29d3a63f8c131f63d7e93d98257d179/lib/charms/prometheus_k8s/v0/prometheus_scrape.py#L1846-L1857).

### Integrations


Use [Charm Libraries](https://juju.is/docs/sdk/libraries) to distribute code that simplifies implementing any integration for people who wish to integrate with your application. Name charm libraries after the integration interfaces they manage ([example](https://charmhub.io/prometheus-k8s/libraries/prometheus_scrape)).

Implement a separate class for each side of the relation in the same library, for instance:

```python
class MetricsEndpointProvider(ops.Object):

# …

class MetricsEndpointRequirer(ops.Object):

# …
```

These classes should do whatever is necessary to handle any relation events specific to the relation interface you are implementing, throughout the lifecycle of the application. By passing the charm object into the constructor to either the Provider or Requirer, you can gain access to the on attribute of the charm and register event handlers on behalf of the charm, as required.

### Application and unit statuses


Only make changes to the charm’s application or unit status directly within an event handler.

An example:

```python
class MyCharm(ops.CharmBase):

    # This is an event handler, and can therefore set status
    def _on_config_changed(self, event):
        if self._some_helper():
            self.unit.status = ops.ActiveStatus()

    # This is a helper method, not an event handler, so don't set status here
    def _some_helper(self):
        # do stuff
        return True
```

Libraries should never mutate the status of a unit or application. Instead, use return values, or raise exceptions and let them bubble back up to the charm for the charm author to handle as they see fit.

In cases where the library has a suggested default status to be raised, use a custom exception with a .status property containing the suggested charm status as shown [here](https://github.com/juju-solutions/resource-oci-image/blob/fca2ff473e96db170811b81ffe70505ac70612e8/oci_image.py#L57) or [here](https://github.com/canonical/kubeflow-dashboard-operator/blob/2e96dcea52ce6995b49ab439c0d5c04ead22c08c/src/charm.py#L28). The calling charm can then choose to accept the default by setting self.unit.status to raised_exception.status or do something else.

### Logging

#### Templating

Use the default Python logging module. The default charmcraft init template will set this up for you. Do not build strings for the logger. This avoids the string formatting if it's not needed at the given log level.

Prefer

```python
logger.info("something %s", var)
```

over

```python
logger.info("something {}".format(var))

# or

logger.info(f"something {var}")
```

Due to logging features, using f-strings or str.format is a security risk (see [issue46200](https://bugs.python.org/issue46200)) when creating log messages and also causes the string formatting to be done even if the log level for the message is disabled.

#### Frequency


Avoid spurious logging, ensure that log messages are clear and meaningful and provide the information a user would require to rectify any issues.

Avoid excess punctuation or capital letters.

```python
logger.error("SOMETHING WRONG!!!")
```

is significantly less useful than

```python
logger.error("configuration failed: '8' is not valid for field 'enable_debug'.")
```

#### Sensitive information


Never log credentials or other sensitive information. If you really have to log something that could be considered sensitive, use the trace error level.

### Charm configuration option description

The description of configuration in config.yaml is a string type (scalar). YAML supports two types of formats for that: block scalar and flow scalar (more information in [YAML Multiline](https://yaml-multiline.info/)). Prefer to use the block style (using |) to keep new lines. Using > will replace new lines with spaces and make the result harder to read on Charmhub.io.

### When to use Python or Shell

Limit the use of shell scripts and commands as much as possible in favour of writing Python for charm code. There needs to be a good reason to use a shell command rather than Python. Examples where it could be reasonable to use a script include:

* Extracting data from a machine or container which can't be obtained through Python
* Issuing commands to applications that do not have Python bindings (e.g., starting a process on a machine)

(charm-best-practices-documentation)=
## Documentation

Documentation should be considered the user’s handbook for using a charmed application safely and successfully.

It should apply to the charm, and not to the application that is being charmed. Assume that the user already has basic competency in the use of the application. Documentation should include:

* on the [home page](https://docs.google.com/document/d/1O1COQz5SdHb4aQIEVmNioB1baUE0ZswDYqaVeM_6fM8/edit?usp=sharing): what this is, what it does, what problem it solves, who it’s useful for
* an [introductory tutorial](https://diataxis.fr/tutorials/) that gives the new user an experience of what it’s like to use the charm, and an insight into what they’ll be able to do with it - by the end of this, they should have deployed the charm and had a taste of success with it
* [how-to guides](https://diataxis.fr/how-to-guides/) that cover common tasks/problems/application cases
* [reference](https://diataxis.fr/reference/) detailing what knobs and controls the charm offers
* [guides that explain](https://diataxis.fr/explanation/) the bigger picture, advise on best practice, offer context

A good rule of thumb when testing your documentation is to ask yourself whether it provides a means for “guaranteed getting started”. You only get one chance at a first impression, so your quick start should be rock solid.

The front page of your documentation should not carry information about how to build, test or deploy the charm from the local filesystem: put this information in other documentation pages specific to the development of and contribution to your charm. This information can live as part of your [Charm Documentation](https://juju.is/docs/sdk/charm-documentation), or in the version control repository for your charm ([example](https://github.com/jnsgruk/kubernetes-dashboard-operator/blob/main/CONTRIBUTING.md)).

If you’d like some feedback or advice on your Charm’s documentation, ask in our [Mattermost Charmhub Docs channel](https://chat.charmhub.io/charmhub/channels/docs).

## Custom events


Charms should never define custom events themselves. They have no need for emitting events (custom or otherwise) for their own consumption, and as they lack consumers, they don’t need to emit any for others to consume either. Instead, custom events should only be defined in a library.

## Backward compatibility


When authoring your charm, consider the target Python runtime. Kubernetes charms will have access to the default Python version on the Ubuntu version they are running.

Your code should be compatible with the operating system and Juju versions it will be executed on. For example, if your charm is to be deployed with Juju 2.9, its Python code should be compatible with Python 3.5.

Compatibility checks for Python 3.5 can be automated [in your CI](https://github.com/canonical/grafana-operator/pull/42) or using [mypy](https://github.com/canonical/alertmanager-operator/pull/47/files#diff-ef2cef9f88b4fe09ca3082140e67f5ad34fb65fb6e228f119d3812261ae51449).

## Dependency management


External dependencies must be specified in a requirements.txt file. If your charm depends on other charm libraries, you should vendor and version the library you depend on (see the [prometheus-k8s-operator](https://github.com/canonical/prometheus-operator/tree/main/lib/charms)). This is the default behaviour when using charmcraft fetch-lib. For more information see the docs on [Charm Libraries](https://juju.is/docs/sdk/libraries).

Including an external dependency in a charm is a significant choice. It can help with reducing the complexity and development cost. However, a poor dependency pick can lead to critical issues, such as security incidents around the software supply chain. The [Our Software Dependency Problem](https://research.swtch.com/deps) article describes how to assess dependencies for a project in more detail.

## Code style

### Error handling


Use the following mapping of errors that can occur to the state the charm should enter:

* Automatically recoverable error: the charm should go into maintenance status until the error is resolved and then back to active status. Examples of automatically recoverable errors are those where the operation that resulted in the error can be retried.
* Operator recoverable error: The charm should go into the blocked state until the operator resolves the error. An example is that a configuration option is invalid.
* Unexpected/unrecoverable error: the charm should enter the error state. The operator will need to file a bug and potentially downgrade to a previous version of the charm that doesn’t have the bug.

The charm should not catch the parent Exception class and instead only catch specific exceptions. When the charm is in error state, the event that caused the error will be retried by juju until it can be processed without an error. More information about charm statuses is in the [juju documentation](https://juju.is/docs/sdk/constructs#heading--statuses).

### Clarity


Charm authors should choose clarity over cleverness when writing code. A lot more time is spent reading code than writing it, opt for clear code that is easily maintained by anyone. For example, don't write nested, multi-line list comprehensions, and don't overuse itertools.

### User experience (UX)

Charms should aim to keep the user experience of the operator as simple and obvious as possible. If it is harder to use your charm than to set up the application from scratch, why should the user even bother with your charm?

Ensure that the application can be deployed without providing any further configuration options, e.g.

```text
juju deploy foo
```

is preferable over

```text
juju deploy foo --config something=value
```

This will not always be possible, but will provide a better user experience where applicable. Also consider if any of your configuration items could instead be automatically derived from a relation.

A key consideration here is which of your application’s configuration options you should initially expose. If your chosen application has many config options, it may be prudent to provide access to a select few, and add support for more as the need arises.

For very complex applications, consider providing “configuration profiles” which can group values for large configs together. For example, "profile: large" that tweaks multiple options under the hood to optimise for larger deployments, or "profile: ci" for limited resource usage during testing.

### Event handler visibility


Charms should make event handlers private: _on_install, not on_install. There is no need for any other code to directly access the event handlers of a charm or charm library.

### Subprocess calls within Python


For simple interactions with an application or service or when a high quality Python binding is not available, a Python binding may not be worth the maintenance overhead and shell/ subprocess should be used to perform the required operations on the application or service.

For complex use cases where a high quality Python binding is available, using subprocess or the shell for interactions with an application or service will carry a higher maintenance burden than using the Python binding. In these cases, the Python binding for the application or service should be used.

When using subprocess or the shell:

* Log `exit_code` and `stderr` when errors occur.
* Use absolute paths to prevent security issues.
* Prefer subprocess over the shell
* Prefer array-based execution over building strings to execute

For example:

```python
import subprocess

try:
    # Comment to explain why subprocess is used.
    result = subprocess.run(
        # Array based execution.
        ["/usr/bin/echo", "hello world"],
        capture_output=True,
        check=True,
    )
    logger.debug("Command output: %s", result.stdout)
except subprocess.CalledProcessError as err:
    logger.error("Command failed with code %i: %s", err.returncode, err.stderr)
    raise

```

### Linting

Use linters to make sure the code has a consistent style regardless of the author. An example configuration can be found in the [pyproject.toml the charmcraft init template](https://github.com/canonical/charmcraft/blob/main/charmcraft/templates/init-simple/pyproject.toml.j2).

This config makes some decisions about code style on your behalf. At the time of writing, it configures type checking using Pyright, code formatting using Black, and uses ruff to keep imports tidy and to watch for common coding errors.

In general, run these tools inside a tox environment named lint, and one called fmt alongside any testing environments required. See the [Recommended tooling](#heading--recommended-tooling) section for more details.

### Docstrings


Charms should have docstrings. Use the [Google docstring](https://google.github.io/styleguide/pyguide.html#38-comments-and-docstrings) format when writing docstrings for charms. To enforce this, use [ruff](https://github.com/charliermarsh/ruff) as part of our [linter suite](https://juju.is/docs/sdk/styleguide#heading--linters). See [this example](https://google.github.io/styleguide/pyguide.html#doc-function-raises) from the Google style guide.

### Class layout


The class layout of a charm should be organised in the following order:

* Constructor (inside which events are subscribed to, roughly in the order they would be activated)
* Factory methods (classmethods), if any
* Event handlers, placed in order that they’re subscribed to
* Public methods
* Private methods

Further, the use of nested functions is discouraged, instead, use either private methods or module-level functions. Likewise, the use of static methods that could be functions defined near the class in the same module is also discouraged.

## String formatting

f-strings are the preferred way of including variables in a string. For example:

```python
​​foo = "substring"

# .format is not preferred

bar = "string {}".format(foo)

# string concatenation is not preferred

bar = "string " + foo

# f-strings are preferred

bar = f"string {foo}"
```

The only exception to this is logging, where %-formatting should be used. See [above](https://docs.google.com/document/d/1H5W2oi7cuTaz86taTQiITv49jpJNKzzDZ2G25IQpE4w/edit#heading=h.84ez402ai5cq).

Note: f-strings are supported as of Python 3.6. Charms that are based on pre-Bionic Ubuntu versions or libraries needing to support these versions will not have access to f-strings.

### Type hints

Declare type hints on function parameters, return values, and class and instance variables.

Type hints should be checked during development and CI using [Pyright](https://microsoft.github.io/pyright/#/). Although there are other options, Pyright is the recommended one, as it is what is used in `ops` itself (see an [example Pyright config](https://github.com/canonical/alertmanager-k8s-operator/blob/main/pyproject.toml#L31)). More information on type hints can be found in [PEP 484](https://peps.python.org/pep-0484/) and related PEPs.

This will help users know what functions expect as parameters and return and catch more bugs earlier.

Note that there are some cases when type hints might be impractical, for example:

* dictionaries with many nested dictionaries
* decorator functions

## Patterns

### Fetching network information


As a majority of workloads integrate through the means of network traffic, it is common that a charm needs to share its network address over any established relations, or use it as part of its own configuration.

Depending on timing, routing, and topology, some approaches might make more sense than others. Likewise, charms in Kubernetes won’t be able to communicate their cluster FQDN externally, as this address won’t be routable outside of the cluster.

Below you’ll find a couple of different alternatives.

#### Using the bind address


This alternative has the benefit of not relying on name resolution to work. Trying to get a bind_address too early after deployment might result in a None if the DHCP has yet to assign an address.

```python
@property
def _address(self) -> Optional[str]:
    binding = self.model.get_binding(self._peer_relation_name)
    address = binding.network.bind_address

    return str(address) if address else None
```

#### Using FQDN


This alternative has the benefit of being available immediately after the charm is deployed, which eliminates the possible race of the previous example. However, it will in most cases only work for deployments that share a DNS provider (for instance inside a Kubernetes cluster), while in a cross-substrate deployment it most likely won’t resolve.

```python
import socket
...

    @property
    def address(self) -> str:
        """Unit's hostname."""
        return socket.getfqdn()
```


#### Using the ingress URL


This alternative has the benefit of working in most Kubernetes deployment scenarios, even if the opposite side of the relation is not within the same cluster, or even the same substrate.

This does however add a dependency, as it requires an ingress to be available. In the example below, the [traefik](https://charmhub.io/traefik-k8s) ingress is used, falling back to the FQDN if it isn’t available.

Further, keep in mind that unless using ingress-per-unit, the ingress url will not point to the individual unit, but based on the ingress strategy (i.e. per app, or the leader).

```python
@property
def external_url(self) -> str:
    try:
        if ingress_url := self.ingress.url:
            return ingress_url
    except ModelError as e:
        logger.error("Failed obtaining external url: %s. Shutting down?", e)
    return f"http://{socket.getfqdn()}:{self._port}"
```

### Random values


While creating tests, sometimes you need to assign values to variables or parameters in order to simulate a user behaviour, for example. In this case, instead of using constants or fixed values, consider using random ones generated by secrets.token_hex(). This is preferred because:

* If you use the same fixed values in your tests every time, your tests may pass even if there are underlying issues with your code. This can lead to false positives and make it difficult to identify and fix real issues in your code.
* Using random values generated by secrets.token_hex() can help to prevent collisions or conflicts between test data.
* In the case of sensitive data, if you use fixed values in your tests, there is a risk that may be exposed or leaked, especially if your tests are run in a shared environment.

For example:

```python
from secrets import token_hex

email = token_hex(16)
```

## Testing


Charms should have tests to verify that they are functioning correctly. These tests should cover the behaviour of the charm both in isolation (unit tests) and when used with other charms (integration tests). Charm authors should use [tox](https://tox.wiki/en/latest/index.html) to run these automated tests.

The unit and integration tests should be run on the same minor Python version as is shipped with the OS as configured under the charmcraft.yaml bases.run-on key. With tox, for Ubuntu 22.04, this can be done using:

```text
[testenv]

basepython = python3.10
```

### Unit tests


Unit tests are written using the unittest library shipped with Python or [pytest](https://pypi.org/project/pytest/). To facilitate unit testing of charms, use the testing harness specifically designed for charms which is available in [`ops.testing`](https://ops.readthedocs.io/en/latest/#module-ops.testing). An example of charm unit tests can be found [here](https://github.com/canonical/prometheus-operator/blob/main/tests/unit/test_charm.py).

### Functional tests


Functional tests in charms often take the form of integration-, performance- and/or end-to-end tests.

Use the [pytest](https://pytest.org/) library for integration and end-to-end tests. [Pytest-operator](https://github.com/charmed-kubernetes/pytest-operator) is a testing library for interacting with Juju and your charm in integration tests. Examples of integration tests for a charm can be found in the [prometheus-k8-operator repo](https://github.com/canonical/prometheus-operator/blob/main/tests/integration/test_charm.py).

### Integration tests


Integration tests ensure that the charm operates as expected when deployed by a user. Integration tests should cover:

* Charm actions
* Charm integrations
* Charm configurations
* That the workload is up and running, and responsive

When writing an integration test, it is not sufficient to simply check that Juju reports that running the action was successful. Additional checks need to be executed to ensure that whatever the action was intended to achieve worked.

## Recommended tooling

### Continuous integration


The quality assurance pipeline of a charm should be automated using a continuous integration (CI) system. The CI should be configured to use the same version of Ubuntu as configured under the charmcraft.yaml bases.run-on key.

For repositories on GitHub, use the [actions-operator](https://github.com/charmed-kubernetes/actions-operator), which will take care of setting up all dependencies needed to be able to run charms in a CI workflow. You can see an example configuration for linting and testing a charm using Github Actions [here](https://github.com/jnsgruk/zinc-k8s-operator/blob/main/.github/workflows/pull-request.yaml).

The [charming-actions repo](https://github.com/canonical/charming-actions) provides GitHub actions for common workflows, such as publishing a charm or library to charmhub.

The automation should also allow the maintainers to easily see whether the tests failed or passed for any available commit. Provide enough data for the reader to be able to take action, i.e. dumps from juju status, juju debug-log, kubectl describe and similar. To have this done for you, you may integrate [charm-logdump-action](https://github.com/canonical/charm-logdump-action) into your CI workflow.

### Linters


At the time of writing, linting modules commonly used by charm authors include [black](https://github.com/psf/black), [ruff](https://github.com/charliermarsh/ruff), and [codespell](https://github.com/codespell-project/codespell). [bandit](https://bandit.readthedocs.io/en/latest/) can be used to statically check for common security issues. [Pyright](https://microsoft.github.io/pyright/#/) is recommended for static type checking (though MyPy can be used as well).,

## Common integrations

### Observability

Charms need to be observable, meaning that they need to allow the Juju administrator to reason about their internal state and health from the outside. This means that charm authors need to ensure that their charms expose appropriate telemetry, alert rules and dashboards.

#### Metrics


Metrics should be provided in a format compatible with Prometheus. This means that the metrics either should be provided using the [prometheus_remote_write](https://github.com/canonical/charm-relation-interfaces/tree/main/interfaces/prometheus_remote_write/v0) or the [prometheus_scrape](https://github.com/canonical/charm-relation-interfaces/tree/main/interfaces/prometheus_scrape/v0) relation interface.

Some charm workloads have native capabilities of exposing metrics, while others might rely on external tooling in the form of [exporters](https://prometheus.io/docs/instrumenting/exporters/). For Kubernetes charms, these exporters should be placed in a separate container in the same pod as the workload itself.

#### Logs

Any logs relevant for the charm should also be forwarded to Loki over the loki_push_api relation interface. For Kubernetes charms, this is usually accomplished using the [loki_push_api](https://charmhub.io/loki-k8s/libraries/loki_push_api) library, while machine charms will want to integrate with the [Grafana Agent subordinate charm](https://charmhub.io/grafana-agent).

#### Alert rules


Based on the telemetry exposed above, charms should also include opinionated, generic, and robust, alert rules. See [the how-to article on CharmHub](https://charmhub.io/topics/canonical-observability-stack/how-to/add-alert-rules) for more information.

#### Grafana dashboards


Just as with alert rules, charms should be shipped with good, opinionated Grafana dashboards. The goal of these dashboards should be to provide an at-a-glance image of the health and performance of the charm. See [the grafana_dashboard library](https://charmhub.io/grafana-k8s/libraries/grafana_dashboard) for more information.

## Security considerations

Use Juju secrets to share secrets between units or applications.

Do not log secret or private information: no passwords, no private keys, and even be careful with logging “unexpected” keys in json or yaml dictionaries. Logs are likely to be pasted to public pastebin sites in the process of troubleshooting problems.

If you need to generate a password or secret token, prefer the Python `secrets` library over the `random` library or writing your own tool. 16 bytes of random data (32 hex chars) is a reasonable minimum; longer may be useful, but 32 bytes (64 hex chars) is probably a reasonable upper limit for “tokens”.

Enforce a ‘chain of trust’ for all executable content: Ubuntu’s apt configuration defaults to a Merkle-tree of `sha-512` hashes, rooted with GPG keys. This is preferred. Simplestreams can be used with CA-verified HTTPS transfers and `sha-512` hashes. This is acceptable but much more permissive. If pulling content from third-party sites, consider embedding `sha-384` or `sha-512` hashes into the charm, and verifying the hash before operating on the file in any way. Verifying GPG signatures would work but is more challenging to do correctly. Use `gpgv` for GPG signature verification.

Make use of standard Unix user accounts to the extent that it is possible: programs should only run as root when absolutely necessary. Beware of race conditions: it is safer to create or modify files as the target account with the correct permissions in comparison to creating or modifying files as root and then changing ownership or permissions.

Consider using `systemd` security features:

* Seccomp `SystemCallArchitectures=native` and `SystemCallFilter=`
* `User=` and `Group=` are often safer than program’s built-in privilege dropping
* `CapabilityBoundingSet=` can limit the capabilities a service can use
* `AmbientCapabilities=` can provide limited capabilities without requiring root – this can be helpful for eg `CAP_NET_BIND_SERVICE`. (Many capabilities can be leveraged to full root, so it’s not perfect. Binding low ports, for when systemd socket-activation doesn’t work, is probably the best use case.)
* Systemd’s various `ProtectHome=` or `ProtectSystem=` may not play nicely with AppArmor policies.

Consider writing AppArmor profiles for services: Juju ‘owns’ the configuration of services, so there should be minimal conflict with local configurations.

## Example repositories

There are a number of sample repositories you could use for inspiration and a demonstration of good practice.

Kubernetes charms:

* [zinc-k8s](https://github.com/jnsgruk/zinc-k8s-operator)
* [loki-k8s](https://github.com/canonical/loki-k8s-operator)

Machine charms:

* [parca](https://github.com/jnsgruk/parca-operator)
* [mongodb](https://github.com/canonical/mongodb-operator)
* [postgresql](https://github.com/canonical/postgresql-operator)


<!--
The following items from https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md were skipped:

* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#ci-cd: specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#repository-setup: specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#pr-comments-and-requests-for-changes: specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#failing-status-checks: Not directly related to charming
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#test-structure: specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#test-fixture: specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#test-coverage: specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#static-code-analysis: specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#static-code-analysis: Specific to IS DevOps
* https://github.com/canonical/is-charms-contributing-guide/blob/main/CONTRIBUTING.md#handling-typing-issues-with-python-libjuju: perhaps it is better to change the behaviour of python-libjuju than include these in the official juju style guide

-->

