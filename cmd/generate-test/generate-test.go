// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0

// Generates a full netsim test, given a log-splicer processed ssb-fixtures folder and a replication expectations file
// expectations.json.
package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/expectations"
	"github.com/ssb-ngi-pointer/netsim/generation"
	"os"
	"path"
)

func main() {
	var args generation.Args
	var expectationsArgs expectations.Args
	flag.StringVar(&args.FixturesRoot, "fixtures", "./fixtures-output", "root folder containing spliced out ssb-fixtures")
	flag.StringVar(&args.SSBServer, "sbot", "ssb-server", "the ssb server to start puppets with")
	flag.IntVar(&args.MaxHops, "hops", 2, "the max hops count to use")
	flag.BoolVar(&expectationsArgs.ReplicateBlocked, "replicate-blocked", false, "if flag is present, blocked peers will be replicated")
	flag.IntVar(&args.FocusedCount, "focused", 2, "number of puppets to use for focus group (i.e. # of puppets that verify they are replicating others)")
	flag.Parse()

	if len(os.Args) == 1 {
		fmt.Println("Usage: generate-test <flags>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	expectationsArgs.MaxHops = args.MaxHops
	expectations, err := expectations.ProduceExpectations(expectationsArgs, path.Join(args.FixturesRoot, "follow-graph.json"))

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", "expectations", err)
		os.Exit(1)
	}

	generation.GenerateTest(args, expectations, os.Stderr)
}
