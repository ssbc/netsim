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

const MAX_COUNT = 4
const WAIT_DURATION = 2000
const BATCH_SIZE = 3
const SSB_SERVER = "ssb-server-REPLACEME"

// uses:
// * expectations.json
// * root folder containing cmd&log-splicer processed fixtures
var currentlyExecuting map[string]bool

func main() {
	currentlyExecuting = make(map[string]bool)
	var fixturesRoot string
	flag.StringVar(&fixturesRoot, "fixtures", "./fixtures-output", "root folder containing spliced out ssb-fixtures")
	var expectationsPath string
	flag.StringVar(&expectationsPath, "expectations", "./expectations.json", "path to expectations.json")
	flag.Parse()

	expectations, err := readExpectations(expectationsPath)
	check(err)

	idsToNames, err := getIdentities(fixturesRoot)
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
	// getIds := func (src []string) []string {
	// 	extractedIds := make([]string, 0, len(src))
	// 	for _, name := range src {
	// 		extractedIds = append(extractedIds, namesToIDs[name])
	// 	}
	// 	return extractedIds
	// }

	// the cohort of peers we care about; the ones who will be issuing `has` stmts, the ones whose data we will inspect
	focusGroup := make([]string, MAX_COUNT)
	for i := 0; i < MAX_COUNT; i++ {
		focusGroup[i] = fmt.Sprintf("puppet-%05d", i)
	}
	// deterministically shuffle the focus group
	// TODO: accept --seed flag to change shuffling
	rand.Shuffle(len(focusGroup), func(i, j int) {
		focusGroup[i], focusGroup[j] = focusGroup[j], focusGroup[i]
	})

	// puppetIds := getUniques(expectations)

	// output `enter`, `load` stmts, sorted by puppet name
	for _, puppetName := range puppetNames {
		puppetId := namesToIDs[puppetName]
		fmt.Printf("enter %s\n", puppetName)
		fmt.Printf("load %s %s\n", puppetName, puppetId)
	}

	// start the focus group
	start(focusGroup)

	// batch start, connect & disconnect from each focused puppet to the peers they are expected to replicate
	// TODO: when this is confirmed to work, change the behaviour to only connect the followed peers (instead of all peers they are expected to
	// replicate)
	for _, focusedName := range focusGroup {
		focusedId := namesToIDs[focusedName]
		replicateeNames := getNames(expectations[focusedId])
		batchConnect(focusedName, replicateeNames)
	}

	// output `has` stmts
	for _, name := range focusGroup {
		focusedId := namesToIDs[name]
		has(name, getNames(expectations[focusedId]))
	}

	stop(focusGroup)
	fmt.Printf("\ntotal time: %d seconds\n", totalTime/1000)
}

// batching logic for connections from each focused puppet to their expected replicatees
func batchConnect(issuer string, replicateeNames []string) {
	var count int
	var finished bool
	var endRange int

	for {
		startRange := count * BATCH_SIZE
		endRange = (count + 1) * BATCH_SIZE
		if endRange >= len(replicateeNames) {
			endRange = len(replicateeNames)
			finished = true
		}

		subset := replicateeNames[startRange:endRange]

		start(subset)
		connect(issuer, subset)
		wait()

		disconnect(issuer, subset)
		wait()
		stop(subset)

		if finished {
			break
		}
		count += 1
	}
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
			fmt.Printf("start %s %s\n", name, SSB_SERVER)
			currentlyExecuting[name] = true
		}
	}
}

func stop(names []string) {
	for _, name := range names {
		if _, exists := currentlyExecuting[name]; exists {
			delete(currentlyExecuting, name)
			fmt.Printf("stop %s\n", name)
		}
	}
}

var totalTime int

func wait() {
	totalTime += WAIT_DURATION
	fmt.Printf("wait %d\n", WAIT_DURATION)
}
