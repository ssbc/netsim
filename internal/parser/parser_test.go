package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type variant struct {
	input  string
	output map[string]interface{}
}

func TestParsing(t *testing.T) {
	a := assert.New(t)

	var cases = []variant{
		easyVariant,
		nestedVariant,
		brokenVariant1,
		// brokenVariant2,
	}

	for _, v := range cases {
		res := ParsePostLine(v.input)
		a.Equal(v.output, res)
	}
}

var easyVariant = variant{
	input: "(type post) (text hello) (channel ssb-help)",
	output: map[string]interface{}{
		"type":    "post",
		"channel": "ssb-help",
		"text":    "hello",
	},
}

var nestedVariant = variant{
	input: "(type post) (value.content hello) (channel ssb-help)",
	output: map[string]interface{}{
		"type":    "post",
		"channel": "ssb-help",
		"value": map[string]interface{}{
			"content": "hello",
		},
	},
}

var brokenVariant1 = variant{
	input:  "(what) (if)",
	output: map[string]interface{}{},
}
