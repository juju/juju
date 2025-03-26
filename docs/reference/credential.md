(credential)=
# Credential

> See also: {ref}`manage-credentials`

In Juju, a **credential** represents a collection of authentication material (like username & password, or client id & secret key) that is specific to a Juju {ref}`user <user>` and a {ref}`cloud <cloud-substrate>` and allows that user to interact with that cloud. 

```{important}
In Juju a 'credential' always refers to authentication material used to access a *cloud*. 
```

Clouds can have one or more sets of credentials associated with them. 

When you create a  {ref}`model <model>` in Juju it must always be associated with a cloud/credential pair -- the model needs that to create resources on the underlying cloud. 

<!--DOUBLE-CHECK: Credentials are stored in .local/share/juju/credentials.yaml. You can verify this by running 
```text
cat .local/share/juju/credentials.yaml
```
-->


## Client vs. controller credential

Juju credentials can be created for either the Juju client or the Juju controller or both -- where a **client credential** (previously known as a 'local credential') denotes a credential that the client is aware of and a **controller credential** (previously known as a 'remote credential') denotes a credential that a controller is aware of. When you bootstrap a controller and use a client credential, this credential gets automatically uploaded to the controller, so it becomes a controller credential also. 

```{important}
The set of client credentials and controller credentials can end up being the same. However, they don't have to.
```

## Credential definition
> See also: {ref}`add-a-credential`

If your cloud is a built-in cloud (`microk8s` or `lxd` on your local machine), Juju will define and add your credential automatically. For all other clouds you will have to define and add your credential(s) yourself. Depending on the method you choose, you may need to be aware of the environment variables that you can use or the structure of the `credentials.yaml` file.

### Credential environment variables

Credential environment variables are not available for all the clouds and, when they are, they are cloud-specific. They are used by Juju to automatically populate your `credentials.yaml` file.

### File `credentials.yaml`

The `credentials.yaml` file is the file in your Juju installation where Juju stores your cloud credential definitions. 

This includes definitions that Juju has created for your (e.g., for the built-in `localhost` (LXD) and `microk8s` clouds) as well as any definitions you have provided yourself, whatever the chosen method. 


This file (on Linux usually located at `~/.local/share/juju/credentials.yaml`) has the following keys:


````{dropdown} Expand to view the schema all at once


> [Source](https://github.com/juju/juju/blob/ecd609d9e8700e87f630b6fb8c8b6690f211092d/cloud/credentials.go#L87)

```text
credentials:
  <cloud>:
    <credential>:
      auth-type: <auth type>
      <attribute>: <value>
      <attribute>: <value>
    ...
  <cloud>:
    <credential>:
      auth-type: <auth type>
      <attribute>: <value>
      <attribute>: <value>
    ...
    <credential>:
      auth-type: <auth type>
      <attribute>: <value>
      <attribute>: <value>
    ...
```

````

````{dropdown} Expand to view an example of five credentials being stored against four clouds

```text
credentials:
  aws:
    <credential>:
      auth-type: access-key
      access-key: <key>
      secret-key: <key>
  azure:
    <credential>:
      auth-type: service-principal-secret
      application-id: <uuid>
      application-password: <password>
      subscription-id: <uuid>
  lxd:
    <credential a>:
      auth-type: interactive
      trust-password: <password>
    <credential b>:
      auth-type: interactive
      trust-password: <password>
  google:
    <credential>:
      auth-type: oauth2
      project-id: <project-id>
      private-key: <private-key>
      client-email: <email>
      client-id: <client-id>
```

````

#### `credentials`


**Status:** Required.

**Purpose:** Holds the credentials for every cloud.

**Value:** Map from clouds to their associated credentials.

#### `credentials.<cloud>`


**Status:** Required.

**Purpose:** Holds the credentials for a particular cloud.

**Name:** The cloud name, as defined by Juju (built-in clouds or public clouds) or by you.

**Value:** Map from credentials to their attributes.

### `credentials.<cloud>.<credential>`

**Status:** Required.

**Purpose:** Holds the details for a particular credential.

**Name:** The credential name, as defined by Juju (built-in clouds) or by you. Must be unique within the cloud. :warning: Because there is no central way to constrain what credential names get used nor oversee what authentication material they represent, different devices can associate the same name with different material, or different names with the same material. This is something to keep in mind, especially in a multi-user context.


**Value:** Map from credential attributes (e.g., `auth-type`) to their values.

<!--
Every credential is born (created) from a specific Juju client, which, in turn, is bound to an independent computer host (“device”). There is thus no central way to constrain what credential names get used nor oversee what authentication material they represent. Because of this, different devices can associate the same name with different material, or the opposite, a different name with the same material. This is something to keep in mind, especially in a multi-user context.
-->


#### `credentials.<cloud>.<credential>.auth-type`

**Status:** Required.

**Purpose:** Holds the authentication type associated with this credential.

**Value:** Scalar denoting (one of) the authentication type(s) from your cloud definition. Cloud-specific. See more: {ref}`list-of-supported-clouds` > `<cloud name>`.

#### `credentials.<cloud>.<credential>.auth-type.<attribute>`

**Status:** Required.

**Purpose:** Holds information associated with a particular attribute of the authentication type (e.g., `subscription-id`). Authentication-type-specific. See more: {ref}`list-of-supported-clouds` > `<cloud name>`.

**Value:** Scalar denoting the value for the attribute.
