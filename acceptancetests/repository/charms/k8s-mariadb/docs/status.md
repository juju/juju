<h1 id="charms.layer.status.WorkloadState">WorkloadState</h1>

```python
WorkloadState(self, /, *args, **kwargs)
```

Enum of the valid workload states.

Valid options are:

  * `WorkloadState.MAINTENANCE`
  * `WorkloadState.BLOCKED`
  * `WorkloadState.WAITING`
  * `WorkloadState.ACTIVE`

<h1 id="charms.layer.status.maintenance">maintenance</h1>

```python
maintenance(message)
```

Set the status to the `MAINTENANCE` state with the given operator message.

__Parameters__

- __`message` (str)__: Message to convey to the operator.

<h1 id="charms.layer.status.maint">maint</h1>

```python
maint(message)
```

Shorthand alias for
[maintenance](status.md#charms.layer.status.maintenance).

__Parameters__

- __`message` (str)__: Message to convey to the operator.

<h1 id="charms.layer.status.blocked">blocked</h1>

```python
blocked(message)
```

Set the status to the `BLOCKED` state with the given operator message.

__Parameters__

- __`message` (str)__: Message to convey to the operator.

<h1 id="charms.layer.status.waiting">waiting</h1>

```python
waiting(message)
```

Set the status to the `WAITING` state with the given operator message.

__Parameters__

- __`message` (str)__: Message to convey to the operator.

<h1 id="charms.layer.status.active">active</h1>

```python
active(message)
```

Set the status to the `ACTIVE` state with the given operator message.

__Parameters__

- __`message` (str)__: Message to convey to the operator.

<h1 id="charms.layer.status.status_set">status_set</h1>

```python
status_set(workload_state, message)
```

Set the status to the given workload state with a message.

__Parameters__

- __`workload_state` (WorkloadState or str)__: State of the workload.  Should be
    a [WorkloadState](status.md#charms.layer.status.WorkloadState) enum
    member, or the string value of one of those members.
- __`message` (str)__: Message to convey to the operator.

