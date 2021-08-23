# Network Simulator Language
_okay okay hear me out, it's not as nuts as it might seem_

After a call with Anders, my thoughts on the concrete details of the network simulator I am
building were flipped upside-down. My initial plan was to have a set of standard tests that
would always be run, and the test scenario variations would come from a supplied configuration
file—setting percentages on which peers run what type of ssb implementation (e.g. go or
nodejs), how many peers by percent are always available to connect with, etc.

In the call, Anders voiced concerns on the difficulty of creating a perfect system;
because it has to be perfect, if it's supposed to cover all of the needs and future usecases of
the network simulator in a top-down manner (which a config-driven approach is in its essence).

So I had to pause and rethink my approach. I ended up with the following idea.

## DSL Overview
Instead of having a default, built-into-the-code set of tests, I am thinking of proposing a
small domain specific language (DSL). The DSL will be inspired by the api signature found in
[previous iterations](https://github.com/ssbc/epidemic-broadcast-trees/blob/master/test/three.js) of
network-adjacent tests for ssb.

Through the DSL, you can create peers running a particular software configuration
(e.g. go or nodejs), post follow and unfollow messages, control the network graph through
connecting and disconnecting specific peers, asserting the latest known sequence number of one
peer from another, etc.

These statements will be composed of behind-the-scenes muxrpc calls (as well as specifics for
starting a particular ssb peer; see the [previous netsim writeup](./docs/initial-design-doc.md) for
thoughts on the specifics), with a goal to use the smallest set of muxrpc calls that lets us
implement the most useful testing functionality.

Each DSL statement, when run, will output [TAP-compliant messages](https://testanything.org/tap-specification.html) 
on standard output. That is, existing TAP frameworks and tooling (like [`tape`](https://github.com/substack/tape) and
[`tap-spec`](https://github.com/scottcorgan/tap-spec)) can be used to parse the testing output.
When failures happen, you will be able to see what line and statement they fail on:

    3 not ok has alice, bob@latest

Extraneous error information will be output as TAP-compliant comments in the output, with
other output e.g. `DEBUG=*` logs from each peer being redirected to a log file.

## DSL Example

    create alice
    create bob
    follow alice, bob
    bob post
    has alice, bob@latest

## Preliminary statements
To show off the syntax and the current (somewhat rushed) list of statements, see the
following list. `follow alice, bob` may be read as _alice.follow(bob)_.

	create alice
	connect alice, bob
	disconnect alice, bob
	follow alice, bob
	unfollow alice, bob
	block alice, bob
	unblock alice, bob
	post alice, 10
	have alice, bob@latest
	have alice, bob@10
	wait
	follows alice, bob
	alias alice, @b33fb01<..>.25519
	preload ssb-fixtures-3-authors_300-messages

These statements are in a state of (understandable) flux at the moment, and feedback is welcome
on which statements are missing, suggestions on how to handle trickier parts like `wait`, and
which muxrpc calls might be interesting to use.

There are details to work out, such as `create alice` likely being `create alice,
ssb-nodejs-v19` where `ssb-nodejs-v19` is a folder passed to the netsim binary when executing,
or specified in a (much smaller than planned) configuration file.

The focus at the onset will be to make and validate a system that works without using
[ssb-fixtures](https://github.com/ssb-ngi-pointer/ssb-fixtures), and then moving on to create a
hybrid approach which makes use of its more complex messages.

## Proof of concept
I have already written a small proof of concept in Go, which executes a subset of the dsl. It
parses a test file and outputs TAP messages, and it can start nodes, follow & post messages
using muxrpc calls. 

The prototype is currently only running against a nodejs peer, but ought to be flexible enough
to test against a go-ssb peer as soon as I get into setting one up.
