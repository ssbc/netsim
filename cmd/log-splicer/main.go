// SPDX-License-Identifier: MIT

// Tasks the log splicer takes care of during its runtime:
// * Creates folder super structure, encapsulating each identity in its own folder
// * Maps identities to folder names they live in
// * Copies secrets from the fixtures folder to each identity folder
// * Splics out messages from the one ssb-fixtures log.offset to the many log.offset in said folder structure
// * Persists an identity mapping of ID to {folderName, latest sequence number} to secret-ids.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/legacyflumeoffset"
)

// TODO: collapse feedinfo + feedjson into just feedinfo? and use temp struct for unmarshaling ID?
type FeedInfo struct {
	ID             string
	log            margaret.Log
	latest         int
	identityFolder string
}

func inform(e error, message string) error {
	if len(e.Error()) > 0 {
		return fmt.Errorf("%s (%w)", message, e)
	}
	// the receiver error had no useful info, don't include it in our informative error message
	return fmt.Errorf("%s", message)
}

func mapIdentitiesToSecrets(indir, outdir string, removeExistingLogs bool) (map[string]FeedInfo, error) {
	feeds := make(map[string]FeedInfo)
	err := filepath.WalkDir(indir, func(path string, info fs.DirEntry, err error) error {
		if info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), "secret") {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			// load the secret & pick out its feed id
			v := FeedInfo{}
			err = json.Unmarshal(b, &v)
			if err != nil {
				return err
			}

			puppetname, err := derivePuppetName(info.Name())
			if err != nil {
				return err
			}

			v.identityFolder, err = createFolderStructure(outdir, puppetname)
			if err != nil {
				return err
			}

			logpath := filepath.Join(v.identityFolder, "flume", "log.offset")
			err = checkLogEmpty(logpath, removeExistingLogs)
			if err != nil {
				return err
			}

			// open a margaret log for the specified output format (lfo)
			v.log, err = openLog(logpath)
			if err != nil {
				inform(err, fmt.Sprintf("failed to create output log for %s", v.ID))
			}
			// save the feed info in the identity mapping
			feeds[v.ID] = v

			err = copySecret(v.identityFolder, b)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return feeds, nil
}

type FeedJSON struct {
	Folder string `json:"folder"`
	Latest int    `json:"latest"`
}

func persistIdentityMapping(feeds map[string]FeedInfo, outdir string) error {
	// map id to the identity folder (where the secret + offset.log lives) as well as the feed's latest sequence number
	idsToFolders := make(map[string]FeedJSON)
	for id, feedInfo := range feeds {
		idsToFolders[id] = FeedJSON{Folder: feedInfo.identityFolder, Latest: feedInfo.latest}
	}

	// write a json blob mapping the identities to the folders containing their secret + log.offset
	// (we cant use the pubkey ids as folder names since unix does not like base64's charset)
	b, err := json.MarshalIndent(idsToFolders, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(outdir, "secret-ids.json"), b, 0644)
	return err
}

func copySecret(identityFolder string, b []byte) error {
	newSecretPath := filepath.Join(identityFolder, "secret")
	// copy the secret file to the prepared puppet folder
	err := os.WriteFile(newSecretPath, b, 0600)
	return err
}

func derivePuppetName(secretFilename string) (string, error) {
	// write puppet names based on secret names, to preserve the implicit pareto distribution of post authors
	// (authors with lower secret ids make more posts)
	parts := strings.Split(secretFilename, "-")
	// handle the case where first file is just called "secret"
	if len(parts) == 1 {
		parts = append(parts, "0")
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("puppet-%05d", n), nil
}

// Returns the folder the spliced out identity lives in, and an error if os.MkdirAll failed
func createFolderStructure(outdir, puppetName string) (string, error) {
	// prepare folder path
	identityFolder := filepath.Join(outdir, puppetName)
	// create correct folder structure
	err := os.MkdirAll(filepath.Join(identityFolder, "flume"), os.ModePerm)
	return identityFolder, err
}

func checkLogEmpty(logpath string, removeExistingLogs bool) error {
	// check if the output log exists
	info, err := os.Stat(logpath)
	if err != nil && !os.IsNotExist(err) {
		return inform(err, fmt.Sprintf("failed to stat output log"))
	}
	// the output log does exist
	if err == nil && info.Size() > 0 {
		// -prune was not passed; abort
		if !removeExistingLogs {
			return inform(errors.New("-prune was not passed"), "output log already contains data. has the splicer already run?\nsplicer: use -prune to delete pre-existing logs")
		}
		// if -prune flag passed -> remove the log before we use it
		err = os.Remove(logpath)
		if err != nil {
			return inform(err, "failed to delete pre-existing output log")
		}
	}
	return nil
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
	var prune bool
	flag.BoolVar(&prune, "prune", false, "removes existing output logs before writing to them (if -prune omitted, the splicer will instead exit with an error)")
	var limit int
	flag.IntVar(&limit, "limit", -1, "how many entries to copy (defaults to unlimited)")
	flag.Parse()

	logpaths := flag.Args()
	if len(logpaths) != 2 {
		cmdName := os.Args[0]
		fmt.Fprintf(os.Stderr, "Usage: %s <options> <path to ssb-fixtures folder> <output path>\nOptions:\n", cmdName)
		flag.PrintDefaults()
		os.Exit(1)
	}
	indir, outdir := logpaths[0], logpaths[1]

	if dryRun || verbose {
		fmt.Fprintf(os.Stderr, "splicer: will read log.offset from %s and output to %s\n", indir, outdir)
		if dryRun {
			return
		}
	}

	var (
		err   error
		input margaret.Log
	)

	sourceFile := filepath.Join(indir, "flume", "log.offset")
	input, err = openLog(sourceFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open input log %s: %s\n", indir, err)
		os.Exit(1)
	}
	feeds, err := mapIdentitiesToSecrets(indir, outdir, prune)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "fixture had %d feeds\n", len(feeds))
	}

	src, err := input.Query(margaret.Limit(limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create query on input log %s: %s\n", indir, err)
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
			fmt.Fprintf(os.Stderr, "failed to get log entry %s: %s\n", indir, err)
			os.Exit(1)
		}

		msg := v.(lfoMessage)
		// siphon out the author
		a, has := feeds[msg.author.Ref()]
		if !has {
			continue
		}
		a.latest += 1
		feeds[msg.author.Ref()] = a

		_, err = a.log.Append(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write entry to output log %s: %s\n", outdir, err)
			os.Exit(1)
		}
		i++
	}

	err = persistIdentityMapping(feeds, outdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Fprintln(os.Stderr, "all done. closing output log. Copied:", i)
	}

	for _, a := range feeds {
		if c, ok := a.log.(io.Closer); ok {
			if err = c.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to close output log %s: %s\n", outdir, err)
			}
		}
	}
}

func openLog(path string) (margaret.Log, error) {
	return legacyflumeoffset.Open(path, FlumeToMultiMsgCodec{})
}
