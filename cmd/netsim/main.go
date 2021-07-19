package main

import (
	"flag"
	"fmt"
	"os"
)

func usageExit() {
	fmt.Println("Usage: netsim [generate, test] <flags>")
	os.Exit(0)
}

func main() {
	if len(os.Args) < 2 {
		usageExit()
	}
	// get the command
	cmd := getCommand()

	// define flags common across all commands
	var fixturesDir string
	var testfile string
	var hops int
	flag.StringVar(&fixturesDir, "fixtures", "fixtures-output", "fixtures directory")
	flag.StringVar(&testfile, "spec", "netsim-test.txt", "path to netsim test")
	flag.IntVar(&hops, "hops", 2, "the hops setting controls the distance from a peer that information should still be retrieved")

	// handle each command, optionally defining command-specific flags, and finally invoking the command
	switch cmd {
	case "generate":
		var replicateBlocked bool
		var outpath string
		var ssbServer string
		var focusedPuppets int
		flag.BoolVar(&replicateBlocked, "replicate-blocked", false, "if flag is present, blocked peers will be replicated")
		flag.StringVar(&outpath, "out", "./expectations.json", "the filename and path where the expectations will be dumped")
		flag.StringVar(&ssbServer, "sbot", "ssb-server", "the ssb server to start puppets with")
		flag.IntVar(&focusedPuppets, "focused", 2, "number of puppets that verify they are fully replicating their hops")
		flag.Parse()
		checkArgs(cmd)
		fmt.Printf("do generate with %s %s\n", fixturesDir, testfile)
	case "test":
		var caps string
		var outdir string
		var basePort int
		var verbose bool
		flag.StringVar(&caps, "caps", "TODO DEFAULT CAPS", "the secret handshake capability key")
		flag.StringVar(&outdir, "out", "./puppets", "the output directory containing instantiated netsim peers")
		flag.IntVar(&basePort, "port", 18888, "start of port range used for each running sbot")
		flag.BoolVar(&verbose, "v", false, "increase logging verbosity")
		flag.Parse()
		checkArgs(cmd)
		fmt.Printf("do test with %s %s\n", fixturesDir, testfile)
	default:
		usageExit()
	}
}

func checkArgs(cmd string) {
	if len(flag.Args()) == 0 {
		fmt.Printf("Usage: netsim %s <flags>\nOptions:\n", cmd)
		flag.PrintDefaults()
		os.Exit(0)
	}
}

func getCommand() string {
	cmd := os.Args[1]
	// splice out the command argument from os.Args
	var args []string
	args = append(args, os.Args[0])
	args = append(args, os.Args[2:]...)
	os.Args = args
	return cmd
}
