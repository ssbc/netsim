// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0-or-later

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
		disallowedVariant1,
		// disallowedVariant2,
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

var disallowedVariant1 = variant{
	input:  "(what) (if) (beep)",
	output: map[string]interface{}{},
}
