(configuration)=
# Configuration

```{toctree}
:hidden:

list-of-controller-configuration-keys
list-of-model-configuration-keys
```


In Juju, a **configuration** is a rule or a set of rules that define the behavior of your controller, model, or application.

(controller-configuration)=
## Controller configuration

Controller configuration affects the operation of the controller as a whole.

> See more:  {ref}`configure-a-controller`, {ref}`list-of-controller-configuration-keys`

(model-configuration)=
## Model configuration

Model configuration affects behavior of a model, including the `controller` model.
> See more: {ref}`configure-a-model`, {ref}`list-of-model-configuration-keys`

(application-configuration)=
## Application configuration

Application configuration affects the behavior of an application. 

Application configuration keys are generally application-specific. Depending on what the charm author has decided, they can be used to allow the charm user to make certain decisions about the application, for example, the server port on which the  application should be available, the resource profile, the DNS name, etc. 

> See examples: [Charmhub | `mysql` > Configurations](https://charmhub.io/mysql/configure#cluster-name), [Charmhub | `traefik-k8s` > Configurations](https://charmhub.io/traefik-k8s/configure), etc. 

> See more: {ref}`configure-an-application`

However, there is also a generic key, `trust`, that can be changed via `juju trust`. 

> See more: {ref}`trust-an-application-with-a-credential`


<!-- Heather and I decided to include `trust` under application configuration keys because, if you run, e.g., `$ juju config juju-qa-test`, you'll find something like:

```
$ juju config juju-qa-test
application: juju-qa-test
application-config:
  trust:
```

with `trust` being listed there (even if it's not in the config of the charm). 
-->
