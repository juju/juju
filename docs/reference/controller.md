(controller)=
# Controller

> See also: {ref}`manage-controllers`

`````{note}
Starting with Juju 4, Juju will replace MongoDB with Dqlite. In a production setting, make sure to specify an external object store provider for blob storage.

````{dropdown} See more

 A Juju controller has two basic persistent storage needs: database access and blob storage. MongoDB satisfied both. However, Dqlite will only satisfy the former -- for blob storage the Juju controller will require an external object store. 

By default, Juju will use the filesystem of the controller’s supporting infrastructure. However, you can also use [object-store-related controller configuration keys](https://github.com/juju/juju/blob/495f27b9301949e666d2a854b37ee8ed50ec59ee/controller/configschema.go#L63C2-L67C52) to specify any object store (highly recommended in a production setting!) you want, so long as it is S3-compatible (e.g., AWS S3, MicroCeph, MinIO, etc.), either during bootstrap or later. 

Also, Juju will apply default S3 policy permissions, but you are free to change them, so long as they satisfy the following as a minimum (at least, during model creation):

```text
{
   "Version" : "2012-10-17",
   "Statement" : {ref}`
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

````
`````

In software design, a **controller** is an architectural component responsible for managing the flow of data and interactions within a system, and for mediating between different parts of the system. In Juju, it is defined in the same way, with the mention that:

- It is set up via the boostrap process.
- It refers to the initial controller {ref}`unit <unit>` as well as any units added later on (for machine clouds, for the purpose of {ref}`high-availability <high-availability>`) -- each of which includes 
    - a {ref}`unit agent <unit-agent>`, 
    - [`juju-controller`](https://charmhub.io/juju-controller) charm code, 
    - a {ref}`controller agent <controller-agent>` (running, among other things, the Juju API server), and 
    - a copy of [`juju-db`](https://snapcraft.io/juju-db), Juju's internal database. <p>
- It is responsible for implementing all the changes defined by a Juju {ref}`user <user>` via a Juju client post-bootstrap.
- It stores state in an internal MongoDB database.
