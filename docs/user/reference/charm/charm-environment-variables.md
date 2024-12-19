(charm-environment-variables)=
# Charm environment variables

When a charm runs, it will gather all sorts of information from its execution context. These are environment variables. They are used to parametrise the charm's runtime.   

> Charms written with the operator framework benefit from the various abstraction levels on top of the environment variables, so charm authors rarely (if at all) need to work with them directly.

<!--TODO: ADD COMPLETE LIST OF ENVIRONMENT VARIABLES-->

Depending on the event type, there might be some differences in what environment variables are set. For example, in a `pebble-ready` event they would be as below, where the `JUJU_WORKLOAD_NAME` environment variable is actually unique to this event.

```python
{
 'APT_LISTCHANGES_FRONTEND': 'none',
 'CHARM_DIR': '/var/lib/juju/agents/unit-bare-0/charm',
 'CLOUD_API_VERSION': '1.23.0',
 'DEBIAN_FRONTEND': 'noninteractive',
 'JUJU_AGENT_SOCKET_ADDRESS': '@/var/lib/juju/agents/unit-bare-0/agent.socket',
 'JUJU_AGENT_SOCKET_NETWORK': 'unix',
 'JUJU_API_ADDRESSES': '10.152.183.4:17070 '
                       'controller-service.controller-charm-dev.svc.cluster.local:17070',
 'JUJU_AVAILABILITY_ZONE': '',
 'JUJU_CHARM_DIR': '/var/lib/juju/agents/unit-bare-0/charm',
 'JUJU_CHARM_FTP_PROXY': '',
 'JUJU_CHARM_HTTPS_PROXY': '',
 'JUJU_CHARM_HTTP_PROXY': '',
 'JUJU_CHARM_NO_PROXY': '127.0.0.1,localhost,::1',
 'JUJU_CONTEXT_ID': 'bare/0-workload-pebble-ready-1918120275707858680',
 'JUJU_DISPATCH_PATH': 'hooks/workload-pebble-ready',
 'JUJU_HOOK_NAME': 'workload-pebble-ready',
 'JUJU_MACHINE_ID': '',
 'JUJU_METER_INFO': 'not set',
 'JUJU_METER_STATUS': 'AMBER',
 'JUJU_MODEL_NAME': 'welcome',
 'JUJU_MODEL_UUID': 'cdac5656-2423-4388-8f30-41854b4cca7d',
 'JUJU_PRINCIPAL_UNIT': '',
 'JUJU_SLA': 'unsupported',
 'JUJU_UNIT_NAME': 'bare/0',
 'JUJU_VERSION': '2.9.29',
 'JUJU_WORKLOAD_NAME': 'workload',
 'KUBERNETES_PORT': 'tcp://10.152.183.1:443',
 'KUBERNETES_PORT_443_TCP': 'tcp://10.152.183.1:443',
 'KUBERNETES_PORT_443_TCP_ADDR': '10.152.183.1',
 'KUBERNETES_PORT_443_TCP_PORT': '443',
 'KUBERNETES_PORT_443_TCP_PROTO': 'tcp',
 'KUBERNETES_SERVICE_HOST': '10.152.183.1',
 'KUBERNETES_SERVICE_PORT': '443',
 'KUBERNETES_SERVICE_PORT_HTTPS': '443',
 'LANG': 'C.UTF-8',
 'OPERATOR_DISPATCH': '1',
 'PATH': '/var/lib/juju/tools/unit-bare-0:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/charm/bin',
 'PWD': '/var/lib/juju/agents/unit-bare-0/charm',
 'PYTHONPATH': 'lib:venv',
 'TERM': 'tmux-256color',
}
```

The environment variables for a specific event can be obtained by running `juju debug-hooks <unit name>` and waiting for the desired event to fire. If the next prompt looks as below

    root@database-0:/var/lib/juju#

this means that we are still waiting for an event to occur. As soon as that happens, the prompt will look similar to the below

    root@database-0:/var/lib/juju/agents/unit-database-0/charm#

and this means we're inside the charm execution context. At this point,  typing `printenv` will print out the environment variables.

## Event variables

### Core hooks
| `JUJU_HOOK_NAME`                                                                                    | `JUJU_DISPATCH_PATH`              | Notes                                                  |
|-----------------------------------------------------------------------------------------------------|-----------------------------------|--------------------------------------------------------|
| [`'workload-pebble-ready'`](https://discourse.charmhub.io/t/container-name-pebble-ready-event/6468) | `'hooks/workload-pebble-ready'`   | This is the only event that sets `JUJU_WORKLOAD_NAME`. |
| [`'update-status'`](https://discourse.charmhub.io/t/update-status-event/6484)                       | `'hooks/update-status'`           |                                                        |
| [`'install'`](https://discourse.charmhub.io/t/install-event/6469)                                   | `'hooks/install'`                 |                                                        |
| [`'leader-elected'`](https://discourse.charmhub.io/t/leader-elected-event/6470)                     | `'hooks/leader-elected'`          |                                                        |
| [`'leader-settings-changed'`](https://discourse.charmhub.io/t/leader-settings-changed-event/6471)   | `'hooks/leader-settings-changed'` |                                                        |
| [`'start'`](https://discourse.charmhub.io/t/start-event/6482)                                       | `'hooks/start'`                   |                                                        |
| [`'stop'`](https://discourse.charmhub.io/t/stop-event/6483)                                         | `'hooks/stop'`                    |                                                        |
| [`'remove'`](https://discourse.charmhub.io/t/remove-event/6481)                                     | `'hooks/remove'`                  |                                                        |
| [`'config-changed'`](https://discourse.charmhub.io/t/config-changed-event/6465)                     | `'hooks/config-changed'`          |                                                        |
| [`'upgrade-charm'`](https://discourse.charmhub.io/t/upgrade-charm-event/6485)                       | `'hooks/upgrade-charm'`           |                                                        |


### Relation hooks
Only relation hooks set `JUJU_RELATION`, `JUJU_RELATION_ID`, `JUJU_REMOTE_UNIT` and `JUJU_REMOTE_APP`.

| `JUJU_HOOK_NAME`                                                                                     | `JUJU_DISPATCH_PATH`         | Notes                                                   |
|------------------------------------------------------------------------------------------------------|------------------------------|---------------------------------------------------------|
| [`'<name>-relation-created'`](https://discourse.charmhub.io/t/relation-name-relation-created-event/6476)   | `'hooks/<name>-relation-created'`  | JUJU_REMOTE_UNIT is set but is empty                    |
| [`'<name>-relation-joined'`](https://discourse.charmhub.io/t/relation-name-relation-joined-event/6478)     | `'hooks/<name>-relation-joined'`   |                                                         |
| [`'<name>-relation-changed'`](https://discourse.charmhub.io/t/relation-name-relation-changed-event/6475)   | `'hooks/<name>-relation-changed'`  |                                                         |
| [`'<name>-relation-departed'`](https://discourse.charmhub.io/t/relation-name-relation-departed-event/6477) | `'hooks/<name>-relation-departed'` | This is the only event that sets `JUJU_DEPARTING_UNIT`. |
| [`'<name>-relation-broken'`](https://discourse.charmhub.io/t/relation-name-relation-broken-event/6474)     | `'hooks/<name>-relation-broken'`   | JUJU_REMOTE_UNIT is set but is empty                    |

### Storage hooks
Only storage hooks set `JUJU_STORAGE_LOCATION`, `JUJU_STORAGE_KIND` and `JUJU_STORAGE_ID`.

| `JUJU_HOOK_NAME`                                                                                        | `JUJU_DISPATCH_PATH`              | Notes |
|---------------------------------------------------------------------------------------------------------|-----------------------------------|-------|
| [`'<name>-storage-attached'`](https://discourse.charmhub.io/t/storage-name-storage-attached-event/6479) | `'hooks/<name>-storage-attached'` |       |
| [`'<name>-storage-detaching'`](https://discourse.charmhub.io/t/storage-name-storage-detached-event/6480) | `'hooks/<name>-storage-detaching'` |       |

### Actions
Only [actions](https://discourse.charmhub.io/t/action-name-action-event/6466) set `JUJU_ACTION_NAME`, `JUJU_ACTION_TAG` and `JUJU_ACTION_UUID`.

- `JUJU_HOOK_NAME` set, but is empty.
- `JUJU_DISPATCH_PATH` looks like this: `actions/do-something`
- The special `juju-run` action can be used to print out environment variables for its own context: `juju run --unit my-app/0  -- env`

### Secrets
All [secrets hooks](https://discourse.charmhub.io/t/secret-events/7191) set `JUJU_SECRET_ID` and `JUJU_SECRET_LABEL`. Only `secret-expired`, `secret-rotate` and `secret-remove` also set `JUJU_SECRET_REVISION`.

```{note}
 
Do note that the `JUJU_SECRET_LABEL` will be set to some non-empty value only if the secret at hand *has* a label at the time the event is emitted by the agent.

```

| `JUJU_HOOK_NAME`                                                                                        | Notes |
|---------------------------------------------------------------------------------------------------------|-------|
| [`'secret-changed'`](https://discourse.charmhub.io/t/7193) | This is the only secrets hook that does not set `JUJU_SECRET_REVISION`    |
| [`'secret-expired'`](https://discourse.charmhub.io/t/7192) |        |
| [`'secret-remove'`](https://discourse.charmhub.io/t/7195) |        |
| [`'secret-rotate'`](https://discourse.charmhub.io/t/7194) |        |

### List of charm environment variables

> [Source (the `HookVars` function)](https://github.com/juju/juju/blob/c3de749971d5abcdcb01ec29f290a45f2fb2493d/worker/uniter/runner/context/context.go)

- CHARM_DIR
- CLOUD_API_VERSION
- JUJU_ACTION_NAME
- JUJU_ACTION_TAG
- JUJU_ACTION_UUID
- JUJU_AGENT_CA_CERT
- JUJU_AGENT_SOCKET_ADDRESS
- JUJU_AGENT_SOCKET_NETWORK
- JUJU_API_ADDRESSES
- JUJU_AVAILABILITY_ZONE
- JUJU_CHARM_DIR
- JUJU_CHARM_FTP_PROXY
- JUJU_CHARM_HTTPS_PROXY
- JUJU_CHARM_HTTP_PROXY
- JUJU_CHARM_NO_PROXY
- JUJU_CONTEXT_ID
- JUJU_DEPARTING_UNIT
- JUJU_HOOK_NAME
- JUJU_MACHINE_ID
- JUJU_METER_INFO
- JUJU_METER_STATUS
- JUJU_MODEL_NAME
- JUJU_MODEL_UUID
- JUJU_NOTICE_ID
- JUJU_NOTICE_KEY
- JUJU_NOTICE_TYPE
- JUJU_PRINCIPAL_UNIT
- JUJU_RELATION
- JUJU_RELATION_ID
- JUJU_REMOTE_APP
- JUJU_REMOTE_UNIT
- JUJU_SECRET_ID
- JUJU_SECRET_LABEL
- JUJU_SECRET_REVISION
- JUJU_SLA
- JUJU_STORAGE_ID
- JUJU_STORAGE_KIND
- JUJU_STORAGE_LOCATION
- JUJU_TARGET_BASE
- JUJU_TARGET_SERIES (deprecated in Juju 3, to be removed in Juju 4; please use JUJU_TARGET_BASE instead)
- JUJU_UNIT_NAME
- JUJU_VERSION

