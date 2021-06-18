// Package parser parses the lisp-like syntax for defining custom SSB messages, written in combination with the
// `publish` command.
package parser

import (
	"regexp"
	"strings"
)

// matches (type post) (value.content hello) (channel ssb-help)
var postPattern = regexp.MustCompile(`\((\S+)\s([^()]+?)\)`)

/* TODO:
* implement number parsing (strconv attempt), boolean parsing (if true|false => bool it up)
 */
func ParsePostLine(line string) map[string]interface{} {
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
