// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const JujuControllers = `

A Juju Controller, also sometimes called the controller model, describes
the model that runs and manages the Juju API servers and the underlying
database.

The controller model is what is created when the bootstrap command is
called.  This controller model is a normal Juju model that just
happens to have machines that manage Juju.

In order to keep a clean separation of concerns, it is considered best
practice to create additional models for real workload deployment.

Services can still be deployed to the controller model, but it is
generally expected that these services are more for management and monitoring
purposes, like landscape and nagios.

When creating a Juju controller that is going to be used by more than one
person, it is good practice to create users for each individual that will be
accessing the models.

Users are managed within the Juju controller using the 'juju user' command.
This allows the creation, listing, and disabling of users. When a juju
controller is initially bootstrapped, there is only one user.  Additional
users are created as follows:

    $ juju add-user bob "Bob Brown"
    user "Bob Brown (bob)" added
    server file written to /current/working/directory/bob.server

This command will create a local file "bob.server". The name of the file is
customisable using the --output option on the command. This 'server file'
should then be sent to Bob. Bob can then use this file to login to the Juju
controller.

The 'server file' contains everything that Juju needs to connect to the API
server of the Juju Controller. It has the network address, server certificate,
username and a randomly generated password.

Juju needs to have a name for the controller when Bob calls the login command.
This is used to identify the controller by name for other commands, like
switch.

When Bob logs in to the controller, a different random password is generated
and cached locally. This does mean that this particular server file is not
usable a second time. If Bob does not want to change the password, he can use
the --keep-password flag.

    $ juju login --server=bob.server staging
    cached connection details as controller "staging"
    -> staging (controller)

Bob can then list all the models within that controller that he has
access to:

    $ juju list-model
    NAME  OWNER  LAST CONNECTION

The list could well be empty. Bob can create an model to use:

    $ juju create-model test
    created model "test"
    staging (controller) -> test

When the model has been created, it becomes the current model. A
new model has no machines, and no services.

    $ juju status
    model: test
    machines: {}
    services: {}

Bob wants to collaborate with Mary on this model. A user for Mary needs
to exist in the controller before Bob is able to share the model with her.

    $ juju share-model mary
    ERROR could not share model: user "mary" does not exist locally: user "mary" not found

Bob gets the controller administrator to add a user for Mary, and then shares the
model with Mary.

    $ juju share-model mary
    $ juju list-shares
    NAME        DATE CREATED    LAST CONNECTION
    bob@local   5 minutes ago   just now
    mary@local  57 seconds ago  never connected

When Mary has used her credentials to connect to the juju controller, she can see
Bob's model.

    $ juju list-models
    NAME  OWNER      LAST CONNECTION
    test  bob@local  never connected

Mary can use this model.

    $ juju use-model test
    mary-server (controller) -> bob-test

The local name for the model is by default 'owner-name', so since this
model is owned by 'bob@local' and called test, for mary the model
is called 'bob-test'.

`
