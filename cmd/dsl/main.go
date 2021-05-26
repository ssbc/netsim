package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	refs "go.mindeco.de/ssb-refs"
)

type Whoami struct {
	ID refs.FeedRef
}

type Latest struct {
	ID       string
	Sequence int
	TS       int
}

type Puppet struct {
	Port       int
	directory  string
	feedID     string
	name       string
	instanceID int
	seqno      int64
}

type TestError struct {
	err     error
	message string
}

func (t TestError) Error() string {
	return t.message
}

type Simulator struct {
	puppetMap    map[string]Puppet
	portCounter  int
	instr        Instruction
	instructions []Instruction
	basePort     int
}

const (
	JS_SHIM = "/home/cblgh/code/netsim-experiments/ssb-server/sim-shim.sh"
	GO_SHIM = "/home/cblgh/code/go/src/go-ssb/cmd/go-sbot/sim-shim.sh"
)

func startPuppet(p Puppet, shim string) error {
	filename := fmt.Sprintf("./%s.txt", p.name)
	outfile, err := os.Create(filename)
	if err != nil {
		return TestError{err: err, message: "could not create log file"}
	}
	defer outfile.Close()
	var cmd *exec.Cmd
	cmd = exec.Command(shim, p.directory, strconv.Itoa(p.Port))
	cmd.Stderr = outfile
	cmd.Stdout = outfile
	err = cmd.Run()
	if err != nil {
		return TestError{err: err, message: fmt.Sprintf("failure when creating puppet, see %s for information", filename)}
	}
	return nil
}

func makeSimulator(basePort int) Simulator {
	puppetMap := make(map[string]Puppet)
	return Simulator{puppetMap: puppetMap, basePort: basePort}
}

func (s Simulator) getSrcPuppet() Puppet {
	return s.puppetMap[s.instr.getSrc()]
}

func (s Simulator) getDstPuppet() Puppet {
	return s.puppetMap[s.instr.getDst()]
}

func (s *Simulator) incrementPort() {
	s.portCounter += 1
}

func (s *Simulator) ParseTest(lines []string) {
	s.instructions = make([]Instruction, 0, len(lines))
	fmt.Println("## Start test file")
	for i, line := range lines {
		instr := parseTestLine(line, i+1)
		instr.Print()
		s.instructions = append(s.instructions, instr)
	}
	fmt.Println("## End test file")
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

func (s *Simulator) acquirePort() int {
	return s.basePort + s.portCounter
}

func (s Simulator) execute() {
	for _, instr := range s.instructions {
		s.updateCurrentInstruction(instr)
		switch instr.command {
		case "start":
			name := instr.args[0]
			langImpl := instr.args[1]
			directory := fmt.Sprintf("/home/cblgh/code/go/src/netsim/puppets/%s-%s-%d", langImpl, name, s.portCounter)
			p := Puppet{name: name, directory: directory, instanceID: s.portCounter, Port: s.acquirePort()}
			switch langImpl {
			case "go":
				go startPuppet(p, GO_SHIM)
			case "js":
				go startPuppet(p, JS_SHIM)
			}
			time.Sleep(1 * time.Second)
			feedID, err := DoWhoami(p)
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			p.feedID = feedID
			s.puppetMap[name] = p
			s.incrementPort()
			instr.TestSuccess()
			taplog(fmt.Sprintf("%s has id %s", name, feedID))
			taplog(fmt.Sprintf("logging to %s.txt", name))
		case "log":
			srcPuppet := s.getSrcPuppet()
			amount, err := strconv.Atoi(instr.getSecond())
			if err != nil {
				log.Fatalln(err)
			}
			msg, err := DoLog(srcPuppet, amount)
			s.evaluateRun(err)
			taplog(msg)
		case "wait":
			ms, err := time.ParseDuration(fmt.Sprintf("%sms", instr.getFirst()))
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			time.Sleep(ms)
			instr.TestSuccess()
		case "unfollow":
			fallthrough
		case "follow":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoFollow(srcPuppet, dstPuppet, instr.command == "follow")
			s.evaluateRun(err)
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
		case "disconnect":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoDisconnect(srcPuppet, dstPuppet)
			s.evaluateRun(err)
		case "connect":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoConnect(srcPuppet, dstPuppet)
			s.evaluateRun(err)
		case "has":
			arg := strings.Split(instr.getSecond(), "@")
			dst, seq := arg[0], arg[1]
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.puppetMap[dst]
			err := DoHast(srcPuppet, dstPuppet, seq)
			s.evaluateRun(err)
		default:
			instr.Print()
		}
	}
	fmt.Printf("1..%d\n", len(s.instructions))
}

func main() {
	var outdir string
	flag.StringVar(&outdir, "out", "./puppets", "the output directory containing instantiated netsim peers")
	flag.Parse()
	fmt.Println("output directory:", outdir)
	if len(os.Args) > 1 {
		fmt.Println("language implementations:")
		for i, dir := range os.Args[1:] {
			fmt.Println(i, dir)
		}
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
	sim := makeSimulator(18888)
	lines := readTest("test.txt")
	sim.ParseTest(lines)
	sim.execute()
}
