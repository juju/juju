(manage-ssh-keys)=
# How to manage SSH keys

> See also: {ref}`ssh-key`

(add-an-ssh-key)=
## Add an SSH key

To add a public `ssh` key to a model, use the `add-ssh-key` command followed by a string containing the entire key or an equivalent shell formula:

```text

# Use the entire ssh key:
juju add-ssh-key "ssh-rsa qYfS5LieM79HIOr535ret6xy
AAAAB3NzaC1yc2EAAAADAQA6fgBAAABAQCygc6Rc9XgHdhQqTJ
Wsoj+I3xGrOtk21xYtKijnhkGqItAHmrE5+VH6PY1rVIUXhpTg
pSkJsHLmhE29OhIpt6yr8vQSOChqYfS5LieM79HIOJEgJEzIqC
52rCYXLvr/BVkd6yr4IoM1vpb/n6u9o8v1a0VUGfc/J6tQAcPR
ExzjZUVsfjj8HdLtcFq4JLYC41miiJtHw4b3qYu7qm3vh4eCiK
1LqLncXnBCJfjj0pADXaL5OQ9dmD3aCbi8KFyOEs3UumPosgmh
VCAfjjHObWHwNQ/ZU2KrX1/lv/+lBChx2tJliqQpyYMiA3nrtS
jfqQgZfjVF5vz8LESQbGc6+vLcXZ9KQpuYDt joe@ubuntu"


# Use an equivalent shell formula:
juju add-ssh-key "$(cat ~/mykey.pub)"

```

<!--SAW THIS SOMEWHERE ELSE. THIS IS SUPPOSED TO BE THE DEFAULT USER FOR A JUJU MACHINE. BUT WHICH JUJU MACHINE ARE WE TALKING ABOUT NOW? WE JUST SAID WE'RE ADDING THIS TO THE MODEL.

This will add the SSH key to the default user account named 'ubuntu'.
-->

> See more: {ref}`command-juju-add-ssh-key`


## Import an SSH key

To import a public SSH key from Launchpad / Github to a model, use the `import-ssh-key` command followed by `lp:` / `gh:` and the name of the user account. For example, the code below imports all the public keys associated with the Github user account ‘phamilton’:

```text
juju import-ssh-key gh:phamilton
```

<!--SAW THIS SOMEWHERE ELSE. THIS IS SUPPOSED TO BE THE DEFAULT USER FOR A JUJU MACHINE. BUT WHICH JUJU MACHINE ARE WE TALKING ABOUT NOW? WE JUST SAID WE'RE ADDING THIS TO THE MODEL.

This will add the SSH key to the default user account named 'ubuntu'.
-->

> See more: {ref}`command-juju-import-ssh-key`

## View the available SSH keys

To list the currently known SSH keys for the current model, use the `ssh-keys` command.

```text
# List the keys known in the current model
juju ssh-keys
```

If you want to get more details, or get this information for a different model, use the `--full` or the `--model / -m <model name>` option.

<!--# List the keys known in the 'jujutest' model
juju ssh-keys -m jujutest --full
-->

> See more: {ref}`command-juju-ssh-keys`


## Remove an SSH key

To remove an SSH key, use the `remove-ssh-key` command followed by the key / a space-separated list of keys. The keys may be specified by either their fingerprint or the text label associated with them. The example below illustrates both:

```text
juju remove-ssh-key 45:7f:33:2c:10:4e:6c:14:e3:a1:a4:c8:b2:e1:34:b4 bob@ubuntu
```

> See more: {ref}`command-juju-remove-ssh-key`

