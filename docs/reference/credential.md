---
myst:
  html_meta:
    description: "How Juju identifies cloud credentials, uses them across models, and handles inspection, updates and revocation."
---

(credential)=
# Credential

```{ibnote}
See also: {ref}`Juju CLI | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

```{ggarch}
:file: ../juju.ggarch
:view: credential-bindings
:caption: Several models can use one controller credential. Here the payments and billing models share it; the client copy can be uploaded separately.
:alt: A client copy can be uploaded as a controller credential. The same controller credential is used by both the payments model and the billing model.
```

In Juju, a **credential** holds the authentication material Juju uses to access a {ref}`cloud <cloud>` on behalf of a {ref}`user <user>`. Several {ref}`models <model>` can use the same {ref}`controller copy <client-vs-controller-credential>`. {ref}`Updating that credential <updating-a-credential>` changes the material for all of them. To change just one model, {ref}`switch it to a different credential <changing-a-models-credential>`.

(credential-definition)=
## Credential definition

A credential has an authentication type and a set of attributes specific to that type. For example, an Amazon EC2 access-key credential contains an access key and a secret key. The {ref}`cloud reference <list-of-supported-clouds>` gives the authentication types and fields for each provider. Granting a user {ref}`cloud add-model access <user-access-cloud-add-model>` in Juju does not supply these values or grant permissions in the cloud account.

(credential-names-and-owners)=
## Credential names and owners

A controller identifies a credential by **cloud, owning user and credential name**. Two users can register different credentials called `production` on the same cloud. The cloud's region is not part of the credential name.

(client-vs-controller-credential)=
## Client vs. controller credential

A **client credential** is a copy stored by a Juju client. A **controller credential** is a copy held by a specific controller for its owning user. A named client credential used to bootstrap a controller is stored on that controller for the bootstrap administrator. After that, editing the client's copy does not update the controller's copy. Inspect the store that the operation will use.

(model-credentials)=
## Model credentials

A {ref}`model <model>` refers to at most one {ref}`controller credential <client-vs-controller-credential>` for its own cloud. A model can start without a credential on a cloud that supports empty authentication. {ref}`Forced revocation <revoking-a-credential>` can leave an existing model without a credential. {ref}`Changing a model's credential <changing-a-models-credential>` switches that model to another record.

(credential-validity)=
## Credential validity

A controller credential is **invalid** when Juju has marked it invalid, for example after the provider reports that it cannot use the credential. A model using that credential can report `suspended` with the reason “suspended since cloud credential is not valid”. Other models using the same credential share its validity state.

**Valid** means the credential is not marked invalid. It is not a live test that a cloud account can list instances, provision machines or create storage. A successful {ref}`update <updating-a-credential>` replaces the stored material and clears the invalid marker; a subsequent cloud operation can still fail.

(registering-a-credential)=
## Registering a credential

```{ibnote}
See also: {ref}`Juju CLI | Add a credential <add-a-credential>`, [Terraform Provider for Juju | Define a credential](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#define-a-credential)
```

Registering a credential stores its authentication type and attributes on the controller. Direct registration with an existing cloud, owner and name replaces the stored contents. When registering multiple credentials, check the {ref}`write results <updating-a-credential>` before retrying.

**Who can do this:** The credential owner or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Registration can fail with these errors:

- `permission denied`: The credential names a different owner and the caller is not a controller superuser. Register the credential under your account or ask a controller superuser to register it.
- Contains `supported auth-types`: The authentication type is not supported on that cloud. Check the registered cloud, choose a type it accepts, and provide that type's attributes.

(listing-credential-names)=
## Listing credential names

A client can request credential identifiers for one user on one cloud. No matches produce an empty list for that user and cloud. The {ref}`CLI credential list <command-juju-credentials>` reads the signed-in account's {ref}`controller credential contents <inspecting-your-credential-contents>`.

**Who can do this:** A user for their own credentials, or a {ref}`controller superuser <user-access-controller-superuser>` for another user.

**Errors:** Listing credential names can fail with this error:

- `permission denied`. The caller lacks access to list another owner's credentials. Request your own records or ask a controller superuser.

(reading-a-named-credential)=
## Reading a named credential

A client that knows a credential's tag (the API identifier for its cloud, owner and name) can read its authentication type and attributes. This read leaves out the provider's hidden fields and identifies them by name. To request hidden values for a credential you own, use {ref}`inspecting your credential contents <inspecting-your-credential-contents>`.

**Who can do this:** The credential owner or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** A named read distinguishes these failures:

- `permission denied`: The caller cannot inspect this owner's tag. Ask the owner to check the record.
- For example, `credential "production" not found`: An authorized lookup did not find the name. Check the cloud and owner as well as the credential name.

(inspecting-your-credential-contents)=
## Inspecting your credential contents

```{ibnote}
See also: {ref}`Juju CLI | View details about a credential <view-details-about-a-credential>`
```

A user can inspect all their controller credentials or select one by cloud and name. Each result includes the authentication type, provider-defined attributes and stored {ref}`credential validity <credential-validity>`. Hidden attributes are omitted unless the caller explicitly requests their values.

The result also identifies active models using the credential for which the owner has a model access grant, together with that access level. An empty model list does **not** establish that the credential is unused.

**Who can do this:** The authenticated user, for their own credentials. This restriction also applies to a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Inspecting credential contents can return these errors:

- Begins with `cloud credential ID` and ends with `not valid`. The cloud or credential name is empty. Supply both names for named inspection.
- Begins with `cloud credential` and ends with `not found` for a named result. The account has stored credentials, but none matches the requested cloud and name. Check the account and both names.
- Begins with `cloud credentials for` and ends with `not found` for the whole request. The account has no stored credentials, so the collection lookup failed before any named inspection. Verify which account is signed in.

(checking-a-credential)=
## Checking a credential

A client can submit proposed credential material for checking without storing it. The controller checks whether the cloud supports the proposed authentication type, then reports the models that refer to this credential's name. The check does not contact the cloud to verify that the proposed material grants access to those models' resources.

**Who can do this:** The credential owner or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Checking a credential can fail with these errors:

- `permission denied`. The caller lacks access to another owner's credential. Check your own credential or ask a controller superuser.
- Contains `supported auth-types`. The cloud rejects the proposed authentication type. Choose a type listed for that cloud and check again.

(updating-a-credential)=
## Updating a credential

```{ibnote}
See also: {ref}`Juju CLI | Update a credential <update-a-credential>`, [Terraform Provider for Juju | Update a credential](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#update-a-credential)
```

Updating a credential replaces its authentication type and **all** its attributes under the same cloud, owner and name. Omitted attributes are removed. If the credential does not yet exist, the controller can create it. All models bound to that credential use the replacement material. A {ref}`credential check <checking-a-credential>` or update can report model results alongside the credential result.

For requests to register, list by user and cloud, read by tag, check, update or revoke several entries, the controller reports one result per entry in request order. This also applies to named inspection once the account's credentials are loaded. An entry failure leaves successful changes in place and processing continues. Check each result before retrying. A request-level refusal can stop processing before any entry is handled.

A forced update can omit a write error from its reply. Inspect the stored credential before relying on the result.

**Who can do this:** The credential owner or a {ref}`controller superuser <user-access-controller-superuser>` can update a credential. Only a controller superuser may force the update.

**Errors:** Updating a credential can fail with these errors:

- Contains `supported auth-types` during an ordinary update. The cloud rejects the supplied authentication type. Supply a supported type and its attributes.
- Begins with `unexpected force specified`. The controller refuses the whole request when the caller asks for force without controller superuser access. Retry with an ordinary update.
- `permission denied`. The caller lacks access to another owner's credential. Update your own credential or ask a controller superuser.

(revoking-a-credential)=
## Revoking a credential

```{ibnote}
See also: {ref}`Juju CLI | Remove a credential <remove-a-credential>`, [Terraform Provider for Juju | Remove a credential](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#remove-a-credential)
```

```{ggarch}
:file: ../juju.ggarch
:view: credential-revocation
:caption: To retire a credential that models still use, switch them to a replacement first. Here a model administrator switches the last model, then the credential owner removes the old credential.
:alt: A model administrator switches the last dependent model to another credential before the credential owner revokes the old one.
```

Ordinary revocation removes an unused credential from this controller. If a model still uses it, a model administrator must {ref}`switch that model to a replacement <changing-a-models-credential>` before ordinary revocation can succeed. Removing the controller copy does not revoke a provider-issued key or token or remove a client's copy.

```{warning}
Forced revocation clears the credential association from **every model** that still uses it and deletes the credential. The controller does not select replacements.
```

**Who can do this:** The credential owner or a {ref}`controller superuser <user-access-controller-superuser>`, including for forced revocation.

**Errors:** Revocation can fail with these errors:

- Contains `still used by`. At least one model still refers to the credential. Arrange to {ref}`switch each dependent model to a replacement <changing-a-models-credential>` for the same cloud, then retry ordinary revocation.
- Begins with `credential` and ends with `not found`. The named credential is absent from this controller. Check the cloud, owner and name.
- `permission denied`. The caller lacks access to another owner's credential. Ask the owner or a controller superuser to revoke it.
