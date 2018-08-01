
# cmd
    import "github.com/juju/cmd"





## Variables
``` go
var DefaultFormatters = map[string]Formatter{
    "smart": FormatSmart,
    "yaml":  FormatYaml,
    "json":  FormatJson,
}
```
DefaultFormatters holds the formatters that can be
specified with the --format flag.

``` go
var ErrNoPath = errors.New("path not set")
```
``` go
var ErrSilent = errors.New("cmd: error out silently")
```
ErrSilent can be returned from Run to signal that Main should exit with
code 1 without producing error output.

``` go
var FormatJson = json.Marshal
```
FormatJson marshals value to a json-formatted []byte.


## func CheckEmpty
``` go
func CheckEmpty(args []string) error
```
CheckEmpty is a utility function that returns an error if args is not empty.


## func FormatSmart
``` go
func FormatSmart(value interface{}) ([]byte, error)
```
FormatSmart marshals value into a []byte according to the following rules:


	* string:        untouched
	* bool:          converted to `True` or `False` (to match pyjuju)
	* int or float:  converted to sensible strings
	* []string:      joined by `\n`s into a single string
	* anything else: delegate to FormatYaml


## func FormatYaml
``` go
func FormatYaml(value interface{}) ([]byte, error)
```
FormatYaml marshals value to a yaml-formatted []byte, unless value is nil.


## func IsErrSilent
``` go
func IsErrSilent(err error) bool
```
IsErrSilent returns whether the error should be logged from cmd.Main.


## func IsRcPassthroughError
``` go
func IsRcPassthroughError(err error) bool
```
IsRcPassthroughError returns whether the error is an RcPassthroughError.


## func Main
``` go
func Main(c Command, ctx *Context, args []string) int
```
Main runs the given Command in the supplied Context with the given
arguments, which should not include the command name. It returns a code
suitable for passing to os.Exit.


## func NewCommandLogWriter
``` go
func NewCommandLogWriter(name string, out, err io.Writer) loggo.Writer
```
NewCommandLogWriter creates a loggo writer for registration
by the callers of a command. This way the logged output can also
be displayed otherwise, e.g. on the screen.


## func NewRcPassthroughError
``` go
func NewRcPassthroughError(code int) error
```
NewRcPassthroughError creates an error that will have the code used at the
return code from the cmd.Main function rather than the default of 1 if
there is an error.


## func ParseAliasFile
``` go
func ParseAliasFile(aliasFilename string) map[string][]string
```
 ParseAliasFile will read the specified file and convert
the content to a map of names to the command line arguments
they relate to.  The function will always return a valid map, even
if it is empty.


## func ZeroOrOneArgs
``` go
func ZeroOrOneArgs(args []string) (string, error)
```
ZeroOrOneArgs checks to see that there are zero or one args, and returns
the value of the arg if provided, or the empty string if not.



## type AppendStringsValue
``` go
type AppendStringsValue []string
```
AppendStringsValue implements gnuflag.Value for a value that can be set
multiple times, and it appends each value to the slice.









### func NewAppendStringsValue
``` go
func NewAppendStringsValue(target *[]string) *AppendStringsValue
```
NewAppendStringsValue is used to create the type passed into the gnuflag.FlagSet Var function.
f.Var(cmd.NewAppendStringsValue(&someMember), "name", "help")




### func (\*AppendStringsValue) Set
``` go
func (v *AppendStringsValue) Set(s string) error
```
Implements gnuflag.Value Set.



### func (\*AppendStringsValue) String
``` go
func (v *AppendStringsValue) String() string
```
Implements gnuflag.Value String.



## type Command
``` go
type Command interface {
    // IsSuperCommand returns true if the command is a super command.
    IsSuperCommand() bool

    // Info returns information about the Command.
    Info() *Info

    // SetFlags adds command specific flags to the flag set.
    SetFlags(f *gnuflag.FlagSet)

    // Init initializes the Command before running.
    Init(args []string) error

    // Run will execute the Command as directed by the options and positional
    // arguments passed to Init.
    Run(ctx *Context) error

    // AllowInterspersedFlags returns whether the command allows flag
    // arguments to be interspersed with non-flag arguments.
    AllowInterspersedFlags() bool
}
```
Command is implemented by types that interpret command-line arguments.











## type CommandBase
``` go
type CommandBase struct{}
```
CommandBase provides the default implementation for SetFlags, Init, and Help.











### func (\*CommandBase) AllowInterspersedFlags
``` go
func (c *CommandBase) AllowInterspersedFlags() bool
```
AllowInterspersedFlags returns true by default. Some subcommands
may want to override this.



### func (\*CommandBase) Init
``` go
func (c *CommandBase) Init(args []string) error
```
Init in the simplest case makes sure there are no args.



### func (\*CommandBase) IsSuperCommand
``` go
func (c *CommandBase) IsSuperCommand() bool
```
IsSuperCommand implements Command.IsSuperCommand



### func (\*CommandBase) SetFlags
``` go
func (c *CommandBase) SetFlags(f *gnuflag.FlagSet)
```
SetFlags does nothing in the simplest case.



## type Context
``` go
type Context struct {
    Dir    string
    Env    map[string]string
    Stdin  io.Reader
    Stdout io.Writer
    Stderr io.Writer
    // contains filtered or unexported fields
}
```
Context represents the run context of a Command. Command implementations
should interpret file names relative to Dir (see AbsPath below), and print
output and errors to Stdout and Stderr respectively.









### func DefaultContext
``` go
func DefaultContext() (*Context, error)
```
DefaultContext returns a Context suitable for use in non-hosted situations.




### func (\*Context) AbsPath
``` go
func (ctx *Context) AbsPath(path string) string
```
AbsPath returns an absolute representation of path, with relative paths
interpreted as relative to ctx.Dir.



### func (\*Context) GetStderr
``` go
func (ctx *Context) GetStderr() io.Writer
```
GetStderr satisfies environs.BootstrapContext



### func (\*Context) GetStdin
``` go
func (ctx *Context) GetStdin() io.Reader
```
GetStdin satisfies environs.BootstrapContext



### func (\*Context) GetStdout
``` go
func (ctx *Context) GetStdout() io.Writer
```
GetStdout satisfies environs.BootstrapContext



### func (\*Context) Getenv
``` go
func (ctx *Context) Getenv(key string) string
```
Getenv looks up an environment variable in the context. It mirrors
os.Getenv. An empty string is returned if the key is not set.



### func (\*Context) Infof
``` go
func (ctx *Context) Infof(format string, params ...interface{})
```
Infof will write the formatted string to Stderr if quiet is false, but if
quiet is true the message is logged.



### func (\*Context) InterruptNotify
``` go
func (ctx *Context) InterruptNotify(c chan<- os.Signal)
```
InterruptNotify satisfies environs.BootstrapContext



### func (\*Context) Setenv
``` go
func (ctx *Context) Setenv(key, value string) error
```
Setenv sets an environment variable in the context. It mirrors os.Setenv.



### func (\*Context) StopInterruptNotify
``` go
func (ctx *Context) StopInterruptNotify(c chan<- os.Signal)
```
StopInterruptNotify satisfies environs.BootstrapContext



### func (\*Context) Verbosef
``` go
func (ctx *Context) Verbosef(format string, params ...interface{})
```
Verbosef will write the formatted string to Stderr if the verbose is true,
and to the logger if not.



## type DeprecationCheck
``` go
type DeprecationCheck interface {

    // Deprecated aliases emit a warning when executed. If the command is
    // deprecated, the second return value recommends what to use instead.
    Deprecated() (bool, string)

    // Obsolete aliases are not actually registered. The purpose of this
    // is to allow code to indicate ahead of time some way to determine
    // that the command should stop working.
    Obsolete() bool
}
```
DeprecationCheck is used to provide callbacks to determine if
a command is deprecated or obsolete.











## type FileVar
``` go
type FileVar struct {
    // Path is the path to the file.
    Path string

    // StdinMarkers are the Path values that should be interpreted as
    // stdin. If it is empty then stdin is not supported.
    StdinMarkers []string
}
```
FileVar represents a path to a file.











### func (FileVar) IsStdin
``` go
func (f FileVar) IsStdin() bool
```
IsStdin determines whether or not the path represents stdin.



### func (\*FileVar) Open
``` go
func (f *FileVar) Open(ctx *Context) (io.ReadCloser, error)
```
Open opens the file.



### func (\*FileVar) Read
``` go
func (f *FileVar) Read(ctx *Context) ([]byte, error)
```
Read returns the contents of the file.



### func (\*FileVar) Set
``` go
func (f *FileVar) Set(v string) error
```
Set stores the chosen path name in f.Path.



### func (\*FileVar) SetStdin
``` go
func (f *FileVar) SetStdin(markers ...string)
```
SetStdin sets StdinMarkers to the provided strings. If none are
provided then the default of "-" is used.



### func (\*FileVar) String
``` go
func (f *FileVar) String() string
```
String returns the path to the file.



## type Formatter
``` go
type Formatter func(value interface{}) ([]byte, error)
```
Formatter converts an arbitrary object into a []byte.











## type Info
``` go
type Info struct {
    // Name is the Command's name.
    Name string

    // Args describes the command's expected positional arguments.
    Args string

    // Purpose is a short explanation of the Command's purpose.
    Purpose string

    // Doc is the long documentation for the Command.
    Doc string

    // Aliases are other names for the Command.
    Aliases []string
}
```
Info holds some of the usage documentation of a Command.











### func (\*Info) Help
``` go
func (i *Info) Help(f *gnuflag.FlagSet) []byte
```
Help renders i's content, along with documentation for any
flags defined in f. It calls f.SetOutput(ioutil.Discard).



## type Log
``` go
type Log struct {
    // If DefaultConfig is set, it will be used for the
    // default logging configuration.
    DefaultConfig string
    Path          string
    Verbose       bool
    Quiet         bool
    Debug         bool
    ShowLog       bool
    Config        string
    Factory       WriterFactory
}
```
Log supplies the necessary functionality for Commands that wish to set up
logging.











### func (\*Log) AddFlags
``` go
func (l *Log) AddFlags(f *gnuflag.FlagSet)
```
AddFlags adds appropriate flags to f.



### func (\*Log) GetLogWriter
``` go
func (l *Log) GetLogWriter(target io.Writer) loggo.Writer
```
GetLogWriter returns a logging writer for the specified target.



### func (\*Log) Start
``` go
func (log *Log) Start(ctx *Context) error
```
Start starts logging using the given Context.



## type MissingCallback
``` go
type MissingCallback func(ctx *Context, subcommand string, args []string) error
```
MissingCallback defines a function that will be used by the SuperCommand if
the requested subcommand isn't found.











## type Output
``` go
type Output struct {
    // contains filtered or unexported fields
}
```
Output is responsible for interpreting output-related command line flags
and writing a value to a file or to stdout as directed.











### func (\*Output) AddFlags
``` go
func (c *Output) AddFlags(f *gnuflag.FlagSet, defaultFormatter string, formatters map[string]Formatter)
```
AddFlags injects the --format and --output command line flags into f.



### func (\*Output) Name
``` go
func (c *Output) Name() string
```


### func (\*Output) Write
``` go
func (c *Output) Write(ctx *Context, value interface{}) (err error)
```
Write formats and outputs the value as directed by the --format and
--output command line flags.



## type RcPassthroughError
``` go
type RcPassthroughError struct {
    Code int
}
```
RcPassthroughError indicates that a Juju plugin command exited with a
non-zero exit code. This error is used to exit with the return code.











### func (\*RcPassthroughError) Error
``` go
func (e *RcPassthroughError) Error() string
```
Error implements error.



## type StringsValue
``` go
type StringsValue []string
```
StringsValue implements gnuflag.Value for a comma separated list of
strings.  This allows flags to be created where the target is []string, and
the caller is after comma separated values.









### func NewStringsValue
``` go
func NewStringsValue(defaultValue []string, target *[]string) *StringsValue
```
NewStringsValue is used to create the type passed into the gnuflag.FlagSet Var function.
f.Var(cmd.NewStringsValue(defaultValue, &someMember), "name", "help")




### func (\*StringsValue) Set
``` go
func (v *StringsValue) Set(s string) error
```
Implements gnuflag.Value Set.



### func (\*StringsValue) String
``` go
func (v *StringsValue) String() string
```
Implements gnuflag.Value String.



## type SuperCommand
``` go
type SuperCommand struct {
    CommandBase
    Name    string
    Purpose string
    Doc     string
    Log     *Log
    Aliases []string
    // contains filtered or unexported fields
}
```
SuperCommand is a Command that selects a subcommand and assumes its
properties; any command line arguments that were not used in selecting
the subcommand are passed down to it, and to Run a SuperCommand is to run
its selected subcommand.









### func NewSuperCommand
``` go
func NewSuperCommand(params SuperCommandParams) *SuperCommand
```
NewSuperCommand creates and initializes a new `SuperCommand`, and returns
the fully initialized structure.




### func (\*SuperCommand) AddHelpTopic
``` go
func (c *SuperCommand) AddHelpTopic(name, short, long string, aliases ...string)
```
AddHelpTopic adds a new help topic with the description being the short
param, and the full text being the long param.  The description is shown in
'help topics', and the full text is shown when the command 'help <name>' is
called.



### func (\*SuperCommand) AddHelpTopicCallback
``` go
func (c *SuperCommand) AddHelpTopicCallback(name, short string, longCallback func() string)
```
AddHelpTopicCallback adds a new help topic with the description being the
short param, and the full text being defined by the callback function.



### func (\*SuperCommand) AllowInterspersedFlags
``` go
func (c *SuperCommand) AllowInterspersedFlags() bool
```
For a SuperCommand, we want to parse the args with
allowIntersperse=false. This will mean that the args may contain other
options that haven't been defined yet, and that only options that relate
to the SuperCommand itself can come prior to the subcommand name.



### func (\*SuperCommand) Info
``` go
func (c *SuperCommand) Info() *Info
```
Info returns a description of the currently selected subcommand, or of the
SuperCommand itself if no subcommand has been specified.



### func (\*SuperCommand) Init
``` go
func (c *SuperCommand) Init(args []string) error
```
Init initializes the command for running.



### func (\*SuperCommand) IsSuperCommand
``` go
func (c *SuperCommand) IsSuperCommand() bool
```
IsSuperCommand implements Command.IsSuperCommand



### func (\*SuperCommand) Register
``` go
func (c *SuperCommand) Register(subcmd Command)
```
Register makes a subcommand available for use on the command line. The
command will be available via its own name, and via any supplied aliases.



### func (\*SuperCommand) RegisterAlias
``` go
func (c *SuperCommand) RegisterAlias(name, forName string, check DeprecationCheck)
```
RegisterAlias makes an existing subcommand available under another name.
If `check` is supplied, and the result of the `Obsolete` call is true,
then the alias is not registered.



### func (\*SuperCommand) RegisterDeprecated
``` go
func (c *SuperCommand) RegisterDeprecated(subcmd Command, check DeprecationCheck)
```
RegisterDeprecated makes a subcommand available for use on the command line if it
is not obsolete.  It inserts the command with the specified DeprecationCheck so
that a warning is displayed if the command is deprecated.



### func (\*SuperCommand) RegisterSuperAlias
``` go
func (c *SuperCommand) RegisterSuperAlias(name, super, forName string, check DeprecationCheck)
```
RegisterSuperAlias makes a subcommand of a registered supercommand
available under another name. This is useful when the command structure is
being refactored.  If `check` is supplied, and the result of the `Obsolete`
call is true, then the alias is not registered.



### func (\*SuperCommand) Run
``` go
func (c *SuperCommand) Run(ctx *Context) error
```
Run executes the subcommand that was selected in Init.



### func (\*SuperCommand) SetCommonFlags
``` go
func (c *SuperCommand) SetCommonFlags(f *gnuflag.FlagSet)
```
SetCommonFlags creates a new "commonflags" flagset, whose
flags are shared with the argument f; this enables us to
add non-global flags to f, which do not carry into subcommands.



### func (\*SuperCommand) SetFlags
``` go
func (c *SuperCommand) SetFlags(f *gnuflag.FlagSet)
```
SetFlags adds the options that apply to all commands, particularly those
due to logging.



## type SuperCommandParams
``` go
type SuperCommandParams struct {
    // UsagePrefix should be set when the SuperCommand is
    // actually a subcommand of some other SuperCommand;
    // if NotifyRun is called, it name will be prefixed accordingly,
    // unless UsagePrefix is identical to Name.
    UsagePrefix string

    // Notify, if not nil, is called when the SuperCommand
    // is about to run a sub-command.
    NotifyRun func(cmdName string)

    Name            string
    Purpose         string
    Doc             string
    Log             *Log
    MissingCallback MissingCallback
    Aliases         []string
    Version         string

    // UserAliasesFilename refers to the location of a file that contains
    //   name = cmd [args...]
    // values, that is used to change default behaviour of commands in order
    // to add flags, or provide short cuts to longer commands.
    UserAliasesFilename string
}
```
SuperCommandParams provides a way to have default parameter to the
`NewSuperCommand` call.











## type UnrecognizedCommand
``` go
type UnrecognizedCommand struct {
    Name string
}
```










### func (\*UnrecognizedCommand) Error
``` go
func (e *UnrecognizedCommand) Error() string
```


## type WriterFactory
``` go
type WriterFactory interface {
    NewWriter(target io.Writer) loggo.Writer
}
```
WriterFactory defines the single method to create a new
logging writer for a specified output target.

















- - -
Generated by [godoc2md](http://godoc.org/github.com/davecheney/godoc2md)