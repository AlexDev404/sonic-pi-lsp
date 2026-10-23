package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
	"github.com/AlexDev404/sonic-pi-lsp/internal/prefs"
)

func init() {
	register(&command{
		name:    "config",
		aliases: []string{"prefs", "settings"},
		summary: "read and change the settings shared with the desktop app",
		usage:   "sonicpi config [list|get KEY|set KEY VALUE|unset KEY|path [FILE]|edit [FILE]]",
		run:     cmdConfig,
	})
}

// configFiles are the files `config path` and `config edit` know.
func configFiles() map[string]string {
	return map[string]string{
		"gui":       paths.GUISettingsPath(),
		"audio":     paths.AudioSettingsPath(),
		"tau":       paths.TauSettingsPath(),
		"init":      paths.InitPath(),
		"theme":     paths.ColourThemePath(),
		"shortcuts": paths.ShortcutsPath(),
		"dir":       paths.ConfigDir(),
	}
}

func cmdConfig(g *Globals, args []string) error {
	fs := newFlags(g, "config", "sonicpi config [list|get|set|unset|path|edit] ...")
	all := fs.Bool("all", false, "list: include desktop-only settings and unknown keys found in the files")
	noApply := fs.Bool("no-apply", false, "set/unset: don't apply the change to the running server")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	root, _ := paths.Root()
	st, err := prefs.Load(root)
	if err != nil {
		return err
	}
	switch sub {
	case "list", "ls":
		return configList(g, st, *all)
	case "get":
		if len(pos) != 1 {
			return usageError("config get takes one key")
		}
		s, err := prefs.Lookup(pos[0])
		if err != nil {
			return err
		}
		v, _ := st.Get(s)
		fmt.Println(v)
		return nil
	case "set":
		if len(pos) != 2 {
			return usageError("config set takes a key and a value")
		}
		s, err := prefs.Lookup(pos[0])
		if err != nil {
			return err
		}
		if err := st.Set(s, pos[1]); err != nil {
			return err
		}
		return afterChange(g, s, *noApply)
	case "unset", "reset":
		if len(pos) != 1 {
			return usageError("config unset takes one key")
		}
		s, err := prefs.Lookup(pos[0])
		if err != nil {
			return err
		}
		if err := st.Unset(s); err != nil {
			return err
		}
		return afterChange(g, s, *noApply)
	case "path":
		files := configFiles()
		if len(pos) == 0 {
			fmt.Println(files["dir"])
			return nil
		}
		p, ok := files[pos[0]]
		if !ok {
			return usageError("unknown config file %q (gui, audio, tau, init, theme, shortcuts, dir)", pos[0])
		}
		fmt.Println(p)
		return nil
	case "edit":
		which := "audio"
		if len(pos) > 0 {
			which = pos[0]
		}
		p, ok := configFiles()[which]
		if !ok || which == "dir" {
			return usageError("unknown config file %q (gui, audio, tau, init, theme, shortcuts)", which)
		}
		if which == "audio" {
			// Seed it from the desktop's commented template, as the server does.
			if t, err := prefs.LoadTOML(p, templatePath(root)); err == nil {
				if _, err := os.Stat(p); os.IsNotExist(err) {
					t.Save()
				}
			}
		}
		if err := edit(p); err != nil {
			return err
		}
		switch which {
		case "audio", "tau", "init":
			fmt.Fprintln(os.Stderr, "Restart the server for this to take effect: sonicpi reboot")
		}
		return nil
	}
	return usageError("unknown config command %q", sub)
}

func templatePath(root string) string {
	if root == "" {
		return ""
	}
	return filepath.Join(root, "app", "config", "user-examples", paths.CurrentLayout().Audio)
}

func configList(g *Globals, st *prefs.Store, all bool) error {
	type row struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Default string `json:"default"`
		Set     bool   `json:"set"`
		Effect  string `json:"effect"`
		File    string `json:"file"`
		Doc     string `json:"doc,omitempty"`
	}
	var rows []row
	major := paths.CurrentLayout().Major
	for _, s := range prefs.Known {
		if !s.For(major) || (s.Effect == prefs.DesktopOnly && !all) {
			continue
		}
		v, set := st.Get(s)
		if s.File != prefs.GUI && !set && !all {
			continue
		}
		rows = append(rows, row{s.Key, v, s.Default, set, s.Effect.String(), string(s.File), s.Doc})
	}
	if all {
		for _, k := range st.Unknown() {
			s, _ := prefs.Lookup(k)
			v, _ := st.Get(s)
			rows = append(rows, row{Key: k, Value: v, Set: true, Effect: s.Effect.String(), File: string(s.File)})
		}
	}
	if g.JSON {
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE\tTAKES EFFECT\tDESCRIPTION")
	for _, r := range rows {
		v := r.Value
		if !r.Set && v != "" {
			v += " (default)"
		}
		if v == "" {
			v = `""`
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Key, v, r.Effect, r.Doc)
	}
	tw.Flush()
	if !all && !g.Quiet {
		fmt.Printf("\nFiles: %s\n       %s\n", paths.GUISettingsPath(), paths.AudioSettingsPath())
		fmt.Println("`sonicpi config list --all` also shows audio, Tau and desktop-only settings.")
	}
	return nil
}

// afterChange applies a live setting to the running server, or says when
// the change will take effect.
func afterChange(g *Globals, s prefs.Setting, noApply bool) error {
	switch s.Effect {
	case prefs.Live:
		if noApply {
			return nil
		}
		c, err := control.Dial()
		if err != nil {
			return nil // nothing running; the service applies it at boot
		}
		if err := c.Call("apply-prefs", nil, nil); err != nil {
			return err
		}
		if !g.Quiet {
			fmt.Fprintln(os.Stderr, "Applied to the running server.")
		}
	case prefs.Restart:
		if !g.Quiet {
			fmt.Fprintln(os.Stderr, "This takes effect when the server restarts: sonicpi reboot")
		}
	case prefs.DesktopOnly:
		if !g.Quiet {
			fmt.Fprintln(os.Stderr, "Saved. Only the desktop app uses this setting.")
		}
	}
	return nil
}

// edit opens path in the user's editor.
func edit(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			for _, e := range []string{"sensible-editor", "nano", "vi"} {
				if _, err := exec.LookPath(e); err == nil {
					editor = e
					break
				}
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("set $EDITOR to edit %s", path)
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
