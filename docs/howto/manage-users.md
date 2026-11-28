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

To manage a user, first choose the controller that holds the account. The examples use `operations` as the controller name, `alex` as a local username and `payments` as a model name. Substitute the names from your deployment. Where a command takes `-c`, use it to select a controller other than the current one.

(add-a-user)=
## Add a user

```{ibnote}
See also: {ref}`Adding a local user <adding-a-local-user>`
```

To invite a local user, sign in as a {ref}`controller superuser <user-access-controller-superuser>` and run `juju add-user` with the username. You may give the user a display name as a second argument. For example, invite Alex Morgan to the `operations` controller:

```text
juju add-user -c operations alex "Alex Morgan"
```

Send Alex the complete `juju register` command printed by the CLI using a private channel. It contains Alex's invitation. Alex can now {ref}`activate the local user <activate-a-local-user>`; an administrator must separately {ref}`grant model access <change-a-users-model-access>` if Alex needs an existing model. If the CLI reports an error generating the invitation, {ref}`look up the username <view-details-about-a-user>` before adding it again: the controller may already have created it.

```{ibnote}
See more: {ref}`juju add-user <command-juju-add-user>`
```

For someone who signs in through an external identity provider, use their {ref}`external username in a grant <users-eligible-for-an-access-grant>`. Adding a local account with a similar spelling would create a different identity.

(activate-a-local-user)=
## Activate a local user

```{ibnote}
See also: {ref}`Activating a local user <activating-a-local-user>`
```

To activate your user, run the `juju register` command exactly as the administrator sent it. Your client must be able to reach the **controller API** at the advertised endpoint, through its configured proxy if applicable. Being able to SSH to a host used to provision a manual cloud does not establish that connection.

Choose and confirm a new password, then accept or enter the local name for the controller. Registration signs you in. If you can access one model, the CLI selects it; if several are available, choose one with {ref}`juju switch <switch-to-a-different-model>`. If none are available, ask an administrator for access to the model you need. Creating your own model is a separate {ref}`cloud-authorized task <add-a-model>`.

If the CLI recognizes the controller address, follow the message for the local name it found. Sign out the saved user before replacing its account. Use `--replace` for an intended update to the local controller entry, not to rename the same controller or obtain a new invitation.

If the controller refuses registration, the CLI reports `Provided registration token may have expired.` That message doesn't establish an expiry time. Check that you used the latest invitation for the intended account. If registration failed **after** the controller accepted your password, use that password for your next login. If you cannot sign in and need a fresh invitation, ask a {ref}`controller superuser <user-access-controller-superuser>` to {ref}`reset your local user's password <reset-another-local-users-password>`.

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

To list the controller users visible to your account, run:

```text
juju users -c operations
```

A controller superuser sees enabled users by default; other users see themselves. In the table, `*` marks the current user. To include disabled accounts in the superuser's directory, run:

```text
juju users -c operations --all
```

```{ibnote}
See more: {ref}`juju users <command-juju-users>`
```

(list-users-of-a-model)=
### List users of a model

```{ibnote}
See also: {ref}`Listing users with model access <listing-users-with-model-access>`
```

To list the users visible to you on the `payments` model, give `users` the model name:

```text
juju users -c operations payments
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

To look up your own record on the current controller, run `juju show-user` without a username. To inspect Alex's record on `operations` as a controller superuser, name both:

```text
juju show-user -c operations alex
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

To change a user's access, choose the target first. Use `grant` or `revoke` without a target argument for a **controller**, with model names for **models**, or with offer URLs for **offers**. Use `grant-cloud` or `revoke-cloud` for **clouds**. A bare application name does not select an application-level user grant.

Before revoking, check the user's direct grant on each target and {ref}`choose the level they hold or a lower one <changes-to-a-users-access-grant>`. Check whether any {ref}`shared external access <shared-access-for-external-users>` still applies.

Use separate arguments of the same kind for multiple targets. A model name-resolution error stops submission. After submission, one target can change even if another fails. Inspect each target before retrying.

(manage-access-at-the-controller-model-application-or-offer-level)=
(change-a-users-controller-access)=
### Change a user's controller access

```{ibnote}
See also: {ref}`Changing controller access <changing-controller-access>`
```

To grant the external user `riley@example.com` login on `operations`, run the grant as a {ref}`controller superuser <user-access-controller-superuser>` and omit a positional target:

```text
juju grant -c operations riley@example.com login
```

```{ibnote}
See more: {ref}`juju grant <command-juju-grant>`
```

To reduce Riley's **existing direct controller superuser grant** to login, revoke `superuser` on that controller:

```text
juju revoke -c operations riley@example.com superuser
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

To let Alex add models on the `engineering` cloud, name that cloud after the level. Act as a {ref}`cloud administrator <user-access-cloud-admin>` on `engineering` or a {ref}`controller superuser <user-access-controller-superuser>`:

```text
juju grant-cloud -c operations alex add-model engineering
```

Alex may also need to {ref}`add a cloud credential <add-a-credential>` before creating a model. This grant does not let Alex assign access on an existing model.

```{ibnote}
See more: {ref}`juju grant-cloud <command-juju-grant-cloud>`
```

To withdraw Alex's direct grant on `engineering`, check that Alex holds a direct grant there and revoke the lowest cloud level:

```text
juju revoke-cloud -c operations alex add-model engineering
```

```{ibnote}
See more: {ref}`juju revoke-cloud <command-juju-revoke-cloud>`
```

(change-a-users-model-access)=
### Change a user's model access

```{ibnote}
See also: {ref}`Changing model access <changing-model-access>`
```

To let Alex change workloads in `payments`, use `grant` with the model name. Act as a {ref}`model administrator <user-access-model-admin>` on `payments` or a {ref}`controller superuser <user-access-controller-superuser>`:

```text
juju grant -c operations alex write payments
```

```{ibnote}
See more: {ref}`juju grant <command-juju-grant>`
```

To remove Alex's direct access to `payments`, check Alex's grant in the {ref}`model user list <list-users-of-a-model>`. If Alex is the {ref}`last active administrator <keeping-an-active-model-administrator>`, grant another enabled user model `admin` first. Revoke the lowest level:

```text
juju revoke -c operations alex read payments
```

To keep Alex as a reader when Alex directly holds `write`, revoke `write`:

```text
juju revoke -c operations alex write payments
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

To set or change your own local password on `operations`, including for the bootstrap user `admin`, run:

```text
juju change-user-password -c operations
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

To give Alex a fresh invitation, sign in as a different controller superuser. Confirm Alex is the intended user: resetting the password replaces both the old password and any outstanding activation key.

```text
juju change-user-password -c operations --reset alex
```

Pass the new registration command privately to Alex to {ref}`activate the local user <activate-a-local-user>` again. With `--reset` and your own username, or without a target username, the CLI tells you to change your own password and sends no reset.

```{ibnote}
See more: {ref}`juju change-user-password <command-juju-change-user-password>`
```

(manage-a-user-s-login-status)=
## Manage a user's login status

```{ibnote}
See also: {ref}`The CLI account for a controller <cli-accounts-by-controller>`
```

One client can be signed in to different controllers under different usernames. To switch users **on the same controller**, sign out the saved user first.

(sign-in-as-a-user)=
### Sign in as a user

```{ibnote}
See also: {ref}`Types of users <types-of-users>`; {ref}`Controller access levels <controller-access-levels>`
```

To sign in as local user `alex` on the registered controller `operations`, run:

```text
juju login -c operations --user alex
```

Enter Alex's password when prompted. On a controller configured for OpenID Connect, omit `--user` and follow the identity provider's sign-in flow. For a local non-interactive login, `cannot deduce user, please pass --user with --no-prompt` means the CLI needs the local username explicitly. If a public controller presents an unverified certificate, confirm its fingerprint with the administrator before accepting it; don't enable unattended trust merely to bypass the prompt.

```{ibnote}
See more: {ref}`juju login <command-juju-login>`
```

(sign-out-as-a-user)=
### Sign out as a user

```{ibnote}
See also: {ref}`The CLI account for a controller <cli-accounts-by-controller>`
```

To sign this client out of `operations` while keeping it registered, run:

```text
juju logout -c operations
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

To suspend Alex's sign-in on `operations`, sign in as a {ref}`controller superuser <user-access-controller-superuser>` and check the models Alex administers. If Alex is a model's {ref}`last active administrator <keeping-an-active-model-administrator>`, {ref}`grant another enabled user model admin access <change-a-users-model-access>` on that model. Then disable Alex:

```text
juju disable-user -c operations alex
```

If the controller reports that `model(s) would be left without an admin user`, make that replacement grant on every affected model before retrying. As a controller superuser, you can {ref}`list disabled users <list-users-on-a-controller>` with `--all` or look up Alex by name.

```{ibnote}
See more: {ref}`juju disable-user <command-juju-disable-user>`
```

(re-enable-a-user)=
### Re-enable a user

```{ibnote}
See also: {ref}`Disabling and re-enabling a user <disabling-and-re-enabling-a-user>`
```

To restore Alex's sign-in on `operations`, sign in as a {ref}`controller superuser <user-access-controller-superuser>` and check that Alex's retained grants are still appropriate. Then re-enable Alex:

```text
juju enable-user -c operations alex
```

```{ibnote}
See more: {ref}`juju enable-user <command-juju-enable-user>`
```

(remove-a-user)=
## Remove a user

```{ibnote}
See also: {ref}`Removing a user <removing-a-user>`
```

To remove Alex permanently, sign in as a {ref}`controller superuser <user-access-controller-superuser>` on `operations`, verify the account and arrange the handover before confirming removal:

1. If the suspension is temporary, {ref}`disable Alex <disable-a-user>` instead. The built-in controller owner `admin` cannot be removed.
2. Give a different enabled user an explicit {ref}`model admin grant <change-a-users-model-access>` on every model that would otherwise fail the {ref}`model administrator check <keeping-an-active-model-administrator>`.
3. Identify models using Alex's cloud credentials. Have their administrators switch each model to a replacement with {ref}`juju set-credential <command-juju-set-credential>`. If Alex's old controller credential is to be retired, have Alex run {ref}`juju remove-credential <command-juju-remove-credential>` for the named cloud and credential with `-c operations` while still able to sign in. Running that command from your account targets your credential, not Alex's.

```{warning}
Removal clears Alex's sign-in material, direct grants and registered SSH keys on this controller. Adding the same username later creates a new user; it won't restore Alex's previous access.
```

To remove the verified account, run:

```text
juju remove-user -c operations alex
```

Confirm the username and controller at the prompt, then answer `y` or `yes`. Skipping confirmation with `-y` does not bypass the controller's checks.

```{ibnote}
See more: {ref}`juju remove-user <command-juju-remove-user>`
```

