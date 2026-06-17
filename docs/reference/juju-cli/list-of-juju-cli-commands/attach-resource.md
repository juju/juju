(command-juju-attach-resource)=
# `juju attach-resource`
> See also: [resources](#resources), [charm-resources](#charm-resources)

## Summary
Update a resource for an application.

## Usage
```juju attach-resource [options] application <resource name>=<resource>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju attach-resource easyrsa easyrsa=./EasyRSA-3.0.7.tgz

    juju attach-resource ubuntu-k8s ubuntu_image=ubuntu

    juju attach-resource redis-k8s redis-image=redis


## Details

This command updates a resource for an application.

The format is

    <resource name>=<resource>

where `<resource name>` is the name from the `metadata.yaml` (`charmcraft.yaml`) file of the charm and `<resource>` is the resource itself, which can be supplied as follows:

- For a resource type `file`:

    a. that has been uploaded to Charmhub: the resource revision number.

    b. that is local to your machine: a path to the local file. Caveat: If you choose this, you will
	not be able to go back to using a resource from Charmhub.

- For a resource type `oci-image`:

    a. that has been uploaded to Charmhub: the resource revision number.

	b. that is local to your machine: a path to the local `json` or `yaml` file
	that contains the details for your private OCI image (local image path, username, password, etc.).
	Caveat: If you choose this, you will not be able to go back to using a resource from Charmhub.

    c. For a resource that has been uploaded to a public OCI registry: a link to the public OCI image.
	Caveat: If you choose this, you will not be able to go back to using a resource from Charmhub.