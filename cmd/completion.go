package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Built-in commands appear inline in each shell script. Custom [UditTool]
// handlers aren't included because completion runs without a live Unity to
// query; static commands cover 95% of daily typing.

const completionUsage = `Usage: udit completion <subcommand|shell> [flags]

Subcommands:
  install [--shell <s>] [--force]   Persist completion into your shell's
                                    rc file (or completions/ for fish).
                                    Idempotent — re-running replaces the
                                    block bracketed by udit's markers.
  uninstall [--shell <s>]           Remove the udit completion block.
  print <shell>                     Emit the completion script to stdout
                                    (alias: bare ` + "`udit completion <shell>`" + `).

Shells supported: bash, zsh, powershell (or pwsh), fish.

  Print to stdout (manual install):
    source <(udit completion bash)
    udit completion zsh  > "${fpath[1]}/_udit"
    udit completion fish > ~/.config/fish/completions/udit.fish
    udit completion powershell | Out-String | Invoke-Expression

  Auto-install into your rc file:
    udit completion install                 # auto-detect shell
    udit completion install --shell zsh     # force a specific shell
    udit completion uninstall               # remove

The install/uninstall path is what install.sh and install.ps1 invoke after
placing the binary, so a fresh install gets tab completion with no extra
step. Pass --no-completion to either installer to skip it.
`

// completionCmd dispatches to the right shell-script generator and writes
// to stdout so users can `source <(...)` or pipe to a file. With "install"
// or "uninstall" as the first arg, persists to the user's shell rc.
func completionCmd(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, completionUsage)
		os.Exit(1)
	}
	switch strings.ToLower(args[0]) {
	case "install":
		return runCompletionInstall(args[1:])
	case "uninstall":
		return runCompletionUninstall(args[1:])
	case "print":
		// Explicit form so we can grow more flags later without colliding
		// with shell names. `udit completion print bash` == `udit completion bash`.
		if len(args) < 2 {
			fmt.Fprint(os.Stderr, completionUsage)
			os.Exit(1)
		}
		return printCompletionScript(args[1])
	case "bash", "zsh", "powershell", "pwsh", "fish":
		return printCompletionScript(args[0])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand or shell: %q\n\n%s", args[0], completionUsage)
		os.Exit(1)
	}
	return nil
}

// printCompletionScript writes the completion script for the named shell
// to stdout. Returns an error only for unknown shells; the caller decides
// the exit code.
func printCompletionScript(shell string) error {
	switch strings.ToLower(shell) {
	case "bash":
		fmt.Print(bashScript)
	case "zsh":
		fmt.Print(zshScript)
	case "powershell", "pwsh":
		fmt.Print(powershellScript)
	case "fish":
		fmt.Print(fishScript)
	default:
		fmt.Fprintf(os.Stderr, "unknown shell: %q\n\n%s", shell, completionUsage)
		os.Exit(1)
	}
	return nil
}

// ----------------------------------------------------------------------
// install / uninstall
// ----------------------------------------------------------------------

// completionMarkerStart and completionMarkerEnd bracket the udit-managed
// region inside any rc file we touch. They match the markers already
// embedded in the printed scripts, so a user who hand-pasted earlier
// still gets a clean re-install.
const (
	completionMarkerStart = "# >>> udit completion >>>"
	completionMarkerEnd   = "# <<< udit completion <<<"
)

// completionInstallTarget describes what install does for one shell.
//
//	rcPath    — file we modify (or replace, in fish's case).
//	mode      — "block" wraps a `source <(udit completion …)` line in our
//	            markers and inserts/replaces between them in rcPath.
//	          — "file"  writes the full completion script as the file
//	            content (fish, where rc-style sourcing is the wrong shape).
type completionInstallTarget struct {
	shell  string
	rcPath string
	mode   string // "block" | "file"
}

// resolveCompletionTarget figures out where the completion goes for a
// given shell. Shell can be "" to autodetect.
func resolveCompletionTarget(shell string) (completionInstallTarget, error) {
	if shell == "" {
		shell = detectShell()
		if shell == "" {
			return completionInstallTarget{}, fmt.Errorf("could not auto-detect shell — pass --shell <bash|zsh|powershell|fish>")
		}
	}
	shell = strings.ToLower(shell)
	if shell == "pwsh" {
		shell = "powershell"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return completionInstallTarget{}, fmt.Errorf("user home dir: %w", err)
	}

	switch shell {
	case "bash":
		// macOS interactive bash sources ~/.bash_profile, not ~/.bashrc.
		// Linux + WSL go to ~/.bashrc. Pick the conventional spot per OS.
		if runtime.GOOS == "darwin" {
			return completionInstallTarget{shell: "bash", rcPath: filepath.Join(home, ".bash_profile"), mode: "block"}, nil
		}
		return completionInstallTarget{shell: "bash", rcPath: filepath.Join(home, ".bashrc"), mode: "block"}, nil
	case "zsh":
		return completionInstallTarget{shell: "zsh", rcPath: filepath.Join(home, ".zshrc"), mode: "block"}, nil
	case "fish":
		return completionInstallTarget{shell: "fish", rcPath: filepath.Join(home, ".config", "fish", "completions", "udit.fish"), mode: "file"}, nil
	case "powershell":
		// Honor $PROFILE when present (PowerShell sets it). Fall back to
		// the documented default location for the current platform.
		if p := os.Getenv("PROFILE"); p != "" {
			return completionInstallTarget{shell: "powershell", rcPath: p, mode: "block"}, nil
		}
		var p string
		if runtime.GOOS == "windows" {
			docs := os.Getenv("USERPROFILE")
			if docs == "" {
				docs = home
			}
			p = filepath.Join(docs, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
		} else {
			p = filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1")
		}
		return completionInstallTarget{shell: "powershell", rcPath: p, mode: "block"}, nil
	default:
		return completionInstallTarget{}, fmt.Errorf("unsupported shell %q (expected bash, zsh, powershell, or fish)", shell)
	}
}

// detectShell guesses the active shell from $SHELL (Unix) or platform
// defaults. Returns "" when it can't decide.
func detectShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		base := filepath.Base(s)
		switch base {
		case "bash", "zsh", "fish":
			return base
		}
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	// Unix without $SHELL set is unusual but possible (e.g. cron, CI).
	// Refuse to guess rather than write to the wrong file.
	return ""
}

// completionBlockBody returns the content that lives between the markers
// for "block"-mode shells.
func completionBlockBody(shell string) string {
	switch shell {
	case "bash":
		return "source <(udit completion bash)"
	case "zsh":
		return "source <(udit completion zsh)"
	case "powershell":
		return "udit completion powershell | Out-String | Invoke-Expression"
	}
	return ""
}

// completionFileContents returns the full file body for "file"-mode
// shells (currently only fish — the printed script is also the
// completion file).
func completionFileContents(shell string) string {
	if shell == "fish" {
		return fishScript
	}
	return ""
}

// runCompletionInstall persists completion for the requested (or
// auto-detected) shell. Idempotent: re-running replaces the marker
// block. Writes a .bak alongside before changing block-mode files.
func runCompletionInstall(args []string) error {
	shell, force := parseCompletionFlags(args)
	tgt, err := resolveCompletionTarget(shell)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(tgt.rcPath), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(tgt.rcPath), err)
	}

	switch tgt.mode {
	case "block":
		body := completionBlockBody(tgt.shell)
		changed, action, err := upsertMarkerBlock(tgt.rcPath, body, force)
		if err != nil {
			return err
		}
		if !changed {
			fmt.Printf("Completion already up to date for %s (%s)\n", tgt.shell, tgt.rcPath)
			return nil
		}
		fmt.Printf("Completion %s for %s → %s\n", action, tgt.shell, tgt.rcPath)
		fmt.Println("Open a new shell or `source` the file to activate.")
		return nil
	case "file":
		body := completionFileContents(tgt.shell)
		if body == "" {
			return fmt.Errorf("internal: no file contents for shell %q", tgt.shell)
		}
		if err := writeFileWithBackup(tgt.rcPath, []byte(body)); err != nil {
			return err
		}
		fmt.Printf("Completion installed for %s → %s\n", tgt.shell, tgt.rcPath)
		fmt.Println("Open a new shell to activate.")
		return nil
	}
	return fmt.Errorf("internal: unknown install mode %q", tgt.mode)
}

// runCompletionUninstall removes the marker block (block mode) or the
// file (file mode) for the given shell.
func runCompletionUninstall(args []string) error {
	shell, _ := parseCompletionFlags(args)
	tgt, err := resolveCompletionTarget(shell)
	if err != nil {
		return err
	}

	switch tgt.mode {
	case "block":
		removed, err := removeMarkerBlock(tgt.rcPath)
		if err != nil {
			return err
		}
		if !removed {
			fmt.Printf("No udit completion block found in %s\n", tgt.rcPath)
			return nil
		}
		fmt.Printf("Completion removed from %s\n", tgt.rcPath)
		return nil
	case "file":
		if _, err := os.Stat(tgt.rcPath); os.IsNotExist(err) {
			fmt.Printf("No completion file at %s\n", tgt.rcPath)
			return nil
		}
		if err := os.Remove(tgt.rcPath); err != nil {
			return fmt.Errorf("remove %s: %w", tgt.rcPath, err)
		}
		fmt.Printf("Completion file removed: %s\n", tgt.rcPath)
		return nil
	}
	return fmt.Errorf("internal: unknown install mode %q", tgt.mode)
}

// parseCompletionFlags pulls --shell / --force out of args. Anything else
// is silently ignored — the surface is small enough that we don't need a
// full flag.FlagSet.
func parseCompletionFlags(args []string) (shell string, force bool) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--shell", "-s":
			if i+1 < len(args) {
				shell = args[i+1]
				i++
			}
		case "--force", "-f":
			force = true
		}
	}
	return
}

// ----------------------------------------------------------------------
// Marker block file editing
// ----------------------------------------------------------------------

// upsertMarkerBlock makes sure rcPath contains exactly one marker-wrapped
// block whose body is the supplied line. Returns (changed, action, err)
// where action is "installed" or "updated" — useful for the user-visible
// log line. Writes a .bak when it changes anything.
//
// The `force` arg currently only affects the "no change needed" decision:
// when true, we rewrite the block even if it matches byte-for-byte. Useful
// for refreshing the body after a future format tweak.
func upsertMarkerBlock(rcPath, bodyLine string, force bool) (bool, string, error) {
	existing, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, "", fmt.Errorf("read %s: %w", rcPath, err)
	}
	current := string(existing)

	wantBlock := completionMarkerStart + "\n" + bodyLine + "\n" + completionMarkerEnd

	startIdx := strings.Index(current, completionMarkerStart)
	if startIdx >= 0 {
		endIdx := strings.Index(current[startIdx:], completionMarkerEnd)
		if endIdx < 0 {
			// Half-open marker — refuse rather than corrupting the file.
			return false, "", fmt.Errorf("%s contains a `%s` line without a matching `%s` — bailing out to avoid corrupting the file",
				rcPath, completionMarkerStart, completionMarkerEnd)
		}
		blockEnd := startIdx + endIdx + len(completionMarkerEnd)
		oldBlock := current[startIdx:blockEnd]
		if !force && oldBlock == wantBlock {
			return false, "", nil
		}
		updated := current[:startIdx] + wantBlock + current[blockEnd:]
		if err := writeFileWithBackup(rcPath, []byte(updated)); err != nil {
			return false, "", err
		}
		return true, "updated", nil
	}

	// No existing block — append, with a leading newline if the file
	// doesn't already end with one (rc files often do, but not always).
	var b strings.Builder
	b.WriteString(current)
	if len(current) > 0 && !strings.HasSuffix(current, "\n") {
		b.WriteByte('\n')
	}
	if len(current) > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(wantBlock)
	b.WriteByte('\n')
	if err := writeFileWithBackup(rcPath, []byte(b.String())); err != nil {
		return false, "", err
	}
	return true, "installed", nil
}

// removeMarkerBlock strips the udit-managed region from rcPath. Returns
// (removed, err) — false when no block was found (and the file is left
// alone). Writes a .bak when it changes anything.
func removeMarkerBlock(rcPath string) (bool, error) {
	existing, err := os.ReadFile(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", rcPath, err)
	}
	current := string(existing)

	startIdx := strings.Index(current, completionMarkerStart)
	if startIdx < 0 {
		return false, nil
	}
	endIdx := strings.Index(current[startIdx:], completionMarkerEnd)
	if endIdx < 0 {
		return false, fmt.Errorf("%s contains a `%s` line without a matching `%s` — leaving alone",
			rcPath, completionMarkerStart, completionMarkerEnd)
	}
	blockEnd := startIdx + endIdx + len(completionMarkerEnd)

	// Eat the trailing newline so we don't leave a blank line behind.
	if blockEnd < len(current) && current[blockEnd] == '\n' {
		blockEnd++
	}
	// Eat one leading blank line too, when we added one in install.
	stripStart := startIdx
	if stripStart > 0 && current[stripStart-1] == '\n' {
		// One newline ends the previous content; if there's another
		// blank line right before our marker, drop it too.
		if stripStart >= 2 && current[stripStart-2] == '\n' {
			stripStart--
		}
	}

	updated := current[:stripStart] + current[blockEnd:]
	if err := writeFileWithBackup(rcPath, []byte(updated)); err != nil {
		return false, err
	}
	return true, nil
}

// writeFileWithBackup atomically replaces path with data, leaving a
// .bak of the prior content next to it (when there was prior content).
// The temp+rename pattern means a partial write can't corrupt the rc
// file: either the new bytes are fully there, or the old file is.
func writeFileWithBackup(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", existing, 0o644); err != nil {
			return fmt.Errorf("backup %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s before backup: %w", path, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

const bashScript = `# >>> udit completion >>>
# udit bash completion
# Install: source <(udit completion bash)
#       or: udit completion bash | sudo tee /etc/bash_completion.d/udit
# Safe re-install — see README.md > "Shell completion".

_udit_complete() {
    local cur prev words cword
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    local commands="asset build completion component console editor exec go help list menu package prefab profiler project reserialize scene screenshot status test tx update version"
    local globals="--port --project --timeout --json --help"

    case "$prev" in
        --port|--project|--timeout)
            return ;;
        editor)
            COMPREPLY=( $(compgen -W "play stop pause refresh" -- "$cur") )
            return ;;
        scene)
            COMPREPLY=( $(compgen -W "list active open save reload tree" -- "$cur") )
            return ;;
        go)
            COMPREPLY=( $(compgen -W "find inspect path create destroy move rename setactive" -- "$cur") )
            return ;;
        component)
            COMPREPLY=( $(compgen -W "list get schema add remove set copy" -- "$cur") )
            return ;;
        asset)
            COMPREPLY=( $(compgen -W "find inspect dependencies references guid path create move delete label" -- "$cur") )
            return ;;
        prefab)
            COMPREPLY=( $(compgen -W "instantiate unpack apply find-instances" -- "$cur") )
            return ;;
        tx)
            COMPREPLY=( $(compgen -W "begin commit rollback status" -- "$cur") )
            return ;;
        project)
            COMPREPLY=( $(compgen -W "info validate preflight" -- "$cur") )
            return ;;
        package)
            COMPREPLY=( $(compgen -W "list add remove info search resolve" -- "$cur") )
            return ;;
        build)
            COMPREPLY=( $(compgen -W "player targets addressables cancel" -- "$cur") )
            return ;;
        test)
            COMPREPLY=( $(compgen -W "run list" -- "$cur") )
            return ;;
        profiler)
            COMPREPLY=( $(compgen -W "hierarchy enable disable status clear" -- "$cur") )
            return ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh powershell fish" -- "$cur") )
            return ;;
    esac

    if [[ "$cur" == --* ]]; then
        COMPREPLY=( $(compgen -W "$globals" -- "$cur") )
        return
    fi

    COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
}
complete -F _udit_complete udit
# <<< udit completion <<<
`

const zshScript = `#compdef udit
# >>> udit completion >>>
# udit zsh completion
# Install: source <(udit completion zsh)
#       or: udit completion zsh > "${fpath[1]}/_udit"
# Safe re-install — see README.md > "Shell completion".

_udit() {
    local -a commands subs

    case $words[2] in
        editor)
            subs=('play:Enter play mode' 'stop:Exit play mode' 'pause:Toggle pause' 'refresh:Refresh assets')
            _describe 'editor action' subs
            return ;;
        scene)
            subs=('list:List scenes' 'active:Describe active scene' 'open:Open scene' 'save:Save open scenes' 'reload:Reload active scene' 'tree:Dump hierarchy tree')
            _describe 'scene action' subs
            return ;;
        go)
            subs=(
                'find:Search GameObjects' 'inspect:Dump components + values' 'path:Hierarchy path string'
                'create:Spawn a GameObject' 'destroy:Destroy a GameObject' 'move:Reparent'
                'rename:Rename in place' 'setactive:Toggle active state'
            )
            _describe 'go action' subs
            return ;;
        component)
            subs=(
                'list:Enumerate components' 'get:Dump component or field' 'schema:Type schema'
                'add:Add a component' 'remove:Remove a component'
                'set:Set a field' 'copy:Copy between GameObjects'
            )
            _describe 'component action' subs
            return ;;
        asset)
            subs=(
                'find:Query assets' 'inspect:Asset metadata' 'dependencies:List deps' 'references:Reverse deps'
                'guid:Path to GUID' 'path:GUID to path'
                'create:Create a new asset' 'move:Move/rename' 'delete:Delete (trash or permanent)' 'label:Manage labels'
            )
            _describe 'asset action' subs
            return ;;
        prefab)
            subs=('instantiate:Spawn a prefab instance' 'unpack:Unpack an instance' 'apply:Apply overrides' 'find-instances:List instances')
            _describe 'prefab action' subs
            return ;;
        tx)
            subs=('begin:Start a transaction' 'commit:Collapse into one Undo entry' 'rollback:Revert all changes since begin' 'status:Is a transaction active?')
            _describe 'tx action' subs
            return ;;
        project)
            subs=('info:Project summary' 'validate:Scan for missing scripts + issues' 'preflight:Validate + build readiness')
            _describe 'project action' subs
            return ;;
        package)
            subs=('list:List declared (or --resolved) packages' 'add:Install by name | name@ver | git URL' 'remove:Uninstall by name' 'info:Package metadata + latest' 'search:Substring search registry' 'resolve:Force re-resolve manifest')
            _describe 'package action' subs
            return ;;
        build)
            subs=('player:Build a standalone player (long-running)' 'targets:List supported BuildTargets' 'addressables:Build Addressables content' 'cancel:Cancel an in-progress build')
            _describe 'build action' subs
            return ;;
        test)
            subs=('run:Execute tests' 'list:Enumerate tests without running')
            _describe 'test action' subs
            return ;;
        profiler)
            subs=('hierarchy:Sample hierarchy' 'enable:Start recording' 'disable:Stop recording' 'status:Show state' 'clear:Clear frames')
            _describe 'profiler action' subs
            return ;;
        completion)
            subs=('bash' 'zsh' 'powershell' 'fish')
            _describe 'shell' subs
            return ;;
    esac

    commands=(
        'editor:Play/stop/pause/refresh editor'
        'scene:List/open/save/reload scenes'
        'go:Query GameObjects (find/inspect/path)'
        'component:Read component values + schemas'
        'asset:Query assets (find/inspect/deps/refs/guid/path)'
        'prefab:Prefab instantiate/unpack/apply/find-instances'
        'tx:Group mutations into a single Undo entry'
        'project:Project info / validate / preflight'
        'package:UPM list/add/remove/info/search/resolve'
        'build:Player builds (player/targets/addressables/cancel)'
        'console:Read console logs'
        'exec:Execute C# code'
        'list:List all registered tools'
        'status:Show Unity Editor state'
        'test:Run EditMode/PlayMode tests'
        'profiler:Profiler control'
        'screenshot:Capture screenshot'
        'reserialize:Force reserialize assets'
        'menu:Execute Unity menu item'
        'update:Self-update CLI'
        'help:Show help'
        'version:Show version'
        'completion:Generate shell completion'
    )

    _arguments \
        '--port[Override Unity port]:port:' \
        '--project[Select Unity instance]:path:_files -/' \
        '--timeout[Request timeout in ms]:ms:' \
        '--json[Emit JSON envelope]' \
        '--help[Show help]' \
        '*::command:->cmds'

    case $state in
        cmds) _describe 'commands' commands ;;
    esac
}

compdef _udit udit
# <<< udit completion <<<
`

const powershellScript = `# >>> udit completion >>>
# udit PowerShell completion
# Install (current session): udit completion powershell | Out-String | Invoke-Expression
# Install (persisted)      : udit completion powershell >> $PROFILE
# Safe re-install — see README.md > "Shell completion".

Register-ArgumentCompleter -Native -CommandName udit -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @(
        'editor', 'scene', 'go', 'component', 'asset', 'prefab', 'tx', 'project', 'package', 'build', 'console', 'exec', 'list', 'status', 'test',
        'profiler', 'screenshot', 'reserialize', 'menu',
        'update', 'help', 'version', 'completion'
    )
    $globals = @('--port', '--project', '--timeout', '--json', '--help')

    $tokens = $commandAst.CommandElements | ForEach-Object { $_.ToString() }
    $previous = if ($tokens.Count -gt 1) { $tokens[$tokens.Count - 2] } else { '' }

    $candidates = switch ($previous) {
        'editor'     { @('play', 'stop', 'pause', 'refresh') }
        'scene'      { @('list', 'active', 'open', 'save', 'reload', 'tree') }
        'go'         { @('find', 'inspect', 'path', 'create', 'destroy', 'move', 'rename', 'setactive') }
        'component'  { @('list', 'get', 'schema', 'add', 'remove', 'set', 'copy') }
        'asset'      { @('find', 'inspect', 'dependencies', 'references', 'guid', 'path', 'create', 'move', 'delete', 'label') }
        'prefab'     { @('instantiate', 'unpack', 'apply', 'find-instances') }
        'tx'         { @('begin', 'commit', 'rollback', 'status') }
        'project'    { @('info', 'validate', 'preflight') }
        'package'    { @('list', 'add', 'remove', 'info', 'search', 'resolve') }
        'build'      { @('player', 'targets', 'addressables', 'cancel') }
        'test'       { @('run', 'list') }
        'profiler'   { @('hierarchy', 'enable', 'disable', 'status', 'clear') }
        'completion' { @('bash', 'zsh', 'powershell', 'fish') }
        '--port'     { @() }
        '--project'  { @() }
        '--timeout'  { @() }
        default {
            if ($wordToComplete -like '--*') { $globals } else { $commands }
        }
    }

    $candidates |
        Where-Object { $_ -like "$wordToComplete*" } |
        ForEach-Object {
            [System.Management.Automation.CompletionResult]::new(
                $_, $_, 'ParameterValue', $_
            )
        }
}
# <<< udit completion <<<
`

const fishScript = `# >>> udit completion >>>
# udit fish completion
# Install: udit completion fish > ~/.config/fish/completions/udit.fish
#
# (fish puts each completion in its own file, so there's no "re-install"
# hazard like the single-profile shells above. Overwriting is always safe.)

complete -c udit -n "__fish_use_subcommand" -a "editor"      -d "Play/stop/pause/refresh editor"
complete -c udit -n "__fish_use_subcommand" -a "scene"       -d "List/open/save/reload scenes"
complete -c udit -n "__fish_use_subcommand" -a "go"          -d "Query GameObjects (find/inspect/path)"
complete -c udit -n "__fish_use_subcommand" -a "component"   -d "Read component values + schemas"
complete -c udit -n "__fish_use_subcommand" -a "asset"       -d "Query assets (find/inspect/deps/refs/guid/path)"
complete -c udit -n "__fish_use_subcommand" -a "prefab"      -d "Prefab instantiate/unpack/apply/find-instances"
complete -c udit -n "__fish_use_subcommand" -a "tx"          -d "Group mutations into a single Undo entry"
complete -c udit -n "__fish_use_subcommand" -a "project"     -d "Project info / validate / preflight"
complete -c udit -n "__fish_use_subcommand" -a "package"     -d "UPM list/add/remove/info/search/resolve"
complete -c udit -n "__fish_use_subcommand" -a "build"       -d "Player builds (player/targets/addressables/cancel)"
complete -c udit -n "__fish_use_subcommand" -a "console"     -d "Read console logs"
complete -c udit -n "__fish_use_subcommand" -a "exec"        -d "Execute C# code"
complete -c udit -n "__fish_use_subcommand" -a "list"        -d "List registered tools"
complete -c udit -n "__fish_use_subcommand" -a "status"      -d "Show Unity Editor state"
complete -c udit -n "__fish_use_subcommand" -a "test"        -d "Run tests"
complete -c udit -n "__fish_use_subcommand" -a "profiler"    -d "Profiler control"
complete -c udit -n "__fish_use_subcommand" -a "screenshot"  -d "Capture screenshot"
complete -c udit -n "__fish_use_subcommand" -a "reserialize" -d "Reserialize assets"
complete -c udit -n "__fish_use_subcommand" -a "menu"        -d "Execute Unity menu item"
complete -c udit -n "__fish_use_subcommand" -a "update"      -d "Self-update CLI"
complete -c udit -n "__fish_use_subcommand" -a "version"     -d "Show version"
complete -c udit -n "__fish_use_subcommand" -a "completion"  -d "Generate shell completion"

complete -c udit -n "__fish_seen_subcommand_from editor"     -a "play stop pause refresh"
complete -c udit -n "__fish_seen_subcommand_from scene"      -a "list active open save reload tree"
complete -c udit -n "__fish_seen_subcommand_from go"         -a "find inspect path create destroy move rename setactive"
complete -c udit -n "__fish_seen_subcommand_from component"  -a "list get schema add remove set copy"
complete -c udit -n "__fish_seen_subcommand_from asset"      -a "find inspect dependencies references guid path create move delete label"
complete -c udit -n "__fish_seen_subcommand_from prefab"     -a "instantiate unpack apply find-instances"
complete -c udit -n "__fish_seen_subcommand_from tx"         -a "begin commit rollback status"
complete -c udit -n "__fish_seen_subcommand_from project"    -a "info validate preflight"
complete -c udit -n "__fish_seen_subcommand_from package"    -a "list add remove info search resolve"
complete -c udit -n "__fish_seen_subcommand_from build"      -a "player targets addressables cancel"
complete -c udit -n "__fish_seen_subcommand_from test"       -a "run list"
complete -c udit -n "__fish_seen_subcommand_from profiler"   -a "hierarchy enable disable status clear"
complete -c udit -n "__fish_seen_subcommand_from completion" -a "bash zsh powershell fish"

complete -c udit -l port    -d "Override Unity port"            -r
complete -c udit -l project -d "Select Unity instance by path"  -r
complete -c udit -l timeout -d "Request timeout (ms)"           -r
complete -c udit -l json    -d "Emit JSON envelope"
complete -c udit -l help    -d "Show help"
# <<< udit completion <<<
`
