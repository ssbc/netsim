package main

import (
	"bytes"
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
}

const (
	PUPPETSCRIPT = "/home/cblgh/code/netsim-experiments/ssb-server/start-nodejs-puppet.sh"
	QUERYSCRIPT  = "/home/cblgh/code/netsim-experiments/ssb-server/query.sh"
)

func startPuppet(p Puppet) error {
	filename := fmt.Sprintf("./log-%s.txt", p.name)
	outfile, err := os.Create(filename)
	if err != nil {
		return TestError{err: err, message: "could not create log file"}
	}
	defer outfile.Close()
	cmd := exec.Command(PUPPETSCRIPT, strconv.Itoa(p.instanceID))
	cmd.Stderr = outfile
	cmd.Stdout = outfile
	err = cmd.Run()
	if err != nil {
		return TestError{err: err, message: fmt.Sprintf("failure when creating puppet, see %s for information")}
	}
	return nil
}

// TODO: replace runline && queryMuxrpc by using sbotcli as a module
func runline(line string) (bytes.Buffer, error) {
	parts := strings.Fields(line)
	cmd := exec.Command(parts[0], parts[1:]...)
	var stderr bytes.Buffer
	var out bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return bytes.Buffer{}, TestError{err: err, message: stderr.String()}
	}
	return out, nil
}

func queryMuxrpc(id int, q string) (bytes.Buffer, error) {
	// runs intermediary script that formats & outputs the actual muxrpc to run
	// see query.sh
	cmd := exec.Command(QUERYSCRIPT, strconv.Itoa(id), q)
	var out bytes.Buffer
	var queryLine bytes.Buffer
	cmd.Stderr = &out
	cmd.Stdout = &queryLine
	err := cmd.Run()
	if err != nil {
		return bytes.Buffer{}, TestError{err: err, message: out.String()}
	}
	return runline(queryLine.String())
}

func makeSimulator() Simulator {
	puppetMap := make(map[string]Puppet)
	return Simulator{puppetMap: puppetMap}
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

func (s Simulator) execute() {
	for _, instr := range s.instructions {
		s.updateCurrentInstruction(instr)
		switch instr.command {
		case "start":
			name := instr.args[0]
			go startPuppet(Puppet{name: name, instanceID: s.portCounter})
			time.Sleep(1 * time.Second)
			feedID, err := DoWhoami(s.portCounter)
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			s.puppetMap[name] = Puppet{name: name, feedID: feedID, instanceID: s.portCounter}
			s.incrementPort()
			instr.TestSuccess()
			taplog(fmt.Sprintf("%s has id %s", name, feedID))
			taplog(fmt.Sprintf("logging to log-%s.txt", name))
		case "log":
			srcPuppet := s.getSrcPuppet()
			amount, err := strconv.Atoi(instr.getSecond())
			if err != nil {
				log.Fatalln(err)
			}
			msg, err := DoLog(srcPuppet.instanceID, amount)
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
			err := DoFollowWithGoClient(srcPuppet.instanceID, dstPuppet.feedID, instr.command == "follow")
			s.evaluateRun(err)
		case "isfollowing":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoIsFollowing(srcPuppet.instanceID, srcPuppet.feedID, dstPuppet.feedID)
			s.evaluateRun(err)
		case "isnotfollowing":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoIsNotFollowing(srcPuppet.instanceID, srcPuppet.feedID, dstPuppet.feedID)
			s.evaluateRun(err)
		case "post":
			srcPuppet := s.getSrcPuppet()
			err := DoPostWithGoClient(srcPuppet.instanceID)
			s.evaluateRun(err)
		case "disconnect":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoDisconnect(srcPuppet, dstPuppet)
			s.evaluateRun(err)
		case "connect":
			srcPuppet := s.getSrcPuppet()
			dstPuppet := s.getDstPuppet()
			err := DoConnectWithGoClient(srcPuppet, dstPuppet)
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
	sim := makeSimulator()
	lines := readTest("test.txt")
	sim.ParseTest(lines)
	sim.execute()
}
