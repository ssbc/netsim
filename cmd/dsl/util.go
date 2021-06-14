package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strings"
)

func trimFeedId(feedID string) string {
	s := strings.ReplaceAll(feedID, "@", "")
	return strings.ReplaceAll(s, ".ed25519", "")
}

func multiserverAddr(p Puppet) string {
	// format: net:localhost:18889~shs:xDPgE3tTTIwkt1po+2GktzdvwJLS37ZEd+TZzIs66UU=
	ip := "localhost"
	return fmt.Sprintf("net:%s:%d~shs:%s", ip, p.Port, trimFeedId(p.feedID))
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

// matches (type post) (value.content hello) (channel ssb-help)
var postPattern = regexp.MustCompile(`\((\S+)\s(.*?)\)`)

/* TODO:
* implement number parsing (strconv attempt), boolean parsing (if true|false => bool it up)
 */
func parsePostLine(line string) map[string]interface{} {
	matched := postPattern.FindAllStringSubmatch(line, -1)
	postMap := make(map[string]interface{})

	for _, match := range matched {
		key, content := match[1], match[2]
		// post contains a nested key value, e.g. (value.content hello) => should produce json { "value": { "content": "hello" } }
		if strings.ContainsAny(key, ".") {
			keys := strings.Split(key, ".")
			m := postMap
			for _, keyPart := range keys[:len(keys)-1] {
				m[keyPart] = make(map[string]interface{})
				m = m[keyPart].(map[string]interface{})
			}
			m[keys[len(keys)-1]] = content
		} else {
			// this group contained a regular key:value mapping (type post) => { "type": "post" }
			postMap[key] = content
		}
	}

	return postMap
}
