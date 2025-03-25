(manage-offers)=
# How to manage offers

> See also: {ref}`offer`

This document shows how to manage offers.

<!--
This document demonstrates the various steps involved in managing a cross-model integration. The step of adding the integration is the same as for a regular, same-model integration. However, as a cross-model integration may in principle cross controller, cloud, and administrative boundaries, there are additional steps before and after, for making an application accessible from another model, or *offering* it, and for relating to it from the other model, or *consuming* it.

### Central monitoring of model workloads
Assume that we have a number of models for which we want to collect performance metrics using a common prometheus deployment.

We'll deploy prometheus to one model and offer it.
``text
juju switch bigbrother:voyeur
juju deploy prometheus2
juju expose prometheus2
juju offer prometheus2:target offerprom
``
In a different model, we'll deploy a workload.
``text
juju switch monitorme
juju deploy ubuntu
juju deploy telegraf
juju integrate ubuntu:juju-info telegraf
``
Now we'll consume the prometheus offer and integrate our workload with it.
``text
juju consume bigbrother:admin/voyeur.offerprom promed
juju integrate telegraf:prometheus2-client promed:target
``

-->


## Create an offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.


An offer stems from an application endpoint. This is how an offer is created:

```text
juju offer <application>:<application endpoint>
```


By default, an offer is named after its underlying application but you may also choose to give it a different name:

```text
juju offer <application>:<application endpoint> <offer name>
```

Example:

```text
juju deploy mysql
juju offer mysql:database hosted-mysql
```

To view the available application endpoints use `juju show-application` and  check the list below `endpoint-bindings`. Example:
```text
juju show-application mysql 
mysql:
  charm: mysql
  ...
  endpoint-bindings:
    "": alpha
    certificates: alpha
    cos-agent: alpha
    database: alpha
    ...
```

To offer both the `certificates` and `database`  endpoints:

```text
juju deploy mysql
juju offer mysql:database,certificates hosted-mysql
```

Although an offer may have multiple (offer) endpoints it is always expressed as a single URL:

`<user>/<model>.<offer_name`

If the above mysql offer were made in the `default` model by user `admin`, the URL would be:

`admin/default.hosted-mysql`

> See more: {ref}`command-juju-offer`


## View an offer’s details
> Who: User with {ref}`offer read access <user-access-offer-read>`.

The `show-offer` command gives details about a given offer.

```text
juju show-offer <offer name>
```

Example:

```text
juju show-offer hosted-mysql
Store        URL                         Access  Description                                    Endpoint      Interface         Role
foo          admin/default.hosted-mysql  admin   MySQL is a widely used, open-source            certificates  tls-certificates  requirer
                                                 relational database management system          database      mysql_client      provider
                                                 (RDBMS). MySQL InnoDB cluster provides a                                       
                                                 complete high availability solution for MySQL                                  
                                                 via Group Replic...  
```

For more details, including which users can access the offer, use the `yaml` format.

Example:

```text
juju show-offer hosted-mysql --format yaml
serverstack:admin/default.hosted-mysql:
  description: |
    MySQL is a widely used, open-source relational database management system
    (RDBMS). MySQL InnoDB cluster provides a complete high availability solution
    for MySQL via Group Replication.

    This charm supports MySQL 8.0 in bare-metal/virtual-machines.
  access: admin
  endpoints:
    certificates:
      interface: tls-certificates
      role: requirer
    database:
      interface: mysql_client
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
```

A non-admin user with read/consume access can also view an offer's details, but they won't see the information for users with access.

> See more: {ref}`command-juju-show-offer`


## Control access to an offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

Offers can have one of three access levels:

-   read (a user can see the offer when searching)
-   consume (a user can relate an application to the offer)
-   admin (a user can manage the offer)

These are applied similarly to how standard model access is applied, via the `juju grant` and `juju revoke` commands:

```text
juju grant <user> <access-level> <offer-url>
```

```text
juju revoke <user> <access-level> <offer-url>
```

Revoking a user's consume access will result in all relations for that user to that offer to be suspended. If the consume access is granted anew, each relation will need to be individually resumed. Suspending and resuming relations are explained in more detail later.

To grant bob consume access to an offer:

```text
juju grant bob consume admin/default.hosted-mysql
```

To revoke bob's consume access (he will be left with read access):

```text
juju revoke bob consume admin/default.hosted-mysql
```

To revoke all of bob's access:

```text
juju revoke bob read admin/default.hosted-mysql
```

> See more: {ref}`command-juju-grant`, {ref}`command-juju-revoke`


## Find an offer to use
> Who: User with {ref}`offer read access <user-access-offer-read>`.


Offers can be searched based on various criteria:

* URL (or part thereof)
* offer name
* model name
* interface

The results will show information about the offer, including the level of access the user making the query has on each offer.

To find all offers on a specified controller:

```text
$ juju find-offers foo:
Store  URL                         Access  Interfaces
foo    admin/default.hosted-mysql  admin   mysql:database
foo    admin/default.postgresql    admin   pgsql:db
```

As with the `show-offer` command, the `yaml` output will show extra information, including users who can access the offer (if an admin makes the query).

```text
juju find-offers --offer hosted-mysql --format yaml
foo:admin/default.hosted-mysql:
  access: admin
  endpoints:
    certificates:
      interface: tls-certificates
      role: requirer
    database:
      interface: mysql_client
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    bob:
      access: read
    everyone@external:
      access: read
```

To find offers in a specified model:

```text
juju find-offers admin/another-model
juju find-offers foo:admin/another-model
```

To find offers with a specified interface on the current controller:

```text
juju find-offers --interface mysql_client
juju find-offers --interface tls-certificates
```

To find offers with a specified interface on a specific controller:

```text
juju find-offers --interface mysql_client foo:
```

To find offers with "sql" in the name:

```text
$ juju find-offers --offer sql foo:
```

> See more: {ref}`command-juju-find-offers`

(integrate-with-an-offer)=
## Integrate with an offer
> Who: User with {ref}`offer consume access <user-access-offer-consume>`.


```{important}
Before Juju `3.0`, `juju integrate` was `juju relate`.
```

If a user has consume access to an offer, they can deploy an application in their model and establish an integration with the offer by way of its URL. 

```text
juju integrate <application>[:<application endpoint>] <offer-url>[:<offer endpoint>]
```

Specifying the endpoint for the application and the offer is analogous to normal integrations. They can be added but are often unnecessary:

```text
juju integrate <application> <offer-url>
```

When you integrate with an offer, a proxy application is made in the consuming model, named after the offer.

An offer can be consumed without integration. This workflow sets up the proxy application in the consuming model and creates a user-defined alias for the offer. This latter is what's used to subsequently relate to. Having an offer alias can avoid a namespace conflict with a pre-existing application.

```text
juju consume <offer-url> <offer-alias>
juju integrate <application> <offer alias>
```

Offers which have been consumed show up in `juju status` in the SAAS section. The integrations (relations) block in status shows any relevant status information about the integrations to the offer in the Message field. This includes any error information due to rejected ingress, or if the relation is suspended etc.

To remove a consumed offer:

```text
juju remove-saas <offer alias>
```
> See more: {ref}`command-juju-integrate`, {ref}`command-juju-consume`, {ref}`command-juju-remove-saas`


## Allow traffic from an integrated offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

When the consuming model is behind a NAT firewall its traffic will typically exit (egress) that firewall with a modified address/network. In this case, the `--via` option can be used with the `juju integrate` command to request the firewall on the offering side to allow this traffic. This option specifies the NATed address (or network) in CIDR notation:

```text
juju integrate <application> <offer url> --via <cidr subnet(s)>
```

Example:

```text
juju integrate mediawiki:db ian:admin/default.mysql --via 69.32.56.0/8
```

The `--via` value is a comma separated list of subnets in CIDR notation. This includes the /32 case where a single NATed IP address is used for egress.

It's also possible to set up egress subnets as a model config value so that all cross model integrations use those subnets without needing to use the `--via` option.

```text
juju model-config egress-subnets=<cidr subnet>
```

Example:

```text
juju model-config egress-subnets=69.32.56.0/8
```

To be clear, the above command is applied to the **consuming** model.

To allow control over what ingress can be applied to the offering model, an administrator can set up allowed ingress subnets by creating a firewall rule.

```text
juju set-firewall-rule juju-application-offer --whitelist <cidr subnet>
```

Where 'juju-application-offer' is a well-known string that denotes the firewall rule to apply to any offer in the current model. If a consumer attempts to create a relation with requested ingress outside the bounds of the whitelist subnets, the relation will fail and be marked as in error.

The above command is applied to the **offering** model.

Example:

```text
juju set-firewall-rule juju-application-offer --whitelist 103.37.0.0/16
```

```{note}

The `juju set-firewall-rule` command only affects subsequently created relations, not existing ones. Only new relations will be rejected if the changed firewall rules preclude the requested ingress.
```

To see what firewall rules have currently been defined, use the list firewall-rules command.

Example:

```text
juju firewall-rules
Service                 Whitelist subnets
juju-application-offer  103.37.0.0/16
```

```{note}

Beyond a certain number of firewall rules, which have been dynamically created to allow access from individual integrations, Juju will revert to using the whitelist subnets as the access rules. The number of rules at which this cutover applies is cloud specific.

```

> See more: {ref}`command-juju-set-firewall-rule`, {ref}`command-juju-firewall-rules`


## Inspect integrations with an offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

The `offers` command is used to see all connections to one more offers.

```text
juju offers [--format (tabular|summary|yaml|json)] [<offer name>]
```

If `offer name` is not provided, all offers are included in the result.

The default `tabular` output shows each user connected (relating to) the offer, the 
relation id of the relation, and ingress subnets in use with that connection. The `summary` output shows one row per offer, with a count of active/total relations. Use the `yaml` output to see extra detail such as the UUID of the consuming model.

The output can be filtered by:
 - interface: the interface name of the endpoint
 - application: the name of the offered application
 - connected user: the name of a user who has an integration to the offer
 - allowed consumer: the name of a user allowed to consume the offer
 - active only: only show offers which are in use (are related to)

See `juju help offers` for more detail.

Example:

```text
juju offers mysql
Offer  User   Relation id  Status  Endpoint  Interface  Role      Ingress subnets
mysql  admin  2            joined  db        mysql      provider  69.193.151.51/32

juju offers --format summary
Offer         Application  Charm        Connected  Store   URL                      Endpoint  Interface  Role
hosted_mysql  mysql        ch:mysql-57  1/1        myctrl  admin/prod.hosted_mysql  db        mysql      provider

```

All offers for a given application:

```text
juju offers --application mysql
```

All offers for a given interface:

```text
juju offers --interface mysql
```

All offers for a given user who has related to the offer:

```text
juju offers --connected-user fred
```

All offers for a given user who can consume the offer:

```text
juju offers --format summary --allowed-consumer mary
```

The above command is best run with `--format` summary as the intent is to see, for a given user, what offers they might relate to, regardless of whether there are existing integrations (which is what the tabular view shows).

> See more: {ref}`command-juju-offers`


## Suspend, resume, or remove an integration with an offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

Before you can suspend, resume, or remove an integration (relation), you need to know the integration (relation) ID. (That is because, once you've made an offer, there could potentially be many instances of the same application integrating with that offer, so the only way to identify uniquely is via the relation ID.)

Given two related apps (app1: endpoint, app2), the integration (relation) ID can be found as follows:


```text
juju exec --unit $UNIT_FOR_APP1 -- relation-ids endpoint
```

The output, `<ENDPOINT>:<REL_ID`, gives you the relation id.

Once you have the integration (relation) id:

To suspend an integration (relation), do:


```text
juju suspend-relation <id1>
```

```{note}
Suspended integrations (relations) will run the relation departed / broken hooks on either end, and any firewall ingress will be closed.
```


And, to resume an integration (relation), do:

```text
juju resume-relation <id1>
```

Finally, to remove an integration (relation) entirely:

```text
juju remove-relation <id1>
```

```{note}

Removing an integration on the offering side will trigger a removal on the consuming side. An integration can also be removed from the consuming side, as well as the application proxy, resulting in all integrations being removed.
```

```{note}

In all cases, more than one integration id can be specified, separated by spaces.

```

Examples:

```text
juju suspend-relation 2 --message "reason for suspension"
juju suspend-relation 3 4 5 --message "reason for suspension"
juju resume-relation 2
```

> See more: {ref}`command-juju-suspend-relation`, {ref}`command-juju-resume-relation`, {ref}`command-juju-remove-relation`


## Remove an offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

An offer can be removed providing it hasn't been used in any integration. To override this behaviour the `--force` option is required, in which case the  integration is also removed. This is how an offer is removed:

```text
juju remove-offer [--force] <offer-url>
```

Note that, if the offer resides in the current model, then the shorter offer name can be used instead of the longer URL.

Similarly, if an application is being offered, it cannot be deleted until all its offers are removed.


> See more: {ref}`command-juju-remove-offer`
