(command-juju-agree)=
# `juju agree`
> See also: [agreements](#agreements)

## Summary
Agree to terms.

## Usage
```juju agree [options] <term>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--yes` | false | Agree to terms non interactively |

## Examples

Displays terms for somePlan revision 1 and prompts for agreement:

    juju agree somePlan/1

Displays the terms for revision 1 of somePlan, revision 2 of otherPlan, and prompts for agreement:

    juju agree somePlan/1 otherPlan/2

Agree to the terms without prompting:

    juju agree somePlan/1 otherPlan/2 --yes


## Details

Agree to the terms required by a charm.

When deploying a charm that requires agreement to terms, use 'juju agree' to
view the terms and agree to them. Then the charm may be deployed.

Once you have agreed to terms, you will not be prompted to view them again.