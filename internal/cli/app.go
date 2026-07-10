package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type App struct {
	ConfigPath string
	APIURL     string
	GitURL     string
	Token      string
	cfg        *Config
}

func (a *App) load() (*Config, error) {
	if a.cfg != nil {
		return a.cfg, nil
	}
	cfg, err := LoadConfig(a.ConfigPath)
	if err != nil {
		return nil, err
	}
	cfg.MergeFlags(a.APIURL, a.GitURL, a.Token)
	a.cfg = cfg
	return cfg, nil
}

func (a *App) client() (*Client, error) {
	cfg, err := a.load()
	if err != nil {
		return nil, err
	}
	if err := cfg.RequireToken(); err != nil {
		return nil, err
	}
	return NewClient(cfg.APIURL, cfg.Token), nil
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	var err error
	switch args[0] {
	case "auth":
		err = a.runAuth(args[1:])
	case "repo":
		err = a.runRepo(args[1:])
	case "issue":
		err = a.runIssue(args[1:])
	case "pr":
		err = a.runPR(args[1:])
	case "release":
		err = a.runRelease(args[1:])
	case "run":
		err = a.runWorkflow(args[1:])
	case "search":
		err = a.runSearch(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		return 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func (a *App) runAuth(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: govnohub auth login|token create")
	}
	switch args[0] {
	case "login":
		return a.authLogin()
	case "token":
		if len(args) < 2 || args[1] != "create" {
			return fmt.Errorf("usage: govnohub auth token create [--name NAME]")
		}
		return a.authTokenCreate(args[2:])
	default:
		return fmt.Errorf("unknown auth command: %s", args[0])
	}
}

func (a *App) authLogin() error {
	cfg, err := a.load()
	if err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Username: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)
	fmt.Print("Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	client := NewClient(cfg.APIURL, "")
	token, user, err := client.Login(username, password)
	if err != nil {
		return err
	}
	cfg.Token = token
	if u, ok := user["username"].(string); ok {
		cfg.Username = u
	}
	if err := SaveConfig(cfg, a.ConfigPath); err != nil {
		return err
	}
	fmt.Printf("Logged in as %s\n", cfg.Username)
	return nil
}

func (a *App) authTokenCreate(args []string) error {
	name := "cli"
	for i := 0; i < len(args); i++ {
		if args[i] == "--name" && i+1 < len(args) {
			name = args[i+1]
			i++
		}
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	token, err := client.CreatePAT(name, []string{"repo", "workflow"})
	if err != nil {
		return err
	}
	fmt.Println(token)
	return nil
}

func (a *App) runRepo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: govnohub repo list|create|clone")
	}
	switch args[0] {
	case "list":
		return a.repoList()
	case "create":
		if len(args) < 2 {
			return fmt.Errorf("usage: govnohub repo create <name>")
		}
		return a.repoCreate(args[1], args[2:])
	case "clone":
		if len(args) < 2 {
			return fmt.Errorf("usage: govnohub repo clone <owner/repo> [dir]")
		}
		dir := ""
		if len(args) > 2 {
			dir = args[2]
		}
		return a.repoClone(args[1], dir)
	default:
		return fmt.Errorf("unknown repo command: %s", args[0])
	}
}

func (a *App) repoList() error {
	client, err := a.client()
	if err != nil {
		return err
	}
	repos, err := client.ListRepos()
	if err != nil {
		return err
	}
	for _, r := range repos {
		name, _ := r["full_name"].(string)
		private, _ := r["is_private"].(bool)
		flag := ""
		if private {
			flag = " (private)"
		}
		fmt.Printf("%s%s\n", name, flag)
	}
	return nil
}

func (a *App) repoCreate(name string, args []string) error {
	client, err := a.client()
	if err != nil {
		return err
	}
	private := false
	desc := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--private":
			private = true
		case "--description":
			if i+1 < len(args) {
				desc = args[i+1]
				i++
			}
		}
	}
	repo, err := client.CreateRepo(name, desc, private)
	if err != nil {
		return err
	}
	full, _ := repo["full_name"].(string)
	fmt.Printf("Created %s\n", full)
	return nil
}

func (a *App) repoClone(spec, dir string) error {
	cfg, err := a.load()
	if err != nil {
		return err
	}
	if err := cfg.RequireToken(); err != nil {
		return err
	}
	owner, repo, err := ParseOwnerRepo(spec)
	if err != nil {
		return err
	}
	return GitClone(CloneURL(cfg, owner, repo), dir)
}

func (a *App) runIssue(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: govnohub issue list|create <owner/repo> ...")
	}
	owner, repo, err := ParseOwnerRepo(args[1])
	if err != nil {
		return err
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		issues, err := client.ListIssues(owner, repo)
		if err != nil {
			return err
		}
		for _, i := range issues {
			fmt.Printf("#%v %s\n", i["number"], i["title"])
		}
		return nil
	case "create":
		if len(args) < 3 {
			return fmt.Errorf("usage: govnohub issue create <owner/repo> <title> [body]")
		}
		body := ""
		if len(args) > 3 {
			body = strings.Join(args[3:], " ")
		}
		issue, err := client.CreateIssue(owner, repo, args[2], body)
		if err != nil {
			return err
		}
		printJSON(issue)
		return nil
	default:
		return fmt.Errorf("unknown issue command: %s", args[0])
	}
}

func (a *App) runPR(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: govnohub pr list|create <owner/repo> ...")
	}
	owner, repo, err := ParseOwnerRepo(args[1])
	if err != nil {
		return err
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		prs, err := client.ListPRs(owner, repo)
		if err != nil {
			return err
		}
		for _, p := range prs {
			fmt.Printf("#%v %s (%s -> %s)\n", p["number"], p["title"], p["head_branch"], p["base_branch"])
		}
		return nil
	case "create":
		if len(args) < 3 {
			return fmt.Errorf("usage: govnohub pr create <owner/repo> <title> [--head BR] [--base BR] [body]")
		}
		head, base := "feature", "main"
		title := args[2]
		body := ""
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--head":
				if i+1 < len(args) {
					head = args[i+1]
					i++
				}
			case "--base":
				if i+1 < len(args) {
					base = args[i+1]
					i++
				}
			default:
				body = strings.Join(args[i:], " ")
				i = len(args)
			}
		}
		pr, err := client.CreatePR(owner, repo, title, body, head, base)
		if err != nil {
			return err
		}
		printJSON(pr)
		return nil
	case "merge":
		if len(args) < 3 {
			return fmt.Errorf("usage: govnohub pr merge <owner/repo> <number>")
		}
		num, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("invalid PR number")
		}
		out, err := client.MergePR(owner, repo, num, false)
		if err != nil {
			return err
		}
		printJSON(out)
		return nil
	default:
		return fmt.Errorf("unknown pr command: %s", args[0])
	}
}

func (a *App) runRelease(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: govnohub release create|upload <owner/repo> ...")
	}
	owner, repo, err := ParseOwnerRepo(args[1])
	if err != nil {
		return err
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	switch args[0] {
	case "create":
		tag, name, body := "", "", ""
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--tag":
				if i+1 < len(args) {
					tag = args[i+1]
					i++
				}
			case "--name":
				if i+1 < len(args) {
					name = args[i+1]
					i++
				}
			case "--body":
				if i+1 < len(args) {
					body = args[i+1]
					i++
				}
			}
		}
		if tag == "" {
			return fmt.Errorf("--tag is required")
		}
		out, err := client.CreateRelease(owner, repo, tag, name, body)
		if err != nil {
			return err
		}
		printJSON(out)
		return nil
	case "upload":
		if len(args) < 4 {
			return fmt.Errorf("usage: govnohub release upload <owner/repo> <tag> <file>")
		}
		out, err := client.UploadReleaseAsset(owner, repo, args[2], args[3])
		if err != nil {
			return err
		}
		printJSON(out)
		return nil
	default:
		return fmt.Errorf("unknown release command: %s", args[0])
	}
}

func (a *App) runWorkflow(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: govnohub run list|trigger <owner/repo> ...")
	}
	owner, repo, err := ParseOwnerRepo(args[1])
	if err != nil {
		return err
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		runs, err := client.ListRuns(owner, repo)
		if err != nil {
			return err
		}
		for _, run := range runs {
			fmt.Printf("#%v %s %s (%s)\n", run["run_number"], run["status"], run["head_branch"], run["head_sha"])
		}
		return nil
	case "trigger":
		workflowID := ""
		branch := "main"
		event := "workflow_dispatch"
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--workflow":
				if i+1 < len(args) {
					workflowID = args[i+1]
					i++
				}
			case "--branch":
				if i+1 < len(args) {
					branch = args[i+1]
					i++
				}
			}
		}
		if workflowID == "" {
			return fmt.Errorf("--workflow is required")
		}
		run, err := client.TriggerRun(owner, repo, workflowID, event, branch)
		if err != nil {
			return err
		}
		printJSON(run)
		return nil
	default:
		return fmt.Errorf("unknown run command: %s", args[0])
	}
}

func (a *App) runSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: govnohub search <query>")
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	hits, err := client.Search(strings.Join(args, " "))
	if err != nil {
		return err
	}
	for _, h := range hits {
		fmt.Printf("[%s] %s — %s\n", h["type"], h["title"], h["repo"])
	}
	return nil
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func printUsage() {
	fmt.Println(`Govnohub CLI

Usage:
  govnohub [flags] <command> [args]

Commands:
  auth login
  auth token create [--name NAME]
  repo list
  repo create <name> [--private] [--description TEXT]
  repo clone <owner/repo> [dir]
  issue list <owner/repo>
  issue create <owner/repo> <title> [body]
  pr list <owner/repo>
  pr create <owner/repo> <title> [--head BR] [--base BR] [body]
  pr merge <owner/repo> <number>
  release create <owner/repo> --tag TAG [--name NAME] [--body TEXT]
  release upload <owner/repo> <tag> <file>
  run list <owner/repo>
  run trigger <owner/repo> --workflow ID [--branch BR]
  search <query>

Flags:
  --api-url URL    API base URL (default http://localhost:8080)
  --git-url URL    Git HTTP URL (default http://localhost:8081)
  --token TOKEN    API token or PAT
  --config PATH    Config file path`)
}
