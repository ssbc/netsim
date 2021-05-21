package main

import (
  "fmt"
)

type Instruction struct {
	command string
	args    []string
	line    string
	id      int
}

func (instr Instruction) Print() {
	taplog(fmt.Sprintf("%d %s", instr.id, instr.line))
}

func (instr Instruction) TestSuccess() {
	fmt.Printf("ok %d - %s\n", instr.id, instr.line)
}

func (instr Instruction) TestFailure(err error) {
	fmt.Printf("not ok %d - %s\n", instr.id, instr.line)
	taplog(err.Error())
}

func (instr Instruction) getSrc() string {
	return instr.args[0]
}

func (instr Instruction) getDst() string {
	if len(instr.args) > 1 {
		return instr.args[1]
	}
	return ""
}

// aliases of getSrc/getDst for args that don't correlate to src & dst :)
func (instr Instruction) getFirst() string {
	return instr.args[0]
}

func (instr Instruction) getSecond() string {
	if len(instr.args) > 1 {
		return instr.args[1]
	}
	return ""
}
