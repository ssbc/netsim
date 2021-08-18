// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: LGPL-3.0

package sim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"go.cryptoscope.co/muxrpc/v2"
	refs "go.mindeco.de/ssb-refs"

	"github.com/ssb-ngi-pointer/netsim/client"
)

type Whoami struct {
	ID refs.FeedRef
}

type Latest struct {
	ID       string
	Sequence int
	TS       int
}

func asyncRequest(p *Puppet, method muxrpc.Method, payload, response interface{}) error {
	c, err := client.NewTCP(p.port, p.caps, fmt.Sprintf("%s/secret", p.directory))
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

func sourceRequest(p *Puppet, method muxrpc.Method, opts interface{}) (muxrpc.Endpoint, *muxrpc.ByteSource, error) {
	c, err := client.NewTCP(p.port, p.caps, fmt.Sprintf("%s/secret", p.directory))
	if err != nil {
		return nil, nil, err
	}

	ctx := context.TODO()
	src, err := c.Source(ctx, muxrpc.TypeJSON, method, opts)
	return c, src, err
}

func DoConnect(src, dst *Puppet) error {
	dstMultiAddr := multiserverAddr(dst)

	var response interface{}
	return asyncRequest(src, muxrpc.Method{"conn", "connect"}, dstMultiAddr, &response)
}

func DoDisconnect(src, dst *Puppet) error {
	dstMultiAddr := multiserverAddr(dst)

	var response interface{}
	return asyncRequest(src, muxrpc.Method{"conn", "disconnect"}, dstMultiAddr, &response)
}

func queryLatest(p *Puppet) ([]Latest, error) {
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

func extractSeqno(dst *Puppet, seqno string) (int, string, error) {
	var assertedSeqno int
	var assumption string
	if seqno == "latest" {
		assertedSeqno = dst.seqno
		assumption = fmt.Sprintf("assuming %s@latest => %s@%d", dst.name, dst.name, dst.seqno)
	} else {
		var err error
		assertedSeqno, err = strconv.Atoi(seqno)
		if err != nil {
			m := fmt.Sprintf("expected keyword 'latest' or a number\nwas %s", seqno)
			return -1, "", TestError{err: errors.New("sequence number wasn't a number (or latest)"), message: m}
		}
	}
	return assertedSeqno, assumption, nil
}

// really bad Rammstein pun, sorry (absolutely not sorry)
func DoHast(src, dst *Puppet, seqno string) (string, error) {
	srcLatestSeqs, err := queryLatest(src)
	if err != nil {
		return "", err
	}
	dstViaSrc, has := getLatestByFeedID(srcLatestSeqs, dst.feedID)

	// what if the dst puppet doesn't even know about it
	if !has {
		if seqno == "0" { // the script expected that it wouldn't anyhow
			return "", nil
		}
		m := fmt.Sprintf("expected %s to have %s; it didn't", src.name, dst.name)
		return m, TestError{err: fmt.Errorf("feed not stored by dst"), message: m}
	}

	// get the asserted seqno and a message, if we're inducting a seqno based on available info
	// (i.e. stating what how we're interpreting alice@latest)
	assertedSeqno, message, err := extractSeqno(dst, seqno)
	if err != nil {
		return "", err
	}

	if dstViaSrc.Sequence == assertedSeqno && dstViaSrc.ID == dst.feedID {
		return message, nil
	} else {
		m := fmt.Sprintf("expected: %s at sequence %d\nwas: %s at sequence %d", dst.feedID, assertedSeqno, dstViaSrc.ID, dstViaSrc.Sequence)
		return "", TestError{err: errors.New("sequences didn't match"), message: m}
	}
}

func DoWaitUntil(src, dst *Puppet, seqno string) (string, error) {
	assertedSeqno, message, err := extractSeqno(dst, seqno)
	if err != nil {
		return "", err
	}

	// with these options createHistoryStream blocks on the destination until we receive assertedSeqno (or timeouts)
	_, err = DoCreateHistoryStream(src, dst.feedID, assertedSeqno, true)
	if err != nil {
		m := fmt.Sprintf("%s expected %s@%d", src.name, dst.name, assertedSeqno)
		return "", TestError{err: err, message: m}
	}

	return message, nil
}

func DoCreateHistoryStream(p *Puppet, who string, n int, live bool) (string, error) {
	type histOptions struct {
		ID    string `json:"id"`
		Seq   int    `json:"seq"`
		Live  bool   `json:"live"`
		Limit int    `json:"limit"`
	}

	// only get the last n logs
	opts := histOptions{
		ID:    who,
		Seq:   n,
		Live:  live,
		Limit: 1,
	}
	c, src, err := sourceRequest(p, muxrpc.Method{"createHistoryStream"}, opts)
	if err != nil {
		return "", err
	}
	defer c.Terminate()

	var response []string
	ctx := context.TODO() // TODO get simulation context
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !src.Next(ctx) {
		if err := src.Err(); err != nil {
			return "", fmt.Errorf("createHistStream failed: %w", err)
		}
	}

	err = src.Reader(prettyPrintSourceJSON(&response))
	if err != nil {
		err = fmt.Errorf("failed to read body: %w (mux source error: %v)", err, src.Err())
		return "", err
	}

	return strings.Join(response, "\n"), nil
}

func DoWhoami(p *Puppet) (string, error) {
	var parsed Whoami
	var empty interface{}
	err := asyncRequest(p, muxrpc.Method{"whoami"}, &empty, &parsed)
	if err != nil {
		return "", err
	}
	return parsed.ID.Ref(), nil
}

func DoLog(p *Puppet, n int) (string, error) {
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

	var response []string
	ctx := context.TODO()

	for src.Next(ctx) {
		err = src.Reader(prettyPrintSourceJSON(&response))
		if err != nil {
			return "", err
		}
	}
	return strings.Join(response, "\n"), nil
}

func prettyPrintSourceJSON(response *[]string) func(rd io.Reader) error {
	var genericMap map[string]interface{}

	return func(rd io.Reader) error {
		// read all the json-encoded data as bytes
		b, err := io.ReadAll(rd)
		if err != nil {
			return err
		}
		// decode it into a generic map
		err = json.Unmarshal(b, &genericMap)
		if err != nil {
			return err
		}

		// marshall everything as bytes of json to get the correct indentation, prettifying the output
		b, err = json.MarshalIndent(genericMap, "", " ")
		if err != nil {
			return err
		}

		// encode the json bytes as utf-8 characters
		*response = append(*response, string(b))
		return err
	}
}

func DoFollow(srcPuppet, dstPuppet *Puppet, isFollow bool) error {
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

func DoPost(p *Puppet) error {
	post := refs.NewPost("bep")

	var response string
	return asyncRequest(p, muxrpc.Method{"publish"}, post, &response)
}

func DoPublish(p *Puppet, post map[string]interface{}) error {
	var response string
	return asyncRequest(p, muxrpc.Method{"publish"}, post, &response)
}

func queryIsFollowing(srcPuppet, dstPuppet *Puppet) (bool, error) {
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

func DoIsFollowing(srcPuppet, dstPuppet *Puppet) error {
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

func DoIsNotFollowing(srcPuppet, dstPuppet *Puppet) error {
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
