(manage-spaces)=
# How to manage spaces
> See also: {ref}`space`

Juju users are able to create, view, rename, or delete spaces.

```{caution}

Juju can deploy to an IPv6 stack or an IPv4 stack, but not both at once (i.e., dual stacks are not supported).

```

## Add a space

Spaces are created with the `add-space` command. The following example creates a new space called `db-space` and associates the `172.31.0.0/20` subnet with it:

``` text
juju add-space db-space 172.31.0.0/20
added space "db-space" with subnets 172.31.0.0/20
```

> See more: {ref}`command-juju-add-space`


## Reload spaces

To reload spaces, along with their subnets, use the `reload-spaces` command:

```text
juju reload-spaces
```

This will show you any new spaces (whether added via `add-space` or directly on the provider end), or any new subnets of an existing space.

> See more: {ref}`command-juju-reload-spaces`

```{important}
This command is especially relevant for a MAAS cloud. There, you cannot add a space via `juju add-space`. Rather, you must add it directly using the MAAS UI/CLI and then run `juju reload-spaces` to make it known to Juju.
```

## View  available spaces

The spaces known to `juju` can be viewed with the `spaces` command, as follows:

```text
$ juju spaces
Name   Space ID  Subnets
alpha  0         172.31.0.0/20
                 172.31.16.0/20
                 172.31.32.0/20
                 172.31.48.0/20
                 172.31.64.0/20
                 172.31.80.0/20
                 252.0.0.0/12
                 252.16.0.0/12
                 252.32.0.0/12
                 252.48.0.0/12
                 252.64.0.0/12
                 252.80.0.0/12
```

> See more: {ref}`command-juju-spaces`


## View details about a space

To view details about a space, run the `show-space` command:

```text
juju show-space
```

The command also allows you to specify a model to operate in, an output format, etc.

> See more: {ref}`command-juju-show-space`

## Rename a space

To rename a space `db-space` to `public-space`, do:

```text
$ juju rename-space db-space public-space
renamed space "db-space" to "public-space"
```

```{important}
Spaces can also be renamed during controller configuration, via the `juju-ha-space` and `juju-mgmt-space` key, or during model configuration, via the `default-space` key.
```

> See more: {ref}`command-juju-rename-space`


## Remove a space

You can delete a space using the `remove-space` command. 

```text
$ juju remove-space public-space
removed space "public-space"
```

```{important}
Deleting a space will cause any subnets in it to move back to the `alpha` space. See [How to manage subnets <how-to-manage-subnets`.
```

> See more: {ref}`command-juju-remove-space`

