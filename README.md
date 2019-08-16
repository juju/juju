![](doc/juju-logo.png?raw=true)

## Why Juju?

* Reduce complexity  
* Enable repeatability  
* Codify operations knowledge  
* Simplify day two 
* Maintain portability

If your infrastructure can’t be understood by everyone in your organisation, there’s an issue.
Juju focuses on the applications that your software model defines and how they are related.

Requiring everyone to know every hostname, every machine, every subnet and every storage volume is brittle.
This means change is complicated, on-boarding is difficult and tends to create knowledge silos.
Juju makes those details available, but places the software model at the front.

With Juju, your team maintains a practical high-level view that makes your backend more adaptable to changes over time. 
Extending your product should be as simple as deploying its first prototype.


## What is Juju?

Juju is a devops tool that reduces operational complexity through application modelling.
Once a model is described, Juju identifies the necessary steps to put that plan into action.

Juju has three core concepts: models, applications and relations.
Consider a whiteboard drawing of your service.
The whiteboard's border is the model, its boxes are applications and the lines between the boxes are relations. 

Juju uses an active agent deployed alongside your applications.
That agent orchestrates infrastructure, manages applications through the product life cycle.
The agent’s capabilities are confined by user permissions and all communication is fully encrypted.


## Next steps

Read the documentation https://jaas.ai/docs

Ask a question https://discourse.jujucharms.com/

Install Juju https://jaas.ai/docs/installing