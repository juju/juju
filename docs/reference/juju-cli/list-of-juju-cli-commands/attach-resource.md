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

- If the resource is type `file`, you can specify it by providing one of the following:

    a. the resource revision number.

    b. a path to a local file. Caveat: If you choose this, you will not be able
	 to go back to using a resource from Charmhub.

- If the resource is type `oci-image`, you can specify it by providing one of the following:

    a. the resource revision number.

	b. a path to the local file for your private OCI image as well as the
	username and password required to access the private OCI image.
	Caveat: If you choose this, you will not be able to go back to using a
	resource from Charmhub.

    c. a link to a public OCI image. Caveat: If you choose this, you will not be
	 able to go back to using a resource from Charmhub.