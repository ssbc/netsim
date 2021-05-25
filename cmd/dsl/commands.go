package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"go.cryptoscope.co/muxrpc/v2"
	refs "go.mindeco.de/ssb-refs"

	"github.com/ssb-ngi-pointer/netsim/client"
)

func asyncRequest(instance int, method muxrpc.Method, payload, response interface{}) error {
	port := 18888 + instance
	secretFile := fmt.Sprintf(`/home/cblgh/code/netsim-experiments/ssb-server/puppet_%d/secret`, instance)

	c, err := client.NewTCP(port, secretFile)
	if err != nil {
		return err
	}

	ctx := context.TODO()
	err = c.Async(ctx, &response, muxrpc.TypeJSON, method, payload)
	if err != nil {
		return err
	}

	c.Terminate()
	return nil
}

func sourceRequest(instance int, method muxrpc.Method) (muxrpc.Endpoint, *muxrpc.ByteSource, error) {
	secretFile := fmt.Sprintf(`/home/cblgh/code/netsim-experiments/ssb-server/puppet_%d/secret`, instance)

	c, err := client.NewTCP(18888+instance, secretFile)
	if err != nil {
		return nil, nil, err
	}

	ctx := context.TODO()
	src, err := c.Source(ctx, muxrpc.TypeJSON, method)
	return c, src, err
}

func DoConnect(src, dst Puppet) error {
	portDst := 18888 + dst.instanceID
	dstMultiAddr := multiserverAddr(dst.feedID, portDst)

	var response interface{}
	return asyncRequest(src.instanceID, muxrpc.Method{"conn", "connect"}, dstMultiAddr, &response)
}

func DoDisconnect(src, dst Puppet) error {
	portDst := 18888 + dst.instanceID
	dstMultiAddr := multiserverAddr(dst.feedID, portDst)

	var response interface{}
	return asyncRequest(src.instanceID, muxrpc.Method{"conn", "disconnect"}, dstMultiAddr, &response)
}

func queryLatest(p Puppet) ([]Latest, error) {
	c, src, err := sourceRequest(p.instanceID, muxrpc.Method{"latest"})
	defer c.Terminate()
	if err != nil {
		return nil, err
	}

	var seqnos []Latest
	ctx := context.TODO()
	for src.Next(ctx) {
		var l Latest // todo: can switch out Latest with refs.KeyValueRaw / refs.KeyValueMap
		err = src.Reader(func(rd io.Reader) error {
			return json.NewDecoder(rd).Decode(&l)
		})
		if err != nil {
			return nil, err
		}
		seqnos = append(seqnos, l)
	}

	if err := src.Err(); err != nil {
		return nil, err
	}

	return seqnos, nil
}

// really bad Rammstein pun, sorry (absolutely not sorry)
func DoHast(src, dst Puppet, seqno string) error {
	srcLatestSeqs, err := queryLatest(src)
	if err != nil {
		return err
	}
	dstLatestSeqs, err := queryLatest(dst)
	if err != nil {
		return err
	}

	dstViaSrc := getLatestByFeedID(srcLatestSeqs, dst.feedID)
	dstViaDst := getLatestByFeedID(dstLatestSeqs, dst.feedID)

	var assertedSeqno int
	if seqno == "latest" {
		assertedSeqno = dstViaDst.Sequence
	} else {
		assertedSeqno, err = strconv.Atoi(seqno)
		if err != nil {
			m := fmt.Sprintf("expected keyword 'latest' or a numberd\nwas %s", seqno)
			return TestError{err: errors.New("sequence number wasn't a number (or latest)"), message: m}
		}
	}

	if dstViaSrc.Sequence == assertedSeqno && dstViaSrc.ID == dstViaDst.ID {
		return nil
	} else {
		m := fmt.Sprintf("expected sequence: %s at seq %d\nwas sequence %s at seq %d", dstViaDst.ID, assertedSeqno, dstViaSrc.ID, dstViaSrc.Sequence)
		return TestError{err: errors.New("sequences didn't match"), message: m}
	}
	return nil
}

func DoWhoami(p Puppet) (string, error) {
	var parsed Whoami
	var empty interface{}
	err := asyncRequest(p.instanceID, muxrpc.Method{"whoami"}, &empty, &parsed)
	if err != nil {
		return "", err
	}
	return parsed.ID.Ref(), nil
}

func DoLog(p Puppet, n int) (string, error) {
	c, src, err := sourceRequest(p.instanceID, muxrpc.Method{"createLogStream"})
	defer c.Terminate()
	if err != nil {
		return "", err
	}

	var v interface{}
	var response []string
	ctx := context.TODO()
	for src.Next(ctx) {
		err = src.Reader(func(rd io.Reader) error {
			// read all the json-encoded data as bytes
			b, err := io.ReadAll(rd)
			if err != nil {
				return err
			}
			// decode it into a generic struct
			err = json.Unmarshal(b, &v)
			if err != nil {
				return err
			}
			// interpret the struct as a map of [strings -> empty interfaces]
			s := v.(map[string]interface{})
			// marshall everything as bytes of json to get the correct indentation, prettifying the output
			b, err = json.MarshalIndent(s, "", " ")
			if err != nil {
				return err
			}
			// encode the json bytes as utf-8 characters
			response = append(response, string(b))
			return err
		})
		if err != nil {
			return "", err
		}
	}
	length := len(response)
	return strings.Join(response[length-n:length], "\n"), nil
}

func DoFollow(srcPuppet, dstPuppet Puppet, isFollow bool) error {
	feedRef, err := refs.ParseFeedRef(dstPuppet.feedID)
	if err != nil {
		return err
	}

	followContent := refs.NewContactFollow(feedRef)
	followContent.Following = isFollow

	var response interface{}
	err = asyncRequest(srcPuppet.instanceID, muxrpc.Method{"publish"}, followContent, &response)
	return err
}

func DoPost(p Puppet) error {
	post := refs.NewPost("bep")

	var response interface{}
	return asyncRequest(p.instanceID, muxrpc.Method{"publish"}, post, &response)
}

func queryIsFollowing(srcPuppet, dstPuppet Puppet) (bool, error) {
	srcRef, err := refs.ParseFeedRef(srcPuppet.feedID)
	if err != nil {
		return false, err
	}
	dstRef, err := refs.ParseFeedRef(dstPuppet.feedID)
	if err != nil {
		return false, err
	}

	arg := struct {
		Source *refs.FeedRef `json:"source"`
		Dest   *refs.FeedRef `json:"dest"`
	}{Source: srcRef, Dest: dstRef}

	var response interface{}
	err = asyncRequest(srcPuppet.instanceID, muxrpc.Method{"friends", "isFollowing"}, arg, &response)
	if err != nil {
		return false, err
	}
	return response.(bool), nil
}

func DoIsFollowing(srcPuppet, dstPuppet Puppet) error {
	isFollowing, err := queryIsFollowing(srcPuppet, dstPuppet)
	if err != nil {
		return err
	}
	if !isFollowing {
		m := fmt.Sprintf("%s did not follow %s", srcPuppet.feedID, dstPuppet.feedID)
		return TestError{err: errors.New("isfollowing returned false"), message: m}
	}
	return nil
}

func DoIsNotFollowing(srcPuppet, dstPuppet Puppet) error {
	isFollowing, err := queryIsFollowing(srcPuppet, dstPuppet)
	if err != nil {
		return err
	}
	if isFollowing {
    srcID := srcPuppet.feedID
    dstID := dstPuppet.feedID
		m := fmt.Sprintf("%s should not follow %s\nactual: %s is following %s", srcID, dstID, srcID, dstID)
		return TestError{err: errors.New("isfollowing returned true"), message: m}
	}
	return nil
}
