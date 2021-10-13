<!--
SPDX-FileCopyrightText: 2021 the netsim authors

SPDX-License-Identifier: CC0-1.0
-->

# Tutorial
A netsim tutorial that shows how you can generate your own
[`ssb-fixtures`](https://github.com/ssb-ngi-pointer/ssb-fixtures).

```sh
# generate a set of fixtures using https://github.com/ssb-ngi-pointer/ssb-fixtures
# note: --followGraph and --allkeys are essential for netsim to work
npx ssb-fixtures --authors=20 --messages=9000 --allkeys --followGraph --outputDir=ssb-fixtures-output

# convert the ssb-fixtures to the format netsim uses, and generate an automatic test for netsim
netsim generate --out . ssb-fixtures-output # creates test netsim-test.txt, and folder fixtures-output

# run the generated test with the adapted fixtures, using the ssb implementation in ssb-server
netsim run --spec netsim-test.txt --fixtures fixtures-output ~/code/ssb-server

# if you have go-ssb downloaded somewhere, you can adapt the implementation run by some puppets in the netsim-test
sed -i 's/start puppet-00001 ssb-server/start puppet-00001 go-sbot/g' # note sed might work differently on OSX

# and then re-run the simulation
netsim run --spec netsim-test.txt --fixtures fixtures-output ~/code/ssb-server ~/code/go-sbot
```
