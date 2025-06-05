(manage-users)=
# How to manage users

> See also: {ref}`user`

(add-a-user)=
## Add a user

```{tip}
**If you're the controller creator:** <br> Juju has already set up a user for you. Your username is `admin` and your access level is that of controller `superuser`. Run `juju logout` to be prompted to set up a password. Use `juju change-user-password` to set the password.
```

To add a user to a controller, run the `add-user` command followed by the username you want to assign to this user. For example:

```text
juju add-user alex
```

This will create a user with username 'alex' and a controller `login` access level.

> See more: {ref}`user-access-levels`

It will also print a line of code that you must give this user to run using their Juju client -- this will register the controller with their client and also prompt them to set up a password for the user.


````{dropdown} Example user setup

Admin adding a new user 'alex' to the controller:

```text
# Add a user named `alex`:
$ juju add-user alex
User "alex" added
Please send this command to alex:
    juju register MFUTBGFsZXgwFRMTMTAuMTM2LjEzNi4xOToxNzA3MAQghBj6RLW5VgmCSWsAesRm5unETluNu1-FczN9oVfNGuYTFGxvY2FsaG9zdC1jb250cm9sbGVy

"alex" has not been granted access to any models. You can use "juju grant" to grant access.
```

New user 'alex' accessing the controller:

```text
$ juju register MFUTBGFsZXgwFRMTMTAuMTM2LjEzNi4xOToxNzA3MAQghBj6RLW5VgmCSWsAesRm5unETluNu1-FczN9oVfNGuYTFGxvY2FsaG9zdC1jb250cm9sbGVy
Enter a new password: ********
Confirm password: ********
Enter a name for this controller {ref}`localhost-controller]: localhost-controller
Initial password successfully set for alex.

Welcome, alex. You are now logged into "localhost-controller".

There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".

```

````

```{note}
Controller registration (and any other Juju operations that involves communication between a client and a controller) requires that the client be able to contact the controller over the network on TCP port 17070. In particular, if using a LXD-based cloud, network routes need to be in place (i.e. to contact the controller LXD container the client traffic must be routed through the LXD host).
```

> See more: {ref}`command-juju-add-user`, {ref}`register-a-controller`


## View all the known users

To view a list of all the users known (i.e., allowed to log in) to the current controller, run the `users` command:


```text
juju users
```

The command also has flags that will allow you to specify a different controller, an output file, an output format, whether to print the full timestamp for connection times, etc.

> See more: {ref}`command-juju-users`

## View details about a user

To view details about a specific user, run the `show-user` command followed by the name of the user. For example:

```text
juju show-user alice
```

This will display the user's username, display name (if available), access level, creation date, and last connection time, in a YAML format.


````{dropdown} Expand to see a sample output for user 'admin'

```text
user-name: admin
display-name: admin
access: superuser
date-created: 8 minutes ago
last-connection: just now
```

````

> See more: {ref}`command-juju-show-user`


## View details about the current user

To see details about the current user, run the `whoami` command:

```text
juju whoami
```

This will print the current controller, model, and user username.


````{dropdown} Example output

```text
Controller:  microk8s-controller
Model:       <no-current-model>
User:        admin
```

````

> See more: {ref}`command-juju-whoami`


## Manage a user's access level
> See also: {ref}`user-access-levels`

The procedure for how to control a user's access level depends on whether you want to grant access at the level of the controller, model, application, or application offer or rather at the level of a cloud.

```{important}
This division doesn't currently align perfectly with the scope hierarchy, which is rather controller > cloud > model > application > offer (because the cloud scope is designed as a restriction on the controller scope for cases where multiple clouds are managed via the same controller).
```

### Manage access at the controller, model, application, or offer level

**Grant access.** To grant a user access at the controller, model, application, or offer level, run the `grant` command, specifying the user, applicable desired access level, and the target controller, model, application, or offer. For example:

```text
juju grant jim write mymodel
```

The command also has a flag that allows you to specify a different controller to operate in.

> See more: {ref}`command-juju-grant`

**Revoke access.** To revoke a user's access at the controller, model, application, or offer level, run the `revoke` command, specifying the user, access level to be revoked, and the controller, model, application, or offer to be revoked from. For example:

```text
juju revoke joe read mymodel
```

The command also has a flag that allows you to specify a different controller to operate in.

> See more: {ref}`command-juju-revoke`


### Manage access at the cloud level

**Grant access.** To grant a user's access at the cloud level, run the `grant-cloud` command followed by the name of the user, the access level, and the name of the cloud. For example:

```text
juju grant-cloud joe add-model fluffy
```

> See more: {ref}`command-juju-grant-cloud`

**Revoke access.** To revoke a user's access at the cloud level, run the `revoke-cloud` command followed by the name of the user, the access level to be revoked, and the name of the cloud. For example:

```text
juju revoke-cloud joe add-model fluffy
```

> See more: {ref}`command-juju-revoke-cloud`

(manage-a-users-login-details)=
## Manage a user's login details

**Set a password.** The procedure for how to set a password depends on whether you are the controller creator or rather some other user.

-  To set a password as a controller creator user ('admin'), run the `change-user-password` command, optionally followed by your username, 'admin'.

```text
juju change-user-password
```

This will prompt you to type, and then re-type, your desired password.

> See more: {ref}`command-juju-change-user-password`


- To set a password as a non-controller-creator user, follow the prompt you get when registering the controller via the `register` command.

> See more: {ref}`register-a-controller`

**Change a password.** To change the current user's password, run the `change-user-password` command:

```text
juju change-user-password
```

This will prompt you to type, and then re-type, your desired password.

The command also allows an optional username argument, and flags, allowing an admin to change / reset the password for another user.

> See more: {ref}`command-juju-change-user-password`

## Manage a user's login status

**Log in.**

```{important}
**If you're the controller creator:** <br> You've already been logged in as the `admin` user. To verify, run `juju whoami` or `juju show-user admin`; to set a password, run `juju change-user-password`; to log out, run `juju logout`.
```

```{important}
**If you've just registered an external controller with your client (via `juju register`):** <br> You're already logged in. Run `juju whoami` or `juju show-user <username>` to view your user details.

```

To log in as a user on the current controller, run the `login` command, using the `-u` flag to specify the user you want to log in as. For example:

```text
juju login -u alice
```

This will prompt you to enter the password.

The command also has flags that allow you to specify a controller, etc.

> See more: {ref}`command-juju-login`

**Log out.**

```{important}
**If you're the controller creator, and you haven't set a password yet:** <br> You will be prompted to set a password. Make sure to set it before logging out.
```

To log a user out of the current controller, run the `logout` command:

```text
juju logout
```

> See more: {ref}`command-juju-logout`

## Manage a user's enabled status

To disable a user on the current controller, run the `disable-user` command followed by the name of the user. For example:

```text
juju disable-user mike
```

> See more: {ref}`command-juju-disable-user`

```{tip}

**To view disabled users in the output of `juju users`:** Use the `--all` flag.

```

To re-enable a disabled user on a controller, run the `enable-user` command followed by the name of the user. For example:

```text
juju enable-user mike
```

> See more: {ref}`command-juju-enable-user`

## Remove a user

To remove a user from the current controller, run the `remove-user` command followed by the name of the user. For example:

```text
juju remove-user bob
```

This will prompt you to confirm, and then proceed to remove.

The command also has flags that allow you to specify a different controller, skip the confirmation, etc.

> See more: {ref}`command-juju-remove-user`

