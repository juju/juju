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

(manage-an-offer-on-the-offering-side)=
## Manage an offer on the offering side

### Create an offer

```{ibnote}
See also: {ref}`Creating an offer <creating-an-offer>`, {ref}`Offer URLs <offer-urls>`
```

To create an offer, give `juju offer` the application and endpoint as `application:endpoint`. You can also give the offer a name. For example, to deploy `mysql` and offer its `database` endpoint as `hosted-mysql`:

```text
juju deploy mysql
juju offer mysql:database hosted-mysql
```

To offer several endpoints of the application, separate them with commas:

```text
juju offer mysql:database,certificates hosted-mysql
```

To find an application's endpoints, run the `show-application` command followed by the application, and check the list under `endpoint-bindings`:

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

```{ibnote}
See more: {ref}`command-juju-offer`
```

(view-an-offers-details)=
### View an offer’s details

```{ibnote}
See also: {ref}`Viewing an offer <viewing-an-offer>`
```

To view the details of an offer, run the `show-offer` command followed by the offer URL. For example:

```text
juju show-offer admin/default.hosted-mysql
Store        URL                         Access  Description                                    Endpoint      Interface         Role
foo          admin/default.hosted-mysql  admin   MySQL is a widely used, open-source            certificates  tls-certificates  requirer
                                                 relational database management system          database      mysql_client      provider
                                                 (RDBMS). MySQL InnoDB cluster provides a
                                                 complete high availability solution for MySQL
                                                 via Group Replic...
```

To see the full description and the access entries visible to you, add `--format yaml`:

```text
juju show-offer admin/default.hosted-mysql --format yaml
foo:admin/default.hosted-mysql:
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
```

```{ibnote}
See more: {ref}`command-juju-show-offer`
```

### Control access to an offer

```{ibnote}
See also: {ref}`Changing access to an offer <changing-access-to-an-offer>`
```

**Grant access.** To grant a user access to an offer, run the `grant` command followed by the user, the access level and the offer URL. For example, to grant `bob` `consume` access:

```text
juju grant bob consume admin/default.hosted-mysql
```

**Revoke access.** To revoke a user's access to an offer:

1. Check the user's connections to the offer, because {ref}`revoking access <changing-access-to-an-offer>` can suspend their relations:

   ```text
   juju offers hosted-mysql
   ```

2. Run the `revoke` command followed by the user, the access level and the offer URL. For example, to revoke `bob`'s `consume` access:

   ```text
   juju revoke bob consume admin/default.hosted-mysql
   ```

   To remove all of `bob`'s access to the offer, revoke `read`:

   ```text
   juju revoke bob read admin/default.hosted-mysql
   ```

```{ibnote}
See more: {ref}`command-juju-grant`, {ref}`command-juju-revoke`
```

(inspect-relations-to-an-offer)=
### Inspect relations to an offer

```{ibnote}
See also: {ref}`Listing offers and their connections <listing-offers-and-their-connections>`
```

To view the connections to offers in the current model, run `juju offers`. To find a connection made by a particular user, look in the `User` column. For example, to inspect connections to offers with `mysql` in the name:

```text
juju offers mysql
Offer  User   Relation ID  Status  Endpoint  Interface  Role      Ingress subnets
mysql  admin  2            joined  db        mysql      provider  69.193.151.51/32
```

To see how many connections are joined out of the total for each offer, add `--format summary`. To see the UUID of the consuming model for a connection, use `--format yaml`.

```text
juju offers --format summary
Offer         Application  Charm        Connected  Store   URL                      Endpoint  Interface  Role
hosted-mysql  mysql        ch:mysql-57  1/1        myctrl  admin/prod.hosted-mysql  db        mysql      provider
```

To list the offers of one application, use `--application`:

```text
juju offers --application mysql
```

To see which offers grant a user `consume` or `admin` access, use `--allowed-consumer` with `--format summary`:

```text
juju offers --format summary --allowed-consumer mary
```

```{ibnote}
See more: {ref}`command-juju-offers`
```

(suspend-resume-or-remove-a-relation-to-an-offer)=
### Suspend, resume, or remove a relation to an offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

**Find the relation ID.** Several applications can relate to the same offer, so you identify a relation by its ID. To find the ID, run the `relation-ids` hook tool on a unit of one of the related applications, followed by the endpoint:

```text
juju exec --unit $UNIT_FOR_APP1 -- relation-ids endpoint
```

The output has the form `<endpoint>:<relation ID>`.

**Suspend a relation.** To suspend a relation, run the `suspend-relation` command followed by the relation ID and, optionally, a message. For example:

```text
juju suspend-relation 2 --message "reason for suspension"
```

```{note}
Suspended relations will run the relation departed / broken hooks on either end, and any firewall ingress will be closed.
```

**Resume a relation.** To resume a suspended relation, run the `resume-relation` command followed by the relation ID:

```text
juju resume-relation 2
```

The `suspend-relation` and `resume-relation` commands also take several relation IDs, separated by spaces:

```text
juju suspend-relation 3 4 5 --message "reason for suspension"
```

**Remove a relation.** To remove a relation, run the `remove-relation` command followed by the relation ID:

```text
juju remove-relation 2
```

```{note}
Removing a single relation on either side removes that relation on the other side. To remove **all** of a SAAS application's relations from the consuming side, {ref}`remove the SAAS application <remove-a-saas-application>`.
```

```{ibnote}
See more: {ref}`command-juju-suspend-relation`, {ref}`command-juju-resume-relation`, {ref}`command-juju-remove-relation`
```

(remove-an-offer)=
### Remove an offer

```{ibnote}
See also: {ref}`Removing an offer <removing-an-offer>`
```

To remove an offer:

1. Check whether the offer has connections:

   ```text
   juju offers --format summary hosted-mysql
   ```

2. If the offer has connections, {ref}`remove each relation to the offer <suspend-resume-or-remove-a-relation-to-an-offer>`, and check again until it has none.

3. Run the `remove-offer` command followed by the offer URL:

   ```text
   juju remove-offer admin/default.hosted-mysql
   ```

4. If another model consumed the offer, watch that consuming model. For example, for the `frontend` model on the `production` controller:

   ```text
   watch -n 1 juju status -m production:frontend
   ```

   When the offer's SAAS application disappears from the SAAS section, press {kbd}`Ctrl`+{kbd}`C` to stop watching.

```{ibnote}
See more: {ref}`command-juju-remove-offer`
```

(use-an-offer-on-the-consuming-side)=
## Use an offer on the consuming side

(find-an-offer-to-use)=
### Find an offer to use

```{ibnote}
See also: {ref}`Finding offers <finding-offers>`
```

To find offers you can see on the current controller, run `juju find-offers`. To search another controller, give its name followed by a colon:

```text
juju find-offers foo:
Store  URL                         Access  Interfaces
foo    admin/default.hosted-mysql  admin   mysql:database
foo    admin/default.postgresql    admin   pgsql:db
```

To see the access entries visible to you for each offer, add `--format yaml`:

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
```

To find the offers in a model, give the model, with or without a controller:

```text
juju find-offers admin/another-model
juju find-offers foo:admin/another-model
```

To find the offers with an interface, on the current controller or on another one, add `--interface`:

```text
juju find-offers --interface mysql_client
juju find-offers --interface mysql_client foo:
```

To find the offers with "sql" in their name, add `--offer`:

```text
juju find-offers --offer sql
```

```{ibnote}
See more: {ref}`command-juju-find-offers`
```

(consume-an-offer)=
### Consume an offer

```{ibnote}
See also: {ref}`Consuming an offer <consuming-an-offer>`, {ref}`SAAS application names <saas-application-names>`
```

To consume an offer, run the `consume` command followed by the offer URL. This creates a SAAS application for the offer in your model. For example:

```text
juju consume admin/default.hosted-mysql
Added admin/default.hosted-mysql as hosted-mysql
```

If the offer's name is already in use in your model, choose a different local name when you consume the offer:

```text
juju consume admin/default.hosted-mysql mysql-prod
```

If the offer is on another controller, put the controller name and a colon in front of the offer URL:

```text
juju consume prod:admin/default.hosted-mysql
```

To check which offers the consuming model has consumed, run `juju status`. In the SAAS section, match the offering controller (`Store`) and offer URL to the local name (`SAAS`). Use that name to {ref}`inspect the SAAS application <view-a-saas-application>`.

```text
juju status
...
SAAS          Status  Store  URL
hosted-mysql  active  prod   admin/default.hosted-mysql
...
```

```{ibnote}
See more: {ref}`command-juju-consume`, {ref}`command-juju-status`
```

(integrate-with-an-offer)=
### Integrate with an offer

```{ibnote}
See also: {ref}`Consuming an offer <consuming-an-offer>`, {ref}`Cross-model relation <cross-model-relation>`
```

To integrate an application in your model with an offer, run the `integrate` command followed by the application and the offer URL. For example:

```text
juju integrate mediawiki admin/default.hosted-mysql
```

Given an offer URL, `integrate` first consumes the offer and then adds the relation. If the relation then fails, the SAAS application stays in your model: integrate with it again, or {ref}`remove it <remove-a-saas-application>`.

If you consumed the offer under another name, integrate with that name, not with the offer URL:

```text
juju integrate mediawiki mysql-prod
```

As for any relation, you can add a colon and an endpoint to either side, but you usually don't need to:

```text
juju integrate mediawiki:db admin/default.hosted-mysql:database
```

```{ibnote}
See more: {ref}`command-juju-integrate`
```

### Allow traffic from an integrated offer
> Who: User with {ref}`offer admin access <user-access-offer-admin>`.

**Set egress subnets on the consuming side.** When the consuming model is behind NAT, its traffic leaves the network from a translated address or network. To have the offering side open its firewall to that traffic, run the `integrate` command with `--via`, followed by the translated subnets in CIDR notation, separated by commas (a single address is a `/32`). For example:

```text
juju integrate mediawiki:db ian:admin/default.mysql --via 69.32.56.0/8
```

To use the same subnets for every cross-model relation of the consuming model, set the `egress-subnets` model configuration key in the consuming model instead:

```text
juju model-config egress-subnets=69.32.56.0/8
```

**Limit ingress on the offering side.** To limit the ingress that consumers can request, run the `set-firewall-rule` command in the offering model, for the `juju-application-offer` rule, followed by `--whitelist` and the allowed subnets. A relation that requests ingress outside these subnets fails and is marked as in error. For example:

```text
juju set-firewall-rule juju-application-offer --whitelist 103.37.0.0/16
```

```{note}
The `juju set-firewall-rule` command only affects subsequently created relations, not existing ones. Only new relations will be rejected if the changed firewall rules preclude the requested ingress.
```

To view the firewall rules, run the `firewall-rules` command:

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
