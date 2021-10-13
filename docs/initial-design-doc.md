<!--
SPDX-FileCopyrightText: 2021 the netsim authors

SPDX-License-Identifier: CC0-1.0
-->

# Network Simulator 
_a brief exposition on the network simulator to be built as part of SSB's NGI Pointer grant_ 

_2021-05-04_

For the remainder of May—and likely parts of June, if we're being realistic—I'll be working on
creating a tool that will be able to simulate network traffic across different kinds of SSB
peers (Go & Nodejs, to start), verify that messages are being received correctly, and record
performance metrics of the replication process.

This write up aims to share the thoughts I have going into the tool-building phase, and
provides an early point of receiving feedback—before decisions become fossilized in code.

## Goals
The network simulator should..
* be a tool to measure performance metrics before & after partial replication
* be reusable by other scuttlebutts for verifying changes & debugging situations—without requiring any build step to run the tool
* be flexible enough to add new types of peers (e.g. rust)
* provide assurance + insurance that the bedrock of scuttlebutt works as intended, before introducing radical changes
* shake out bugs all across the stack

## Outline
Last week, I spent time thinking about the network simulator and how to approach it. While
thinking about the project, I broke it down into 6 different areas to make it easier to reason
about. Broadly speaking, these areas boil down to the questions:
1. How do we control the peers across language implementation boundaries?
1. How do we specify the network topology, determining which peers will receive what data?
1. How do we verify that the peers of a network are syncronizing messages properly?
1. How do we juggle different runtime versions of language implementations?
1. What should we measure?
1. What types of situations should we simulate?

The rest of the write-up intends to cover the answers with a bit more depth, but here are the
answers in brief:

1. **How do we control the peers across language implementation boundaries?**
    * We define a set of muxrpc calls that must be implemented for a language implementation
      to be tested in the network simulator. We then use those muxrpc calls to control
      replication, and extract information (synced sequence numbers) from peers. Peers are
      created by executing a language shim—e.g. a short script that knows how to start up a
      peer of the designated type.
1. **How do we specify the network topology, determining which peers will receive what data?**
    * We use [ssb-fixtures](https://github.com/ssb-ngi-pointer/ssb-fixtures), which generates a
      follow-graph for a specified number of authors. For more thoughts on the topic, [see this 
      issue](https://github.com/ssb-ngi-pointer/ssb-fixtures/issues/5).
1. **How do we verify that the peers of a network are syncronizing messages properly?**
    * We use our knowledge of the network topology and the set of starting data to determine
      which sequence numbers should be synchronized after the simulation ends. We then query a
      peer over muxrpc for its set of latest sequence numbers. We compare the peer's answer
      with what we know the sequences should be.
1. **How do we juggle different runtime versions of language implementations?**
    * We pass in folders containing language implementations as command-line arguments when running the
      netsim cli tool. In the initial phase, the simulator will start peers in proportion to
      the passed in amount of folders. Given 9 peers, and 3 language implementations -> 3
      peers of each language implementation will be started.
    * **In later stages of the project**, if time allows, we can expand the cli to act as a sort of
      nvm-like tool that can be used to download a particular language implementation. The
      source of the data used by the nvm-like command should be a repository containing one
      folder per language-implementation & version (e.g. `go-ssb@0.2.1`). The contents of the
      folder would either be a binary (go) or a set of files for building the corresponding
      language implementation (for node: package-lock.json, .node-version). It would also
      contain the shim used to start a ssb peer for the given language implementation.
      Finally, the repository should also contain brief instructions for installing a
      particular type of language implementation (npm commands to run, rust cargo commands,
      etc).
1. **What should we measure?**
    * Total synchronization time, total space on disk before/after simulation, total messages
      synced, messages synced per peer.
1. **What types of situations should we simulate?**
    * One peer has no information other than their own (onboarding case)
    * All peers lack information regarding others (extreme case)
    * Some peers lack some information regarding others (normal case)
    * More specific situations, as determined by options set in a config file.

## Implementation
The network simulator will be implemented in Go, using the fundamentals of go-ssb
([cryptoscope](https://github.com/cryptoscope)) to control peer replication over muxrpc. There
will be a cli for starting the network simulation by passing arguments for:
* which language implementations should be used for peers in the simulation, 
* what data to use to populate peers' logs, 
* determining the network topology (follow graph). 

The tool will be usable as a standalone executable, and binary releases published to
github—making it easier for developers across programming languages to use the simulator.

The tool will use pre-generated [ssb-fixtures](https://github.com/ssb-ngi-pointer/ssb-fixtures)
to:
* source identities and their public keypair, 
* determine the follow graph, 
* the amount of peers in the simulation, and 
* the total amount of messages. 

Depending on the specified test scenario, the tool will adapt the generated data such that there will
be a need to synchronize messages from other peers in order to have the latest messages for a
given peer's set of direct & indirect follows (hops).

The fixtures data will be in `log.offset`-form (aka
[`flumelog-offset`](https://github.com/flumedb/flumelog-offset), the legacy ssb log) and
adapted `log.offset` files will be produced for each peer. This means there will be a
requirement on the language implementation and its shim to ingest the `log.offset` data into a
form it wants to use. For example, `offset2` for go, or the new
[ssb-db2](https://github.com/ssb-ngi-pointer/ssb-db2)'s [bipf
format](https://github.com/ssbc/bipf/) for nodejs.

Regarding how to parse `log.offset`, see:
* [`flumelog-offset`](https://github.com/flumedb/flumelog-offset) 
* [`gossb/margaret`](https://github.com/cryptoscope/margaret/pull/13)
* cel's legacy-offset reader (C): `%NB+ufIkKqgFOakcgi2Cgv75etpHO1M8vzBW8EouF4JM=.sha256`
