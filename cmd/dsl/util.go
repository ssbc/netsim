package main

import (
  "fmt"
  "io/ioutil"
  "strings"
  "log"
)

func trimFeedId(feedID string) string {
	s := strings.ReplaceAll(feedID, "@", "")
	return strings.ReplaceAll(s, ".ed25519", "")
}

func multiserverAddr(feedID string, port int) string {
	// format: net:192.168.88.18:18889~shs:xDPgE3tTTIwkt1po+2GktzdvwJLS37ZEd+TZzIs66UU=
	ip := "192.168.88.18"
	return fmt.Sprintf("net:%s:%d~shs:%s", ip, port, trimFeedId(feedID))
}

func taplog(str string) {
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

// Splits a string of json items into a slice of strings, each containing a json item
func splitResponses(response string) []string {
	return strings.Split(strings.TrimSpace(response), "\n\n")
}

func parseTestLine(line string, id int) Instruction {
	parts := strings.Fields(strings.ReplaceAll(line, ",", ""))
	return Instruction{command: parts[0], args: parts[1:], line: line, id: id}
}

func readTest(filename string) []string {
  testfile, err := ioutil.ReadFile(filename)
  if err != nil {
    log.Fatalln(err)
  }
  return strings.Split(strings.TrimSpace(string(testfile)), "\n")
}

