package cmd

import (
	"fmt"
	"os"
	"strings"
)

// Built-in commands appear inline in each shell script. Custom [UditTool]
// handlers aren't included because completion runs without a live Unity to
// query; static commands cover 95% of daily typing.

const completionUsage = `Usage: udit completion <bash|zsh|powershell|fish>

Print a shell completion script. Source it (or persist to a known location)
to enable Tab completion for udit commands and global flags.

  Bash       (sourced)   : source <(udit completion bash)
  Bash       (persisted) : udit completion bash | sudo tee /etc/bash_completion.d/udit
  Zsh        (sourced)   : source <(udit completion zsh)
  Zsh        (persisted) : udit completion zsh > "${fpath[1]}/_udit"
  PowerShell             : udit completion powershell | Out-String | Invoke-Expression
  PowerShell (persisted) : udit completion powershell >> $PROFILE
  Fish                   : udit completion fish > ~/.config/fish/completions/udit.fish
`

// completionCmd dispatches to the right shell-script generator and writes
// to stdout so users can `source <(...)` or pipe to a file.
func completionCmd(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, completionUsage)
		os.Exit(1)
	}
	switch strings.ToLower(args[0]) {
	case "bash":
		fmt.Print(bashScript)
	case "zsh":
		fmt.Print(zshScript)
	case "powershell", "pwsh":
		fmt.Print(powershellScript)
	case "fish":
		fmt.Print(fishScript)
	default:
		fmt.Fprintf(os.Stderr, "unknown shell: %q\n\n%s", args[0], completionUsage)
		os.Exit(1)
	}
	return nil
}

const bashScript = `# udit bash completion
# Install: source <(udit completion bash)
#       or: udit completion bash | sudo tee /etc/bash_completion.d/udit

_udit_complete() {
    local cur prev words cword
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    local commands="completion console editor exec help list menu profiler reserialize screenshot status test update version"
    local globals="--port --project --timeout --json --help"

    case "$prev" in
        --port|--project|--timeout)
            return ;;
        editor)
            COMPREPLY=( $(compgen -W "play stop pause refresh" -- "$cur") )
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
`

const zshScript = `#compdef udit
# udit zsh completion
# Install: source <(udit completion zsh)
#       or: udit completion zsh > "${fpath[1]}/_udit"

_udit() {
    local -a commands subs

    case $words[2] in
        editor)
            subs=('play:Enter play mode' 'stop:Exit play mode' 'pause:Toggle pause' 'refresh:Refresh assets')
            _describe 'editor action' subs
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
`

const powershellScript = `# udit PowerShell completion
# Install (current session): udit completion powershell | Out-String | Invoke-Expression
# Install (persisted)      : udit completion powershell >> $PROFILE

Register-ArgumentCompleter -Native -CommandName udit -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @(
        'editor', 'console', 'exec', 'list', 'status', 'test',
        'profiler', 'screenshot', 'reserialize', 'menu',
        'update', 'help', 'version', 'completion'
    )
    $globals = @('--port', '--project', '--timeout', '--json', '--help')

    $tokens = $commandAst.CommandElements | ForEach-Object { $_.ToString() }
    $previous = if ($tokens.Count -gt 1) { $tokens[$tokens.Count - 2] } else { '' }

    $candidates = switch ($previous) {
        'editor'     { @('play', 'stop', 'pause', 'refresh') }
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
`

const fishScript = `# udit fish completion
# Install: udit completion fish > ~/.config/fish/completions/udit.fish

complete -c udit -n "__fish_use_subcommand" -a "editor"      -d "Play/stop/pause/refresh editor"
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
complete -c udit -n "__fish_seen_subcommand_from profiler"   -a "hierarchy enable disable status clear"
complete -c udit -n "__fish_seen_subcommand_from completion" -a "bash zsh powershell fish"

complete -c udit -l port    -d "Override Unity port"            -r
complete -c udit -l project -d "Select Unity instance by path"  -r
complete -c udit -l timeout -d "Request timeout (ms)"           -r
complete -c udit -l json    -d "Emit JSON envelope"
complete -c udit -l help    -d "Show help"
`
