(manage-subnets)=
# How to manage subnets

> See also: {ref}`subnet`


## List subnets

To view the subnets known to `juju`, run:

```text
juju subnets
```

````{dropdown} Expand to see a sample output

```
subnets:
  172.31.0.0/20:
    type: ipv4
    provider-id: subnet-9b4ed4fc
    provider-network-id: vpc-54a7112e
    status: in-use
    space: alpha
    zones:
    * us-east-1c
  172.31.16.0/20:
    type: ipv4
    provider-id: subnet-eca389a6
    provider-network-id: vpc-54a7112e
    status: in-use
    space: alpha
    zones:
    * us-east-1a
...
```

````

> See more: {ref}`command-juju-subnets`

## Move a subnet to another space

For all providers other than MAAS, all subnets are initially in a default `alpha` space. 
To move a subnet `172.31.16.0/20` to a different space, `db-space`, execute:

```text
juju move-to-space db-space 172.31.16.0/20
```

````{dropdown} Example output
```text

Subnet 172.31.16.0/20 moved from alpha to db-space

```
````

> See more: {ref}`command-juju-move-to-space`
