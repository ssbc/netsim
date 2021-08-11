package expectations

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

type testcase struct {
	origin string
	hops   int
	output []string
}

/* expected replications for various hops:
hops 1
twk -> rm1
twk -> tsa

hops 2
twk -> rm1
twk -> tsa
(via rm1) twk -> dx2
(via tsa) twk -> 3ei
*/

func TestHops(t *testing.T) {
	a := assert.New(t)
	b, err := os.ReadFile("./follow-graph.json")
	a.Empty(err, "os.ReadFile could not read follow graph")
	var v map[string]interface{}
	err = json.Unmarshal(b, &v)
	a.Empty(err, "reading follow-graph.json failed")
	a.True(len(v) > 0, "follow graph does not contain content")

	tests := []testcase{
		testcase{
			origin: "@TWKY4Bq5ezVqbXV2D7NyODxCXgu8o4rgp/sf1GdHbCw=.ed25519",
			hops:   1,
			output: []string{"@Rm1v78lIH1FulC+S+eD8k6Y2hLtlY4rqk8DyuuMzAUg=.ed25519",
				"@TsAbcGkt/u7Gx9KcWPeASPdT5o0hv0B4GjvZ+bMMYj8=.ed25519"},
		},
		testcase{
			origin: "@TWKY4Bq5ezVqbXV2D7NyODxCXgu8o4rgp/sf1GdHbCw=.ed25519",
			hops:   2,
			output: []string{"@Rm1v78lIH1FulC+S+eD8k6Y2hLtlY4rqk8DyuuMzAUg=.ed25519",
				"@TsAbcGkt/u7Gx9KcWPeASPdT5o0hv0B4GjvZ+bMMYj8=.ed25519", "@dx2U7d+hNdCQBoL/hiXZSYbpDusv74eDI8Il72ZTDPo=.ed25519", "@3eIPXptMcbrFPw8seKbywmPbRogERylNuoFVaZ9AlOg=.ed25519"},
		},
	}

	for _, test := range tests {
		hopsMap, err := ProduceExpectations(Args{MaxHops: test.hops}, "follow-graph.json")
		a.Empty(err, "ProduceExpectations had an error")

		expectedFeeds, ok := hopsMap[test.origin]
		a.True(ok, fmt.Sprintf("%s did not exist in follow graph", test.origin))

		a.EqualValues(len(test.output), len(expectedFeeds), fmt.Sprintf("expectations for hops %d did not match asserted output", test.hops))
		for i := range expectedFeeds {
			a.EqualValues(expectedFeeds[i], test.output[i])
		}
	}
}
