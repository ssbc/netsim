// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0

package expectations

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

type Args struct {
	MaxHops          int
	ReplicateBlocked bool
	Outpath          string
}

type peer struct {
	id      string
	hops    [][]string
	blocked map[string]bool
}

var output string

func makePeer(args Args, id string) peer {
	p := peer{id: id}
	p.hops = make([][]string, args.MaxHops+1)
	p.blocked = make(map[string]bool)
	return p
}

func populateHopsAt(args Args, count int, peers map[string]peer) {
	for _, my := range peers {
		for _, friendId := range my.hops[count-1] {
			friend := peers[friendId]
			// jump to next iteration if this friend doesn't have any hops in the range, count-1, we are interested in atm
			if len(friend.hops) == 0 || len(friend.hops) < count-1 {
				continue
			}
			for _, hopsFollow := range friend.hops[count-1] {
				// don't add blocked peers to hops
				if _, exists := my.blocked[hopsFollow]; exists && !args.ReplicateBlocked {
					continue
				}
				my.hops[count] = append(my.hops[count], hopsFollow)
			}
		}
	}
}

// collapses the crawled follow structure of all known peers into a single map of id -> ids expected to be replicated
func collapse(args Args, peers map[string]peer, blocked map[string]map[string]bool) map[string][]string {
	// prune out duplicates when collapsing the map
	collapsedHops := make(map[string]map[string]bool)
	for id, p := range peers {
		collapsedHops[id] = make(map[string]bool)
		for i := 0; i <= args.MaxHops; i++ {
			for _, otherId := range p.hops[i] {
				// omit hops[0], i.e. the peer we are inspecting
				if id == otherId {
					continue
				}
				collapsedHops[id][otherId] = true
			}
		}
	}

	// massage deduplicated data into a nicer form for later use
	outputMap := make(map[string][]string)
	for id, others := range collapsedHops {
		for otherId := range others {
			// otherId has blocked id -> we should not expect to replicate them
			if blocked[otherId][id] && !args.ReplicateBlocked {
				continue
			}
			outputMap[id] = append(outputMap[id], otherId)
		}
	}
	return outputMap
}

func PathAndFile(dirpath, name string) string {
	if strings.HasSuffix(dirpath, "json") {
		dirpath = path.Dir(dirpath)
	}
	return path.Join(dirpath, name)
}

func informError(msg string, err error) error {
	return fmt.Errorf("%s (%w)", msg, err)
}

// ProduceExpectations returns a map of id -> ids expected to be replicated
func ProduceExpectations(args Args, graphpath string) (map[string][]string, error) {
	b, err := os.ReadFile(graphpath)
	if err != nil {
		return nil, informError(fmt.Sprintf("couldn't read graph %s", graphpath), err)
	}

	var v map[string]map[string]interface{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, informError("couldn't unmarshal graph", err)
	}

	// start the party by populating hops 0 via interpreting follow-graph.json:
	// nil => can't deduce info
	// true => peer is followed
	// false => peer is blocked
	peers := make(map[string]peer)
	blocked := make(map[string]map[string]bool)
	for id, relations := range v {
		p := makePeer(args, id)
		blocked[id] = make(map[string]bool)
		p.hops[0] = append(p.hops[0], id)
		for relationId, status := range relations {
			if followed, ok := status.(bool); ok {
				// non-nil relations are followed if status is true
				if followed {
					p.hops[1] = append(p.hops[1], relationId)
				} else { // and blocked if false
					p.blocked[relationId] = true
					blocked[id][relationId] = true
				}
			}
		}
		peers[id] = p
	}

	if args.MaxHops >= 2 {
		for i := 2; i <= args.MaxHops; i++ {
			populateHopsAt(args, i, peers)
		}
	}
	outputMap := collapse(args, peers, blocked)
	return outputMap, nil
}
