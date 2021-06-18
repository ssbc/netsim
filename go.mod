module github.com/ssb-ngi-pointer/netsim

go 1.16

require (
	github.com/stretchr/testify v1.7.0 // indirect
	go.cryptoscope.co/muxrpc/v2 v2.0.6
	go.cryptoscope.co/netwrap v0.1.0
	go.cryptoscope.co/nocomment v0.0.0-20210520094614-fb744e81f810
	go.cryptoscope.co/secretstream v1.2.3
	go.mindeco.de/ssb-refs v0.2.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
)

// We need our internal/extra25519 since agl pulled his repo recently.
// Issue: https://github.com/cryptoscope/ssb/issues/44
// Ours uses a fork of x/crypto where edwards25519 is not an internal package,
// This seemed like the easiest change to port agl's extra25519 to use x/crypto
// Background: https://github.com/agl/ed25519/issues/27#issuecomment-591073699
// The branch in use: https://github.com/cryptix/golang_x_crypto/tree/non-internal-edwards
replace golang.org/x/crypto => github.com/cryptix/golang_x_crypto v0.0.0-20200924101112-886946aabeb8
