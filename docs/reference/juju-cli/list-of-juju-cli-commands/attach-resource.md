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

## Details

This command updates a resource for an application.

The format is

    <resource name>=<resource>

where the resource name is the name from the metadata.yaml file of the charm
and where, depending on the type of the resource, the resource can be specified
as follows:

(1) If the resource is type 'file', you can specify it by providing
(a) the resource revision number or
(b) a path to a local file.

(2) If the resource is type 'oci-image', you can specify it by providing
(a) the resource revision number,
(b) a path to a local file = private OCI image,
(c) a link to a public OCI image.


Note: If you choose (1b) or (2b-c), i.e., a resource that is not from Charmhub:
You will not be able to go back to using a resource from Charmhub.

Note: If you choose (1b) or (2b): This uploads a file from your local disk to the juju
controller to be streamed to the charm when "resource-get" is called by a hook.

Note: If you choose (2b): You will need to specify:
(i) the local path to the private OCI image as well as
(ii) the username/password required to access the private OCI image.