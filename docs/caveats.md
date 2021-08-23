# Caveats

## `go-ssb`
### Following in go-ssb takes three seconds take effect (wrt connections)
Given a netsim puppet `peer` running [go-ssb](https://github.com/cryptoscope/ssb) and the following netsim snippet:

```
follow puppet alice
connect puppet alice
```

The connect statement will likely fail. The reason is that for technical tradeoff reasons it takes, at writing, 3 seconds for
a follow action to propagate through to go-ssb's hops connection allowlist, [source code](https://github.com/cryptoscope/ssb/blob/80b8875e81408101f83c24eb83ec620037e68f77/sbot/replicate.go#L73
).

**Mitigation:**

```
follow puppet alice
wait 4000
connect puppet alice
```




