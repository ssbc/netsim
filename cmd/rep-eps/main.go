package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
)

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

type peer struct {
	id      string
	hops    [][]string
	blocked map[string]bool
}

func makePeer(id string) peer {
	p := peer{id: id}
	p.hops = make([][]string, MAX_HOPS+1)
	p.blocked = make(map[string]bool)
	return p
}

func populateHopsAt(count int, peers map[string]peer) {
	for _, my := range peers {
		for _, friendId := range my.hops[count-1] {
			friend := peers[friendId]
			if len(friend.hops) == 0 || len(friend.hops) < count-1 {
				continue
			}
			for _, hopsFollow := range friend.hops[count-1] {
				// don't add blocked peers to hops
				// TODO: toggle this behaviour with flag?
				if _, exists := my.blocked[hopsFollow]; exists {
					continue
				}
				my.hops[count] = append(my.hops[count], hopsFollow)
			}
		}
	}
}

// TODO: should we include hops[0]? i.e. the peer we are inspecting
func collapse(peers map[string]peer) {
	// prune out duplicates when collapsing the map
	collapsedHops := make(map[string]map[string]bool)
	for id, p := range peers {
		for i := 0; i <= MAX_HOPS; i++ {
			collapsedHops[id] = make(map[string]bool)
			for _, otherId := range p.hops[i] {
				collapsedHops[id][otherId] = true
			}
		}
	}

	// massage deduplicated data into a nicer form for later use
	outputMap := make(map[string][]string)
	for id, others := range collapsedHops {
		for otherId := range others {
			outputMap[id] = append(outputMap[id], otherId)
		}
	}

	// persist to disk
	b, err := json.MarshalIndent(outputMap, "", "  ")
	check(err)
	err = os.WriteFile("./expectations.json", b, 0666)
	check(err)
}

var MAX_HOPS int

func main() {
	flag.IntVar(&MAX_HOPS, "hops", 3, "the default global hops setting")
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Println("usage: expectations <path to fixtures folder>")
	}

	graphpath := path.Join(flag.Args()[0], "follow-graph.json")
	b, err := os.ReadFile(graphpath)
	check(err)

	var v map[string]map[string]interface{}
	err = json.Unmarshal(b, &v)
	check(err)

	// start the party by populating hops 0 via interpreting follow-graph.json:
	// nil => can't deduce info
	// true => peer is followed
	// false => peer is blocked
	peers := make(map[string]peer)
	for id, relations := range v {
		p := makePeer(id)
		p.hops[0] = append(p.hops[0], id)
		for relationId, status := range relations {
			if followed, ok := status.(bool); ok {
				// non-nil relations are followed if status is true
				if followed {
					p.hops[1] = append(p.hops[1], relationId)
					// and blocked if false
				} else {
					p.blocked[relationId] = true
				}
			}
		}
		peers[id] = p
	}

	if MAX_HOPS >= 2 {
		for i := 2; i <= MAX_HOPS; i++ {
			populateHopsAt(i, peers)
		}
	}
	collapse(peers)
}
