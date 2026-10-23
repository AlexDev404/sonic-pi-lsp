package cli

import (
	"fmt"
	"sort"
	"strings"
)

func init() {
	register(&command{
		name:    "completion",
		summary: "print a shell completion script (bash, zsh, fish, powershell)",
		usage:   "sonicpi completion bash|zsh|fish|powershell",
		run:     cmdCompletion,
	})
}

func commandNames() []string {
	var names []string
	for _, c := range commands {
		names = append(names, c.name)
	}
	sort.Strings(names)
	return names
}

func cmdCompletion(g *Globals, args []string) error {
	fs := newFlags(g, "completion", "sonicpi completion bash|zsh|fish|powershell")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("completion takes a shell name")
	}
	words := strings.Join(commandNames(), " ")
	switch pos[0] {
	case "bash":
		fmt.Printf(`# sonicpi bash completion: source <(sonicpi completion bash)
_sonicpi() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=( $(compgen -W "%s" -- "$cur") $(compgen -f -X '!*.rb' -- "$cur") $(compgen -d -- "$cur") )
    return
  fi
  case "${COMP_WORDS[1]}" in
    config) COMPREPLY=( $(compgen -W "list get set unset path edit" -- "$cur") ) ;;
    service) COMPREPLY=( $(compgen -W "status start stop restart install uninstall run" -- "$cur") ) ;;
    logs) COMPREPLY=( $(compgen -W "-f --watch -n --synths --no-synths --cues --no-cues --puts --no-puts --info --no-info --errors --run --thread --timestamps --since --source --path --clear --json" -- "$cur") ) ;;
    midi) COMPREPLY=( $(compgen -W "ports enable disable reset channel" -- "$cur") ) ;;
    osc) COMPREPLY=( $(compgen -W "status enable disable public private send" -- "$cur") ) ;;
    link) COMPREPLY=( $(compgen -W "status enable disable bpm reset" -- "$cur") ) ;;
    buffer) COMPREPLY=( $(compgen -W "list show run edit load save path" -- "$cur") ) ;;
    record) COMPREPLY=( $(compgen -W "start stop save discard" -- "$cur") $(compgen -f -- "$cur") ) ;;
    *) COMPREPLY=( $(compgen -f -- "$cur") ) ;;
  esac
}
complete -o filenames -F _sonicpi sonicpi
`, words)
	case "zsh":
		fmt.Printf(`#compdef sonicpi
# sonicpi zsh completion: sonicpi completion zsh > "${fpath[1]}/_sonicpi"
_sonicpi() {
  if (( CURRENT == 2 )); then
    _alternative 'commands:command:(%s)' 'files:file:_files -g "*.rb"'
  else
    _files
  fi
}
_sonicpi "$@"
`, words)
	case "fish":
		fmt.Printf("# sonicpi fish completion: sonicpi completion fish | source\ncomplete -c sonicpi -n '__fish_use_subcommand' -a '%s'\n", words)
	case "powershell", "pwsh":
		fmt.Printf(`# sonicpi PowerShell completion: sonicpi completion powershell | Out-String | Invoke-Expression
Register-ArgumentCompleter -Native -CommandName sonicpi -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $cmds = '%s' -split ' '
  if ($commandAst.CommandElements.Count -le 2) {
    $cmds | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
    Get-ChildItem -Filter "$wordToComplete*.rb" | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_.Name, $_.Name, 'ProviderItem', $_.Name) }
  }
}
`, words)
	default:
		return usageError("unknown shell %q", pos[0])
	}
	return nil
}
