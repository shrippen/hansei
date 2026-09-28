// Command hansei reviews AI edits of an Obsidian vault.
//
//	hansei                      terminal UI (uses the daemon if one runs)
//	hansei daemon               JSON-RPC service for the KDE app
//	hansei init --vault PATH    write a first config
//	hansei befunde [--json]     rule findings without AI
//	hansei auftrag TEXT         start an AI task (scriptable, e.g. from a timer)
//	hansei batches [--json]     list batches
//	hansei key set NAME         store an API key (read from stdin)
//	hansei import               read the old review-queue/
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/i18n"
	"git.arianw.de/shrippen/hansei/core/rpc"
	"git.arianw.de/shrippen/hansei/core/service"
	"git.arianw.de/shrippen/hansei/internal/cli"
	"git.arianw.de/shrippen/hansei/tui"
)

// version is set at build time (-ldflags "-X main.version=1.0.0").
var version = "dev"

const (
	idleCheck   = 30 * time.Second
	waitPoll    = time.Second
	defaultIdle = 0 // run until stopped
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hansei:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return cmdTUI()
	}
	switch args[0] {
	case "tui":
		return cmdTUI()
	case "daemon":
		return cmdDaemon(args[1:])
	case "init":
		return cmdInit(args[1:])
	case "befunde", "findings":
		return cmdFindings(args[1:])
	case "auftrag", "task":
		return cmdTask(args[1:])
	case "batches":
		return cmdBatches(args[1:])
	case "key":
		return cmdKey(args[1:])
	case "import":
		return cmdImport()
	case "version", "--version":
		fmt.Println("hansei", version)
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	}
	if c, ok := cli.Extra[args[0]]; ok {
		return c.Run(args[1:])
	}
	usage()
	return fmt.Errorf("unknown command %q", args[0])
}

func usage() {
	fmt.Print(`hansei ` + version + `

  hansei                          terminal UI
  hansei daemon [--idle 10m]      service for the KDE app (socket ` + rpc.SocketPath() + `)
  hansei init --vault PATH [--allow IT] [--block Diary,…]
  hansei befunde [--json]         rule findings without AI
  hansei auftrag [--in IT/Netzwerk,…] [--provider NAME] [--wait] TEXT
  hansei batches [--json]
  hansei key set NAME             store an API key from stdin in the keyring
  hansei import                   read the old review-queue/ (import_queue in the config)
`)
	names := make([]string, 0, len(cli.Extra))
	for n := range cli.Extra {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("  %s\n", cli.Extra[n].Usage)
	}
}

// connect uses a running daemon, or opens the service in this process.
func connect() (service.API, func(), error) {
	if cl, err := rpc.Dial(rpc.SocketPath()); err == nil {
		return cl, func() { cl.Close() }, nil
	}
	cfg, err := config.Load("")
	if errors.Is(err, config.ErrNoConfig) {
		return nil, nil, fmt.Errorf("%w\nstart with: hansei init --vault ~/Obsidian/Vault --allow IT", err)
	}
	if err != nil {
		return nil, nil, err
	}
	svc, err := service.Open(cfg, service.Options{Version: version})
	if errors.Is(err, service.ErrBusy) {
		return nil, nil, errors.New("another Hansei process holds the data folder, but its socket does not answer")
	}
	if err != nil {
		return nil, nil, err
	}
	return svc, svc.Close, nil
}

func cmdTUI() error {
	api, closeFn, err := connect()
	if err != nil {
		return err
	}
	defer closeFn()

	// In-process: serve the socket too, so the KDE app and scripts can join this session.
	if svc, ok := api.(*service.Service); ok {
		if srv, err := rpc.Listen(svc, rpc.SocketPath()); err == nil {
			go func() { _ = srv.Serve() }()
			defer srv.Close()
		}
	}
	st := api.Status()
	return tui.Run(api, st.Lang, st.Style)
}

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	idle := fs.Duration("idle", defaultIdle, "exit after this long without clients and running tasks (0: never)")
	socket := fs.String("socket", rpc.SocketPath(), "socket path")
	_ = fs.Parse(args)

	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	svc, err := service.Open(cfg, service.Options{Version: version})
	if err != nil {
		return err
	}
	defer svc.Close()
	return serve(svc, *socket, *idle)
}

// serve runs the RPC server until a signal arrives or it idles out.
func serve(svc *service.Service, socket string, idle time.Duration) error {
	srv, err := rpc.Listen(svc, socket)
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve() }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(idleCheck)
	defer tick.Stop()
	for {
		select {
		case <-sig:
			return srv.Close()
		case err := <-errc:
			return err
		case <-tick.C:
			if idle > 0 && srv.Idle() > idle && svc.Status().Running == 0 {
				return srv.Close()
			}
		}
	}
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	vault := fs.String("vault", "", "absolute path of the Obsidian vault")
	allow := fs.String("allow", "", "folders the AI may read and change, comma separated")
	block := fs.String("block", "", "folders that are never read, comma separated")
	force := fs.Bool("force", false, "overwrite an existing config")
	_ = fs.Parse(args)

	path := config.DefaultPath()
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s exists (use --force to overwrite)", path)
	}
	abs, err := filepath.Abs(expand(*vault))
	if err != nil || *vault == "" {
		return errors.New("--vault is required")
	}
	c := config.New(path, abs)
	c.Allow = split(*allow)
	c.Block = split(*block)
	if err := c.Validate(); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}

func cmdFindings(args []string) error {
	fs := flag.NewFlagSet("befunde", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	api, closeFn, err := connect()
	if err != nil {
		return err
	}
	defer closeFn()
	rep := api.Findings(true)
	if *asJSON {
		return printJSON(rep)
	}
	fmt.Printf("%d notes, %.0f %% conform\n", rep.Notes, rep.Conformity*100)
	for _, s := range rep.Summary {
		fmt.Printf("%-32s %4d  (%d notes)\n", rep.Titles[string(s.Rule)], s.Count, len(s.Paths))
	}
	for _, f := range rep.Findings {
		fmt.Printf("  %s:%d  %s  %s\n", f.Path, f.Line, f.Rule, f.Detail)
	}
	return nil
}

func cmdTask(args []string) error {
	fs := flag.NewFlagSet("auftrag", flag.ExitOnError)
	in := fs.String("in", "", "folders in scope, comma separated (default: all allowed)")
	provider := fs.String("provider", "", "provider name (default from the config)")
	wait := fs.Bool("wait", false, "wait until the AI finished")
	_ = fs.Parse(args)
	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		return errors.New("task text missing")
	}

	api, closeFn, err := connect()
	if err != nil {
		return err
	}
	defer closeFn()
	sum, err := api.Task(service.TaskInput{Instruction: text, Scope: split(*in), Provider: *provider})
	if err != nil {
		return err
	}
	fmt.Println(sum.ID)

	// In-process runs end with the process, so without a daemon the command always waits.
	if _, isDaemon := api.(*rpc.Client); !*wait && isDaemon {
		return nil
	}
	for {
		b, err := api.Batch(sum.ID)
		if err != nil {
			return err
		}
		if !b.Running && b.Status != "working" && !b.Revising {
			fmt.Printf("%s: %s (%d files, %d hunks)\n", b.Status, b.Title, b.Counts.Files, b.Counts.Hunks)
			if b.Error != "" {
				fmt.Println(b.Error)
			}
			return nil
		}
		time.Sleep(waitPoll)
	}
}

func cmdBatches(args []string) error {
	fs := flag.NewFlagSet("batches", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	api, closeFn, err := connect()
	if err != nil {
		return err
	}
	defer closeFn()
	list := api.Batches()
	if *asJSON {
		return printJSON(list)
	}
	for _, b := range list {
		fmt.Printf("%s  %-9s %-10s %s (%d/%d files)\n", b.ID, b.Column, b.Topic, b.Title, b.Counts.Done, b.Counts.Files)
	}
	return nil
}

func cmdKey(args []string) error {
	if len(args) != 2 || (args[0] != "set" && args[0] != "delete") {
		return errors.New("usage: hansei key set|delete NAME")
	}
	key := ""
	if args[0] == "set" {
		fmt.Fprint(os.Stderr, "key: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		key = strings.TrimSpace(line)
	}
	api, closeFn, err := connect()
	if err != nil {
		return err
	}
	defer closeFn()
	return api.SetKey(args[1], key)
}

func cmdImport() error {
	api, closeFn, err := connect()
	if err != nil {
		return err
	}
	defer closeFn()
	n, err := api.Import()
	if err != nil {
		return err
	}
	fmt.Println(i18n.T(api.Status().Lang, "importSummary"), n)
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), p[2:])
	}
	return p
}
