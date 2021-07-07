// Prints out a follow graph, as seen from the point of view of the focused peers.
// This command is literally a pared down version of generate-test.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
)

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func pickName(splicedFixturesMap map[string]interface{}) string {
	return splicedFixturesMap["folder"].(string)
}

// Returns a map of follows (id -> slice of ids that are followed), a map of blocks (isBlocking[id][otherId] = true if id blocks otherId)
func getFollowMap(followGraphPath string) (map[string][]string, map[string]map[string]bool, error) {
	// get the json map of all known relations
	b, err := os.ReadFile(followGraphPath)
	if err != nil {
		return nil, nil, err
	}

	// unpack into goland
	var v map[string]map[string]interface{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, nil, err
	}

	followMap := make(map[string][]string)
	blockMap := make(map[string]map[string]bool)
	for id, relations := range v {
		var following []string
		blockMap[id] = make(map[string]bool)
		for relationId, status := range relations {
			if isFollowing, ok := status.(bool); ok {
				if isFollowing {
					following = append(following, relationId)
				} else {
					blockMap[id][relationId] = true
				}
			}
			followMap[id] = following
		}
	}
	return followMap, blockMap, nil
}

func getIdentities(fixturesRoot string) (map[string]string, error) {
	b, err := os.ReadFile(path.Join(fixturesRoot, "secret-ids.json"))
	if err != nil {
		return nil, err
	}
	// secret-ids.json contains a map of ids -> {latest, folder}
	var v map[string]map[string]interface{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	identities := make(map[string]string)
	// however, we only want a mapping from id->foldername, so let's get that
	for id, feedInfo := range v {
		identities[id] = pickName(feedInfo)
	}
	return identities, nil
}

type runtimeArgs struct {
	fixturesRoot   string
	focusedPuppets int
	maxHops        int
}

// uses:
// * expectations.json
// * root folder containing cmd&log-splicer processed fixtures
var idsToNames map[string]string
var focusGroup []string
var isBlocking map[string]map[string]bool
var args runtimeArgs

func main() {
	flag.StringVar(&args.fixturesRoot, "fixtures", "./fixtures-output", "root folder containing spliced out ssb-fixtures")
	flag.IntVar(&args.maxHops, "hops", 2, "the max hops count to use")
	flag.IntVar(&args.focusedPuppets, "focused", 2, "number of puppets to use for focus group (i.e. # of puppets that verify they are replicating others)")
	flag.Parse()

	var err error
	// map of id -> [list of followed ids]
	var followMap map[string][]string
	followMap, isBlocking, err = getFollowMap(path.Join(args.fixturesRoot, "follow-graph.json"))
	check(err)

	idsToNames, err = getIdentities(args.fixturesRoot)
	check(err)

	namesToIDs := make(map[string]string)
	for id, secretFolder := range idsToNames {
		namesToIDs[secretFolder] = id
	}

	// the cohort of peers we care about; the ones who will be issuing `has` stmts, the ones whose data we will inspect
	focusGroup = make([]string, args.focusedPuppets)
	for i := 0; i < args.focusedPuppets; i++ {
		focusGroup[i] = fmt.Sprintf("puppet-%05d", i)
	}
	// deterministically shuffle the focus group
	// TODO: accept --seed flag to change shuffling
	rand.Shuffle(len(focusGroup), func(i, j int) {
		focusGroup[i], focusGroup[j] = focusGroup[j], focusGroup[i]
	})

	getIds := func(src []string) []string {
		extractedIds := make([]string, 0, len(src))
		for _, name := range src {
			extractedIds = append(extractedIds, namesToIDs[name])
		}
		return extractedIds
	}
	focusIds := getIds(focusGroup)
	for _, id := range focusIds {
		fmt.Println(0, idsToNames[id])
		g := graph{followMap: followMap, seen: make(map[string]bool)}
		g.recurseFollows(id, args.maxHops)
		fmt.Println("")
	}
}

type pair struct {
	src, dst string
}

type graph struct {
	followMap map[string][]string
	seen      map[string]bool
}

func (g graph) recurseFollows(id string, hopsLeft int) []pair {
	g.seen[id] = true
	if hopsLeft <= 0 {
		return []pair{}
	}
	var pairs []pair
	for _, otherId := range g.followMap[id] {
		if g.seen[otherId] {
			continue
		}
		if hopsLeft == args.maxHops {
			fmt.Printf("%d %s\n", args.maxHops-hopsLeft+1, idsToNames[otherId])
		} else {
			fmt.Printf("%d %s (via %s)\n", args.maxHops-hopsLeft+1, idsToNames[otherId], idsToNames[id])
		}
		pairs = append(pairs, pair{src: id, dst: otherId})
	}
	for _, otherId := range g.followMap[id] {
		if g.seen[otherId] {
			continue
		}
		pairs = append(pairs, g.recurseFollows(otherId, hopsLeft-1)...)
	}
	return pairs
}
