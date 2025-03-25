(manage-secrets)=
# How to manage secrets

> See also: {ref}`secret`

Charms can use relations to share secrets, such as API keys, a database’s address, credentials and so on. This document demonstrates how to interact with them as a Juju user. 

```{caution}
The write operations are only available (a) starting with Juju 3.3 and (b) to model admin users looking to manage user-owned secrets. See more: {ref}`secret`.
```

## Add a secret

To add a (user) secret, run the `add-secret` command followed by a secret name and a (space-separated list of) key-value pair(s). This will return a secret ID. For example:

```text
$ juju add-secret dbpassword foo=bar
secret:copp9vfmp25c77di8nm0
```

The command also allows you to specify the type of key, whether you want to supply its value from a file, whether you want to give it a label, etc.

> See more: {ref}`command-juju-add-secret`


## View all the available secrets

To view all the (user and charm) secrets available in a model, run:

```text
juju secrets
```

You can also add options to specify an output format, a model other than the current model, an owner, etc.

> See more: {ref}`command-juju-secrets`

## View details about a secret

To drill down into a (user or charm) secret, run the `show-secret` command followed by the secret name or ID. For example:

```text
juju show-secret 9m4e2mr0ui3e8a215n4g
```

You can also add options to specify the format, the revision, whether to reveal the value of a secret, etc.

> See more: {ref}`command-juju-show-secret`


## Grant access to a secret

Given a charm that has a configuration option that allows it to be configured with a user secret, to grant the application deployed from it access to the secret, run the `grant-secret` command followed by the secret name or ID and by the name of the application. For example:

```text
juju grant-secret dbpassword mysql
```

Note that this only gives the application *permission* to use the secret, so you must follow up by giving the application the secret itself, by setting its relevant secret-relation configuration  option to the secret URI:

```text
juju config <application> <option>=<secret URI>
```

> See more: {ref}`command-juju-grant-secret`



## Update a secret
> *This feature is opt-in because Juju automatically removing secret content might result in data loss.*


To update a (user) secret, run the `update-secret` command followed by the secret ID and the updated (space-separated list of) key-value pair(s). For example:

```text
juju update-secret secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4
```

> See more: {ref}`command-juju-update-secret`


## Remove a secret

To remove all the revisions of a (user) secret, run the `remove-secret` command followed by the secret ID. For example:

```text
juju remove-secret  secret:9m4e2mr0ui3e8a215n4g
```

The command also allows you to specify a model or to provide a specific revision to remove instead of the default all.

> See more: {ref}`command-juju-remove-secret`

