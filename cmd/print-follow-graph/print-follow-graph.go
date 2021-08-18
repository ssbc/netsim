// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0

// Prints out a follow graph, as seen from the point of view of the focused peers.
// This command is literally a pared down version of generate-test.go
package main

import (
	"flag"
	"fmt"
	"github.com/ssb-ngi-pointer/netsim/generation"
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

func main() {
	var args generation.Args
	flag.IntVar(&args.MaxHops, "hops", 2, "the max hops count to use")
	flag.IntVar(&args.FocusedCount, "focused", 2, "number of puppets to use for focus group (i.e. # of puppets that verify they are replicating others)")
	flag.Parse()
	if len(flag.Args()) == 0 {
		fmt.Printf("print-follow-graph <options> path-to-spliced-fixtures\n")
		fmt.Println("prints the follow graph described by a spliced out fixtures folder (netsim generate output), and its replication expectations")
		fmt.Println("Options:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	args.FixturesRoot = flag.Args()[0]

	var err error
	g := generation.Generator{Args: args, Output: os.Stdout}
	// map of id -> [list of followed ids]
	var followMap map[string][]string
	followMap, _, err = generation.GetFollowMap(path.Join(args.FixturesRoot, "follow-graph.json"))
	check(err)

	g.IDsToNames, err = generation.GetIdentities(args.FixturesRoot)
	check(err)

	puppetNames := make([]string, 0, len(g.IDsToNames))
	g.NamesToIDs = make(map[string]string)
	// map puppet names to their ids with g.NamesToIDs
	for id, secretFolder := range g.IDsToNames {
		g.NamesToIDs[secretFolder] = id
		puppetNames = append(puppetNames, g.IDsToNames[id])
	}
	sort.Strings(puppetNames)

	// g.FocusGroup is the cohort of peers we care about; the ones who will be issuing `has` stmts, the ones whose data we
	// will inspect
	g.FocusGroup = make([]string, args.FocusedCount)
	for i := 0; i < args.FocusedCount; i++ {
		g.FocusGroup[i] = fmt.Sprintf("puppet-%05d", i)
	}
	// deterministically shuffle the focus group
	// TODO: accept --seed flag to change shuffling
	rand.Shuffle(len(g.FocusGroup), func(i, j int) {
		g.FocusGroup[i], g.FocusGroup[j] = g.FocusGroup[j], g.FocusGroup[i]
	})

	/* given our starting set of puppets, called focus, and hops = 3, we will want to generate
	the following connection graph:
	focus -> hops 1 -> hops 2 -> hops 3

			 ========================
			  start start start start
			v hops 3 > connect > hops 2 v
			v hops 2 > connect > hops 1 v
			v hops 1 > connect > focus  v
			  done done done done done
			 ========================
	*/
	focusIds := g.GetIDs(g.FocusGroup)
	var hopsPairs []generation.Pair
	for _, id := range focusIds {
		fmt.Println(0, g.IDsToNames[id])
		graph := generation.Graph{FollowMap: followMap, Gen: g, Seen: make(map[string]bool)}
		hopsPairs = append(hopsPairs, graph.RecurseFollows(id, args.MaxHops, true)...)
		fmt.Println("")
	}
}
