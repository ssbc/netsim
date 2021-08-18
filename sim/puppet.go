// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0

package sim

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

type Puppet struct {
	directory     string
	feedID        string
	name          string
	caps          string
	secretDir     string
	omitOffset    bool
	allOffsets    bool
	port          int
	hops          int
	seqno         int
	totalMessages int
	totalTime     time.Duration
	slept         time.Duration
	lastStart     time.Time
	process       Process // holds cmd & logfile of a running puppet process
}

func (p Puppet) String() string {
	return fmt.Sprintf("[%s@%d] %s", p.name, p.seqno, p.feedID)
}

func (p *Puppet) stopTimer() {
	if p.lastStart.IsZero() {
		return
	}
	p.totalTime += time.Since(p.lastStart)
	var zero time.Time
	p.lastStart = zero
}

func (p *Puppet) countMessages() error {
	seqnos, err := queryLatest(p)
	if err != nil {
		return err
	}
	count := 0
	for _, seqno := range seqnos {
		count += seqno.Sequence
	}
	p.totalMessages = count
	return nil
}

func (p *Puppet) addSleepDuration(d time.Duration) {
	p.slept += d
}

func (p *Puppet) start(s Simulator, shim string) error {
	filename := filepath.Join(s.puppetDir, fmt.Sprintf("%s.txt", p.name))
	// open the log file and append to it. if it doesn't exist, create it first
	logfile, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// io.MultiWriter is golang's equivalent of running unix pipes with tee
	var writer io.Writer
	writer = logfile
	if s.verbose {
		writer = io.MultiWriter(os.Stdout, logfile)
	}
	if err != nil {
		return TestError{err: err, message: "could not create log file"}
	}
	var cmd *exec.Cmd

	// currently the simulator has a requirement that each language implementation folder must contain a sim-shim.sh file
	// sim-shim.sh contains logic for starting the corresponding sbot correctly.
	// e.g. reading the passed in ssb directory ($1) and port ($2)
	shimPath := filepath.Join(s.implementations[shim], "sim-shim.sh")
	cmd = exec.CommandContext(s.rootCtx, shimPath, p.directory, strconv.Itoa(p.port))

	// the environment variables CAPS and HOPS contains the caps (default: ssb caps) and hops (default: 2) settings for
	// the puppet, and must be set correctly in each implementation's sim-shim.sh
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CAPS=%s", p.caps),
		fmt.Sprintf("HOPS=%d", p.hops))

	if s.fixtures != "" && p.usesFixtures() {
		// pass in LOG_OFFSET and SECRET separately, to allow for using a secret w/ no log.offset.
		// this allows us to simulate when a peer friend-restores their database using only their secret
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("SECRET=%s", filepath.Join(s.fixtures, p.secretDir, "secret")))
		if !p.omitOffset {
			offsetLoc := filepath.Join(s.fixtures, p.secretDir, "flume", "log.offset")
			// this puppet knows about **ALL* historic messages (their log.offset is exactly the same as the input ssb-fixtures)
			if p.allOffsets {
				offsetLoc = filepath.Join(s.fixtures, "puppet-all", "flume", "log.offset")
			}
			cmd.Env = append(cmd.Env, fmt.Sprintf("LOG_OFFSET=%s", offsetLoc))
		}
	}

	cmd.Stderr = writer
	cmd.Stdout = writer
	// store cmd & logfile in puppet for use when we shut it down with e.g. the stop command
	p.process = Process{cmd: cmd, logfile: logfile}
	err = cmd.Start()
	if err != nil {
		return TestError{err: err, message: fmt.Sprintf("failure when creating puppet, see %s for information", filename)}
	}

	return nil
}

func (p *Puppet) stop() error {
	// update the total message count before we stop this puppet
	err := p.countMessages()
	if err != nil {
		taplog(fmt.Sprintf("%s had an error when trying to count db messages (%s)", p.name, err))
	}
	cmd, logfile := p.process.cmd, p.process.logfile
	taplog(fmt.Sprintf("stopping %s (%s)", p.name, p.feedID))
	// issue an interrupt to the process (allows us to do cleanup in sbots)
	// Windows doesn't support Interrupt
	if runtime.GOOS == "windows" {
		cmd.Process.Signal(os.Kill)
	} else {
		cmd.Process.Signal(os.Interrupt)
	}

	// last resort shutdown
	go func() {
		time.Sleep(2 * time.Second)
		_ = cmd.Process.Signal(os.Kill)
	}()

	// wait for the process to wrap up
	err = cmd.Wait()
	if err != nil {
		return TestError{err: err, message: fmt.Sprintf("failure when stopping puppet")}
	}
	// close the logfile
	err = logfile.Close()
	if err != nil {
		return TestError{err: err, message: fmt.Sprintf("failure when closing logfile")}
	}
	p.process = Process{}
	return nil
}

func (p Puppet) usesFixtures() bool {
	return len(p.feedID) > 0 && len(p.secretDir) > 0
}

func (p Puppet) isExecuting() bool {
	return p.process != Process{}
}

func (p *Puppet) bumpSeqno() {
	p.seqno += 1
}
