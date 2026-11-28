---
myst:
  html_meta:
    description: "Learn to create and manage offers in Juju, enabling application integration across different models, controllers, and clouds."
---

(manage-offers)=
# How to manage offers

```{ibnote}
See also: {ref}`offer`
```

This document shows how to manage offers.

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

```{ibnote}
See more: {ref}`command-juju-offer`
```

(view-an-offers-details)=
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

Use `--format yaml` for more detail. A {ref}`model admin <user-access-model-admin>` on the offering model or a {ref}`controller superuser <user-access-controller-superuser>` on its controller sees the offer's user grants under `users`. Other callers see only their own entry.

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

```{ibnote}
See more: {ref}`command-juju-show-offer`
```

(control-access-to-an-offer)=
## Control access to an offer

```{ibnote}
See also: {ref}`Changing access to an offer <changing-access-to-an-offer>`
```

To grant or restore `consume` for the enabled user `bob`, use an account {ref}`authorised to change this offer's grants <changing-access-to-an-offer>` on the offering controller:

```text
juju grant -c foo bob consume admin/default.hosted-mysql
```

```{ibnote}
See more: {ref}`command-juju-grant`
```

```{warning}
Before revoking, ask a {ref}`model admin <user-access-model-admin>` on the offering model or a {ref}`controller superuser <user-access-controller-superuser>` on its controller to {ref}`inspect Bob's direct grant and connections <inspect-relations-to-an-offer>`. Offer admin alone cannot inspect another user's grant or connections. Lowering Bob's direct grant below `consume` can suspend his relations. Record their offering-model IDs before changing the grant.
```

To lower Bob's direct grant to `read` when he holds `consume` or `admin`, revoke `consume`:

```text
juju revoke -c foo bob consume admin/default.hosted-mysql
```

To remove Bob's direct grant when he has one, revoke `read`:

```text
juju revoke -c foo bob read admin/default.hosted-mysql
```

```{ibnote}
See more: {ref}`command-juju-revoke`
```

After restoring Bob's `consume` access, have someone with at least {ref}`model write access <user-access-model-write>` on the offering model {ref}`resume the suspended relations <manage-relations-to-an-offer>` using the recorded IDs. Restoring the grant does not resume them.

(find-an-offer-to-use)=
## Find an offer to use

To find an offer, search using at least {ref}`offer read access <user-access-offer-read>` on the offer, at least {ref}`model read access <user-access-model-read>` on the offering model or {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller.


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

```{ibnote}
See more: {ref}`command-juju-find-offers`
```

(integrate-with-an-offer)=
## Integrate with an offer

To integrate with an offer, use an account with at least {ref}`model write access <user-access-model-write>` on the consuming model. On the offering side, the account needs at least {ref}`offer consume access <user-access-offer-consume>` on the offer, {ref}`model admin access <user-access-model-admin>` on the offering model or {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller.

```text
juju integrate <application>[:<application endpoint>] <offer-url>[:<offer endpoint>]
```

Specifying the endpoint for the application and the offer is analogous to normal relations. They can be added but are often unnecessary:

```text
juju integrate <application> <offer-url>
```

When you integrate with an offer, a proxy application is made in the consuming model, named after the offer.

An offer can be consumed without integration. This workflow sets up the proxy application in the consuming model and creates a user-defined alias for the offer. This latter is what's used to subsequently relate to. Having an offer alias can avoid a namespace conflict with a pre-existing application.

```text
juju consume <offer-url> <offer-alias>
juju integrate <application> <offer alias>
```

Offers which have been consumed show up in `juju status` in the SAAS section. The relations block in status shows any relevant status information about the integrations to the offer in the Message field. This includes any error information due to rejected ingress, or if the relation is suspended etc.

To remove a consumed offer:

```text
juju remove-saas <offer alias>
```

```{ibnote}
See more: {ref}`command-juju-integrate`, {ref}`command-juju-consume`, {ref}`command-juju-remove-saas`
```


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

It's also possible to set up egress subnets as a model config value so that all cross model relations use those subnets without needing to use the `--via` option.

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

Beyond a certain number of firewall rules, which have been dynamically created to allow access from individual relations, Juju will revert to using the whitelist subnets as the access rules. The number of rules at which this cutover applies is cloud specific.

```

```{ibnote}
See more: {ref}`command-juju-set-firewall-rule`, {ref}`command-juju-firewall-rules`
```

(inspect-relations-to-an-offer)=
## Inspect relations to an offer

To inspect an offer's grants and connections, use a {ref}`model admin <user-access-model-admin>` account on the offering model or a {ref}`controller superuser <user-access-controller-superuser>` account on its controller. For example, inspect `hosted-mysql` in model `admin/default` on controller `foo`:

```text
juju offers -m foo:admin/default hosted-mysql --format yaml
```

Check `users.bob.access` for Bob's direct grant. Under `connections`, match each `username` to `bob` and record its `relation-id`. These IDs belong to the offering model; `source-model-uuid` identifies the consuming model. Omit the offer name to list offers in the selected model. The default table shows connected users and relation IDs; `--format summary` shows counts rather than individual connections.

The output can be filtered by:
 - interface: the interface name of the endpoint
 - application: the name of the offered application
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

All offers for a given user who can consume the offer:

```text
juju offers --format summary --allowed-consumer mary
```

The above command is best run with `--format` summary as the intent is to see, for a given user, what offers they might relate to, regardless of whether there are existing relations (which is what the tabular view shows).

```{ibnote}
See more: {ref}`command-juju-offers`
```

(manage-relations-to-an-offer)=
## Suspend, resume, or remove a relation to an offer

To suspend, resume or remove a relation, use an account with at least {ref}`model write access <user-access-model-write>` on the model where you run the command. Select that model with `-m` and use **its** relation ID. The examples use ID `2` from the {ref}`offering model's connection list <inspect-relations-to-an-offer>`.

To suspend that relation:

```text
juju suspend-relation -m foo:admin/default 2 --message "maintenance"
```

Suspension triggers the `relation-departed` and `relation-broken` hooks on both sides and closes firewall ingress for the relation.

To resume it after a revocation, first {ref}`restore the user's offer access <control-access-to-an-offer>`. Then resume the relation in the same offering model:

```text
juju resume-relation -m foo:admin/default 2
```

To remove the relation instead:

```text
juju remove-relation -m foo:admin/default 2
```

Removing the relation from either side also removes it on the other. If you remove it from the consuming model, use that model's relation ID, not the offering model's. Suspending or resuming can take several space-separated IDs; removing by ID takes one.

```{ibnote}
See more: {ref}`command-juju-suspend-relation`, {ref}`command-juju-resume-relation`, {ref}`command-juju-remove-relation`
```

## Remove an offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

An offer can be removed providing it hasn't been used in any relation. To override this behaviour the `--force` option is required, in which case the relation is also removed. This is how an offer is removed:

```text
juju remove-offer [--force] <offer-url>
```

Note that, if the offer resides in the current model, then the shorter offer name can be used instead of the longer URL.

Similarly, if an application is being offered, it cannot be deleted until all its offers are removed.

```{ibnote}
See more: {ref}`command-juju-remove-offer`
```
