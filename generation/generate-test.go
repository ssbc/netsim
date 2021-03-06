// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0-or-later

// Generates a full netsim test, given a log-splicer processed ssb-fixtures folder and a replication expectations file
// expectations.json.
package generation

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path"
	"sort"
)

type Args struct {
	SSBServer    string
	FixturesRoot string
	FocusedCount int
	MaxHops      int
	Seed         int64
}

type Generator struct {
	// The cohort of peers we care about; the ones who will be issuing `has` stmts, the ones whose data we will inspect
	FocusGroup         []string
	IDsToNames         map[string]string
	NamesToIDs         map[string]string
	currentlyExecuting map[string]bool
	isBlocking         map[string]map[string]bool
	Args               Args

	Output io.Writer
}

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func PickName(splicedFixturesMap map[string]interface{}) string {
	return splicedFixturesMap["folder"].(string)
}

// Returns a map of follows (id -> slice of ids that are followed), a map of blocks (g.isBlocking[id][otherId] = true if id blocks otherId)
func GetFollowMap(followGraphPath string) (map[string][]string, map[string]map[string]bool, error) {
	// get the json map of all known relations
	b, err := os.ReadFile(followGraphPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read followGraph (%q): %w", followGraphPath, err)
	}

	// unpack into goland
	var v map[string]map[string]interface{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal json from followGraph file (%q): %w", followGraphPath, err)
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

// Produces a map of hex identifiers to the folder storing the puppet's id & log.offset
func GetIdentities(fixturesRoot string) (map[string]string, error) {
	filename := path.Join(fixturesRoot, "secret-ids.json")
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("GetIdentities: could not read %s (%w)", filename, err)
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
		identities[id] = PickName(feedInfo)
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
		extractedNames = append(extractedNames, g.IDsToNames[id])
	}
	return extractedNames
}

func (g Generator) GetIDs(src []string) []string {
	extractedIds := make([]string, 0, len(src))
	for _, name := range src {
		extractedIds = append(extractedIds, g.NamesToIDs[name])
	}
	return extractedIds
}

func GenerateTest(args Args, expectations map[string][]string, outputWriter io.Writer) {
	g := Generator{
		Args:               args,
		currentlyExecuting: make(map[string]bool),
		Output:             outputWriter,
	}

	var err error
	// map of id -> [list of followed ids]
	var followMap map[string][]string
	followMap, g.isBlocking, err = GetFollowMap(path.Join(args.FixturesRoot, "follow-graph.json"))
	check(err)

	// read the puppet name -> id mapping contained in secret-ids.json
	g.IDsToNames, err = GetIdentities(args.FixturesRoot)
	check(err)

	puppetNames := make([]string, 0, len(g.IDsToNames))
	g.NamesToIDs = make(map[string]string)
	for id, secretFolder := range g.IDsToNames {
		g.NamesToIDs[secretFolder] = id
		puppetNames = append(puppetNames, g.IDsToNames[id])
	}
	sort.Strings(puppetNames)

	// the cohort of peers we care about; the ones who will be issuing `has` stmts, the ones whose data we will inspect
	g.FocusGroup = make([]string, args.FocusedCount)
	for i := 0; i < args.FocusedCount; i++ {
		g.FocusGroup[i] = fmt.Sprintf("puppet-%05d", i)
	}
	// deterministically shuffle the focus group
	// TODO: accept --seed flag to change shuffling
	if g.Args.Seed > 0 {
		rand.Seed(g.Args.Seed)
	}
	rand.Shuffle(len(g.FocusGroup), func(i, j int) {
		g.FocusGroup[i], g.FocusGroup[j] = g.FocusGroup[j], g.FocusGroup[i]
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
	focusIds := g.GetIDs(g.FocusGroup)
	var hopsPairs []Pair
	for _, id := range focusIds {
		graph := Graph{FollowMap: followMap, Gen: g, Seen: make(map[string]bool)}
		hopsPairs = append(hopsPairs, graph.RecurseFollows(id, args.MaxHops, false)...)
	}

	// reverse hopsPairs, so that the pairs the furthest from a focus puppet are at the start of the slice
	// this is important as we want to get as much data as possible when finally syncing the focus puppets
	for i, j := 0, len(hopsPairs)-1; i < j; i, j = i+1, j-1 {
		hopsPairs[i], hopsPairs[j] = hopsPairs[j], hopsPairs[i]
	}

	// init all puppets from the fixtures
	// output `enter`, `load` stmts, sorted by puppet name
	for _, puppetName := range puppetNames {
		puppetId := g.NamesToIDs[puppetName]
		fmt.Fprintf(g.Output, "enter %s\n", puppetName)
		fmt.Fprintf(g.Output, "load %s %s\n", puppetName, puppetId)
	}

	// start the focus group
	g.start(g.FocusGroup)

	// go through each hops pair and connect them, starting with the pairs the furthest away from the focus peers
	for _, pair := range hopsPairs {
		g.batchConnect(pair)
	}
	// issue another round of connections to be sure we have flooded the network & receive all data from the hops.
	// short-circuits a scheduling problem by paying with more execution time.
	//
	// the problem:
	// we order the connection statements so that the outermost hops are connected to their followers, and so on until
	// the focused peers connect to their direct follows. the purpose is to trickle down data along the follows & hops and
	// into the focused peers, whose local db we inspect using expectations & `has` statements.
	//
	// in some cases, however, the focused peers will not get an indirect follow's data due to the order connection
	// statements can happen. to solve this, we perform two rounds and are ensured that all data should flow along the
	// hops correctly.
	//
	// the primary reason this happens is because in netsim's spliced out fixtures, each peer initially only holds their
	// own data. when they connect with others, the connecting peer gets the data the other has as well (the peer's own
	// messages, and messages from their previously-connected-with follows)
	for _, pair := range hopsPairs {
		g.batchConnect(pair)
	}

	// output `has` stmts
	for _, name := range g.FocusGroup {
		focusedId := g.NamesToIDs[name]
		g.has(name, g.getNames(expectations[focusedId]))
	}

	g.stop(g.FocusGroup)
}

func (g Generator) batchConnect(p Pair) {
	if g.isBlocking[p.dst][p.src] {
		return
	}
	srcName, dstName := g.IDsToNames[p.src], g.IDsToNames[p.dst]
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
		fmt.Fprintf(g.Output, "has %s %s@latest\n", issuer, name)
	}
}

func (g Generator) disconnect(issuer string, names []string) {
	for _, name := range names {
		fmt.Fprintf(g.Output, "disconnect %s %s\n", issuer, name)
	}
}

func (g Generator) connect(issuer string, names []string) {
	for _, name := range names {
		fmt.Fprintf(g.Output, "connect %s %s\n", issuer, name)
	}
}

func (g Generator) start(names []string) {
	for _, name := range names {
		if _, exists := g.currentlyExecuting[name]; !exists {
			fmt.Fprintf(g.Output, "start %s %s\n", name, g.Args.SSBServer)
			g.currentlyExecuting[name] = true
		}
	}
}

func (g Generator) stop(names []string) {
	for _, name := range names {
		var skip bool
		for _, focusName := range g.FocusGroup {
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
			fmt.Fprintf(g.Output, "stop %s\n", name)
		}
	}
}

type Pair struct {
	src, dst string
}

type Graph struct {
	FollowMap map[string][]string
	Seen      map[string]bool
	Gen       Generator
}

// TODO: change pair to { src: string, dst: []string }?
// the above ^ change can also take care of batching the dst's into correct sized buckets.
// if len(dst) > SOME_MAX, then we push what we already have to the pairs slice and start populating a new slice of
// destinations
func (g Graph) RecurseFollows(id string, hopsLeft int, verbose bool) []Pair {
	g.Seen[id] = true
	if hopsLeft <= 0 {
		return []Pair{}
	}
	var pairs []Pair
	// from the pov of `id`: discover all new, direct follows `otherId`
	for _, otherId := range g.FollowMap[id] {
		// other was already known, continue search for the new in next iteration
		if g.Seen[otherId] {
			continue
		}
		// print out the newly discovered node and where it is in the hops graph
		if verbose {
			// a direct hop from our initial starting point
			if hopsLeft == g.Gen.Args.MaxHops {
				fmt.Fprintf(g.Gen.Output, "%d %s\n", g.Gen.Args.MaxHops-hopsLeft+1, g.Gen.IDsToNames[otherId])
			} else { // an indirect hop, make a note of which node followed the other
				fmt.Fprintf(g.Gen.Output, "%d %s (via %s)\n", g.Gen.Args.MaxHops-hopsLeft+1, g.Gen.IDsToNames[otherId], g.Gen.IDsToNames[id])
			}
		}
		pairs = append(pairs, Pair{src: id, dst: otherId})
	}
	for _, otherId := range g.FollowMap[id] {
		if g.Seen[otherId] {
			continue
		}
		pairs = append(pairs, g.RecurseFollows(otherId, hopsLeft-1, verbose)...)
	}
	return pairs
}

func (g Generator) waitUntil(issuer string, names []string) {
	for _, name := range names {
		fmt.Fprintf(g.Output, "waituntil %s %s@latest\n", issuer, name)
	}
}

func (g Generator) waitMs(ms int) {
	fmt.Fprintf(g.Output, "wait %d\n", ms)
}
