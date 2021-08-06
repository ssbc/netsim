package sim

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
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
	return fmt.Sprintf("net:%s:%d~shs:%s", ip, p.port, trimFeedId(p.feedID))
}

func taplog(str string) {
	if str == "" {
		return
	}
	for _, line := range strings.Split(str, "\n") {
		fmt.Printf("# %s\n", line)
	}
}

func getLatestByFeedID(seqnos []Latest, feedID string) (Latest, bool) {
	for _, seqnoWrapper := range seqnos {
		if seqnoWrapper.ID == feedID {
			return seqnoWrapper, true
		}
	}
	return Latest{}, false
}

func parseTestLine(line string, id int) Instruction {
	parts := strings.Fields(strings.ReplaceAll(line, ",", ""))
	return Instruction{command: parts[0], args: parts[1:], line: line, id: id}
}

func readTest(filename string) []string {
	_, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		bail(fmt.Sprintf("test file %s not found", filename))
	}
	absfilename, err := filepath.Abs(filename)
	if err != nil {
		log.Fatalln(err)
	}
	testfile, err := ioutil.ReadFile(absfilename)
	if err != nil {
		log.Fatalln(err)
	}
	var lines []string
	for _, line := range strings.Split(string(testfile), "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
