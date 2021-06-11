// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/legacyflumeoffset"
)

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

type FeedInfo struct {
	Secret string `json:"secret"`
	id     string
	log    margaret.Log
}

func mapIdentitiesToSecrets(indir, outdir string) map[string]FeedInfo {
	feeds := make(map[string]FeedInfo)
	err := filepath.WalkDir(indir, func(path string, info fs.DirEntry, err error) error {
		if info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), "secret") {
			file, err := os.Open(path)
			check(err)
			b, err := io.ReadAll(file)
			check(err)

			foldername := fmt.Sprintf("puppet-%03d", len(feeds))
			// save the final location for this identity's secret
			v := FeedInfo{Secret: filepath.Join(foldername, "secret")}

			// load the secret & pick out its feed id
			var id struct {
				ID string
			}
			err = json.Unmarshal(b, &id)
			check(err)
			v.id = id.ID

			// prepare folder paths
			dest := filepath.Join(outdir, foldername)
			basedir := filepath.Join(dest, "flume")
			// create correct folder structure
			err = os.MkdirAll(basedir, os.ModePerm)
			check(err)
			// set log location
			logdest := filepath.Join(basedir, "log.offset")

			// open a margaret log for the specified output format (lfo)
			v.log, err = openLog(logdest)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create output log for %s: %s\n", v.Secret, err)
				os.Exit(1)
			}
			feeds[v.id] = v
			// copy the secret file to the prepared puppet folder
			err = os.WriteFile(filepath.Join(dest, "secret"), b, 0600)
			check(err)
		}
		return nil
	})
	check(err)
	// write a json blob mapping the folders to identities
	// (we cant use the pubkey ids as folder names since unix does not like base64's charset)
	b, err := json.MarshalIndent(feeds, "", "  ")
	check(err)
	err = os.WriteFile(filepath.Join(outdir, "secret-ids.json"), b, 0644)
	check(err)
	return feeds
}

/*
* Given a ssb-fixtures directory, and its monolithic flume log legacy.offset (mfl)
* 1. read all the secrets to figure out which authors exist
* 2. for each discovered author create a key in a map[string]margaret.Log
* 3. go through each message in the mfl and filter out the messages into the corresponding log of the map
* 4. finally, create folders for each author, using the author's pubkey as directory name, and dump an lfo
* version of their log.offset representation. inside each folder, dump the correct secret as well
 */
func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "verbose: talks a bit more than than the tool otherwise is inclined to do")
	var dryRun bool
	flag.BoolVar(&dryRun, "dry", false, "only output what it would do")
	var limit int
	flag.IntVar(&limit, "limit", -1, "how many entries to copy (defaults to unlimited)")
	flag.Parse()

	logPaths := flag.Args()
	if len(logPaths) != 2 {
		cmdName := os.Args[0]
		fmt.Fprintf(os.Stderr, "Usage: %s <options> <path to ssb-fixtures folder> <output path>\nOptions:\n", cmdName)
		flag.PrintDefaults()
		os.Exit(1)
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "splicer: will read log.offset from %s and output to %s\n", logPaths[0], logPaths[1])
		return
	}

	var (
		err   error
		input margaret.Log
	)

	sourceFile := filepath.Join(logPaths[0], "flume", "log.offset")
	input, err = openLog(sourceFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open input log %s: %s\n", logPaths[0], err)
		os.Exit(1)
	}
	feeds := mapIdentitiesToSecrets(logPaths[0], logPaths[1])

	if verbose {
		fmt.Fprintf(os.Stderr, "fixture had %d feeds\n", len(feeds))
	}

	src, err := input.Query(margaret.Limit(limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create query on input log %s: %s\n", logPaths[0], err)
		os.Exit(1)
	}

	i := 0
	ctx := context.Background()
	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			fmt.Fprintf(os.Stderr, "failed to get log entry %s: %s\n", logPaths[0], err)
			os.Exit(1)
		}

		msg := v.(lfoMessage)
		// siphon out the author
		a, has := feeds[msg.author.Ref()]
		if !has {
			continue
		}

		_, err = a.log.Append(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write entry to output log %s: %s\n", logPaths[1], err)
			os.Exit(1)
		}
		i++
	}

	if verbose {
		fmt.Fprintln(os.Stderr, "all done. closing output log. Copied:", i)
	}

	for _, a := range feeds {
		if c, ok := a.log.(io.Closer); ok {
			if err = c.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to close output log %s: %s\n", logPaths[1], err)
			}
		}
	}
}

func openLog(path string) (margaret.Log, error) {
	return legacyflumeoffset.Open(path, FlumeToMultiMsgCodec{})
}
