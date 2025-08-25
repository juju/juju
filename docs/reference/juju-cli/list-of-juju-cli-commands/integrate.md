(command-juju-integrate)=
# `juju integrate`
> See also: [consume](#consume), [find-offers](#find-offers), [set-firewall-rule](#set-firewall-rule), [suspend-relation](#suspend-relation)

**Aliases:** relate

## Summary
Integrate two applications.

## Usage
```juju integrate [options] <application>[:<endpoint>] <application>[:<endpoint>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--via` |  | For cross model relations, specify the egress subnets for outbound traffic |

## Examples

Integrate wordpress and percona-cluster, asking Juju to resolve
 the endpoint names. Expands to `wordpress:db` (with the `requires` role) and
 `percona-cluster:server` (with the `provides` role).

    juju integrate wordpress percona-cluster

Integrate wordpress and postgresql, using an explicit
endpoint name:

    juju integrate wordpress postgresql:db

Integrate an etcd instance within the current model to centrally managed
EasyRSA Certificate Authority hosted in the `secrets` model:

    juju integrate etcd secrets.easyrsa

Integrate a wordpress application with a mysql application hosted within the
`prod` model, using the `automation` user. Facilitate firewall management
by specifying the routes used for integration data.

    juju integrate wordpress automation/prod.mysql --via 192.168.0.0/16,10.0.0.0/8


## Details

Integrate two applications. Integrated applications communicate over a common
interface provided by the Juju controller that enables units to share information.
This topology allows units to share data, without needing direct connectivity
between units is restricted by firewall rules. Charms define the logic for
transferring and interpreting integration data.

The most common use of `juju integrate` specifies two applications that co-exist
within the same model:

    juju integrate <application> <application>

Occasionally, more explicit syntax is required. Juju is able to integrate
units that span models, controllers and clouds, as described below.

### Integrating applications in the same model

The most common case specifies two applications, adding specific endpoint
name(s) when required.

    juju integrate <application>[:<endpoint>] <application>[:<endpoint>]

The role and endpoint names are described by charms' `metadata.yaml` file.

The order does not matter, however each side must implement complementary roles.
One side implements the `provides` role and the other implements the `requires`
role. Juju can always infer the role that each side is implementing, so specifying
them is not necessary as command-line arguments.

`<application>` is the name of an application that has already been added to the
model. The Applications section of `juju status` provides a list of current
applications.

`<endpoint>` is the name of an endpoint defined within the metadata.yaml
of the charm for `<application>`. Valid endpoint names are defined within the
`provides:` and `requires:` section of that file. Juju will request that you
specify the `<endpoint>` if there is more than one possible integration between
the two applications.


### Subordinate applications

Subordinate applications are designed to be deployed alongside a primary
application. They must define a container scoped endpoint. When that endpoint
is related to a primary application, wherever a unit of the primary application
is deployed, a corresponding unit of the subordinate application will also be
deployed. Integration with the primary application has the same syntax as
integration any two applications within the same model.


### Peer relations

Relations within an application between units (known as 'peer relations') do
not need to be added manually. They are created when the `juju add-unit` and
`juju scale-application` commands are executed.


### Cross-model relations

Applications can be integrated, even when they are deployed to different models.
Those models may be managed by different controllers and/or be hosted on
different clouds. This functionality is known as 'cross-model relation' (CMR).


#### Cross-model relations: different models on the same controller

Integrating applications in models managed by the same controller
is very similar to adding an integration between applications in the same model:

    juju integrate <application>[:<endpoint>] <model>.<application>[:<endpoint>]

`<model>` is the name of the model outside of the current context. This enables the
Juju controller to bridge two models. You can list the currently available
models with `juju models`.

To integrate models outside of the current context, add the `-m <model>` option:

    juju integrate -m <model> <application>[:<endpoint>] \
                     <model>.<application>[:<endpoint>]


#### Cross-model relations: different controllers

Applications can be integrated with a remote application via an offer URL that has
been generated by the `juju offer` command. The syntax for adding a cross-model
relation is similar to adding a local relation:

    juju integrate <application>[:<endpoint>] <offer-endpoint>

`<offer-endpoint> `describes the remote application, from the point of view of the
local one. An `<offer-endpoint>` takes one of two forms:

    <offer-alias>
    <offer-url>[:<endpoint>]

`<offer-alias>` is an alias that has been defined by the `juju consume` command.
Use the `juju find-offers` command to list aliases.

`<offer-url>` is a path to enable Juju to resolve communication between
controllers and the models they control.

    [[<controller>:]<user>/]<model-name>.<application-name>

`<controller>` is the name of a controller. The `juju controllers` command
provides a list of controllers.`<user>` is the user account of the model's owner.


### Cross-model relations: network management

When the consuming side (the local application) is behind a firewall and/or
NAT is used for outbound traffic, it is possible to use the `--via` option to
inform the offering side (the remote application) the source of traffic to
enable network ports to be opened.

    ... --via <cidr-subnet>[,<cidr-subnet>[, ...]]