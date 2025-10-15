(controller)=
# Controller

```{ibnote}
See also: {ref}`manage-controllers`
```

In software design, a **controller** is an architectural component responsible for managing the flow of data and interactions within a system, and for mediating between different parts of the system. In Juju, it is defined in the same way, with the mention that:

- It is set up via the boostrap process.
- It refers to the initial controller {ref}`unit <unit>` as well as any units added later on (for machine clouds, for the purpose of {ref}`high-availability <high-availability>`) -- each of which includes
    - a {ref}`unit agent <unit-agent>`,
    - [`juju-controller`](https://charmhub.io/juju-controller) charm code, and
    - a {ref}`controller agent <controller-agent>` running, among other things, the Juju API server and an in-process embedded [Dqlite](https://canonical.com/dqlite) database. <p>
- It is responsible for implementing all the changes defined by a Juju {ref}`user <user>` via a Juju client post-bootstrap.
- It stores state in the internal Dqlite database.

(controller-storage)=
## Controller storage

A Juju controller has two basic persistent storage needs: database access and blob storage.

Prior to Juju 4, both these needs were satisfied by a MongoDB database; however, with the switch to a Dqlite database in Juju 4, only the database access need is satisfied -- for blob storage the Juju controller will require an external object store.

By default, Juju will store blobs on the filesystem of the controller’s supporting infrastructure. However, either during bootstrap or later, you can (and, in a production-setting, should!) specify any S3-compatible object store you want (e.g., AWS S3, MicroCeph, MinIO, etc.) using the object-store-related controller configuration keys ({ref}`controller-config-object-store-type`, {ref}`controller-config-object-store-s3-endpoint`, {ref}`controller-config-object-store-s3-static-key`, {ref}`controller-config-object-store-s3-static-secret`, {ref}`controller-config-object-store-s3-static-session`).

Also, Juju will apply default S3 policy permissions, but you are free to change them, so long as they satisfy the following as a minimum (at least, during model creation):

```text
{
   "Version" : "2012-10-17",
   "Statement" : [
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:CreateBucket",
            "s3:PutBucketPolicy",
            "s3:PutBucketTagging",
            "s3:PutBucketVersioning",
            "s3:PutBucketObjectLockConfiguration"
         ],
         "Resource" : "arn:aws:s3:::*"
      },
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:ListBucket",
            "s3:ListBucketVersions",
            "s3:ListAllMyBuckets",
            "s3:GetBucketLocation",
            "s3:GetBucketPolicy",
            "s3:GetBucketTagging",
            "s3:GetBucketVersioning",
            "s3:GetBucketObjectLockConfiguration",
            "s3:GetObject",
            "s3:GetObjectLegalHold",
            "s3:GetObjectRetention",
            "s3:PutObject",
            "s3:PutObjectLegalHold",
            "s3:BypassGovernanceRetention",
            "s3:PutObjectRetention",
            "s3:DeleteObject"
         ],
         "Resource" : "arn:aws:s3:::*"
      }
   ]
}
```



