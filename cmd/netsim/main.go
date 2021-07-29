package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/expectations"
	"github.com/ssb-ngi-pointer/netsim/generation"
	"github.com/ssb-ngi-pointer/netsim/splicer"
	"os"
	"path"
	"strings"
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
		flag.StringVar(&outpath, "out", "./netsim-gen", "the output path of the generated netsim test & its auxiliary files")
		flag.StringVar(&ssbServer, "sbot", "ssb-server", "the ssb server to start puppets with")
		flag.IntVar(&focusedPuppets, "focused", 2, "number of puppets that verify they are fully replicating their hops")
		flag.Parse()

		// splice out the logs into a separate folder
		fixturesOutput := path.Join(outpath, "fixtures-output")
		spliceLogs(fixturesDir, fixturesOutput)
		// use the spliced logs to generate expectations
		expectations := generateExpectations(fixturesOutput, hops, replicateBlocked)
		// use the generated expectations & generate the test
		generatedTest := generateTest(fixturesOutput, ssbServer, focusedPuppets, hops, expectations)
		// echo
		fmt.Println(generatedTest)
		// save test file to disk
		os.WriteFile(path.Join(outpath, testfile), []byte(generatedTest), 0666)
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
		/* todo */
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

func spliceLogs(fixturesPath, dst string) {
	var args splicer.Args
	args.Prune = true
	args.Indir, args.Outdir = fixturesPath, dst
	err := splicer.SpliceLogs(args)
	if err != nil {
		errorOut("splicer", err)
	}
}

func generateExpectations(fixturesRoot string, hops int, replicateBlocked bool) map[string][]string {
	var args expectations.Args
	args.MaxHops = hops
	args.ReplicateBlocked = replicateBlocked
	outputMap, err := expectations.ProduceExpectations(args, path.Join(fixturesRoot, "follow-graph.json"))
	if err != nil {
		errorOut("expectations", err)
	}
	return outputMap
}

func generateTest(fixturesRoot, sbot string, focused, hops int, expectations map[string][]string) string {
	var generationArgs generation.Args
	generationArgs.FixturesRoot = fixturesRoot
	generationArgs.SSBServer = sbot
	generationArgs.MaxHops = hops
	generationArgs.FocusedCount = focused

	s := new(strings.Builder)
	generation.GenerateTest(generationArgs, expectations, s)
	return s.String()
}

func errorOut(tool string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", "tool", err)
	os.Exit(1)
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
