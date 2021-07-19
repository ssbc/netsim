package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/sim"
)

func main() {
	var args sim.Args
	flag.StringVar(&args.Caps, "caps", sim.DefaultShsCaps, "the secret handshake capability key")
	flag.IntVar(&args.Hops, "hops", 2, "the hops setting controls the distance from a peer that information should still be retrieved")
	flag.StringVar(&args.FixturesDir, "fixtures", "", "optional: path to the output of a ssb-fixtures run, if using")
	flag.StringVar(&args.Testfile, "spec", "./test.txt", "test file containing network simulator test instructions")
	flag.StringVar(&args.Outdir, "out", "./puppets", "the output directory containing instantiated netsim peers")
	flag.IntVar(&args.BasePort, "port", 18888, "start of port range used for each running sbot")
	flag.BoolVar(&args.Verbose, "v", false, "increase logging verbosity")
	flag.Parse()

	if len(flag.Args()) == 0 {
		PrintUsage()
		return
	}

	sim.Run(args, flag.Args())
}

func PrintUsage() {
	fmt.Println("netsim: <options> path-to-sbot1 path-to-sbot2.. path-to-sbotn")
	fmt.Println("Options:")
	flag.PrintDefaults()
}
