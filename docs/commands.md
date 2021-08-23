# Network Simulator Test Commands
As described in the [initial syntax proposal](/docs/domain-specific-language.md), the network
simulator works by executing a test specification file. The current set of implemented commands
can be read below.

### Command Parameter Legend
The listing below provides brief explanations of various commands and their
parameters provided by the network simulator and used when writing test files.

* `<name>` is non-spaced name used to refer to the same test puppet (i.e. an ssb identity),
* `<seqno>` is a sequence number as derived from the log of a particular peer e.g. 1, 2, 5, 1337
    * the keyword `<name>@latest` is implemented as a shorthand for referring to the latest
      sequence number according to the identity whose log it is.
* `<implementation-folder>` is defined as the last folder name of the path passed to the
  network simulator on startup. Since many implementations may be passed and used during a
  single simulation, they are provided as flagless arguments **after** any command-line flags.
  Example: `netsim -out ~/netsim-tests ~/code/ssb-server-19 ~/code/ssb-server-new ~/code/go-ssb-sbot`
    * the -out flag defines the output directory for logs and generated identities (`~/netsim-tests` in this case)
    * `~/code/ssb-server-19` would be referenced as `ssb-server-19` in the test specification
      as e.g. `start alice ssb-server-19`

## Implemented Commands
```
enter <name>            // should be called before any other command dealing with the named peer
hops <name> <number>    // should be called before starting a peer to have any effect
caps <name> <string>    // should be called before starting a peer to have any effect
skipoffset <name>       // should be called before starting a peer to have any effect (omits copying over log.offset when loading identity from fixtures)
alloffsets <name>       // should be called before starting a peer to have any effect (preloads the non-spliced input ssb-fixtures => puppet acts like a pub)
load <name> @<base64>.ed25519               // loads an id & its associated secret + log.offset from fixtures
start <name> <implementation-folder>        // spin up name as ssb peer using the specifed sbot implementation
stop <name>                                 // stop a currently running peer
log <name> <amount of messages from the end to debug print>
wait <milliseconds>                         // pause script execution
waituntil <name1> <name2>@<latest||seqno>   // pause script execution until name1 has name2 at seqno in local db
timerstart <label>                          // start a timer with the name <label>
timerstop <label>                           // stop the timer named <label> and output the elapsed time
has <name1> <name2>@<latest||seqno>         // assert name1 has at least name2's seqno in local db
post <name>                                 // add a predefined message (`bep`) of type `type: post` to name's local database
publish <name> (key1 value) (key2.nestedkey value)... // example: publish alice (type post) (value.content hello) (channel ssb-help)
follow <name1> <name2>                      // name1 adds a contact message for name2 to local db
unfollow <name1> <name2>                    // the inverse of above
isfollowing <name1> <name2>                 // assert that name1 is following name2
isnotfollowing <name1> <name2>              // the inverse of above
connect <name1> <name2>                     // attempt to establish a network connection between name1 and name2
disconnect <name1> <name2>                  // close an established network connection
comment <...>                               // always passes; use to write comments
# <...>                                     // always passes; use to write comments. alias for `comment`
```

## Not yet implemented
The following commands might, or might not, be implemented—or they might be implemented with another name.

```
block <name1> <name2>
isblocked <name1> <name2>
isnotblocked <name1> <name2>
```
