## Facades

A facade is a collection of API methods organised for a specific purpose. It's a pretty fundamental concept: our
versioning is at facade granularity, and a facade's most fundamental responsibility is to validate incoming API calls
and *ensure that they're sensible* before taking action.

### How is a Facade instantiated?

When an api request is received, the apiserver chooses a facade constructor, based on the request's rootName and
version; then constructs the facade and calls the method corresponding to the request's methodName. (Many details
elided.) Note that the facade is created for the use of, and persists only for the lifetime of, a single request.

### How do I write a Facade?

* define local use interfaces that expose the *capabilities* your facade requires
* write a simple constructor for your facade, most likely using
  the [config-struct pattern](howto-implement-effective-config-structs.md), and making
  sure to include an `apiserver/common.Authorizer`:

      package whatever

      // Backend exposes capabilities required by the whatever facade.
      //
      // It's an example of a locally defined interface that's fine.
      //
      // Like all good interfaces, it should be tightly focused on its
      // actual purpose and prefer not to include methods that won't be
      // used.
      type Backend interface {
          DoSomething(to Something) error
          // more..?
      }

      // Config holds the dependencies and parameters needed for a
      // whatever Facade.
      //
      // Keep in mind that this is *completely specific to a class of
      // client*: the internal `whatever` facade is intended for the
      // exclusive use of the `whatever` worker; and all external facades
      // are for the exclusive use of, uh, external users.
      //
      // That is to say: there probably won't be much to configure apart
      // from the direct dependencies. Anything else is part of the
      // fundamental character of the facade in play, and should probably
      // not be tweakable at all.
      type Config struct {

          // Authorizer represents the authenticated entity that you're 
          // responsible for granting or restricting access to. Possibly
          // "Authorisee" would have been a better name? Wouldn't object
          // if someone fixed it, I think it just came from the time when
          // Fooer was the default name for everything.
          Authorizer common.Authorizer

          // Backend exposes something-doing capabilities blah blah.
          Backend Backend

          // more..? many facades will want to access several distinct
          // backendy capabilities, consider carefully the granularity
          // at which you expose them. It's not a bad thing to pass in
          // the same object in several different fields, for example.
      }

      // Validate returns an error if any part of the Config is unfit for use.
      func (config Config) Validate() error {

          // This bit is significant! This is the moment at which you get to
          // choose who can use the facade at all. So a user-facing facade
          // would want to AuthClient(), and ErrPerm if false; an environment
          // management job would want to AuthEnvironManager(); or perhaps this
          // is just for some worker that runs on all machine agents, in which
          // case we do:
          if !config.Auth.AuthMachineAgent() {
              return common.ErrPerm
          }
          // ...and now, bingo, you know your methods will not be called without
          // the certainty that you're dealing with an authenticated machine agent.
          // Yay!

          // May as well check these too, it's better than panicking if someone
          // has messed up and passed us garbage.
          if config.Backend == nil {
              return errors.NotValidf("nil Backend")
          }
          return nil
      }

      // NewFacade creates a Facade according to the supplied configuration, or
      // returns an error. Note that it returns a concrete exported type, which
      // is right and proper for a constructor.
      func NewFacade(config Config) (*Facade, error) {
          if err := config.Validate(); err != nil {
              return nil, errors.Trace(err)
          }
          return Facade{config}, nil
      }

      // Facade exposes methods for the use of the whatever worker. It lives only
      // for the duration of a single method call, and should usually be stateless
      // in itself.
      type Facade struct {
          config Config
      }

* ensure that you test the authorizer
* and finally you can get to implementing the actual API methods :). Do realise, though, that that's really quite a
  small amount of code, and the responsibilities are hopefully as unambiguous as can be.

### What should Facade methods actually look like?

* They should always take a two arguments, the first should be a `context.Context`, of a type defined in `rpc/params`,
  that contains a *slice* of independent instructions.
* That type should very frequently be `params.Entities`, which specifies any number of juju-defined entities using the
  canonical homogeneous id representation.
* They may or may not wish to return an error, and frequently shouldn't; if a facade method does return a top-level
  error, that error should indicate complete failure of the required machinery -- e.g. failure to construct a
  fine-grained authorization func -- rather than mere failure of one of the list of operations requested in the call.
* With that in mind, many Facade methods will want to return `params.ErrorResults` alone, which contains the list of
  errors corresponding to the list of requested operations. It's fine and expected to, e.g., return a whole list of
  permissions errors: that wouldn't mean the *request itself* was denied -- just that the specific operations all
  happened to be disallowed for some specific reasons.

For example:

    // Resnyctopate causes resnyctopation to be applied to the supplied entities.
    //
    // Let's keep going with the worker-that-runs-on-every-machine-agent approach;
    // in this case, we think a little bit about it and decide that the only entity
    // a machine is actually allowed to resnyctopate is itself. 
    func (facade *Facade) Resnyctopate(arg params.Entities) (result ErrorResults) {

        // Boilerplate, pretty much:
        result.Results = make([]params.ErrorResult, len(arg.Entities))

        // Also boilerplate, because "only allow the caller to know about itself" is
        // such a common pattern. Needing to write a more complex AuthFunc, and being
        // unable to do so, would be a good reason to return an actual error from this
        // method.
        authFunc := facade.config.Auth.AuthOwner()

        // ...
        for i, entity := range arg.Entities {

            // We don't seem to have common code for this stuff. We should either marshal
            // the over-the-wire strings into actual tags in another layer, or redefine:
            //
            //     type AuthFunc(tag string) error
            //
            // ...which would be a lot more flexible anyway.
            //
            // Note that the forces here push us towards an `if success {` model so we can
            // translate errors Once And Only Once, below. (Or we could introspect the
            // results and translate via marshalling in an outer layer..? nobody has yet,
            // though...)
            tag, err := names.ParseTag(entity.Tag)
            if err == nil {  
                 switch {
                 case authFunc(tag):
                     err = facade.config.Backend.DoSomething(tag)
                 default:
                     err = common.ErrPerm
                 }
            }

            // This step adds stuff like result codes that the client can derive
            // meaning from. It's kinda ugly once, and it's terribly ugly seeing
            // it repeated; if your loop body is any more complex than the above,
            // *strongly* consider extracting the complications into their own
            // method in service of simplicity here.
            result.Results[i].Error = common.ServerError(err) 
        }
        return result
    }

### How do I test a Facade?

TBA
