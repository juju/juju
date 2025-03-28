(scaling)=
# Scaling

> See also: {ref}`scale-an-application`

In the context of a cloud deployment in general, **scaling**  means modifying the amount of resources thrown at an application, which can be done *vertically* (modifying the memory, CPU, or disk for a cloud resource) or *horizontally* (modifying the number of resources), where each can be *up* (more) or down (*less*). In the context of Juju, scaling means exactly the same, with the mention that 

- Vertical scaling is handled through {ref}`constraints <constraint>` and horizontal scaling through {ref}`units <unit>`. 
- Horizontal scaling up can be used to achieve {ref}`high availability (HA) <high-availability>` -- though, depending on whether the charm delivering the application supports HA natively or not, you may also have to perform additional steps.


