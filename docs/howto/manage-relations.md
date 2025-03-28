(manage-relations)=
# How to manage relations

> See also: {ref}`relation`

## Add a relation

<!--TODO: Streamline story, e.g.: Suppose you have two applications, `mysql` and `wordpress`. These applications can only be related in one way-->

The procedure differs slightly depending on whether the applications that you want to integrate are on the same model or rather on different models.

### Add a same-model relation


To set up a relation between two applications on the same model, run the `integrate` command followed by the names of the applications. For example:

```text
juju integrate mysql wordpress
```

This will satisfy WordPress's database requirement where MySQL provides the appropriate schema and access credentials required for WordPress to run properly.

The code above however works only if there is no ambiguity in what relation the charm _requires_ and what the related charm _provides_. 

If the charms in question are able to establish multiple relation types, Juju may need to be supplied with more information as to how the charms should be joined. For example, if we try instead to relate the 'mysql' charm to the 'mediawiki' charm:

```text
juju integrate mysql mediawiki 
```

the result is an error:

``` text
error: ambiguous relation: "mediawiki mysql" could refer to
  "mediawiki:db mysql:db"; "mediawiki:slave mysql:db"
```

The solution is to be explicit when referring to an *endpoint*, where the latter has a format of `<application>:<application endpoint>`. In this case, it is 'db' for both applications. However, it is not necessary to specify the MySQL endpoint because only the MediaWiki endpoint is ambiguous (according to the error message). Therefore, the command becomes:

```text
juju integrate mysql mediawiki:db
```
```{note}

The integration endpoints provided or required by a charm are listed in the result of the `juju info` command. They are also listed on the page for the charmed operator at [Charmhub](https://charmhub.io).

```

> See more: {ref}`command-juju-integrate`


(add-a-cross-model-relation)=
### Add a cross-model relation
> See also: {ref}`cross-model-relation`


In a cross-model relation there is also an 'offering' model and a 'consuming' model. The admin of the 'offering' model 'offers' an application for consumption outside of the model and grants an external user access to it. The user on the 'consuming' model can then find an offer to use, consume the offer, and integrate an application on their model with the 'offer' via the same `integrate` command as in the same-model case (just that the offer must be specified in terms of its offer URL or its consume alias). This creates a local proxy for the offer in the consuming model, and the application is subsequently treated as any other application in the model.

> See more: {ref}`integrate-with-an-offer`

## View all the current relations

To view the current relations in the model, run `juju status --relations`. The example below shows a peer relation and a regular relation:

```text
[...]
Relation provider  Requirer       Interface  Type     Message
mysql:cluster      mysql:cluster  mysql-ha   peer     
mysql:db           mediawiki:db   mysql      regular
```

To view just a specific relation and the applications it integrates,  run `juju status --relations` followed by the provider and the requirer application (and endpoint). For example, based on the output above, `juju status --relations mysql mediawiki` would output: 

```text
[...]
Relation provider  Requirer       Interface  Type     Message  
mysql:db           mediawiki:db   mysql      regular
```

> See more: {ref}`command-juju-status`


## Get the relation ID

To get the ID of a relation, for any unit participating in the relation, run the `show-unit` command -- the output will also include the relation ID. For example:

```text
$ juju show-unit synapse/0
  
...
  - relation-id: 7
    endpoint: synapse-peers
    related-endpoint: synapse-peers
   application-data:
      secret-id: secret://1234
    local-unit:
      in-scope: true
```



## Remove a relation

Regardless of whether the relation is same-model or cross-model, to remove an relation, run the `remove-relation` command followed by  the names of the two applications involved in the integration:

`juju remove-relation <application-name> <application-name>`

For example:

```text
juju remove-relation mediawiki mysql
```

In cases where there is more than one relation between the two applications, specify the interface at least for one of the applications:

```text
juju remove-relation mediawiki mysql:db
```

> See more: {ref}`command-juju-remove-relation`

