# `audit-log-exclude-methods`

This document describes the `audit-log-exclude-methods` controller configuration key.

|Key|Type|Default|Valid values|Purpose|
|---|---|---|---|---|
|`audit-log-exclude-methods` [RT]|string|ReadOnlyMethods|[Some.Method,...]|The list of Facade.Method names that aren't interesting for audit logging purposes.|

The `audit-log-exclude-methods`  is used to exclude information (API calls/methods) from the audit log.  

The default value of key `audit-log-exclude-methods` is the special value of 'ReadOnlyMethods'. As the name suggests, this represents all read-only events.

<!--Click the heading below to reveal a listing of API methods designated by the key value of 'ReadOnlyMethods'.-->

```{dropdown} Expand to see the contents of `ReadOnlyMethods`

```
Action.Actions
Action.ApplicationsCharmsActions
Action.FindActionsByNames
Action.FindActionTagsByPrefix
Application.GetConstraints
ApplicationOffers.ApplicationOffers
Backups.Info
Client.FullStatus
Client.GetModelConstraints
Client.StatusHistory
Controller.AllModels
Controller.ControllerConfig
Controller.GetControllerAccess
Controller.ModelConfig
Controller.ModelStatus
MetricsDebug.GetMetrics
ModelConfig.ModelGet
ModelManager.ModelInfo
ModelManager.ModelDefaults
Pinger.Ping
UserManager.UserInfo
Action.ListAll
Action.ListPending
Action.ListRunning
Action.ListComplete
ApplicationOffers.ListApplicationOffers
Backups.List
Block.List
Charms.List
Controller.ListBlockedModels
FirewallRules.ListFirewallRules
ImageManager.ListImages
ImageMetadata.List
KeyManager.ListKeys
ModelManager.ListModels
ModelManager.ListModelSummaries
Payloads.List
PayloadsHookContext.List
Resources.ListResources
ResourcesHookContext.ListResources
Spaces.ListSpaces
Storage.ListStorageDetails
Storage.ListPools
Storage.ListVolumes
Storage.ListFilesystems
Subnets.ListSubnets
```

```


The recommended approach for configuring the filter is to view the log and make a list of those calls deemed undesirable. For example, to remove the following log message:

``` text
"request-id":4428,"when":"2018-02-12T20:03:45Z","facade":"Pinger","method":"Ping","version":1}}
```

we provide a `facade.method` of 'Pinger.Ping', while keeping the default value described above, in this way:

``` text
juju model-config -m controller audit-log-exclude-methods=[ReadOnlyMethods,Pinger.Ping]
```

```{note}

Only those Conversations whose methods have *all* been excluded will be omitted. For instance, assuming a default filter of 'ReadOnlyMethods', if a Conversation contains several read-only events and a single write event then all these events will appear in the log. A Conversation is a collection of API methods associated with a single top-level CLI command.

```

There is no definitive API call list available in this documentation.
