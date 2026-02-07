// Package main implements a shim for downloading binaries from Github
// Releases and executing them. This is meant to be used within Github
// Actions.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"text/template"

	"github.com/blang/semver/v4"
	"github.com/jaredallard/cmdexec"
	"github.com/jaredallard/vcs/git"
	"github.com/jaredallard/vcs/releases"
	"github.com/jaredallard/vcs/resolver"
	"github.com/sethvargo/go-githubactions"
)

// ghServerURL is the base for Github.
var ghServerURL = func() string {
	srvUrl := os.Getenv("GITHUB_SERVER_URL")
	if srvUrl == "" {
		srvUrl = "https://github.com"
	}
	return strings.TrimSuffix(srvUrl, "/") + "/"
}()

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
func getTagFromRev(ctx context.Context, cfg *Config) (string, error) {
	if cfg.GithubActionRef == "latest" {
		v, err := resolver.NewResolver().Resolve(ctx, ghServerURL+cfg.GithubActionRepository, &resolver.Criteria{
			Constraint: "*",
		})
		if err != nil {
			return "", fmt.Errorf("failed to get the latest version: %w", err)
		}

		return v.Tag, nil
	}

	// If the provided ref is a semver tag, we can skip looking up remotes
	// and assume its valid.
	if sv, err := semver.Parse(strings.TrimPrefix(cfg.GithubActionRef, "v")); err == nil {
		return "v" + sv.String(), nil
	}

	remotes, err := git.ListRemote(ctx, ghServerURL+cfg.GithubActionRepository)
	if err != nil {
		return "", fmt.Errorf("failed to list remotes: %w", err)
	}

	for _, remote := range remotes {
		commit := remote[0]
		remoteRef := remote[1]

		// Find the first tag or branch that matches the provided ref.
		if "refs/tags/"+cfg.GithubActionRef == remoteRef ||
			"refs/heads/"+cfg.GithubActionRef == remoteRef {
			cfg.GithubActionRef = commit
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

		if commit != cfg.GithubActionRef {
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
		return "", fmt.Errorf("failed to find tag for %q", cfg.GithubActionRef)
	}

	return "v" + greatest.String(), nil
}

// getBinaryPath gets a path suitable for storing (and caching) the
// specific tag.
func getBinaryPath(cfg *Config, tag string) (string, error) {
	cacheDir := cfg.CacheDirectory
	if cacheDir == "" {
		var err error
		cacheDir, err = os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine cache directory: %w", err)
		}
		cacheDir = filepath.Join(cacheDir, ".action-go-shim")
	}

	dir := filepath.Join(
		cacheDir,
		strings.ReplaceAll(cfg.GithubActionRepository, "/", "--"),
		tag,
	)
	return filepath.Join(
		dir,
		filepath.Base(cfg.GithubActionRepository)+"-"+runtime.GOOS+"-"+runtime.GOARCH,
	), nil
}

// downloadBinary downloads the latest binary for the current runtime
// environment.
func downloadBinary(ctx context.Context, cfg *Config, tag string) (string, error) {
	dlPath, err := getBinaryPath(cfg, tag)
	if err != nil {
		return "", fmt.Errorf("failed to get path for caching: %w", err)
	}
	if _, err := os.Stat(dlPath); err == nil {
		return dlPath, nil
	}

	assetName, err := getBinaryName(cfg, tag)
	if err != nil {
		return "", err
	}

	contents, _, err := releases.Fetch(ctx, &releases.FetchOptions{
		RepoURL:   ghServerURL + cfg.GithubActionRepository,
		Tag:       tag,
		AssetName: assetName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to download repo %s release %q: %w", cfg.GithubActionRepository, tag, err)
	}
	defer contents.Close()

	if err := os.MkdirAll(filepath.Dir(dlPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to ensure cache dir exists: %w", err)
	}

	f, err := os.Create(dlPath)
	if err != nil {
		return "", fmt.Errorf("failed to create cache file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, contents); err != nil {
		return "", fmt.Errorf("failed to download release: %w", err)
	}

	if err := f.Chmod(0o755); err != nil {
		return "", fmt.Errorf("failed to make downloaded file executable: %w", err)
	}

	if cfg.ValidateAttestations != nil && *cfg.ValidateAttestations {
		cmd := cmdexec.Command("gh", "attestation", "verify", "--repo", cfg.GithubActionRepository, dlPath)
		cmd.SetEnviron([]string{"GH_TOKEN=" + cfg.GithubToken})
		out, err := cmd.Output()
		if err != nil {
			var execErr *exec.ExitError
			if errors.As(err, &execErr) {
				return "", fmt.Errorf("attestation validation failed %q: %w", string(execErr.Stderr), err)
			}

			return "", fmt.Errorf("attestation validation failed (no stderr): %w", err)
		}

		os.Stdout.Write(out) //nolint:errcheck // Why: Best effort
	}

	return dlPath, nil
}

// getBinaryName returns the expected binary name by parsing
// [Config.Pattern].
func getBinaryName(cfg *Config, tag string) (string, error) {
	tpl, err := template.New("pattern.tpl").Parse(cfg.Pattern)
	if err != nil {
		return "", fmt.Errorf("failed to parse template string: %w", err)
	}

	var ext string
	switch runtime.GOOS {
	case "windows":
		ext = ".ext"
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]string{
		"GOOS":                   runtime.GOOS,
		"GOARCH":                 runtime.GOARCH,
		"GithubActionRepository": cfg.GithubActionRepository,
		"RepoName":               filepath.Base(cfg.GithubActionRepository),
		"Tag":                    tag,
		"Ext":                    ext,
	}); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func entrypoint(ctx context.Context) error {
	cfg, err := ParseAs[Config]()
	if err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	tagName, err := getTagFromRev(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get tag for rev %q: %w", cfg.GithubActionRef, err)
	}

	log("Evaluated ref %s into tag %s", cfg.GithubActionRef, tagName)
	log("Downloading action from %s@%s...", cfg.GithubActionRepository, tagName)
	binPath, err := downloadBinary(ctx, cfg, tagName)
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
	defer func() {
		if r := recover(); r != nil {
			githubactions.Errorf("[actions-go-shim] panic: %v\n\n%s\n", r, debug.Stack())
			exitCode = 1
		}

		os.Exit(exitCode)
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
	defer cancel()

	if err := entrypoint(ctx); err != nil {
		githubactions.Errorf("[actions-go-shim] Error: %v", err.Error())
		exitCode = 1
	}
}
