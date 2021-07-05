package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
)

func trimFeedId(feedID string) string {
	s := strings.ReplaceAll(feedID, "@", "")
	return strings.ReplaceAll(s, ".ed25519", "")
}

func multiserverAddr(p *Puppet) string {
	// format: net:localhost:18889~shs:xDPgE3tTTIwkt1po+2GktzdvwJLS37ZEd+TZzIs66UU=
	ip := "localhost"
	return fmt.Sprintf("net:%s:%d~shs:%s", ip, p.Port, trimFeedId(p.feedID))
}

func taplog(str string) {
	if str == "" {
		return
	}
	for _, line := range strings.Split(str, "\n") {
		fmt.Printf("# %s\n", line)
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

func parseTestLine(line string, id int) Instruction {
	parts := strings.Fields(strings.ReplaceAll(line, ",", ""))
	return Instruction{command: parts[0], args: parts[1:], line: line, id: id}
}

func readTest(filename string) []string {
	absfilename, err := filepath.Abs(filename)
	if err != nil {
		log.Fatalln(err)
	}
	testfile, err := ioutil.ReadFile(absfilename)
	if err != nil {
		log.Fatalln(err)
	}
	return strings.Split(strings.TrimSpace(string(testfile)), "\n")
}
