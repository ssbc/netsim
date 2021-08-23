# Caveats

## `go-ssb`
### Hops in Go equals hops in Nodejs + 1 
As of writing, [go-ssb](https://github.com/cryptoscope/ssb) currently has a different
interpretation of SSB's `hops` parameter, which decides how many layers removed from yourself
and your direct follows you want to replicate.



<table>
<tr>
<th>Interpretation</th>
<th>Hops (Go)</th>
<th>Hops (nodejs)</th>
</tr>
<tr>
<th> Only yourself </th>
<td>—</td>
<td> 0 </td>
</tr>
<tr>
<th>Include direct follows</th>
<td>0</td>
<td>1</td>
</tr>
<tr>
<th>Include follower's follows</th>
<td>1</td>
<td>2</td>
</tr>
</table>

In nodejs the hops are interpreted as:

* hops 0: just you
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

