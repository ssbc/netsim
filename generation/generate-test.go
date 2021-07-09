// Generates a full netsim test, given a log-splicer processed ssb-fixtures folder and a replication expectations file
// expectations.json.
package generation

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
	"sort"
)

type Args struct {
	SSBServer        string
	FixturesRoot     string
	ExpectationsPath string
	FocusedCount     int
	MaxHops          int
}

type Generator struct {
	focusGroup         []string
	idsToNames         map[string]string
	namesToIDs         map[string]string
	currentlyExecuting map[string]bool
	isBlocking         map[string]map[string]bool
	args               Args
}

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

// Returns a map of follows (id -> slice of ids that are followed), a map of blocks (g.isBlocking[id][otherId] = true if id blocks otherId)
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

// a couple useful helper functions :)
func (g Generator) getNames(src []string) []string {
	extractedNames := make([]string, 0, len(src))
	for _, id := range src {
		extractedNames = append(extractedNames, g.idsToNames[id])
	}
	return extractedNames
}

func (g Generator) getIds(src []string) []string {
	extractedIds := make([]string, 0, len(src))
	for _, name := range src {
		extractedIds = append(extractedIds, g.namesToIDs[name])
	}
	return extractedIds
}

// TO DO:
// * DONE		finish refactoring cli tool to use Generator struct instead of globals
// * DONE		rewrite batch connect to use generator, and pass in a pair?
// * think about how to pass in runtime args properly (document + export runtime args? try cryptix's functional pattern?)
// * write tests to make senpai happy :^)
// * fprintf to a slice or something, return the string from generateTest, then echo it in cli tool usecase
// * some kind of workflow where we pass expectations as a slice of data? maybe not

func GenerateTest(args Args) {
	g := Generator{args: args, currentlyExecuting: make(map[string]bool)}
	expectations, err := readExpectations(args.ExpectationsPath)
	check(err)

	// map of id -> [list of followed ids]
	var followMap map[string][]string
	followMap, g.isBlocking, err = getFollowMap(path.Join(args.FixturesRoot, "follow-graph.json"))
	check(err)

	g.idsToNames, err = getIdentities(args.FixturesRoot)
	check(err)

	puppetNames := make([]string, 0, len(g.idsToNames))
	g.namesToIDs = make(map[string]string)
	for id, secretFolder := range g.idsToNames {
		g.namesToIDs[secretFolder] = id
		puppetNames = append(puppetNames, g.idsToNames[id])
	}
	sort.Strings(puppetNames)

	// the cohort of peers we care about; the ones who will be issuing `has` stmts, the ones whose data we will inspect
	g.focusGroup = make([]string, args.FocusedCount)
	for i := 0; i < args.FocusedCount; i++ {
		g.focusGroup[i] = fmt.Sprintf("puppet-%05d", i)
	}
	// deterministically shuffle the focus group
	// TODO: accept --seed flag to change shuffling
	rand.Shuffle(len(g.focusGroup), func(i, j int) {
		g.focusGroup[i], g.focusGroup[j] = g.focusGroup[j], g.focusGroup[i]
	})

	/* given our starting set of puppets, called focus, and hops = 3, we will want to generate
	the following connection graph:
	focus -> hops 1 -> hops 2 -> hops 3

			 ========================
			  start start start start
			v hops 3 connect hops 2 v
			v hops 2 connect hops 1 v
			v hops 1 connect focus  v
			  done done  done done
			 ========================
	*/
	focusIds := g.getIds(g.focusGroup)
	var hopsPairs []pair
	for _, id := range focusIds {
		g := graph{followMap: followMap, seen: make(map[string]bool)}
		hopsPairs = append(hopsPairs, g.recurseFollows(id, args.MaxHops)...)
	}

	// reverse hopsPairs, so that the pairs the furthest from a focus puppet are at the start of the slice
	// this is important as we want to get as much data as possible when finally syncing the focus puppets
	for i, j := 0, len(hopsPairs)-1; i < j; i, j = i+1, j-1 {
		hopsPairs[i], hopsPairs[j] = hopsPairs[j], hopsPairs[i]
	}

	// init all puppets from the fixtures
	// output `enter`, `load` stmts, sorted by puppet name
	for _, puppetName := range puppetNames {
		puppetId := g.namesToIDs[puppetName]
		fmt.Printf("enter %s\n", puppetName)
		fmt.Printf("load %s %s\n", puppetName, puppetId)
	}

	// start the focus group
	g.start(g.focusGroup)

	// go through each hops pair and connect them, starting with the pairs the furthest away from the focus peers
	for _, pair := range hopsPairs {
		g.batchConnect(pair)
	}

	// output `has` stmts
	for _, name := range g.focusGroup {
		focusedId := g.namesToIDs[name]
		g.has(name, g.getNames(expectations[focusedId]))
	}

	g.stop(g.focusGroup)
}

func (g Generator) batchConnect(p pair) {
	if g.isBlocking[p.dst][p.src] {
		return
	}
	srcName, dstName := g.idsToNames[p.src], g.idsToNames[p.dst]
	batchPair := []string{srcName, dstName}
	dst := []string{dstName}
	g.start(batchPair)
	g.waitUntil(srcName, []string{srcName})
	g.connect(srcName, dst)
	g.waitUntil(srcName, dst)
	g.disconnect(srcName, dst)
	g.stop(batchPair)
}

func (g Generator) has(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("has %s %s@latest\n", issuer, name)
	}
}

func (g Generator) disconnect(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("disconnect %s %s\n", issuer, name)
	}
}

func (g Generator) connect(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("connect %s %s\n", issuer, name)
	}
}

func (g Generator) start(names []string) {
	for _, name := range names {
		if _, exists := g.currentlyExecuting[name]; !exists {
			fmt.Printf("start %s %s\n", name, g.args.SSBServer)
			g.currentlyExecuting[name] = true
		}
	}
}

func (g Generator) stop(names []string) {
	for _, name := range names {
		var skip bool
		for _, focusName := range g.focusGroup {
			if name == focusName {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if _, exists := g.currentlyExecuting[name]; exists {
			delete(g.currentlyExecuting, name)
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

// TODO: change pair to { src: string, dst: []string }?
// the above ^ change can also take care of batching the dst's into correct sized buckets.
// if len(dst) > SOME_MAX, then we push what we already have to the pairs slice and start populating a new slice of
// destinations
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

func (g Generator) waitUntil(issuer string, names []string) {
	for _, name := range names {
		fmt.Printf("waituntil %s %s@latest\n", issuer, name)
	}
}

func (g Generator) waitMs(ms int) {
	fmt.Printf("wait %d\n", ms)
}
