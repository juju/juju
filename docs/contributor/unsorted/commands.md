(commands)=
# Commands


The base `Command` interface is found in `cmd/cmd.go`.

Commands need to provide an `Info` method that returns an Info struct.

The info struct contains: name, args, purpose and a detailed description.
This information is used to provide the default help for the command.

In the same package, there is `CommandBase` whose purpose is to be composed
into new commands, and provides a default no-op SetFlags implementation, a
default Init method that checks for no extra args, and a default Help method.


## Supercommands

`Supercommand`s are commands that do many things, and have "sub-commands" that
provide this functionality.  Git and Bazaar are common examples of
"supercommands".  Subcommands must also provide the `Command` interface, and
are registered using the `Register` method.  The name and aliases are
registered with the supercommand.  If there is a duplicate name registered,
the whole thing panics.

Supercommands need to be created with the `NewSuperCommand` function in order
to provide a fully constructed object.

## The 'help' subcommand

All supercommand instances get a help command.  This provides the basic help
functionality to get all the registered commands, with the addition of also
being able to provide non-command help topics which can be added.

Help topics have a `name` which is what is matched from the command line, a
`short` one line description that is shown when `<cmd> help` is called,
and a `long` text that is output when the topic is requested.


## Execution

The `Main` method in the cmd package handles the execution of a command.

A new `gnuflag.FlagSet` is created and passed to the command in `SetFlags`.
This is for the command to register the flags that it knows how to handle.

The args are then parsed, and passed through to the `Init` method for the
command to decide what to do with the positional arguments.

The command is then `Run` and passed in an execution `Context` that defines
the standard input and output streams, and has the current working directory.


(interactive-commands)=
## Interactive commands
**this page is a WIP and is subject to change**

Some commands in Juju are interactive. To keep the UX consistent across the product, please follow these guidelines.
Note, many of these guidelines are supported by the interact package at github.com/juju/juju/cmd/juju/interact.

* All interactive commands should begin with a short blurb telling the user that they've started an interactive wizard.
  `Starting interactive bootstrap process.`
* Prompts should be a short imperative sentence with the first letter capitalized, ending with a colon and a space
  before waiting for user input.
  `Enter your mother's maiden name: `
* Prompts should tell the user what to do, not ask them questions.
  `Enter a name: ` rather than `What name do you want?`
* The only time a prompt should end with a question mark is for yes/no questions.
  `Use foobar as network? (Y/n): `
* Yes/no questions should always end with (y/n), with the default answer capitalized.
* Try to always format the question so you can make yes the default.
* Prompts that request the user choose from a list of options should start `Select a ...`
* Prompts that request the user enter text not from a list should start `Enter ....`
* Most prompts should have a reasonable default, shown in brackets just before the colon, which can be accepted by just
  hitting enter.
  `Select a cloud [localhost]: `
* Questions that have a list of answers should print out those options before the prompt.
* Options should be consistently sorted in a logical manner (usually alphabetical).
* Options should be a single short word, if possible.
* Selecting from a list of options should always be case insensitive.
* If an incorrect selection is entered, print a short error and reprint the prompt, but do not reprint the list of
  options. No blank line should be printed between the error and the corresponding prompt.
* Always print a blank line between prompts.

Sample:

```
$ juju bootstrap --upload-tools
Starting interactive bootstrap process.

Cloud        Type
aws          ec2
aws-china    ec2
aws-gov      ec2
azure        azure
azure-china  azure
cloudsigma   cloudsigma
google       gce
joyent       joyent
localhost    lxd
rackspace    rackspace

Select a cloud by name [localhost]: goggle
Invalid cloud.
Select a cloud by name [localhost]: google

Regions in google:
asia-east1
europe-west1
us-central1
us-east1

Select a region in google [us-east1]:

Enter a name for the Controller [google-us-east1]: my-google

Creating Juju controller "my-google" on google/us-east1
Bootstrapping model "controller"
Starting new instance for initial controller
Launching instance
[...]
```

