package parser

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type variant struct {
	input  string
	output map[string]interface{}
}

func TestParsing(t *testing.T) {
	a := assert.New(t)

	cases := make([]variant, 0)
	cases = append(cases, easyVariant())
	cases = append(cases, nestedVariant())

	for _, v := range cases {
		res := ParsePostLine(v.input)
		a.Equal(res, v.output)
	}
}

func easyVariant() variant {
	return variant{
		input: "(type post) (text hello) (channel ssb-help)",
		output: map[string]interface{}{
			"type":    "post",
			"channel": "ssb-help",
			"text":    "hello",
		},
	}
}

func nestedVariant() variant {
	return variant{
		input: "(type post) (value.content hello) (channel ssb-help)",
		output: map[string]interface{}{
			"type":    "post",
			"channel": "ssb-help",
			"value": map[string]interface{}{
				"content": "hello",
			},
		},
	}
}
