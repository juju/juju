(charm-naming-guidelines)=
# Charm naming guidelines

> See also: {ref}`Charm publication checklist <stage-1-important-qualities>`

When you create your charm, an important decision is how to name it. Below are some recommendations.

<!--
Here we are: guidelines (and requirements) for naming Charms, filling out maintainer lists and naming repositories - it is collected feedback input from various charmers and juju developers!

Naming is important for ensuring an intuitive discovery and handling of Charms - good names can be easily searched for and accessed when working with command line tools.
-->

## Naming charms

Of course, there is a large number of existing charms already with all their variants on naming. Please check, if naming of existing Charms can be improved, it will help others to find your Charms. But more importantly, consider the following points for new charming efforts:

| Required / Recommended                                                   | Background                                                                                              | Examples                                                                                                    |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| Slug-oriented naming required: ASCII lowercase letters, numbers, and hyphens. | Ensure URL-safeness,<br>finding charms easier.                                                             |   :stop_sign: `node_red`<br> :white_check_mark: `node-red`                                                                                       |
| Do not abbreviate application names.                                          | Finding charms easier.                                                                                     | :stop_sign: `matterm` <br> :white_check_mark: `mattermost`                                                                                       |
| Do not consider camelcase naming.                                             | Think of slugs, also not supported.                                                                                         |:stop_sign: `NodeRed` <br> :white_check_mark: `node-red`      |
| No charm or operator suffix                                                   | It is about Charms<br> **Exception**: Bundles need to be distinguished from the Charm.                                                                 | :stop_sign: `ghost-operator`<br>:white_check_mark: `ghost`<br>:stop_sign: `grafana-charm`<br>:white_check_mark: `grafana`<br>*However:*<br>:white_check_mark: `discourse`<br>:white_check_mark: `discourse-bundle` |
| No organisation as prefix                                                     | Hard to have it consistently across all charms,<br>Belongs to `charmcraft.yaml`                             | :stop_sign: apache-kafka<br>:white_check_mark: `kafka`<br>:stop_sign: `oracle-mysql`<br>:white_check_mark: `mysql`                                                            |
| No publisher name as prefix to the charm name.                                | Hard to have it consistently across all charms,<br>Belongs to `charmcraft.yaml`                             | :stop_sign: `mysql-charmers-mysql-server`<br>:white_check_mark: `mysql`<br>:stop_sign: `charmteam-kafka`<br>:white_check_mark: `kafka` |
| Avoid the purpose, use the name of the application instead.                   | Multiple charms for same purpose exist or will come.                                                    | :stop_sign: `general-cache`<br> :white_check_mark: `varnish` *(if the varnish OSS project represents the application)*                            |
| Do not use non-production identifiers, for public.                            | Not useful for the community of Charmhub users.                                                         | :stop_sign: `jenkins-test`<br> :stop_sign: `mysql-draft`<br>:stop_sign: `hadoop-internal`<br>:stop_sign: `kafka-demo`                                                      |
| Single charms: Avoid mix of app name and purpose.                             | Suffixes or prefixes shall be omitted and can be part of summary.                                       | :stop_sign: `drupal-server`<br>:white_check_mark: `drupal`<br>*However:*<br>:white_check_mark: `kubeflow-dashboard`<br>:white_check_mark: `kubeflow-pipeline`<br>:white_check_mark: `kubeflow-volumes` |
| Multiple charms: Put the name before the function.                            | Sorting.                                                                                                | :stop_sign: `pipeline-kubeflow`<br>:white_check_mark: `kubeflow-pipeline`                                                                      |
| Explain short names if used.                                                          | Three letter acronyms: prone to be confused.                                                            |                                                                                                             |
| Exception: Use “-k8s” for kubernetes only charms                              | In general, do not use a suffix for platforms. Only a) for “kubernetes only” charms and b) another non-k8s-charm is around, use the suffix "-k8s” to distinguish the k8s-charm from the other. For a charm covering a workload that makes only sense for k8s and nowhere else, avoid the suffix. | :white_check_mark: `graylog-k8s`                                                                                                 |

## Maintainer contact

Currently, the `charmcraft.yaml` supports listing the maintainer of the charm:

`A list of maintainers in the format "First Last <email>`

The naming of the maintainer has three main goals:
* Identify the right person to contact
* Visibility (for merits) of the publisher(s) or author(s) of the Charm
* Transparent responsibility for the published software. However, please note that the maintainer list does not necessarily denote the responsible party (copyright holder) of the published Charm implementation.

The maintainer field in `charmcraft.yaml` must be filled out, either with

1. ```{tip}
A maintaining individual contact
```
2. ```{tip}
Multiple maintaining individual contacts if that applies *(preferred)*,
```
3. ```{tip}
The maintaining organisation if that applies.
```
4. ```{note}
Do not use fantasy / virtual team names as feedback has shown it causes confusion.
```

## Naming repositories

Charms shall have a publicly accessible (git) repository. Opposed to the charm listing on Charmhub.io, a distinguishing element in the name of a repository helps to identify the right repository with the Charm implementation.

The distinguishing element is the suffix “-operator”, because that aligns also with the names of the other parts in the Juju world, such as the [Charmed Operator Framework](https://juju.is/about) or the [Charmed Operator SDK](https://juju.is/docs/sdk). So, in summary, we have the following steps:

1. ```{tip}
Name the repository like your charm (see naming rules above)
```
2. ```{tip}
Add a suffix to it: -operator (because: Charmed Operator SDK)
```
3. ```{tip}
Prefer suffix over prefix (:stop_sign: `operator-mydatabaseserver`)
```
4. ```{caution}
Plural form only if there are in fact multiple charm implementations in the repository, do not mimic an (adverbial) genitive.
```

It is understood that many repositories already exist with the suffix `-charm` along to the repositories with `-operator`. Repository renaming can be hard in some cases. The naming guide defines the preferred naming. 

Of course, some organisations have their own naming conventions for their repositories which cannot be easily overruled.
