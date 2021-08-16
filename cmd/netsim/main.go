package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/expectations"
	"github.com/ssb-ngi-pointer/netsim/generation"
	"github.com/ssb-ngi-pointer/netsim/sim"
	"github.com/ssb-ngi-pointer/netsim/splicer"
	"os"
	"path"
	"strings"
)

func usageExit() {
	fmt.Println("Usage: netsim [generate, run] <flags>")
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usageExit()
	}
	// get the command name and shift the arguments around for flag parsing
	cmd := prepareCommand()

	// define flags common across all commands
	var fixturesDir string
	var testfile string
	var hops int
	flag.StringVar(&testfile, "spec", "netsim-test.txt", "path to netsim test file")
	flag.IntVar(&hops, "hops", 2, "hops controls the distance from a peer that information should still be retrieved")

	// handle each command, optionally defining command-specific flags, and finally invoking the command
	switch cmd {
	case "generate":
		var replicateBlocked bool
		var outpath string
		var ssbServer string
		var focusedPuppets int
		var onlySplice bool
		var generationSeed int64
		flag.BoolVar(&onlySplice, "no-test-script", false, "only converts the input fixtures to netsim-style fixtures")
		flag.BoolVar(&replicateBlocked, "replicate-blocked", false, "if flag is present, blocked peers will be replicated")
		flag.StringVar(&outpath, "out", "./", "the output path of the generated netsim test & its auxiliary files")
		flag.StringVar(&ssbServer, "sbot", "ssb-server", "the ssb server to start puppets with")
		flag.IntVar(&focusedPuppets, "focused", 2, "number of puppets that verify they are fully replicating their hops")
		flag.Int64Var(&generationSeed, "seed", 0, "seed used by test generation")
		flag.Parse()

		if len(flag.Args()) == 0 {
			printHelp("generate",
				"path-to-ssb-fixtures-output",
				"Generate a netsim test from a ssb-fixtures folder")
		}
		fixturesDir = flag.Args()[0]

		// splice out the logs into a separate folder
		fixturesOutput := path.Join(outpath, "fixtures-output")
		spliceLogs(fixturesDir, fixturesOutput)
		if onlySplice {
			os.Exit(0)
		}
		// use the spliced logs to generate expectations
		expectations := generateExpectations(fixturesOutput, hops, replicateBlocked)
		// use the generated expectations & generate the test
		generatedTest := generateTest(fixturesOutput, ssbServer, focusedPuppets, hops, generationSeed, expectations)
		// echo
		fmt.Println(generatedTest)
		// save test file to disk
		err := os.WriteFile(path.Join(outpath, testfile), []byte(generatedTest), 0666)
		if err != nil {
			errOut("netsim generate", fmt.Errorf("failed to write test to disk (%w)", err))
		}
	case "run":
		var simArgs sim.Args
		flag.StringVar(&simArgs.Caps, "caps", sim.DefaultShsCaps, "the secret handshake capability key")
		flag.StringVar(&fixturesDir, "fixtures", "", "optional: path to the output of a ssb-fixtures run, if using")
		flag.StringVar(&simArgs.Outdir, "out", "./puppets", "the output directory containing instantiated netsim peers")
		flag.IntVar(&simArgs.BasePort, "port", 18888, "start of port range used for each running sbot")
		flag.BoolVar(&simArgs.Verbose, "v", false, "increase logging verbosity")
		flag.Parse()

		simArgs.Hops = hops
		simArgs.Testfile = testfile
		simArgs.FixturesDir = fixturesDir

		if len(flag.Args()) == 0 {
			printHelp("run",
				"path-to-sbot1 path-to-sbot2.. path-to-sbotn",
				"Run a simulation with the passed-in sbots and a netsim test")
		}
		sim.Run(simArgs, flag.Args())
	default:
		usageExit()
	}
}

func spliceLogs(fixturesPath, dst string) {
	var args splicer.Args
	args.Prune = true
	args.Indir, args.Outdir = fixturesPath, dst
	err := splicer.SpliceLogs(args)
	errOut("splicer", err)
}

func generateExpectations(fixturesRoot string, hops int, replicateBlocked bool) map[string][]string {
	var args expectations.Args
	args.MaxHops = hops
	args.ReplicateBlocked = replicateBlocked
	outputMap, err := expectations.ProduceExpectations(args, path.Join(fixturesRoot, "follow-graph.json"))
	errOut("expectations", err)
	return outputMap
}

func printHelp(cmd, usage, description string) {
	fmt.Printf("netsim %s: <options> %s\n", cmd, usage)
	fmt.Println(description, "\n")
	fmt.Println("Options:")
	flag.PrintDefaults()
	os.Exit(1)
}

func generateTest(fixturesRoot, sbot string, focused, hops int, seed int64, expectations map[string][]string) string {
	var generationArgs generation.Args
	generationArgs.FixturesRoot = fixturesRoot
	generationArgs.SSBServer = sbot
	generationArgs.MaxHops = hops
	generationArgs.Seed = seed
	generationArgs.FocusedCount = focused

	s := new(strings.Builder)
	generation.GenerateTest(generationArgs, expectations, s)
	return s.String()
}

func errOut(tool string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", tool, err)
		os.Exit(1)
	}
}

func prepareCommand() string {
	cmd := os.Args[1]
	// splice out the command argument from os.Args
	var args []string
	args = append(args, os.Args[0])
	args = append(args, os.Args[2:]...)
	os.Args = args
	return cmd
}
