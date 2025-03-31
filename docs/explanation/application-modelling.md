(application-modelling)=
# Application modelling

> See also: {ref}`Application <application>`, {ref}`Model <model>`

Juju provides simplicity, stability and security. Models reduce the cognitive gap between the whiteboard picture of your service and how it is implemented. An application model is a definition of which applications are providing a service and how they inter-relate.

Technical details such as CPU core counts, disk write throughput, and IP addresses are secondary. They are accessible to administrators, but an application model places the applications at the front.

The primary function of a model is to enable you to maintain an uncluttered view of your service. Operational simplicity improves communication and understanding. Models provide an abstract view of the infrastructure that's hosting your service. 

More specifically, advantages include:

<a id="model-offer-isolation"></a>
### A. Service isolation

Juju models enforce service isolation. A model maintains exclusive access of the resources under its control.

<a id="model-offer-control"></a>
### B. Access control

Models provide access control. Juju enables you to create user accounts that have limited ability to alter the deployment. 

<a id="model-offer-repeat"></a>
### C. Repeatability 

Models provide repeatable infrastructure deployments. Once your model is in-place and functional, it becomes simple to export a model's definition as a bundle, then re-deploy that model in another host. 

<a id="model-offer-boundaries"></a>
### D. Boundaries 

Models respect bureaucratic boundaries. Models enable you to partition compute resources according to your internal guidelines. You may wish to keep a central set of databases in the same model. Juju's access controls are model-specific, enabling you to know exactly who has permissions to perform direct database administration. Those databases could be made available to various consuming applications from other models via relations (which can span models). Other use cases for central models include secrets (using the [vault charm](https://jaas.ai/vault/)) and identity management (using the [keystone charm](https://jaas.ai/keystone)).
