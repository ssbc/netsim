<!--
SPDX-FileCopyrightText: 2021 the netsim authors

SPDX-License-Identifier: CC0-1.0
-->

# Caveats
Unexpected situations you may run into when testing across scuttlebutt implementations, and
some suggested ways forward for getting around them.

## `go-ssb`
### Hops in Go equals hops in Nodejs + 1 
As of writing, [go-ssb](https://github.com/cryptoscope/ssb) currently has a different
interpretation of SSB's hops concept, or how many layers from yourself you want to
download messages.

<table>
<tr>
<th>Interpretation</th>
<th>Hops (Go)</th>
<th>Hops (nodejs)</th>
</tr>
<tr>
<th>Only yourself</th>
<td>—</td>
<td> 0 </td>
</tr>
<tr>
<th>Include direct follows</th>
<td>0</td>
<td>1</td>
</tr>
<tr>
<th>Include transitive follows, one step away</th>
<td>1</td>
<td>2</td>
</tr>
<th>Include transitive follows, two steps away</th>
<td>2</td>
<td>3</td>
</tr>
</table>

**Mitigation:**

Set go-ssb's hops parameter to be one less than is defined on the `${HOPS}` env var ingested by
`sim-shim.sh`:

```diff
go-sbot \
  -lis :"$PORT" \
  -wslis "" \
  -debuglis ":$(($PORT+1))" \
  -repo "$DIR" \
  -shscap "${CAPS}" \
+ -hops "$(( ${HOPS} - 1 ))"
``` 

**Note**: This mitigation is already implemented in the [default go-ssb sim-shim](https://github.com/ssb-ngi-pointer/netsim/blob/main/sim-shims/go-sim-shim.sh#L41-L47).

### Following in go-ssb takes three seconds to take effect (wrt connections)
Given a netsim puppet `peer` running [go-ssb](https://github.com/cryptoscope/ssb) and the following netsim snippet:

```
follow puppet alice
connect puppet alice
```

The `follow` statement will succeed as expected, but the `connect` statement will likely fail.
The reason is that for technical tradeoff reasons it takes, at writing, 3 seconds for a follow
action to propagate through to go-ssb's hops connection allowlist, [source
code](https://github.com/cryptoscope/ssb/blob/80b8875e81408101f83c24eb83ec620037e68f77/sbot/replicate.go#L73).

**Mitigation:**

```
follow puppet alice
wait 4000
connect puppet alice
```

### `nope - access denied` when connecting to peer 
Given two puppets `alice` and `gopher`, where `alice` does not follow `gopher`, the latter is running [go-ssb](https://github.com/cryptoscope/ssb), and the following statement:

```
connect alice gopher
# or, equivalent
connect gopher alice
```

Depending on how you are starting your go-sbot, you might get a failure and an error log (when
running in `-v`erbose mode):
```
node - access denied
```

The reason is that the go server is strict with who it allows to establish connections with it.
If the peer running the go server does not follow the connecting party, then the connection
will be denied.

**Mitigations:**

1. Make sure `alice` follows `gopher`, `gopher` follows `alice` (and observe the 3 second caveat mentioned elsewhere in this document)
2. Run go-ssb in so-called `promiscuous` mode by appending a `-promisc` flag when starting it:

```diff
exec go-sbot \
  -lis :"$PORT" \
  -wslis "" \
  -debuglis ":$(($PORT+1))" \
  -repo "$DIR" \
  -shscap "${CAPS}" \
  -hops "$(( ${HOPS} - 1 ))"
+  -promisc
```

