// SPDX-FileCopyrightText: 2021 the netsim authors
//
// SPDX-License-Identifier: MIT

// Tasks the log splicer takes care of during its runtime:
// * Creates folder super structure, encapsulating each identity in its own folder
// * Maps identities to folder names they live in
// * Copies secrets from the fixtures folder to each identity folder
// * Splics out messages from the one ssb-fixtures log.offset to the many log.offset in said folder structure
// * Persists an identity mapping of ID to {folderName, latest sequence number} to secret-ids.json

// The log splicer command takes an ssb-fixtures generated folder as input, and a destination folder as output. The
// destination folder will be populated with identity folders, one folder per identity found in the generated fixtures.
//
// Each identity folder contains a log.offset, with the posts created by that identity, and the identity's secret file. The
// identity folders are named after the filenames of the secrets found in the ssb-fixtures folder, which preserves the
// pareto distribution of authors (secrets in the lower number ranges have issued more posts).
//
// Finally, a mapping from ssb identities @[...].ed25199 to the identity folders is dumped as json to the root of the
// destination folder. The mapping, in addition to naming the secret folder, also contains an integer count tracking the latest
// sequence number posted by that identity.
package splicer

import (
	"context"
	"encoding/json"
	"errors"
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
				return inform(err, "failed to read secret when mapping id -> secret")
			}

			// load the secret & pick out its feed id
			v := FeedInfo{}
			err = json.Unmarshal(b, &v)
			if err != nil {
				return inform(err, "failed to unmarshal during id -> secret mapping")
			}

			puppetname, err := derivePuppetName(info.Name())
			if err != nil {
				return inform(err, "derive puppet name failed")
			}

			v.identityFolder, err = createFolderStructure(outdir, puppetname)
			if err != nil {
				return inform(err, fmt.Sprintf("failed to create folder structure for %s", puppetname))
			}

			logpath := filepath.Join(v.identityFolder, "flume", "log.offset")
			err = checkLogEmpty(logpath, removeExistingLogs)
			if err != nil {
				return inform(err, "empty log check failed")
			}

			// open a margaret log for the specified output format (lfo)
			v.log, err = openLog(logpath)
			if err != nil {
				inform(err, fmt.Sprintf("failed to create output log for %s", v.ID))
			}
			// save the feed info in the identity mapping
			feeds[v.ID] = v

			err = copySecret(v.identityFolder, b)
			if err != nil {
				return inform(err, fmt.Sprintf("failed to copy secret for %s", v.ID))
			}
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
		idsToFolders[id] = FeedJSON{Folder: filepath.Base(feedInfo.identityFolder), Latest: feedInfo.latest}
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

// copies a file `src` to folder `outdir`, using `filepath.Base(src)` for naming the resultant file
func copyFile(src, outdir string) error {
	// open a file descriptor to the src file (in read only mode)
	srcfile, err := os.Open(src)
	if err != nil {
		return err
	}

	// prepare the destination file for writing
	filename := filepath.Base(src)
	dst := filepath.Join(outdir, filename)
	dstfile, err := os.Create(dst)
	if err != nil {
		return err
	}

	// note: io.Copy copies INTO dst from src (think dst := src), without buffering contents to ram
	_, err = io.Copy(dstfile, srcfile)
	return err
}

// used by the `alloffsets` dsl command, which allows a puppet to have knowledge over all historic messages on start
func copyMonolithicOffset(indir, outdir string) error {
	src := filepath.Join(indir, "flume", "log.offset")
	dst := filepath.Join(outdir, "puppet-all", "flume")
	// create the special folder `puppet-all`
	err := os.MkdirAll(dst, os.ModePerm)
	if err != nil {
		return err
	}
	// copy the monolithic offset from input fixtures into puppet-all/flume/log.offset
	err = copyFile(src, dst)
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
			msg := fmt.Sprintf("output log already contains data. has the splicer already run?\n%s: use -prune to delete pre-existing logs", getToolName())
			return inform(errors.New("-prune was not passed"), msg)
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

func SpliceLogs(args Args) error {
	var err error
	var input margaret.Log

	if args.DryRun || args.Verbose {
		fmt.Fprintf(os.Stderr, "%s: will read log.offset from %s and output to %s\n", getToolName(), args.Indir, args.Outdir)
		if args.DryRun {
			return nil
		}
	}

	sourceFile := filepath.Join(args.Indir, "flume", "log.offset")
	input, err = openLog(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to open input log %s: %w\n", args.Indir, err)
	}

	feeds, err := mapIdentitiesToSecrets(args.Indir, args.Outdir, args.Prune)
	if err != nil {
		return err
	}

	err = copyMonolithicOffset(args.Indir, args.Outdir)
	if err != nil {
		return err
	}

	if args.Verbose {
		fmt.Fprintf(os.Stderr, "fixture had %d feeds\n", len(feeds))
	}

	src, err := input.Query(margaret.Limit(-1))
	if err != nil {
		return fmt.Errorf("failed to create query on input log %s: %w\n", args.Indir, err)
	}

	i := 0
	ctx := context.Background()
	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			return fmt.Errorf("failed to get log entry %s: %w\n", args.Indir, err)
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
			return fmt.Errorf("failed to write entry to output log %s: %w\n", args.Outdir, err)
		}
		i++
	}

	err = persistIdentityMapping(feeds, args.Outdir)
	if err != nil {
		return err
	}

	err = copyFile(filepath.Join(args.Indir, "follow-graph.json"), args.Outdir)
	if err != nil {
		return err
	}

	if args.Verbose {
		fmt.Fprintln(os.Stderr, "all done. closing output log. Copied:", i)
	}

	for _, a := range feeds {
		if c, ok := a.log.(io.Closer); ok {
			if err = c.Close(); err != nil {
				return fmt.Errorf("failed to close output log %s: %w\n", args.Outdir, err)
			}
		}
	}
	return nil
}

type Args struct {
	Verbose bool
	DryRun  bool
	// delete any offset.logs if encountered in Outdir, before appending new messages
	Prune bool
	// directory of ssb-fixtures output
	Indir string
	// directory of the spliced out logs
	Outdir string
}

func getToolName() string {
	return os.Args[0]
}

func openLog(path string) (margaret.Log, error) {
	return legacyflumeoffset.Open(path, FlumeToMultiMsgCodec{})
}
