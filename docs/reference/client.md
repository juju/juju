---
myst:
  html_meta:
    description: "Juju client reference: what a client is, the clients that exist (the juju CLI, the Juju dashboard with its web CLI, JAAS, the Terraform provider, Jubilant), how a client logs a user in, why every call checks the user's access, and the local state the juju CLI keeps."
---

(client)=
# Client

```{ibnote}
See also: {ref}`Juju | Manage the juju CLI client <manage-juju>`
```

A **client** is software that acts for a {ref}`Juju user <user>`: it logs in as the user and makes the user's calls through the {ref}`controller <controller>` API (the set of calls that clients and {ref}`agents <agent>` make to the controller). An {ref}`agent <agent>`, in contrast, acts for its own machine, unit, model, or controller.

(juju-clients)=
## Juju clients

The clients that act for a Juju {ref}`user <user>` are:

- **The {ref}`juju CLI <juju-cli>`:** the command-line client, which manages {ref}`controllers <controller>`, {ref}`models <model>`, and {ref}`applications <application>`.
- **The {ref}`Juju dashboard <juju-dashboard>`:** a web interface that shows the models of many controllers.
- **The {ref}`web CLI <juju-web-cli>`:** model-level CLI commands from inside the dashboard.
- **[JAAS](https://canonical-jaas-documentation.readthedocs-hosted.com/v3/) (Juju as a Service):** a hosted gateway, JIMM, that manages many controllers from one place and speaks the controller API to each, driven through the `jaas` CLI.
- **The [Terraform provider for Juju](https://documentation.ubuntu.com/terraform-provider-juju/latest/):** Juju resources managed from Terraform code.
- **[Jubilant](https://documentation.ubuntu.com/jubilant/):** Juju driven from Python code, for charm integration tests.

(client-login)=
## Login

A client logs a {ref}`user <user>` in when it connects to a {ref}`controller <controller>`; every request on the connection then acts as that user (see {ref}`logging in <user-logging-in>`). A local user logs in with a name and a password. A client that has no password for the user logs in with a macaroon (a bearer token that lets the client log in as the user): the controller answers with a macaroon that needs a discharge (confirmation by the third party named in it), the client collects the discharge from the controller's identity service (served at `/auth` on the controller), and it logs in again with the result. An external user logs in with a macaroon discharged by the identity provider that the controller's {ref}`identity-url <controller-config-identity-url>` setting names.

An {ref}`agent <agent>` logs in as its own unit, machine, application, model, or controller-agent tag, with the agent's own password; a user tag is refused there.

(client-access)=
## Access

The {ref}`access levels <user-access-levels>` attach to the {ref}`user <user>` that a client acts for: the controller checks each call against the level of that user, so every client that logs in as the user can make the calls that the user's level allows.

(client-store)=
## The client store

The {ref}`juju CLI <juju-cli>` keeps local state, the client store, as files in the user's {ref}`Juju data directory <juju-cli>`. The store holds:

- **Controllers:** each controller the CLI knows, with its API addresses, CA certificate, and cloud.
- **Models:** the models the user knows on each controller, with the current one.
- **Accounts:** the user the CLI logs in as on each controller, with the password when the CLI kept one, and the last known controller access.
- **Credentials:** the {ref}`cloud credentials <credential>` the user keeps on the client.
- **Bootstrap settings:** the settings that created each controller, so the CLI can create it again.
- **Cookies:** the login cookies for each controller.