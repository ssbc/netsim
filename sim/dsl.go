package sim

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ssb-ngi-pointer/netsim/internal/parser"
	"golang.org/x/sync/errgroup"
)

const DefaultShsCaps = "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s="

type Args struct {
	Caps        string // global caps setting
	Hops        int    // global hops setting
	FixturesDir string // directory containing the spliced ssb-fixtures
	Testfile    string // path to file containing sim statements
	Outdir      string // directory where puppet logs & files will be dumped
	BasePort    int    // starting port used for instantiating the ports used by puppets
	Verbose     bool   // produce more output when running (echoes puppet output in realtime, in addition to TAP assertions)
}

type Process struct {
	cmd     *exec.Cmd
	logfile *os.File
}

// TODO: convert all uses of testError to fmt.Errorf(msg + %w)
type TestError struct {
	err     error
	message string
}

func (t TestError) Error() string {
	return t.message + ": " + t.err.Error()
}

func (t TestError) Unwrap() error {
	return t.err
}

type FixturesFeedInfo struct {
	Folder string `json:"folder"`
	Latest int    `json:"latest"`
}

type Simulator struct {
	puppetMap map[string]*Puppet
	// map of pubkey: {latest: int, folder: string}.
	// `folder` is the spliced-out fixtures subfolder containing the secret + log.offset for pubkey
	fixturesIds     map[string]FixturesFeedInfo
	implementations map[string]string
	puppetDir       string
	caps            string // secret handshake capability key; also termed `shscap` (and sometimes appkey?) in ssb-go
	portCounter     int
	instr           Instruction
	instructions    []Instruction
	basePort        int
	hops            int
	verbose         bool
	fixtures        string

	rootCtx         context.Context
	cancelExecution context.CancelFunc
}

func bail(msg string) {
	fmt.Printf("Bail out! %s\n", msg)
	os.Exit(1)
}

func makeSimulator(args Args, sbots []string) Simulator {
	puppetMap := make(map[string]*Puppet)
	langMap := make(map[string]string)
	fixturesIdsMap := make(map[string]FixturesFeedInfo)

	for _, bot := range sbots {
		botDir, err := filepath.Abs(bot)
		if err != nil {
			bail(fmt.Sprintf("%v", err))
		}
		// make sure folder exists, otherwise bail out
		_, err = os.Stat(botDir)
		if err != nil {
			msg := fmt.Sprintf("language implementation folder %s does not exist", bot)
			bail(fmt.Sprintf("%s (%v)", msg, err))
		}
		// make sure sim-shim.sh exists, otherwise bail out
		_, err = os.Stat(filepath.Join(botDir, "sim-shim.sh"))
		if err != nil {
			msg := fmt.Sprintf("sim-shim.sh is missing from root of sbot folder %s", bot)
			bail(fmt.Sprintf("%s (%v)", msg, err))
		}
		// index language implementations by the last folder name
		langMap[filepath.Base(botDir)] = botDir
	}

	absPuppetDir, err := filepath.Abs(args.Outdir)
	if err != nil {
		log.Fatalln(err)
	}

	// if we're loading fixtures, parse the identity-to-secret-folders map `secret-ids.json` (see cmd/log-splicer for info)
	// secret-ids.json contains a map from feed id to { latest: int, folder: string }
	if args.FixturesDir != "" {
		fixturesIds, err := os.ReadFile(filepath.Join(args.FixturesDir, "secret-ids.json"))
		if err != nil {
			bail(fmt.Sprintf("--fixtures %s was missing file secret-ids.json\ndid you run the netsim utility `cmd/log-splicer`?\n", args.FixturesDir))
			return Simulator{}
		}
		err = json.Unmarshal(fixturesIds, &fixturesIdsMap)
		if err != nil {
			bail(fmt.Sprintf("%v", err))
			return Simulator{}
		}
	}

	sim := Simulator{
		puppetMap:       puppetMap,
		puppetDir:       absPuppetDir,
		fixturesIds:     fixturesIdsMap,
		implementations: langMap,
		caps:            args.Caps,
		basePort:        args.BasePort,
		hops:            args.Hops,
		verbose:         args.Verbose,
		fixtures:        args.FixturesDir,
	}

	sim.rootCtx, sim.cancelExecution = context.WithCancel(context.Background())
	return sim
}

func (s Simulator) getSecretDir(id string) string {
	info, has := s.fixturesIds[id]
	if !has {
		s.Abort(fmt.Errorf("cannot find id %s when getting secret dir", id))
		return ""
	}
	return info.Folder
}

func (s Simulator) getInstructionArg(n int) string {
	var arg string
	var err error
	switch n {
	case 1:
		arg, err = s.instr.first()
	case 2:
		arg, err = s.instr.second()
	default:
		s.Abort(fmt.Errorf("getInstructionArg(): no such arg %d", n))
	}
	if err != nil {
		s.Abort(err)
	}
	return arg
}

func (s Simulator) getFixturesLatestSeqno(id string) int {
	info, has := s.fixturesIds[id]
	if !has {
		s.Abort(fmt.Errorf("cannot find id %s when getting latest seqno", id))
		return -1
	}
	return info.Latest
}

func (s Simulator) getSrcPuppet() *Puppet {
	return s.getPuppet(s.instr.getSrc())
}

func (s Simulator) getDstPuppet() *Puppet {
	return s.getPuppet(s.instr.getDst())
}

func (s *Simulator) ParseTest(lines []string) {
	s.instructions = make([]Instruction, 0, len(lines))
	if s.verbose {
		fmt.Println("## Start test file")
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			s.Abort(fmt.Errorf("line %d was empty; empty lines are not allowed", i+1))
			return
		}
		instr := parseTestLine(line, i+1)
		if s.verbose {
			instr.Print()
		}
		s.instructions = append(s.instructions, instr)
	}
	if s.verbose {
		fmt.Println("## End test file")
	}
}

func (s Simulator) evaluateRun(err error) {
	if err != nil {
		s.instr.TestFailure(err)
	} else {
		s.instr.TestSuccess()
	}
}

func (s *Simulator) updateCurrentInstruction(instr Instruction) {
	s.instr = instr
}

func listenOnPort(port int) func() error {
	return func() error {
		l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err != nil {
			// we couldn't use this port, close the socket & try a new one
			return err
		}
		// success! we could listen on the port, which means we can use it!
		// close the opened socket and return the port
		l.Close()
		return nil
	}
}

func (s *Simulator) acquirePort() int {
	maxAttempts := 100
	startPort := s.basePort + s.portCounter

	for i := 0; i < maxAttempts; i = i + 2 {
		port := s.basePort + s.portCounter
		s.portCounter += 2
		// try to acquire two sequential ports: one for muxrpc communication, the other for sbot's websockets support.
		// websockets is currently not used by the netsim, but the port needs to be specified for the sbots process
		g := new(errgroup.Group)
		g.Go(listenOnPort(port))
		g.Go(listenOnPort(port + 1))
		err := g.Wait()
		if err != nil {
			continue
		}
		// if we could acquire the two ports, we're done! we have found two usable ports for one of our puppets
		return port
	}
	log.Fatalf("Could not find any connectable ports in the range [%d, %d]", startPort, startPort+maxAttempts)
	return -1
}

type Sleeper struct {
	elapsed time.Time
	sim     Simulator
}

func (s *Sleeper) sleep(d time.Duration) {
	s.elapsed = s.elapsed.Add(d)
	time.Sleep(d)
	// iterate through puppets & record sleep duration for those currently running at time of sleep
	for _, puppet := range s.sim.puppetMap {
		if puppet.isExecuting() {
			puppet.addSleepDuration(d)
		}
	}
}

func (s Simulator) checkCancellation() bool {
	select {
	case <-s.rootCtx.Done():
		taplog("Context canceled, stopping execution")
		return true
	default:
		// keep running
		return false
	}
}

func (s Simulator) execute() {
	sleeper := Sleeper{sim: s}
	start := time.Now()
	for _, instr := range s.instructions {
		// check if we have received any cancellations before continuing on to process test commands
		canceled := s.checkCancellation()
		if canceled {
			return
		}

		s.updateCurrentInstruction(instr)
		switch instr.command {
		case "#", "comment":
			instr.TestSuccess()
		case "enter":
			name := s.getInstructionArg(1)
			p := Puppet{
				name: name,
				caps: s.caps,
				hops: s.hops,
			}
			s.puppetMap[name] = &p
			instr.TestSuccess()
		case "load":
			if s.fixtures == "" {
				s.Abort(errors.New("no fixtures provided with --fixtures, yet tried to load feed from log.offset"))
				continue
			}
			name := s.getInstructionArg(1)
			id := s.getInstructionArg(2)

			p := s.getPuppet(name)
			p.secretDir = s.getSecretDir(id)
			p.seqno = s.getFixturesLatestSeqno(id)
			p.feedID = id
			instr.TestSuccess()
		case "skipoffset":
			name := s.getInstructionArg(1)
			p := s.getPuppet(name)
			p.omitOffset = true
			instr.TestSuccess()
		case "alloffsets":
			name := s.getInstructionArg(1)
			p := s.getPuppet(name)
			p.allOffsets = true
			instr.TestSuccess()
		case "hops":
			name := s.getInstructionArg(1)
			arg := s.getInstructionArg(2)
			hops, err := strconv.Atoi(arg)
			if err != nil {
				s.Abort(err)
				continue
			}
			p := s.getPuppet(name)
			p.hops = hops
			instr.TestSuccess()
		case "caps":
			name := s.getInstructionArg(1)
			caps := s.getInstructionArg(2)
			// perform validation on caps
			_, err := base64.StdEncoding.DecodeString(caps)
			if err != nil {
				s.Abort(err)
				continue
			}
			p := s.getPuppet(name)
			p.caps = caps
			instr.TestSuccess()
		case "start":
			name := s.getInstructionArg(1)
			langImpl := s.getInstructionArg(2)
			if _, ok := s.implementations[langImpl]; !ok {
				err := errors.New(fmt.Sprintf("no such language implementation passed to simulator on startup (%s)", langImpl))
				instr.TestFailure(err)
				continue
			}
			p := s.getPuppet(name)
			subfolder := fmt.Sprintf("%s-%s", langImpl, name)
			fullpath := filepath.Join(s.puppetDir, subfolder)
			p.port = s.acquirePort()
			p.directory = fullpath

			err := p.start(s, langImpl)
			p.lastStart = time.Now()
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			// TODO: make this more resource efficient (have a retry loop that tries to ping puppet's sbot, exit when ok)
			// look at waituntil's implementation for a good solution?
			sleeper.sleep(1 * time.Second)

			// if this puppet is loaded from fixtures, omit doing whoami to find out its feedID
			// (we already know it)
			if !p.usesFixtures() {
				feedID, err := DoWhoami(p)
				if err != nil {
					instr.TestFailure(err)
					continue
				}
				p.feedID = feedID
			}
			err = p.countMessages()
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			instr.TestSuccess()
			taplog(fmt.Sprintf("%s (%d messages) has id %s ", name, p.totalMessages, p.feedID))
			taplog(fmt.Sprintf("logging to %s.txt", name))
		case "stop":
			name := s.getInstructionArg(1)
			p := s.getPuppet(name)
			err := p.stop()
			if err != nil {
				s.Abort(err)
			}
			p.stopTimer()
			instr.TestSuccess()
			taplog(fmt.Sprintf("%s has been stopped", name))
		case "log":
			srcPuppet := s.getSrcPuppet()
			arg := s.getInstructionArg(2)
			amount, err := strconv.Atoi(arg)
			if err != nil {
				log.Fatalln(err)
			}
			msg, err := DoLog(srcPuppet, amount)
			s.evaluateRun(err)
			taplog(msg)
		case "wait":
			arg := s.getInstructionArg(1)
			ms, err := time.ParseDuration(fmt.Sprintf("%sms", arg))
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			sleeper.sleep(ms)
			instr.TestSuccess()
		case "waituntil":
			line := s.getInstructionArg(2)
			arg := strings.Split(line, "@")
			if len(arg) < 2 {
				s.Abort(fmt.Errorf("waituntil statement was missing @<seqno> (%s)", line))
				return
			}
			dst, seq := arg[0], arg[1]
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getPuppet(dst)
			MAX_RETRIES := 10
			var err error
			var message string
			// kludge: we've been having rare issues of go-muxrpc failing on the waituntil command.  this kludge simply
			// retries any failures, as the call is generally likely to succeed. typically, we only saw a failure once in a
			// ~416 line netsim test.
			for retries := 0; retries < MAX_RETRIES; retries++ {
				message, err = DoWaitUntil(srcPuppet, dstPuppet, seq)
				if err == nil {
					break
				} else {
					canceled := s.checkCancellation()
					if canceled {
						return
					}
					taplog(fmt.Sprintf("waituntil had an error on attempt %d/%d (%s) ", retries, MAX_RETRIES, err))
					sleeper.sleep(1 * time.Second)
				}
			}
			s.evaluateRun(err)
			if err == nil {
				// the message we get back is of the type "interpreting <name>@latest as <name>@<seqno>"
				taplog(message)
			}
		case "unfollow":
			fallthrough
		case "follow":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			// TODO: check id and err if id not set
			err := DoFollow(srcPuppet, dstPuppet, instr.command == "follow")
			s.evaluateRun(err)
			srcPuppet.bumpSeqno()
		case "isfollowing":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoIsFollowing(srcPuppet, dstPuppet)
			s.evaluateRun(err)
		case "isnotfollowing":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoIsNotFollowing(srcPuppet, dstPuppet)
			s.evaluateRun(err)
		case "post":
			srcPuppet := s.getSrcPuppet()
			err := DoPost(srcPuppet)
			s.evaluateRun(err)
			srcPuppet.bumpSeqno()
		case "publish":
			postline := strings.Join(instr.args[1:], " ")
			obj := parser.ParsePostLine(postline)
			srcPuppet := s.getSrcPuppet()
			err := DoPublish(srcPuppet, obj)
			s.evaluateRun(err)
			srcPuppet.bumpSeqno()
		case "disconnect":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoDisconnect(srcPuppet, dstPuppet)
			s.evaluateRun(err)
		case "connect":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoConnect(srcPuppet, dstPuppet)
			// TODO: re-evaluate need of sleeping after connection
			// current need: make sure no puppet tries to hit the remote sbot too quickly (saw some error with like EOF
			// something something)
			sleeper.sleep(500 * time.Millisecond)
			s.evaluateRun(err)
		case "has":
			line := s.getInstructionArg(2)
			arg := strings.Split(line, "@")
			if len(arg) < 2 {
				s.Abort(fmt.Errorf("has statement was missing @<seqno> (%s)", line))
				return
			}
			dst, seq := arg[0], arg[1]
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getPuppet(dst)
			message, err := DoHast(srcPuppet, dstPuppet, seq)
			s.evaluateRun(err)
			if err == nil {
				// the message we get back is of the type "interpreting <name>@latest as <name>@<seqno>"
				taplog(message)
			}
		default:
			// unknown command, abort test run
			s.Abort(errors.New("Unknown simulator command"))
			continue
		}
	}
	fmt.Printf("1..%d\n", len(s.instructions))

	elapsed := time.Since(start)
	var t time.Time
	t = t.Add(elapsed)
	cpuTime := t.Sub(sleeper.elapsed)

	taplog("End of simulation")
	taplog(fmt.Sprintf("Total time: %s", elapsed.String()))
	taplog(fmt.Sprintf("Active time: %s", cpuTime.String()))
	taplog(fmt.Sprintf("Puppet count: %d", len(s.puppetMap)))
}

func (s Simulator) Abort(err error) {
	s.instr.TestAbort(err)
	s.exit()
}

func (s Simulator) getPuppet(name string) *Puppet {
	p, exists := s.puppetMap[name]
	if !exists {
		s.Abort(errors.New(fmt.Sprintf("fatal: there is no puppet declared as %s\n# possible fix: add `enter %s` before other statements", name, name)))
	}
	return p
}

func (s Simulator) monitorInterrupts() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		taplog(fmt.Sprintf("received shutdown signal, shutting down (signal %s)\n", sig.String()))
		s.cancelExecution()
	}()
}

func (s Simulator) logMetrics() {
	fmtString := "%-12s %12s %12s %12s"
	taplog(fmt.Sprintf(fmtString, "Puppet", "Total time", "Active time", "# messages"))
	puppets := make([]*Puppet, 0, len(s.puppetMap))
	// put all puppets into a slice so for later sortability, and stop the timers of running puppets
	for _, puppet := range s.puppetMap {
		// if a puppet is still running after the last test has been executed, we need to stop its timer manually
		if puppet.isExecuting() {
			puppet.stopTimer()
			err := puppet.countMessages()
			if err != nil {
				taplog(fmt.Sprintf("%s had an error when trying to count db messages (%s)", puppet.name, err))
			}
		}
		puppets = append(puppets, puppet)
	}
	// sort puppets by total time descending
	sort.Slice(puppets, func(i, j int) bool {
		return puppets[i].totalTime.Milliseconds() > puppets[j].totalTime.Milliseconds()
	})
	// print the time metrics
	for _, puppet := range puppets {
		total := puppet.totalTime.Truncate(time.Millisecond)
		active := (puppet.totalTime - puppet.slept).Truncate(time.Millisecond)
		count := strconv.Itoa(puppet.totalMessages)
		taplog(fmt.Sprintf(fmtString, puppet.name, total, active, count))
	}
}

func (s Simulator) exit() {
	s.logMetrics()
	taplog("Closing all puppets")
	s.cancelExecution()
	time.Sleep(1 * time.Second)
}

func preparePuppetDir(dir string) string {
	// introduce convention that the output dir is called puppets.
	// this fixes edgecases of accidentally removing unintended
	// folders + files
	if filepath.Base(dir) != "puppets" {
		dir = filepath.Join(dir, "puppets")
	}
	absdir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalln(err)
	}
	// remove the puppet dir and its subfolders
	err = os.RemoveAll(absdir)
	if err != nil {
		log.Fatalln(err)
	}
	// recreate it
	err = os.Mkdir(absdir, 0777)
	if err != nil {
		log.Fatalln(err)
	}
	return absdir
}

func Run(args Args, sbots []string) {
	// validate flag-passed caps key
	_, err := base64.StdEncoding.DecodeString(args.Caps)
	if err != nil {
		bail(fmt.Sprintf("--caps %s was not a valid base64 sequence\n", args.Caps))
	}
	/*
	 * the language implementation dir contains the code for starting a puppet, via a shim.
	 * the puppet lives in another directory, which contains its secret.
	 *
	 * the language implementation needs to be passed:
	 *   the directory of the puppet's secret
	 *   the ports it will use
	 *
	 * the puppet directory needs to be created, and a secret needs to be instantiated for it.
	 * requirements:
	 *   an output directory containing all puppet folders
	 *   some way to instantiate seeded secrets for each puppet
	 */

	args.Outdir = preparePuppetDir(args.Outdir)
	sim := makeSimulator(args, sbots)
	// monitor system interrupts via cmd-c/mod-c
	sim.monitorInterrupts()

	fmt.Println("TAP version 13")
	lines := readTest(args.Testfile)
	sim.ParseTest(lines)
	sim.execute()

	// once we are done we want all puppets to exit
	sim.exit()
}
