// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0

package sim

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

func (instr Instruction) TestAbort(err error) {
	fmt.Printf("Bail out! %s (%s)\n", err.Error(), instr.line)
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
func (instr Instruction) first() (string, error) {
	if len(instr.args) == 0 {
		return "", fmt.Errorf("command was missing its first argument (%s:%d)", instr.line, instr.id)
	}
	return instr.args[0], nil
}

func (instr Instruction) second() (string, error) {
	if len(instr.args) < 2 {
		return "", fmt.Errorf("%s was missing its second argument on line %d", instr.command, instr.id)
	}
	return instr.args[1], nil
}
