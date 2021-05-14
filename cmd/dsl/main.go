package main

import (
  // "os"
  "fmt"
  // "log"
  "time"
  "bytes"
  "encoding/json"
  "strings"
  "os/exec"
  "strconv"
  // "bufio"
)

type Whoami struct {
  ID string
}

type Puppet struct {
  feedID  string
  name    string
  instanceID int
  seqno   int64
}

type Instruction struct {
  command string
  args    []string
  line string
  id int
}

type TestError struct {
  err error
  message string
}

func (t TestError) Error () string {
  return t.message
}

func (instr Instruction) Print () {
  fmt.Printf("# %d %s\n", instr.id, instr.line)
}

func (instr Instruction) TestSuccess () {
  fmt.Printf("ok %d %s\n", instr.id, instr.line)
}

func (instr Instruction) TestFailure (err error) {
  fmt.Printf("not ok %d %s\n", instr.id, instr.line)
  fmt.Printf("# %s", err.Error())
}

func (instr Instruction) getSrc () string {
  return instr.args[0]
}

func (instr Instruction) getDst () string {
  if len(instr.args) > 1 {
    return instr.args[1]
  }
  return ""
}

const (
  PUPPETSCRIPT="/home/cblgh/code/netsim-experiments/ssb-server/start-nodejs-puppet.sh"
  QUERYSCRIPT="/home/cblgh/code/netsim-experiments/ssb-server/query.sh"
)

func startPuppet (id int) error {
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

func run (line string) (bytes.Buffer, error) {
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

func query (id int, q string) (bytes.Buffer, error) {
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

func execute (instructions []Instruction) {
  puppetMap := make(map[string]Puppet)
  portCounter := 0
  for _, instr := range instructions {
    switch instr.command {
    case "start":
      go startPuppet(portCounter)
      name := instr.args[0]
      time.Sleep(1 * time.Second)
      feedID, err := DoWhoami(portCounter, name)
      if err != nil {
        instr.TestFailure(err)
        continue
      }
      puppetMap[name] = Puppet{name: name, feedID: feedID, instanceID: portCounter}
      portCounter += 1
      instr.TestSuccess()
    case "follow":
      src := instr.getSrc()
      dst := instr.getDst()
      srcPuppet := puppetMap[src]
      dstPuppet := puppetMap[dst]
      err := DoFollow(srcPuppet.instanceID, dstPuppet.feedID)
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
    default:
      instr.Print()
    }
  }
}

func DoWhoami(instance int, name string) (string, error) {
  response, err := query(instance, "whoami")
  if err != nil {
    return "", err
  }
  var parsed Whoami
  json.Unmarshal(response.Bytes(), &parsed)
  return parsed.ID, nil
}

func DoFollow(instance int, feedID string) error {
  followMsg := fmt.Sprintf(`publish --type contact --contact '%s' --following`, feedID)
  _, err := query(instance, followMsg)
  if err != nil {
    return err
  }
  return nil
}

func DoPost (instance int) error {
  postMsg := "publish --type post --text 'hello hello! this is 0th'"
  _, err := query(instance, postMsg)
  if err != nil {
    return err
  }
  return nil
}

func parseTestLine (line string, id int) Instruction {
  parts := strings.Fields(line)
  return Instruction{command: parts[0], args: parts[1:], line: line, id: id}
}

const testfile = `start alice
start bob
follow alice, bob
post bob`

func main () {
  lines := strings.Split(testfile, "\n")

  instructions := make([]Instruction, 0, len(lines))
  fmt.Println("# Test file")
  for i, line := range lines {
    instr := parseTestLine(line, i+1)
    instr.Print()
    instructions = append(instructions, instr)
  }

  execute(instructions)
}
