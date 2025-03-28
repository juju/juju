(command-juju-help)=
# `juju help`

```
Juju provides easy, intelligent application orchestration on top of Kubernetes,
cloud infrastructure providers such as Amazon, Google, Microsoft, Openstack,
MAAS (and more!), or even your local machine via LXD.

See https://juju.is for getting started tutorials and additional documentation.

Starter commands:

    bootstrap           Initializes a cloud environment.
    add-model           Adds a hosted model.
    deploy              Deploys a new application.
    status              Displays the current status of Juju, applications, and units.
    add-unit            Adds extra units of a deployed application.
    relate              Adds a relation between two applications.
    expose              Makes an application publicly available over the network.
    models              Lists models a user can access on a controller.
    controllers         Lists all controllers.
    whoami              Display the current controller, model and logged in user name.
    switch              Selects or identifies the current controller and model.
    add-k8s             Adds a k8s endpoint and credential to Juju.
    add-cloud           Adds a user-defined cloud to Juju.
    add-credential      Adds or replaces credentials for a cloud.

Interactive mode:

When run without arguments, Juju will enter an interactive shell which can be
used to run any Juju command directly.

Help commands:

    juju help           This help page.
    juju help <command> Show help for the specified command.

For the full list of supported commands run:

    juju help commands
```