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

func asyncRequest(p Puppet, method muxrpc.Method, payload, response interface{}) error {
	c, err := client.NewTCP(p.Port, p.caps, fmt.Sprintf("%s/secret", p.directory))
	if err != nil {
		return err
	}

	ctx := context.TODO()
	muxEncodingType := muxrpc.TypeJSON
	if method[0] == "publish" {
		muxEncodingType = muxrpc.TypeString
	}
	err = c.Async(ctx, response, muxEncodingType, method, payload)
	if err != nil {
		return err
	}

	c.Terminate()
	return nil
}

func sourceRequest(p Puppet, method muxrpc.Method, opts interface{}) (muxrpc.Endpoint, *muxrpc.ByteSource, error) {
	c, err := client.NewTCP(p.Port, p.caps, fmt.Sprintf("%s/secret", p.directory))
	if err != nil {
		return nil, nil, err
	}

	ctx := context.TODO()
	src, err := c.Source(ctx, muxrpc.TypeJSON, method, opts)
	return c, src, err
}

func DoConnect(src, dst Puppet) error {
	dstMultiAddr := multiserverAddr(dst)

	var response interface{}
	return asyncRequest(src, muxrpc.Method{"conn", "connect"}, dstMultiAddr, &response)
}

func DoDisconnect(src, dst Puppet) error {
	dstMultiAddr := multiserverAddr(dst)

	var response interface{}
	return asyncRequest(src, muxrpc.Method{"conn", "disconnect"}, dstMultiAddr, &response)
}

// TODO: use createLogStream opts here, as we use them in DoLog?
func queryLatest(p Puppet) ([]Latest, error) {
	var empty interface{}
	c, src, err := sourceRequest(p, muxrpc.Method{"replicate", "upto"}, empty)
	if err != nil {
		return nil, err
	}
	defer c.Terminate()

	var seqnos []Latest
	ctx := context.TODO()
	for src.Next(ctx) {
		var l Latest
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
		m := fmt.Sprintf("expected: %s at sequence %d\nwas: %s at sequence %d", dstViaDst.ID, assertedSeqno, dstViaSrc.ID, dstViaSrc.Sequence)
		return TestError{err: errors.New("sequences didn't match"), message: m}
	}
}

func DoWhoami(p Puppet) (string, error) {
	var parsed Whoami
	var empty interface{}
	err := asyncRequest(p, muxrpc.Method{"whoami"}, &empty, &parsed)
	if err != nil {
		return "", err
	}
	return parsed.ID.Ref(), nil
}

func DoLog(p Puppet, n int) (string, error) {
	type sourceOptions struct {
		Limit   int  `json:"limit"`
		Reverse bool `json:"reverse"`
	}

	// only get the last n logs
	opts := sourceOptions{Limit: n, Reverse: true}
	c, src, err := sourceRequest(p, muxrpc.Method{"createLogStream"}, opts)
	if err != nil {
		return "", err
	}
	defer c.Terminate()

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
	return strings.Join(response, "\n"), nil
}

func DoFollow(srcPuppet, dstPuppet Puppet, isFollow bool) error {
	feedRef, err := refs.ParseFeedRef(dstPuppet.feedID)
	if err != nil {
		return err
	}

	followContent := refs.NewContactFollow(feedRef)
	followContent.Following = isFollow

	var response string
	err = asyncRequest(srcPuppet, muxrpc.Method{"publish"}, followContent, &response)
	return err
}

func DoPost(p Puppet) error {
	post := refs.NewPost("bep")

	var response string
	return asyncRequest(p, muxrpc.Method{"publish"}, post, &response)
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
	err = asyncRequest(srcPuppet, muxrpc.Method{"friends", "isFollowing"}, arg, &response)
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
		srcID, dstID := srcPuppet.feedID, dstPuppet.feedID
		m := fmt.Sprintf("%s should not follow %s\nactual: %s is following %s", srcID, dstID, srcID, dstID)
		return TestError{err: errors.New("isfollowing returned true"), message: m}
	}
	return nil
}
