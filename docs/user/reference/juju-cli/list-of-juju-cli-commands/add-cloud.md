(command-juju-add-cloud)=
# `juju add-cloud`
> See also: [clouds](#clouds), [update-cloud](#update-cloud), [remove-cloud](#remove-cloud), [update-credential](#update-credential)

## Summary
Add a cloud definition to Juju.

## Usage
```juju add-cloud [options] <cloud name> [<cloud definition file>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `--credential` |  | Credential to use for new cloud |
| `-f`, `--file` |  | The path to a cloud definition file |
| `--force` | false | Force add cloud to the controller |
| `--target-controller` |  | The name of a JAAS managed controller to add a cloud to |

## Examples

    juju add-cloud
    juju add-cloud --force
    juju add-cloud mycloud ~/mycloud.yaml
    juju add-cloud --controller mycontroller mycloud 
    juju add-cloud --controller mycontroller mycloud --credential mycred
    juju add-cloud --client mycloud ~/mycloud.yaml


## Details

Juju needs to know how to connect to clouds. A cloud definition 
describes a cloud's endpoints and authentication requirements. Each
definition is stored and accessed later as &lt;cloud name&gt;.

If you are accessing a public cloud, running add-cloud is unlikely to be 
necessary.  Juju already contains definitions for the public cloud 
providers it supports.

add-cloud operates in two modes:

    juju add-cloud
    juju add-cloud <cloud name> <cloud definition file>

When invoked without arguments, add-cloud begins an interactive session
designed for working with private clouds.  The session will enable you 
to instruct Juju how to connect to your private cloud.

A cloud definition can be provided in a file either as an option -f or as a 
positional argument:

    juju add-cloud mycloud ~/mycloud.yaml
    juju add-cloud mycloud -f ~/mycloud.yaml

When &lt;cloud definition file&gt; is provided with &lt;cloud name&gt;,
Juju will validate the content of the file and add this cloud 
to this client as well as upload it to a controller.

Use --controller option to upload a cloud to a controller. 

Use --client option to add cloud to the current client.

A cloud definition file has the following YAML format:

    clouds:                           # mandatory
      mycloud:                        # <cloud name> argument
        type: openstack               # <cloud type>, see below
        auth-types: [ userpass ]
        regions:
          london:
            endpoint: https://london.mycloud.com:35574/v3.0/

Cloud types for private clouds: 
 - lxd
 - maas
 - manual
 - openstack
 - vsphere

Cloud types for public clouds:
 - azure
 - ec2
 - gce
 - oci

When a running controller is updated, the credential for the cloud
is also uploaded. As with the cloud, the credential needs
to have been added to the current client, use add-credential to
do that. If there's only one credential for the cloud it will be
uploaded to the controller automatically by add-cloud command. 
However, if the cloud has multiple credentials on this client
you can specify which to upload with the --credential option.

When adding clouds to a controller, some clouds are whitelisted and can be easily added:
 - controller cloud type "kubernetes" supports [lxd maas openstack]
 - controller cloud type "lxd" supports [lxd maas openstack]
 - controller cloud type "maas" supports [maas openstack]
 - controller cloud type "openstack" supports [openstack]

Other cloud combinations can only be force added as the user must consider
network routability, etc - concerns that are outside of scope of Juju.
When forced addition is desired, use --force.