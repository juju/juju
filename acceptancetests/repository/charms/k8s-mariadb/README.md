# Overview

mysql for Kubernetes

# Usage

You must specify key configuration attributes when deploying,
or else arbitary defaults will be used. The attributes which
should be set are:
- user
- password
- database
- root_password

eg

$ juju deploy mysql \
&nbsp;&nbsp;&nbsp;&nbsp;--config user=fred \
&nbsp;&nbsp;&nbsp;&nbsp;--config password=secret \
&nbsp;&nbsp;&nbsp;&nbsp;--config database=test \
&nbsp;&nbsp;&nbsp;&nbsp;--config root_password=admin

These values may also be in a config.yaml file, eg

$ juju deploy mysql --config config.yaml

