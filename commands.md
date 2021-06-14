# Network Simulator Test Commands
As described in the [initial syntax proposal](./domain-specific-language.md), the network
simulator works by executing a test specification file. The current set of implemented commands
can be read below.

### Command Parameter Legend
The listing below provides brief explanations of the various parameters that are provided to
the network simulator commands when writing test files.

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
nolog <name>            // should be called before starting a peer to have any effect (omits log.offset when loading identity from fixtures)
load <name> @<base64>.ed25519
start <name> <implementation-folder>
stop <name>
wait <milliseconds>
has <name1> <name2>@<latest||seqno>
post <name>
follow <name1> <name2>
unfollow <name1> <name2>
isfollowing <name1> <name2>
isnotfollowing <name1> <name2>
connect <name1> <name2>
disconnect <name1> <name2>
```

## Not yet implemented
The following commands might, or might not, be implemented—or they might be implemented with another name.

```
block <name1> <name2>
isblocked <name1> <name2>
isnotblocked <name1> <name2>
hasnot <name1> <name2>@<latest||seqno>
// the following commands are to be used in combination with a fixtures folder, passed with the flag --fixtures <folder>
truncate <name1> <name2>@<new length>
```

The commands `load` and `truncate` would be introduced if the simulator implements
support for loading in [ssb-fixtures](https://github.com/ssb-ngi-pointer/ssb-fixtures) pre-generated offset files.
