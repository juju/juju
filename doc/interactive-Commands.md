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

