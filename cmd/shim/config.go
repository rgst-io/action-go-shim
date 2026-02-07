package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"dario.cat/mergo"
	"github.com/caarlos0/env/v11"
	"github.com/sethvargo/go-githubactions"
	"go.yaml.in/yaml/v4"
)

// Config are the options for this action, sourced from the action.yml.
type Config struct {
	// CacheDirectory is the directory to store downloaded binaries in.
	// Defaults to [os.UserCacheDir].
	CacheDirectory string `env:"ACTION_GO_SHIM_CACHE_DIR"`

	// GithubToken is the Github token to use for interacting with Github.
	GithubToken string `githubActions:"github_token" env:"GH_TOKEN"`

	// GithubActionRef is the version of the action to import.
	GithubActionRef string `githubActions:"action_ref" yaml:"action_ref" env:"GITHUB_ACTION_REF"`

	// GithubActionRepository is the import URL of the action
	// (e.g., rgst-io/actions-go-shim).
	GithubActionRepository string `githubActions:"action_repo" yaml:"action_repo" env:"GITHUB_ACTION_REPOSITORY"`

	// Pattern is the the pattern to use for finding an asset from the
	// downloaded release.
	Pattern string `githubActions:"pattern" githubActionsDefault:"{{ .RepoName }}-{{ .GOOS }}-{{ .GOARCH }}{{ .Ext }}" yaml:"pattern"`

	// ValidateAttestations enables validating downloaded assets with
	// Github's attestations. Requires the Github CLI to be available.
	ValidateAttestations *bool `githubActions:"validate_attestations" githubActionsDefault:"true" env:"ACTION_GO_SHIM_VALIDATE_ATTESTATIONS"`
}

// parseShimConfig attempts to parse a shim config in the current
// action's path.
func parseShimConfig[T any](t *T) error {
	confPath := os.Getenv("GITHUB_ACTION_PATH")
	confFilePaths := []string{
		filepath.Join(confPath, "shim-config.yml"),
		filepath.Join(confPath, "shim", "shim-config.yml"),
	}

	var f *os.File
	var confFilePath string
	for _, cfp := range confFilePaths {
		var err error
		if f, err = os.Open(cfp); err != nil {
			continue
		}

		confFilePath = cfp
		break
	}
	if f == nil {
		return fmt.Errorf("failed to open any shim config files, tried %v", confFilePaths)
	}
	defer f.Close()

	loader, err := yaml.NewLoader(f)
	if err != nil {
		return fmt.Errorf("failed to create new yaml loader for %q: %w", confFilePath, err)
	}

	if err := loader.Load(t); err != nil {
		return fmt.Errorf("failed to parse shim config %q: %w", confFilePath, err)
	}

	return nil
}

// ParseAs parses onto the provided type and returns it.
func ParseAs[T any]() (*T, error) {
	var yt T
	var et T
	var at T

	if err := env.Parse(&et); err != nil {
		return nil, fmt.Errorf("failed to parse env variables: %w", err)
	}

	if err := parseShimConfig(&yt); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to parse shim config: %w", err)
	}

	v := reflect.ValueOf(&at).Elem()
	typ := v.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fv := v.Field(i)

		tag := field.Tag.Get("githubActions")
		if tag == "" {
			continue
		}

		actV := githubactions.GetInput(tag)
		if actV == "" {
			defaultV := field.Tag.Get("githubActionsDefault")
			if defaultV != "" {
				actV = defaultV
			}
		}

		if fv.CanSet() {
			switch fv.Kind() {
			case reflect.String:
				fv.SetString(actV)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				var intVal int64
				if actV != "" {
					var err error
					if intVal, err = strconv.ParseInt(actV, 10, 0); err != nil {
						return nil, fmt.Errorf("invalid integer value for %q: %w", field.Name, err)
					}
				}
				fv.SetInt(intVal)
			case reflect.Bool:
				var boolVal bool
				if actV != "" {
					var err error
					if boolVal, err = strconv.ParseBool(actV); err != nil {
						return nil, fmt.Errorf("invalid boolean value for %q: %w", field.Name, err)
					}
				}
				fv.SetBool(boolVal)
			}
		}
	}

	t := et
	if err := mergo.MergeWithOverwrite(&t, at); err != nil {
		return nil, fmt.Errorf("failed to merge actions config into env config: %w", err)
	}
	if err := mergo.MergeWithOverwrite(&t, yt); err != nil {
		return nil, fmt.Errorf("failed to merge yaml config into env+actions config: %w", err)
	}

	return &t, nil
}
