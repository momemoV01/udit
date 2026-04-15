package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/momemoV01/udit/internal/client"
)

var Version = "dev"

var (
	flagPort    int
	flagProject string
	flagTimeout int
	flagJSON    bool
)

func Execute() error {
	flag.IntVar(&flagPort, "port", 0, "Override Unity instance port")
	flag.StringVar(&flagProject, "project", "", "Select Unity instance by project path")
	flag.IntVar(&flagTimeout, "timeout", 120000, "Request timeout in milliseconds")
	flag.BoolVar(&flagJSON, "json", false, "Emit machine-readable JSON envelope to stdout/stderr")

	flag.Usage = func() { printHelp() }

	args := os.Args[1:]
	flagArgs, cmdArgs := splitArgs(args)
	if err := flag.CommandLine.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "flag parse error: %v\n", err)
		os.Exit(1)
	}

	// Load .udit.yaml (walk up from cwd). Apply only fields that the user
	// did NOT set on the CLI, so explicit flags always win. Path is kept
	// so `udit config path` / `udit config show` can surface it.
	if cwd, err := os.Getwd(); err == nil {
		if cfg, path := LoadConfig(cwd); cfg != nil {
			applyConfig(cfg)
			loadedConfigPath = path
		}
	}

	if len(cmdArgs) == 0 {
		printHelp()
		return nil
	}

	category := cmdArgs[0]
	subArgs := cmdArgs[1:]

	// --help / -h on any command
	for _, a := range subArgs {
		if a == "--help" || a == "-h" {
			printTopicHelp(category)
			return nil
		}
	}

	switch category {
	case "help", "--help", "-h":
		if len(subArgs) > 0 {
			printTopicHelp(subArgs[0])
		} else {
			printHelp()
		}
		return nil
	case "version", "--version", "-v":
		fmt.Println("udit " + Version)
		return nil
	case "update":
		return updateCmd(subArgs)
	case "completion":
		return completionCmd(subArgs)
	case "init":
		return initCmd(subArgs)
	case "log":
		return logCmd(subArgs, flagJSON)
	case "run":
		return runCmd(subArgs, flagJSON)
	case "config":
		return configCmd(subArgs, flagJSON)
	case "watch":
		// watch is a long-running command that doesn't require Unity to
		// be alive at startup — hooks may run when Unity is off (e.g.
		// lint-only hooks). Must be handled before DiscoverInstance.
		return runWatch(subArgs, flagJSON)
	case "status":
		inst, err := client.DiscoverInstance(flagProject, flagPort)
		if err != nil {
			reportError(err, "status", nil, flagJSON)
			os.Exit(1)
		}
		statusErr := statusCmd(inst, flagJSON)
		printUpdateNotice()
		if statusErr != nil {
			reportError(statusErr, "status", inst, flagJSON)
			os.Exit(1)
		}
		return nil
	}

	inst, err := client.DiscoverInstance(flagProject, flagPort)
	if err != nil {
		reportError(err, category, nil, flagJSON)
		os.Exit(1)
	}

	if err := waitForAlive(inst.Port, flagTimeout); err != nil {
		reportError(err, category, inst, flagJSON)
		os.Exit(1)
	}

	timeout := flagTimeout
	send := func(command string, params interface{}) (*client.CommandResponse, error) {
		return client.Send(inst, command, params, timeout)
	}

	var resp *client.CommandResponse

	switch category {
	case "editor":
		resp, err = editorCmd(subArgs, send, inst.Port)
	case "scene":
		resp, err = sceneCmd(subArgs, send)
	case "go":
		resp, err = goCmd(subArgs, send)
	case "component":
		resp, err = componentCmd(subArgs, send)
	case "asset":
		resp, err = assetCmd(subArgs, send)
	case "prefab":
		resp, err = prefabCmd(subArgs, send)
	case "tx":
		resp, err = txCmd(subArgs, send)
	case "project":
		resp, err = projectCmd(subArgs, send)
	case "package":
		resp, err = packageCmd(subArgs, send)
	case "build":
		// Player builds run for tens of seconds to many minutes.
		// Override the global 2-minute timeout so the agent doesn't get
		// a deadline-exceeded mid-build. Same trick as test (PlayMode).
		buildSend := func(command string, params interface{}) (*client.CommandResponse, error) {
			return client.Send(inst, command, params, 0)
		}
		resp, err = buildCmd(subArgs, buildSend)
	case "test":
		testSend := func(command string, params interface{}) (*client.CommandResponse, error) {
			return client.Send(inst, command, params, 0)
		}
		resp, err = testCmd(subArgs, testSend, inst.Port)
	case "exec":
		subArgs = readStdinIfPiped(subArgs)
		var params map[string]interface{}
		params, err = buildParams(subArgs, nil)
		if err == nil {
			// Merge config-level default usings with whatever the call already
			// provided. CLI --usings (parsed by buildParams above) wins for
			// duplicates because it lands on top.
			params = mergeExecUsings(params, loadedConfig)
			resp, err = send("exec", params)
		}
	default:
		var params map[string]interface{}
		params, err = buildParams(subArgs, nil)
		if err == nil {
			// Keep CLI path semantics consistent: params that point at a
			// caller-side file (e.g. `screenshot --output_path`) should land
			// where the user typed the command, not in Unity's project root.
			// Narrow allow-list — Unity-side asset paths (e.g. reserialize's
			// `paths`) must stay untouched.
			absolutizePathParam(params, "output_path")
			resp, err = send(category, params)
		}
	}

	if err != nil {
		reportError(err, category, inst, flagJSON)
		os.Exit(1)
	}

	printResponse(resp, category, inst, flagJSON)

	printUpdateNotice()

	if !resp.Success {
		os.Exit(1)
	}

	return nil
}

// loadedConfig is set by Execute() once at startup so subcommand handlers
// (e.g. exec usings injection) can see project-wide settings without being
// passed an extra parameter through every call site.
var loadedConfig *Config

// loadedConfigPath is the absolute path of the .udit.yaml that supplied
// `loadedConfig`. Empty when no config was found during walk-up. Used by
// `udit config path` / `udit config show` / `udit config edit`.
var loadedConfigPath string

// applyConfig pushes config defaults into the global flag variables when the
// CLI did not override them. CLI flags > config > built-in defaults.
func applyConfig(cfg *Config) {
	loadedConfig = cfg
	if flagPort == 0 && cfg.DefaultPort != 0 {
		flagPort = cfg.DefaultPort
	}
	// 120000 is the built-in default for --timeout. Treat it as "unset" so
	// the config can replace it; an explicit `--timeout 120000` is
	// indistinguishable but harmless (same value).
	if flagTimeout == 120000 && cfg.DefaultTimeoutMs != 0 {
		flagTimeout = cfg.DefaultTimeoutMs
	}
}
