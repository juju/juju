(revision)=
# Charm revision

In the context of a charm, a **revision** is a number that uniquely identifies the published charm.

The revision increases with every new version of the charm being published. 

```{caution}

This can lead to situations of mismatch between the semantic version of a charm and its revision number. That is, whether the changes you make to the charm are for a semantically newer or older version, the revision number always goes up.

```

Charm revisions are not published for anybody else until you release them into a {ref}`channel <channel>`. Once you release them, though, users will be able to see them at `charmhub.io/<charm/channel>` or access them via `juju info <charm>` or `juju deploy <charm>`. 


<!--
 For example, revision `100` of the MongoDB charm has been released to `3.6/edge`, `3.6/candidate`, and `3.6/stable, so a user can see it on Charmhub ([MongoDB channel `5/edge`](https://charmhub.io/mongodb?channel=5/edge)), inspect it via `juju info mongodb` (output below), or deploy it via `juju deploy mongodb --channel 5/edge`.

```text
channels: |
  5/stable:       117  2023-04-20  (117)  12MB  amd64  ubuntu@22.04
  5/candidate:    117  2023-04-20  (117)  12MB  amd64  ubuntu@22.04
  5/beta:         ↑
  5/edge:         118  2023-05-03  (118)  13MB   amd64  ubuntu@22.04
  3.6/stable:     100  2023-04-28  (100)  860kB  amd64  ubuntu@20.04, ubuntu@18.04
  3.6/candidate:  100  2023-04-13  (100)  860kB  amd64  ubuntu@20.04, ubuntu@18.04
  3.6/beta:       ↑
  3.6/edge:       100  2023-02-03  (100)  860kB  amd64  ubuntu@20.04, ubuntu@18.04
```
-->
