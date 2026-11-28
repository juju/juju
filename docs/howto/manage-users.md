---
myst:
  html_meta:
    description: "Use the Juju CLI to invite and sign in users, inspect accounts, grant or revoke resource access, reset passwords, and safely suspend or remove users."
---

(manage-users)=
# How to manage users

```{ibnote}
See also: {ref}`User <user>`
```

(add-a-user)=
## Add a user

```{ibnote}
See also: {ref}`Adding a local user <adding-a-local-user>`
```

To invite a local user, sign in as a {ref}`controller superuser <user-access-controller-superuser>` on the controller where you want to create the account. Supply the controller name and the new username. You can optionally add a display name after the username:

```text
juju add-user -c <controller> <username>
```

Send the new user the complete `juju register` command printed by the CLI using a private channel. It contains their invitation to {ref}`activate the local user <activate-a-local-user>`. If they need an existing model, an administrator must separately {ref}`grant model access <change-a-users-model-access>`. If the CLI reports an error generating the invitation, {ref}`look up the username <view-details-about-a-user>` before adding it again: the controller may already have created it.

```{ibnote}
See more: {ref}`juju add-user <command-juju-add-user>`
```

For someone who signs in through an external identity provider, use their {ref}`external username in a grant <users-eligible-for-an-access-grant>`. Adding a local account with a similar spelling would create a different identity.

(activate-a-local-user)=
## Activate a local user

```{ibnote}
See also: {ref}`Activating a local user <activating-a-local-user>`
```

To activate your user, run the `juju register` command exactly as the administrator sent it. Your client must be able to reach the controller API at the advertised endpoint, through its configured proxy if applicable. Being able to SSH to a host used to provision a manual cloud does not establish that connection.

Choose and confirm a new password, then accept or enter the local name for the controller. Registration signs you in. If you can access one model, the CLI selects it; if several are available, choose one with {ref}`juju switch <switch-to-a-different-model>`. If none are available, ask an administrator for access to the model you need. Creating your own model is a separate {ref}`cloud-authorized task <add-a-model>`.

If the CLI recognizes the controller address, follow the message for the local name it found. Sign out the saved user before replacing its account. Use `--replace` for an intended update to the local controller entry, not to rename the same controller or obtain a new invitation.

If the controller refuses registration, the CLI reports `Provided registration token may have expired.` That message doesn't establish an expiry time. Check that you used the latest invitation for the intended account. If registration failed after the controller accepted your password, use that password for your next login. If you cannot sign in and need a fresh invitation, ask a {ref}`controller superuser <user-access-controller-superuser>` to {ref}`reset your local user's password <reset-another-local-users-password>`.

```{ibnote}
See more: {ref}`juju register <command-juju-register>`
```

If the administrator gave you a public controller hostname instead of an invitation, {ref}`register the public controller <register-a-controller>` with that hostname. This starts its sign-in flow; it does not activate a local password with an invitation.

(view-all-the-known-users)=
(list-users-on-a-controller)=
## List users on a controller

```{ibnote}
See also: {ref}`Listing users on a controller <listing-users-on-a-controller>`
```

To list the users visible to your account on a controller, specify the controller name:

```text
juju users -c <controller>
```

A controller superuser sees enabled users by default; other users see themselves. In the table, `*` marks the current user. To include disabled accounts in the superuser's directory, run:

```text
juju users -c <controller> --all
```

```{ibnote}
See more: {ref}`juju users <command-juju-users>`
```

(list-users-of-a-model)=
### List users of a model

```{ibnote}
See also: {ref}`Listing users with model access <listing-users-with-model-access>`
```

To list the users visible to you on a model, specify its controller and model name:

```text
juju users -c <controller> <model>
```

The table shows model access and the last recorded connection to that model. The CLI uses `--all` and `--exact-time` only for controller listings. An empty roster prints `No users to display.` If you receive `user not found on model`, ask a model administrator to check the roster.

```{ibnote}
See more: {ref}`juju users <command-juju-users>`
```

(view-details-about-a-user)=
## View details about a user

```{ibnote}
See also: {ref}`Looking up a controller user <looking-up-a-controller-user>`
```

To look up your own record on the current controller, run `juju show-user` without a username. To inspect another user's record as a controller superuser, specify the controller holding the account and the username to look up:

```text
juju show-user -c <controller> <username>
```

The CLI prints one YAML record by default. For local users, it includes the creation time and reports `never connected` when there is no recorded connection. Use `--exact-time` for full timestamps. The CLI omits both dates for external users. A controller superuser can look up disabled users by name. A `permission denied` error leaves the user's existence unresolved.

```{ibnote}
See more: {ref}`juju show-user <command-juju-show-user>`
```

(view-details-about-the-current-user)=
## View details about the current user

```{ibnote}
See also: {ref}`The CLI account for a controller <cli-accounts-by-controller>`
```

To see which account and model this client has selected, run:

```text
juju whoami
```

The client reads its local state. It can show a signed-in user with `<no-current-model>`, or report that there is no current controller or no saved login. To ask the controller for the current user's record, use {ref}`juju show-user <view-details-about-a-user>` instead.

```{ibnote}
See more: {ref}`juju whoami <command-juju-whoami>`
```

(manage-user-access)=
## Manage user access

```{ibnote}
See also: {ref}`User access levels <user-access-levels>`; {ref}`Changes to a user's access grant <changes-to-a-users-access-grant>`
```

To change a user's access, choose the target first. Use `grant` or `revoke` without a target argument for a controller, with model names for models, or with offer URLs for offers. Use `grant-cloud` or `revoke-cloud` for clouds. A bare application name does not select an application-level user grant.

Before revoking, check the user's direct grant on each target and {ref}`choose the level they hold or a lower one <changes-to-a-users-access-grant>`. Check whether any {ref}`shared external access <shared-access-for-external-users>` still applies.

Use separate arguments of the same kind for multiple targets. A model name-resolution error stops submission. After submission, one target can change even if another fails. Inspect each target before retrying.

(manage-access-at-the-controller-model-application-or-offer-level)=
(change-a-users-controller-access)=
### Change a user's controller access

```{ibnote}
See also: {ref}`Changing controller access <changing-controller-access>`
```

To grant an external user login access, sign in as a {ref}`controller superuser <user-access-controller-superuser>` on the controller they need to access. Specify that controller with `-c` and use the username from their identity provider:

```text
juju grant -c <controller> <external-username> login
```

```{ibnote}
See more: {ref}`juju grant <command-juju-grant>`
```

To reduce an external user's existing direct controller superuser grant to login, revoke `superuser` on the controller that holds the grant:

```text
juju revoke -c <controller> <external-username> superuser
```

```{ibnote}
See more: {ref}`juju revoke <command-juju-revoke>`
```

(manage-access-at-the-cloud-level)=
(change-a-users-cloud-access)=
### Change a user's cloud access

```{ibnote}
See also: {ref}`Changing cloud access <changing-cloud-access>`
```

To change your own cloud grant, ask a different authorised administrator.

To let a user add models on a cloud, sign in as a {ref}`cloud administrator <user-access-cloud-admin>` for that cloud or a {ref}`controller superuser <user-access-controller-superuser>`. Specify the controller where the cloud is registered, the recipient's username and the cloud name:

```text
juju grant-cloud -c <controller> <username> add-model <cloud>
```

The user may also need to {ref}`add a cloud credential <add-a-credential>` before creating a model. This grant does not let them assign access on an existing model.

```{ibnote}
See more: {ref}`juju grant-cloud <command-juju-grant-cloud>`
```

To withdraw a user's direct cloud grant, check that they hold a direct grant on the intended cloud and revoke the lowest cloud level:

```text
juju revoke-cloud -c <controller> <username> add-model <cloud>
```

```{ibnote}
See more: {ref}`juju revoke-cloud <command-juju-revoke-cloud>`
```

(change-a-users-model-access)=
### Change a user's model access

```{ibnote}
See also: {ref}`Changing model access <changing-model-access>`
```

To let a user change workloads in a model, sign in as a {ref}`model administrator <user-access-model-admin>` for that model or a {ref}`controller superuser <user-access-controller-superuser>`. Specify the model's controller, the recipient's username and the model name:

```text
juju grant -c <controller> <username> write <model>
```

```{ibnote}
See more: {ref}`juju grant <command-juju-grant>`
```

To remove a user's direct model access, check their grant in the {ref}`model user list <list-users-of-a-model>`. If they are the {ref}`last active administrator <keeping-an-active-model-administrator>`, grant another enabled user model `admin` first. Revoke the lowest level:

```text
juju revoke -c <controller> <username> read <model>
```

To keep a user as a reader when they directly hold `write`, revoke `write`:

```text
juju revoke -c <controller> <username> write <model>
```

If the error ends with `model would be left without an admin user`, grant another enabled user model `admin` on that model before retrying. {ref}`Check the affected models before retrying a multi-model request <manage-user-access>`.

```{ibnote}
See more: {ref}`juju revoke <command-juju-revoke>`
```

(change-a-users-offer-access)=
### Change a user's offer access

```{ibnote}
See also: {ref}`Changing access to an offer <changing-access-to-an-offer>`
```

To {ref}`change a user's offer grant <control-access-to-an-offer>`, identify the offer URL and the recipient's username on the offering controller. To use the offer, {ref}`integrate with it from the consuming model <integrate-with-an-offer>`.

(manage-a-users-login-details)=
## Manage a user's login details

```{ibnote}
See also: {ref}`Changing a local user's password <changing-a-local-users-password>`; {ref}`Resetting a local user's password <resetting-a-local-users-password>`
```

Change a password to choose the new value now. Reset another user's password to give that person a new invitation instead. For an external identity, use the identity provider's password procedure.

(change-a-local-users-password)=
### Change a local user's password

```{ibnote}
See also: {ref}`Changing a local user's password <changing-a-local-users-password>`
```

To set or change your own local password, specify the controller where you are signed in. This also applies to the bootstrap user `admin`:

```text
juju change-user-password -c <controller>
```

Enter the new password twice. A controller superuser can give another local username to set that user's password. With `--no-prompt`, supply one newline-terminated password on standard input. Keep it out of shell history. If a self-change reports `recording macaroon` or `failed to update client credentials`, the controller has accepted the new password. Use it to sign in.

```{ibnote}
See more: {ref}`juju change-user-password <command-juju-change-user-password>`
```

(reset-another-local-users-password)=
### Reset another local user's password

```{ibnote}
See also: {ref}`Resetting a local user's password <resetting-a-local-users-password>`
```

To give another local user a fresh invitation, sign in as a controller superuser on the controller that holds the account. Confirm the username before resetting the password: the reset replaces both the old password and any outstanding activation key.

```text
juju change-user-password -c <controller> --reset <username>
```

Privately send the new registration command to the user so they can {ref}`activate their account <activate-a-local-user>` again. With `--reset` and your own username, or without a target username, the CLI tells you to change your own password and sends no reset.

```{ibnote}
See more: {ref}`juju change-user-password <command-juju-change-user-password>`
```

(manage-a-user-s-login-status)=
## Manage a user's login status

```{ibnote}
See also: {ref}`The CLI account for a controller <cli-accounts-by-controller>`
```

One client can be signed in to different controllers under different usernames. To switch users on the same controller, sign out the saved user first.

(sign-in-as-a-user)=
### Sign in as a user

```{ibnote}
See also: {ref}`Types of users <types-of-users>`; {ref}`Controller access levels <controller-access-levels>`
```

To sign in to a registered controller with a local account, specify the controller name and your username on that controller:

```text
juju login -c <controller> --user <username>
```

Enter your password when prompted. On a controller configured for OpenID Connect, omit `--user` and follow the identity provider's sign-in flow. For a local non-interactive login, `cannot deduce user, please pass --user with --no-prompt` means the CLI needs the local username explicitly. If a public controller presents an unverified certificate, confirm its fingerprint with the administrator before accepting it; don't enable unattended trust merely to bypass the prompt.

```{ibnote}
See more: {ref}`juju login <command-juju-login>`
```

(sign-out-as-a-user)=
### Sign out as a user

```{ibnote}
See also: {ref}`The CLI account for a controller <cli-accounts-by-controller>`
```

To sign this client out of a controller while keeping the controller registered, specify its name:

```text
juju logout -c <controller>
```

If the CLI reports `preventing account loss`, it has a password saved for this account and has refused to clear the credentials. {ref}`Set a password you know <change-a-local-users-password>`, then repeat the ordinary logout; logout does not prompt you to set one. Use `--force` only if you mean to discard the saved credentials and have a way to sign in again. To remove the controller entry from this client, {ref}`unregister the controller <unregister-a-controller>` instead. Neither action disables the user on the controller.

```{ibnote}
See more: {ref}`juju logout <command-juju-logout>`
```

(manage-a-user-s-enabled-status)=
## Manage a user's enabled status

```{ibnote}
See also: {ref}`Disabling and re-enabling a user <disabling-and-re-enabling-a-user>`
```

Use disable and re-enable when the user should stop signing in to this controller temporarily. To withdraw access to one model, cloud or offer while retaining other access, {ref}`change the user's resource grant <manage-user-access>` instead.

(disable-a-user)=
### Disable a user

```{ibnote}
See also: {ref}`Disabling and re-enabling a user <disabling-and-re-enabling-a-user>`
```

To suspend a user's sign-in, sign in as a {ref}`controller superuser <user-access-controller-superuser>` on the controller that holds the account. Check the models the user administers. If they are a model's {ref}`last active administrator <keeping-an-active-model-administrator>`, {ref}`grant another enabled user model admin access <change-a-users-model-access>` on that model. Then disable the account by username:

```text
juju disable-user -c <controller> <username>
```

If the controller reports that `model(s) would be left without an admin user`, make that replacement grant on every affected model before retrying. As a controller superuser, you can {ref}`list disabled users <list-users-on-a-controller>` with `--all` or look up the user by name.

```{ibnote}
See more: {ref}`juju disable-user <command-juju-disable-user>`
```

(re-enable-a-user)=
### Re-enable a user

```{ibnote}
See also: {ref}`Disabling and re-enabling a user <disabling-and-re-enabling-a-user>`
```

To restore a disabled user's sign-in, sign in as a {ref}`controller superuser <user-access-controller-superuser>` on the controller that holds the account. Check that the user's retained grants are still appropriate, then re-enable the account by username:

```text
juju enable-user -c <controller> <username>
```

```{ibnote}
See more: {ref}`juju enable-user <command-juju-enable-user>`
```

(remove-a-user)=
## Remove a user

```{ibnote}
See also: {ref}`Removing a user <removing-a-user>`
```

To remove a user permanently, sign in as a {ref}`controller superuser <user-access-controller-superuser>` on the controller that holds the account. Verify the username and arrange the handover before confirming removal:

1. If the suspension is temporary, {ref}`disable the user <disable-a-user>` instead. The built-in controller owner `admin` cannot be removed.
2. Give a different enabled user an explicit {ref}`model admin grant <change-a-users-model-access>` on every model that would otherwise fail the {ref}`model administrator check <keeping-an-active-model-administrator>`.
3. Identify models using cloud credentials owned by the user. Have the model administrators switch each model to a replacement with {ref}`juju set-credential <command-juju-set-credential>`. If a credential should also be retired from this controller, ask its owner to run {ref}`juju remove-credential <command-juju-remove-credential>` for the cloud and credential, specifying the controller with `-c`, while they can still sign in. Running that command from your own account targets your credential, not theirs.

```{warning}
Removal clears the user's sign-in material, direct grants and registered SSH keys on this controller. Adding the same username later creates a new user; it won't restore the removed user's access.
```

To remove the verified account, specify the controller name and the username:

```text
juju remove-user -c <controller> <username>
```

Confirm the username and controller at the prompt, then answer `y` or `yes`. Skipping confirmation with `-y` does not bypass the controller's checks.

```{ibnote}
See more: {ref}`juju remove-user <command-juju-remove-user>`
```

