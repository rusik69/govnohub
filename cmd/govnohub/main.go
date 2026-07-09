package main

import (
	"flag"
	"os"

	"github.com/rusik69/govnohub/internal/cli"
)

func main() {
	fs := flag.NewFlagSet("govnohub", flag.ExitOnError)
	apiURL := fs.String("api-url", "", "API base URL")
	gitURL := fs.String("git-url", "", "Git HTTP base URL")
	token := fs.String("token", "", "API token or PAT")
	configPath := fs.String("config", "", "Config file path")
	fs.Parse(os.Args[1:])

	app := &cli.App{
		ConfigPath: *configPath,
		APIURL:     *apiURL,
		GitURL:     *gitURL,
		Token:      *token,
	}
	os.Exit(app.Run(fs.Args()))
}
