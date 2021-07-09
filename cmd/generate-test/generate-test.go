// Generates a full netsim test, given a log-splicer processed ssb-fixtures folder and a replication expectations file
// expectations.json.
package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/generation"
	"os"
)

func main() {
	var args generation.Args
	flag.StringVar(&args.FixturesRoot, "fixtures", "./fixtures-output", "root folder containing spliced out ssb-fixtures")
	flag.StringVar(&args.ExpectationsPath, "expectations", "./expectations.json", "path to expectations.json")
	flag.StringVar(&args.SSBServer, "sbot", "ssb-server", "the ssb server to start puppets with")
	flag.IntVar(&args.MaxHops, "hops", 2, "the max hops count to use")
	flag.IntVar(&args.FocusedCount, "focused", 2, "number of puppets to use for focus group (i.e. # of puppets that verify they are replicating others)")
	flag.Parse()

	if len(os.Args) == 1 {
		fmt.Println("Usage: generate-test <flags>")
		flag.PrintDefaults()
		os.Exit(0)
	}

	generation.GenerateTest(args)
}
