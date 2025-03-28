(user)=
# User

<!--TODOS INHERITED FROM 
Todo:
- bug tracking: https://bugs.launchpad.net/bugs/1808661
- bug tracking: https://bugs.launchpad.net/bugs/1808662
-->

> See also: {ref}`manage-users`

In Juju, a **user** is any person able to log in to a Juju {ref}`controller <controller>`.

```{note}
Juju users are not related in any way to the client system users. 
```

Users can be created in two ways: Implicitly by bootstrapping a controller into a cloud or explicitly by adding a user to a controller (`juju add-user`). 

A user logs in to a Juju controller using a username and a password. The user created implicitly gets the username `admin` and  is prompted to create a password the first time they attempt to log out. A user created explicitly gets the username assigned to them when being added (via `juju add-user`) and is prompted to create login details when they register the new controller with their Juju client. 


```{note}

A user's username and password are entirely different from the credentials referenced in `juju` commands such as `add-credential`---those are about access to a cloud, whereas these are about access to a Juju controller.

```

```{important}

Multiple users can be accommodated by the same Juju client. However, there can only be one user logged in at a time.

```

<!--
Juju has an internal user framework that allows for the sharing of controllers and models. To achieve this, a Juju user can be created, disabled, and have rights granted and revoked. Users remote to the system that created a controller can use their own Juju client to log in to the controller and manage the environment based on the rights conferred. 
-->

Every user is associated with an access level. The default level for the user created implicitly (`admin`) is the controller `superuser` access level, which means they can do everything at the level of the entire controller. The default level for a user created explicitly is the controller `login` level, which means they can do nothing on the controller other than register it with their client and log in to it -- for anything more they must be granted a higher level explicitly.


(user-access-levels)=
## User access levels


<!--This actually replicates the details for juju grant/revoke and juju grant/revoke-cloud + adds some description for each access level. This feels a bit duplicative. On the one hand, it feels like those levels should be defined already in the command help. On the other hand, it doesn't seem ideal for people to find out about the user access levels just from the command help. -->

A Juju user may have different abilities, according to the access level they have been granted. This document describes the various access levels and the corresponding abilities.

### Valid access levels for controllers

(user-access-controller-login)=
#### `login`

Granted: Via `juju register.

Abilities: Log in to the controller.

(user-access-controller-superuser)=
#### `superuser`

Granted: Automatically by bootstrapping a controller or by having the username 'admin'.

Abilities: Do anything that it is possible to do at the level of a controller.


```{note}
A person logged into the `jaas` controller automatically has the login access level. This is automatically granted via ` juju grant login everyone@external`.
```



```{note}
Since multiple controllers—and therefore multiple controller administrators—are possible, there is no such thing as an overarching "Juju administrator". Nevertheless, a user with the superuser access level is usually what people refer to as "the admin".
```


### Valid access levels for clouds

A controller can manage models on many clouds. With cloud-level access you can give a user permission to access one cloud but not another related to that controller.

(user-access-cloud-add-model)=
#### `add-model`

Granted: Via {ref}`command-juju-grant-cloud`.

Abilities: Add a model. Grant another user model-level permissions.

(user-access-cloud-admin)=
#### `admin` 

Granted: Via {ref}`command-juju-grant-cloud`.

Abilities: You can do anything that it is possible to do at the level of a cloud.


### Valid access levels for models

(user-access-model-read)=
#### `read` 

Granted: Via {ref}`command-juju-grant`.

Abilities: View the content of a model without changing it. Use any of the read commands.

(user-access-model-write)=
#### `write`

Granted: Via {ref}`command-juju-grant`.

Abilities: Deploy and manage applications on the model.

(user-access-model-admin)=
#### `admin`

Granted: Via {ref}`command-juju-grant`.

Abilities: Do anything that it is possible to do at the level of a model.

### Valid access levels for application offers

(user-access-offer-read)=
#### `read`

Granted: Via {ref}`command-juju-grant`.

Abilities: View offers during a search with {ref}`command-juju-find-offers`.

(user-access-offer-consume)=
#### `consume`

Granted: Via {ref}`command-juju-grant`.

Abilities: Relate an application to the offer.

(user-access-offer-admin)=
#### `admin`

Granted: Via {ref}`command-juju-grant`.

Abilities: You can do anything that it is possible to do at the level of an offer.





