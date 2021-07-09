// SPDX-License-Identifier: MIT

// Tasks the log splicer takes care of during its runtime:
// * Creates folder super structure, encapsulating each identity in its own folder
// * Maps identities to folder names they live in
// * Copies secrets from the fixtures folder to each identity folder
// * Splics out messages from the one ssb-fixtures log.offset to the many log.offset in said folder structure
// * Persists an identity mapping of ID to {folderName, latest sequence number} to secret-ids.json

// The log splicer command takes an ssb-fixtures generated folder as input, and a destination folder as output. The
// destination folder will be populated with identity folders, one folder per identity found in the generated fixtures.
//
// Each identity folder contains a log.offset, with the posts created by that identity, and the identity's secret file. The
// identity folders are named after the filenames of the secrets found in the ssb-fixtures folder, which preserves the
// pareto distribution of authors (secrets in the lower number ranges have issued more posts).
//
// Finally, a mapping from ssb identities @[...].ed25199 to the identity folders is dumped as json to the root of the
// destination folder. The mapping, in addition to naming the secret folder, also contains an integer count tracking the latest
// sequence number posted by that identity.
package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/splicer"
	"os"
)

/*
* Given a ssb-fixtures directory, and its monolithic flume log legacy.offset (mfl)
* 1. read all the secrets to figure out which authors exist
* 2. for each discovered author create a key in a map[string]margaret.Log
* 3. go through each message in the mfl and filter out the messages into the corresponding log of the map
* 4. finally, create folders for each author, using the author's pubkey as directory name, and dump an lfo
* version of their log.offset representation. inside each folder, dump the correct secret as well
 */

func main() {
	args := splicer.Args{}
	flag.BoolVar(&args.Verbose, "v", false, "verbose: talks a bit more than than the tool otherwise is inclined to do")
	flag.BoolVar(&args.DryRun, "dry", false, "only output what it would do")
	flag.BoolVar(&args.Prune, "prune", false, "removes existing output logs before writing to them (if -prune omitted, the splicer will instead exit with an error)")

	flag.Parse()
	logpaths := flag.Args()
	var err error
	if len(logpaths) != 2 {
		cmdName := os.Args[0]
		fmt.Printf("Usage: %s <options> <path to ssb-fixtures folder> <output path>\nOptions:\n", cmdName)
		flag.PrintDefaults()
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", getToolName(), err)
		os.Exit(1)
	}
	args.Indir, args.Outdir = logpaths[0], logpaths[1]

	err = splicer.SpliceLogs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", getToolName(), err)
		os.Exit(1)
	}
}

func getToolName() string {
	return os.Args[0]
}
