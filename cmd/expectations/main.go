package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/expectations"
	"os"
)

func main() {
	var args expectations.Args
	flag.IntVar(&args.MaxHops, "hops", 2, "the default global hops setting")
	flag.BoolVar(&args.ReplicateBlocked, "replicate-blocked", false, "if flag is present, blocked peers will be replicated")
	flag.StringVar(&args.Outpath, "out", "./expectations.json", "the filename and path where the expectations will be dumped")
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Println("Usage:\n expectations <flags> <path to spliced fixtures folder>")
		os.Exit(0)
	}

	graphpath := expectations.PathAndFile(flag.Args()[0], "follow-graph.json")
	expectations.ProduceExpectations(args, graphpath)
}
