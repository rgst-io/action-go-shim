// Package main implements a shim for downloading binaries from Github
// Releases and executing them. This is meant to be used within Github
// Actions.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/jaredallard/cmdexec"
	"github.com/jaredallard/vcs/git"
	"github.com/jaredallard/vcs/releases"
)

// ghServerURL is the base for Github.
// TODO(jaredallard): Determine this automatically if possible.
var ghServerURL = "https://github.com/"

// log is a wrapper to [fmt.Printf] that handles adding a newline if not
// already added.
func log(format string, a ...any) {
	format = "[action-go-shim] " + format
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}

	fmt.Printf(format, a...)
}

// getTagFromRev returns a tag for the provided ref. If a ref is a
// branch, it will be converted to a commit and said commit will be used
// to match to a tag.
//
// Tags must be semver and are normalized ("v" trimmed from prefix)
func getTagFromRev(ctx context.Context, repo, ref string) (string, error) {
	// If the provided ref is a semver tag, we can skip looking up remotes
	// and assume its valid.
	if sv, err := semver.Parse(strings.TrimPrefix(ref, "v")); err == nil {
		return "v" + sv.String(), nil
	}

	remotes, err := git.ListRemote(ctx, ghServerURL+repo)
	if err != nil {
		return "", fmt.Errorf("failed to list remotes: %w", err)
	}

	for _, remote := range remotes {
		commit := remote[0]
		remoteRef := remote[1]

		// Find the first tag or branch that matches the provided ref.
		if "refs/tags/"+ref == remoteRef || "refs/heads/"+ref == remoteRef {
			ref = commit
			break
		}
	}

	// Find the greatest semver tag for this commit.
	var greatest *semver.Version
	for _, remote := range remotes {
		commit := remote[0]
		remoteRef := remote[1]

		if !strings.HasPrefix(remoteRef, "refs/tags/") {
			continue
		}

		if commit != ref {
			continue
		}

		tagName := strings.TrimPrefix(remoteRef, "refs/tags/")

		sv, err := semver.Parse(strings.TrimPrefix(tagName, "v"))
		if err != nil {
			// TODO(jaredallard): Log this?
			continue
		}

		if greatest == nil || sv.GT(*greatest) {
			greatest = &sv
		}
	}
	if greatest == nil {
		return "", fmt.Errorf("failed to find tag for %q", ref)
	}

	return "v" + greatest.String(), nil
}

// downloadBinary downloads the latest binary for the current runtime
// environment.
//
// TODO(jaredallard): Cache this?
func downloadBinary(ctx context.Context, repo, tagName string) (string, error) {
	// TODO(jaredallard): Should be filepath.Base(repo) but I messed it up
	// on my releases. Post-POC will update.
	fileName := "stencil-action"

	contents, _, err := releases.Fetch(ctx, &releases.FetchOptions{
		RepoURL:   ghServerURL + repo,
		Tag:       tagName,
		AssetName: fileName + "-" + runtime.GOOS + "-" + runtime.GOARCH,
	})
	if err != nil {
		return "", fmt.Errorf("failed to download repo %s release %q: %w", repo, tagName, err)
	}
	defer contents.Close()

	tmpFile, err := os.CreateTemp("", fileName+"-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, contents); err != nil {
		return "", fmt.Errorf("failed to download release: %w", err)
	}

	if err := tmpFile.Chmod(0o755); err != nil {
		return "", fmt.Errorf("failed to make downloaded file executable: %w", err)
	}

	return tmpFile.Name(), nil
}

func entrypoint(ctx context.Context) error {
	repo := os.Getenv("GITHUB_ACTION_REPOSITORY")
	ref := os.Getenv("GITHUB_ACTION_REF")

	if repo == "" {
		return fmt.Errorf("env var GITHUB_ACTION_REPOSITORY not set")
	}

	if ref == "" {
		return fmt.Errorf("env var GITHUB_ACTION_REF not set")
	}

	tagName, err := getTagFromRev(ctx, repo, ref)
	if err != nil {
		return fmt.Errorf("failed to get for rev %q: %w", ref, err)
	}

	log("Evaluated ref %s into tag %s", ref, tagName)
	log("Downloading action from %s@%s...", repo, tagName)
	binPath, err := downloadBinary(ctx, repo, tagName)
	if err != nil {
		return err
	}

	log("Executing action binary %s", binPath)
	cmd := cmdexec.CommandContext(ctx, binPath)
	cmd.UseOSStreams(true)
	return cmd.Run()
}

func main() {
	exitCode := 0
	defer os.Exit(exitCode)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
	defer cancel()

	err := entrypoint(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[action-go-shim] Error: %v\n", err)
		exitCode = 1
	}
}
