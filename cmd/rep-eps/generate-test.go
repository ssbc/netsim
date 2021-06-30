// Generates a full netsim test, given a log-splicer processed ssb-fixtures folder and a replication expectations file
// expectations.json.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
	"sort"
)

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func readExpectations(expectationsPath string) (map[string][]string, error) {
	b, err := os.ReadFile(expectationsPath)
	if err != nil {
		return nil, err
	}
	var v map[string][]string
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	return v, nil
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

func getUniques(expectations map[string][]string) []string {
	uniqueMap := make(map[string]bool)
	uniques := make([]string, 0, len(expectations))

	for id, replicatees := range expectations {
		if _, ok := uniqueMap[id]; !ok && len(replicatees) > 0 {
			uniques = append(uniques, id)
			uniqueMap[id] = true
		}
		for _, replicateeId := range replicatees {
			if _, ok := uniqueMap[replicateeId]; !ok {
				uniques = append(uniques, replicateeId)
				uniqueMap[replicateeId] = true
			}
		}
	}
	return uniques
}

type runtimeArgs struct {
	ssbServer        string
	fixturesRoot     string
	expectationsPath string
	batchSize        int
	waitDuration     int
	focusedPuppets   int
}

// uses:
// * expectations.json
// * root folder containing cmd&log-splicer processed fixtures
var currentlyExecuting map[string]bool

var args runtimeArgs

var focusGroup []string
var isBlocking map[string]map[string]bool

func main() {
	currentlyExecuting = make(map[string]bool)
	flag.StringVar(&args.fixturesRoot, "fixtures", "./fixtures-output", "root folder containing spliced out ssb-fixtures")
	flag.StringVar(&args.expectationsPath, "expectations", "./expectations.json", "path to expectations.json")
	flag.StringVar(&args.ssbServer, "sbot", "ssb-server", "the ssb server to start puppets with")
	flag.IntVar(&args.focusedPuppets, "focused", 2, "number of puppets to use for focus group (i.e. # of puppets that verify they are replicating others)")
	flag.IntVar(&args.batchSize, "batch", 3, "number of puppets to run concurrently")
	flag.IntVar(&args.waitDuration, "wait", 2000, "the default wait duration")
	flag.Parse()

	expectations, err := readExpectations(args.expectationsPath)
	check(err)

	// map of id -> [list of followed ids]
	var followMap map[string][]string
	followMap, isBlocking, err = getFollowMap(path.Join(args.fixturesRoot, "follow-graph.json"))
	check(err)

	idsToNames, err := getIdentities(args.fixturesRoot)
	check(err)

	puppetNames := make([]string, 0, len(idsToNames))
	namesToIDs := make(map[string]string)
	for id, secretFolder := range idsToNames {
		namesToIDs[secretFolder] = id
		puppetNames = append(puppetNames, idsToNames[id])
	}
	sort.Strings(puppetNames)

	getNames := func(src []string) []string {
		extractedNames := make([]string, 0, len(src))
		for _, id := range src {
			extractedNames = append(extractedNames, idsToNames[id])
		}
		return extractedNames
	}

	// Note: do not remove, it's a useful help function: )
	getIds := func(src []string) []string {
		extractedIds := make([]string, 0, len(src))
		for _, name := range src {
			extractedIds = append(extractedIds, namesToIDs[name])
		}
		return extractedIds
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

	/* given our starting set of puppets, called focus, and hops = 3, we will want to generate
	the following connection graph:
	focus
				-> hops 1 / direct follows
						      -> hops 2
												    -> hops 3

			 =======================
			  start start start start
			v hops 3 connect hops 2 v
			v hops 2 connect hops 1 v
		  v hops 1 connect focus  v
				done done  done done
	*/
	focusIds := getIds(focusGroup)
	g := graph{followMap: followMap, seen: make(map[string]bool)}
	var hopsPairs []pair
	for _, id := range focusIds {
		hopsPairs = append(hopsPairs, g.recurseFollows(id, 3)...)
	}
	// reverse hopsPairs, so that the pairs the furthest from a focus puppet are at the start of the slice
	// this is important as we want to get as much data as possible when finally syncing the focus puppets
	for i, j := 0, len(hopsPairs)-1; i < j; i, j = i+1, j-1 {
		hopsPairs[i], hopsPairs[j] = hopsPairs[j], hopsPairs[i]
	}

	// init all puppets from the fixtures
	// output `enter`, `load` stmts, sorted by puppet name
	for _, puppetName := range puppetNames {
		puppetId := namesToIDs[puppetName]
		fmt.Printf("enter %s\n", puppetName)
		fmt.Printf("load %s %s\n", puppetName, puppetId)
	}

	// start the focus group
	start(focusGroup)

	// go through each hops pair and connect them, starting with the pairs the furthest away from the focus peers
	for _, pair := range hopsPairs {
		pair.batchConnect(idsToNames)
	}

	// output `has` stmts
	for _, name := range focusGroup {
		focusedId := namesToIDs[name]
		has(name, getNames(expectations[focusedId]))
	}

	stop(focusGroup)
	fmt.Printf("comment total time: %d seconds\n", totalTime/1000)
}

func (p pair) batchConnect(idsToNames map[string]string) {
	if isBlocking[p.dst][p.src] {
		return
	}
	srcName, dstName := idsToNames[p.src], idsToNames[p.dst]
	start([]string{srcName, dstName})
	waitUntil(srcName, []string{srcName})
	connect(srcName, []string{dstName})
	waitUntil(srcName, []string{dstName})
	disconnect(srcName, []string{dstName})
	waitMs(500)
	stop([]string{srcName, dstName})
}

func has(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("has %s %s@latest\n", issuer, name)
	}
}

func disconnect(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("disconnect %s %s\n", issuer, name)
	}
}

func connect(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("connect %s %s\n", issuer, name)
	}
}

func start(names []string) {
	for _, name := range names {
		if _, exists := currentlyExecuting[name]; !exists {
			fmt.Printf("start %s %s\n", name, args.ssbServer)
			currentlyExecuting[name] = true
		}
	}
}

func stop(names []string) {
	for _, name := range names {
		var skip bool
		for _, focusName := range focusGroup {
			if name == focusName {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if _, exists := currentlyExecuting[name]; exists {
			delete(currentlyExecuting, name)
			fmt.Printf("stop %s\n", name)
		}
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

func waitUntil(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("waituntil %s %s@latest\n", issuer, name)
	}
}

var totalTime int

func waitMs(ms int) {
	totalTime += ms
	fmt.Printf("wait %d\n", ms)
}

func wait() {
	totalTime += args.waitDuration
	fmt.Printf("wait %d\n", args.waitDuration)
}
