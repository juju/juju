---
myst:
  html_meta:
    description: "Juju annotation reference: the key and value pairs that a client attaches to a model, an application, a charm, a machine, a unit, or a storage instance, and the calls that set and read them."
---

(annotation)=
# Annotation

An **annotation** is a key and value that a {ref}`client <client>` attaches to an entity of a {ref}`model <model>`, to hold information of its own: a ticket number, an owner, a note. The model database keeps one set of annotation records for each kind of entity that carries them. Setting and reading them needs access to the model. The values reach no part of Juju: nothing in the controller or the agents reads them.

(annotation-database-schema)=
## Annotation database schema

The annotation database schema is the set of records that the model database keeps about annotations. Every record has the same shape: one row per key, with its value. The records, with the rules that apply to each, are:

- **Annotation:** One key and value of one entity. The entities that carry annotations are the {ref}`model <model>` itself, an {ref}`application <application>`, a {ref}`charm <charm>`, a {ref}`machine <machine>`, a {ref}`unit <unit>`, and a storage instance of the model ({ref}`storage <storage>`). A model that arrived from another controller can also carry them for a storage volume or a storage filesystem; a client sets and reads the annotations of a storage instance only. The model database keeps the model's own annotations without an entity key, because the database is the model's.
  - *Rules:*
    - An entity holds each key once.
    - The value is always present.
    - The charm and unit records are the only ones the database links to their entities; the application, machine, and storage records name their entity by UUID with no link. The removal job of a unit or an application also clears its annotations; a removed machine or storage instance leaves its annotations in place.

(annotation-calls)=
## Annotation calls

Annotation calls are the calls a {ref}`client <client>` can take on annotations through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): set them and read them. The entity a request names is a tag, such as `application-mysql`, or a charm's URL after `charm-`.

(annotation-setting)=
### Setting annotations

Setting annotations attaches key and value pairs to entities, one result per entity. A key that the request leaves out keeps its value; a key with an empty value is removed.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. No block applies.
- *Rules:*
  - The call takes a list of entities, each with its pairs. Each entity in the list gets its own result.
  - A key holds no period.
  - A key is not empty and holds no whitespace only.
- *Errors:* The client receives each in the result for the entity, with the entity's tag in the text:
  - **`setting annotations for "<tag>": key "<key>" contains period`**: *Trigger:* The key holds a period. The client receives it with no error code. *Remediation:* Use a key with no period.
  - **`setting annotations for "<tag>": key is empty string`**: *Trigger:* A key is empty or whitespace only. The client receives it with no error code. *Remediation:* Name each key.
  - **`setting annotations for "<tag>": unknown kind "<kind>"`**: *Trigger:* The tag names an entity kind that carries no annotations, such as a controller. The client receives it with no error code. *Remediation:* Name a model, application, charm, machine, unit, or storage instance.
  - **`setting annotations for "charm-<URL>": <reason>`**: *Trigger:* The charm URL cannot be parsed. The client receives it with no error code. *Remediation:* Send the charm's URL.

(annotation-reading)=
### Reading annotations

Reading annotations returns the key and value pairs of entities, one result per entity.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The call takes a list of entities. Each entity in the list gets its own result.
  - An entity with no annotations returns an empty set.
- *Errors:* The client receives each in the result for the entity:
  - **`getting annotations for "<tag>": unknown kind "<kind>"`**: *Trigger:* The tag names an entity kind that carries no annotations. The client receives it with no error code. *Remediation:* Name a model, application, charm, machine, unit, or storage instance.
  - **`getting annotations for "charm-<URL>": <reason>`**: *Trigger:* The charm URL cannot be parsed. The client receives it with no error code. *Remediation:* Send the charm's URL.

(annotation-and-model-migration)=
## Annotations and model migration

A migration carries the annotations of every entity with the model to the target controller, in the import that recreates the model. See {ref}`Migrating models <model-migrating>`.
