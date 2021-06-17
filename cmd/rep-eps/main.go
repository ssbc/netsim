package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	// "strings"
)

const MAX_HOPS = 2

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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: expectations <path to fixtures folder>")
	}
	graphpath := path.Join(os.Args[1], "follow-graph.json")
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
		for relationId, status := range relations {
			if followed, ok := status.(bool); ok {
				// non-nil relations are followed if status is true
				if followed {
					p.hops[0] = append(p.hops[0], relationId)
					// and blocked if false
				} else {
					p.blocked[relationId] = true
				}
			}
		}
		peers[id] = p
	}
	for i := 1; i <= MAX_HOPS; i++ {
		populateHopsAt(i, peers)
	}
}
