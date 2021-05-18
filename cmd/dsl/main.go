package main

import (
	// "os"
	"fmt"
	// "log"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
	// "context"
	"os/exec"
	"strconv"
	// "bufio"
)

type Whoami struct {
	ID string
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

type Instruction struct {
	command string
	args    []string
	line    string
	id      int
}

type TestError struct {
	err     error
	message string
}

func (t TestError) Error() string {
	return t.message
}

func (instr Instruction) Print() {
	fmt.Printf("# %d %s\n", instr.id, instr.line)
}

func (instr Instruction) TestSuccess() {
	fmt.Printf("ok %d %s\n", instr.id, instr.line)
}

func (instr Instruction) TestFailure(err error) {
	fmt.Printf("not ok %d %s\n", instr.id, instr.line)
	fmt.Printf("# %s\n", err.Error())
}

func (instr Instruction) getSrc() string {
	return instr.args[0]
}

func (instr Instruction) getFirst() string {
	return instr.args[0]
}

func (instr Instruction) getSecond() string {
	if len(instr.args) > 1 {
		return instr.args[1]
	}
	return ""
}

func (instr Instruction) getDst() string {
	if len(instr.args) > 1 {
		return instr.args[1]
	}
	return ""
}

const (
	PUPPETSCRIPT = "/home/cblgh/code/netsim-experiments/ssb-server/start-nodejs-puppet.sh"
	QUERYSCRIPT  = "/home/cblgh/code/netsim-experiments/ssb-server/query.sh"
)

func startPuppet(id int) error {
	cmd := exec.Command(PUPPETSCRIPT, strconv.Itoa(id))
	var stderr bytes.Buffer
	var out bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return TestError{err: err, message: stderr.String()}
	}
	return nil
}

func run(line string) (bytes.Buffer, error) {
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

func query(id int, q string) (bytes.Buffer, error) {
	cmd := exec.Command(QUERYSCRIPT, strconv.Itoa(id), q)
	var out bytes.Buffer
	var queryLine bytes.Buffer
	cmd.Stderr = &out
	cmd.Stdout = &queryLine
	err := cmd.Run()
	if err != nil {
		return bytes.Buffer{}, TestError{err: err, message: out.String()}
	}
	return run(queryLine.String())
}

func taplog(str string) {
	fmt.Printf("# %s\n", str)
}

func execute(instructions []Instruction) {
	puppetMap := make(map[string]Puppet)
	portCounter := 0
	for _, instr := range instructions {
		switch instr.command {
		case "start":
			go startPuppet(portCounter)
			name := instr.args[0]
			time.Sleep(1 * time.Second)
			feedID, err := DoWhoami(portCounter)
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			taplog(fmt.Sprintf("%s has id %s", name, feedID))
			puppetMap[name] = Puppet{name: name, feedID: feedID, instanceID: portCounter}
			portCounter += 1
			instr.TestSuccess()
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
			src := instr.getSrc()
			dst := instr.getDst()
			srcPuppet := puppetMap[src]
			dstPuppet := puppetMap[dst]
			err := DoFollow(srcPuppet.instanceID, dstPuppet.feedID, instr.command == "follow")
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			instr.TestSuccess()
		case "isfollowing":
			src := instr.getSrc()
			dst := instr.getDst()
			srcPuppet := puppetMap[src]
			dstPuppet := puppetMap[dst]
			err := DoIsFollowing(srcPuppet.instanceID, srcPuppet.feedID, dstPuppet.feedID)
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			instr.TestSuccess()
		case "post":
			src := instr.getSrc()
			srcPuppet := puppetMap[src]
			err := DoPost(srcPuppet.instanceID)
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			instr.TestSuccess()
		case "has":
			src := instr.getSrc()
			arg := strings.Split(instr.getSecond(), "@")
			dst, seq := arg[0], arg[1]
			srcPuppet := puppetMap[src]
			dstPuppet := puppetMap[dst]
			err := DoHast(srcPuppet, dstPuppet, seq)
			if err != nil {
				instr.TestFailure(err)
				continue
			}
			instr.TestSuccess()
		default:
			instr.Print()
		}
	}
}

func getLatestByFeedID(seqnos []Latest, feedID string) Latest {
	for _, seqnoWrapper := range seqnos {
		if seqnoWrapper.ID == feedID {
			return seqnoWrapper
		}
	}
	return Latest{}
}

func queryLatest(src Puppet) ([]Latest, error) {
	response, err := query(src.instanceID, "latest")
	if err != nil {
		return nil, err
	}
	responses := strings.Split(strings.TrimSpace(response.String()), "\n\n")
	seqnos := make([]Latest, 0, len(responses))
	for _, str := range responses {
		var parsed Latest
		json.Unmarshal(bytes.NewBufferString(str).Bytes(), &parsed)
		seqnos = append(seqnos, parsed)
	}
	return seqnos, nil
}

// really bad Rammstein pun, sorry absolutely not sorry
func DoHast(src, dst Puppet, seqno string) error {
	srcLatestSeqs, err := queryLatest(src)
	if err != nil {
		return err
	}
	dstLatestSeqs, err := queryLatest(dst)
	if err != nil {
		return err
	}
	dstBySrc := getLatestByFeedID(srcLatestSeqs, dst.feedID)
	dstByDst := getLatestByFeedID(dstLatestSeqs, dst.feedID)
	var assertedSeqno int
	if seqno == "latest" {
		assertedSeqno = dstByDst.Sequence
	} else {
		assertedSeqno, err = strconv.Atoi(seqno)
		if err != nil {
			m := fmt.Sprintf("expected keyword 'latest' or a numberd\nwas %s", seqno)
			return TestError{err: errors.New("sequence number wasn't a number (or latest)"), message: m}
		}
	}

	if dstBySrc.Sequence == assertedSeqno && dstBySrc.ID == dstByDst.ID {
		return nil
	} else {
		m := fmt.Sprintf("expected sequence: %s at seq %d\nwas sequence %s at seq %d", dstByDst.ID, assertedSeqno, dstBySrc.ID, dstBySrc.Sequence)
		return TestError{err: errors.New("sequences didn't match"), message: m}
	}
	return nil
}

func DoWhoami(instance int) (string, error) {
	response, err := query(instance, "whoami")
	if err != nil {
		return "", err
	}
	var parsed Whoami
	json.Unmarshal(response.Bytes(), &parsed)
	return parsed.ID, nil
}

func DoFollow(instance int, feedID string, isFollow bool) error {
	var followType string
	if !isFollow { // => unfollow message
		followType = "no-"
	}
	followMsg := fmt.Sprintf(`publish --type contact --contact '%s' --%sfollowing`, feedID, followType)
	_, err := query(instance, followMsg)
	if err != nil {
		return err
	}
	return nil
}

func DoIsFollowing(instance int, srcID, dstID string) error {
	msg := fmt.Sprintf(`friends.isFollowing --source %s --dest %s`, srcID, dstID)
	res, err := query(instance, msg)
	if err != nil {
		return err
	}
	isFollowing := strings.TrimSpace(res.String()) == "true"
	if !isFollowing {
		m := fmt.Sprintf("%s did not follow %s", srcID, dstID)
		return TestError{err: errors.New("isfollowing returned false"), message: m}
	}
	return nil
}

func DoPost(instance int) error {
	port := 18888 + instance
	CLI := "/home/cblgh/code/go/src/ssb/cmd/sbotcli/sbotcli"
	cmd := fmt.Sprintf(`%s -addr 192.168.88.18:%d --key ~/code/netsim-experiments/ssb-server/puppet_%s/secret --unixsock '' publish post bep`, CLI, port, instance)
	_, err := run(cmd)
	if err != nil {
		return err
	}
	return nil
}

func DoLatest(instance int) error {
	postMsg := "latest"
	_, err := query(instance, postMsg)
	if err != nil {
		return err
	}
	return nil
}

func parseTestLine(line string, id int) Instruction {
	parts := strings.Fields(strings.ReplaceAll(line, ",", ""))
	return Instruction{command: parts[0], args: parts[1:], line: line, id: id}
}

const testfile = `start alice
start bob
wait 1000
follow alice, bob
follow bob, alice
post alice
post alice
wait 700
post alice
post alice
has bob, alice@latest
has bob, alice@10
wait 100`

func main() {
	lines := strings.Split(testfile, "\n")

	instructions := make([]Instruction, 0, len(lines))
	fmt.Println("## Start test file")
	for i, line := range lines {
		instr := parseTestLine(line, i+1)
		instr.Print()
		instructions = append(instructions, instr)
	}
	fmt.Println("## End test file")

	execute(instructions)
}
