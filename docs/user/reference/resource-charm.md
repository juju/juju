(charm-resource)=
# Resource (charm)
>
> See also: {ref}`manage-charm-resources`

In Juju, a **charm resource** is additional content that a {ref}`charm <charm>` can make use of, or may require, to run. 

Resources are used where a charm author needs to include large blobs (perhaps a database, media file, or otherwise) that may not need to be updated with the same cadence as the charm or workload itself. By keeping resources separate, they can control the lifecycle of these elements more carefully, and in some situations avoid the need for repeatedly downloading large files from Charmhub during routine upgrades/maintenance.


A resource can have one of two basic types -- `file` and `oci-image`. These can be specified as follows:

1. If the resource is type 'file', you can specify it by providing
    
    a. the resource revision number or

    b.  a path to a local file.

2. If the resource is type 'oci-image', you can specify it by providing

    a. the resource revision number,

    b. a path to a local file = private OCI image,

    c. a link to a public OCI image.

If you choose to provide a path to a  local file, the file can be a JSON or a YAML file with an image reference and optionally a username and a password (i.e., an OCI image resource).


````{dropdown} Expand to view an example JSON file

```text
{
  "ImageName": "my.private.repo.com/a/b:latest",
  "username": "harry",
  "password": "supersecretpassword"
}
```

````


````{dropdown} Expand to view an example YAML file

```text
registrypath: my.private.repo.com/a/b:latest
username: harry
password: supersecretpassword

```
````
