package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/expectations"
	"log"
	"os"
)

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	var args expectations.Args
	flag.IntVar(&args.MaxHops, "hops", 2, "the default global hops setting")
	flag.BoolVar(&args.ReplicateBlocked, "replicate-blocked", false, "if flag is present, blocked peers will be replicated")
	flag.StringVar(&args.Outpath, "out", "./expectations.json", "the filename and path where the expectations will be dumped")
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Println("Usage:\n expectations <flags> <path to spliced fixtures folder>")
		os.Exit(1)
	}

	graphpath := expectations.PathAndFile(flag.Args()[0], "follow-graph.json")
	outputMap, err := expectations.ProduceExpectations(args, graphpath)
	check(err)

	// persist to disk
	b, err := json.MarshalIndent(outputMap, "", "  ")
	check(err)
	err = os.WriteFile(expectations.PathAndFile(args.Outpath, "expectations.json"), b, 0666)
	check(err)
}
