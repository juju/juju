(plugin-juju-stash)=
# Plugin `juju-stash`

Switching between models and controllers can be laborious, as you have to remember what the previous model or controller name was. I suggested on IRC that `juju switch` should have `-`, similar to `git checkout -` or `cd -`. Rather than giving Juju the ability to remember, make a plugin and you can do what you want...

[`juju stash`](https://github.com/SimonRichardson/juju-stash) is a {ref}`plugin <plugin>` for Juju, which allows you to jump between models as if you have a stack; pushing and popping between models.

To switch to a model:
```text
juju stash push modelB
```

To switch back to the previous model:
```text
juju stash pop
```

To see what's in your history:
```text
juju stash list
```

If you want to ping-pong between models, you can do the following:
```text
juju stash pop --store
```
This will store the popped model into the history. Calling it multiple times will mean you can ping-pong between `modelA` and `modelB` with out having to remember their names.

Supplying `--status` along with the `pop` command will also dump the `juju status` into the stdout so you can keep track of what's happening where!


