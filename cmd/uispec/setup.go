package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// AgentDef defines how to detect and configure one AI agent.
type AgentDef struct {
	ID          string
	DisplayName string
	Method      string            // "cli" or "file"
	Binary      string            // for CLI agents: binary name on PATH
	DirMarkers  []string          // for file-based: dirs that indicate presence
	ConfigPath  func() string     // returns resolved config file path
	ServersKey  string            // JSON key: "servers" (VS Code) or "mcpServers" (others)
	NeedsScope  bool              // whether to prompt for project/user scope
	ExtraFields map[string]string // extra JSON fields (e.g. "type": "stdio" for VS Code)
}

// DetectedAgent is an agent found on the system.
type DetectedAgent struct {
	Def            AgentDef
	AlreadySetup   bool
	ResolvedConfig string // resolved config path for file-based agents
}

// setupOptions holds parsed flags for the setup command.
type setupOptions struct {
	auto bool
}

// Replaceable for testing.
var lookPathFunc = exec.LookPath
var statFunc = os.Stat

// agentRegistry lists all supported agents in display order.
var agentRegistry = []AgentDef{
	{
		ID: "claude_code", DisplayName: "Claude Code",
		Method: "cli", Binary: "claude", NeedsScope: true,
	},
	{
		ID: "openai_codex", DisplayName: "OpenAI Codex",
		Method: "cli", Binary: "codex", NeedsScope: true,
	},
	{
		ID: "vscode_copilot", DisplayName: "VS Code Copilot",
		Method: "file", DirMarkers: []string{".vscode"},
		ConfigPath: func() string { return filepath.Join(".vscode", "mcp.json") },
		ServersKey:  "servers",
		ExtraFields: map[string]string{"type": "stdio"},
	},
	{
		ID: "cursor", DisplayName: "Cursor",
		Method: "file", DirMarkers: []string{".cursor"},
		ConfigPath: func() string { return filepath.Join(".cursor", "mcp.json") },
		ServersKey: "mcpServers",
	},
	{
		ID: "claude_desktop", DisplayName: "Claude Desktop",
		Method:     "file",
		ConfigPath: claudeDesktopConfigPath,
		ServersKey: "mcpServers",
	},
}

// claudeDesktopConfigPath returns the OS-specific Claude Desktop config path.
func claudeDesktopConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json")
	default: // linux
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}
}

// detectAgents scans the system for installed/accessible AI agents.
func detectAgents() []DetectedAgent {
	var detected []DetectedAgent

	for _, def := range agentRegistry {
		switch def.Method {
		case "cli":
			if _, err := lookPathFunc(def.Binary); err == nil {
				d := DetectedAgent{Def: def}
				d.AlreadySetup = isAlreadyConfiguredCLI()
				detected = append(detected, d)
			}

		case "file":
			found := false
			configPath := ""

			// Check dir markers (project-level agents like VS Code, Cursor).
			for _, marker := range def.DirMarkers {
				if _, err := statFunc(marker); err == nil {
					found = true
					if def.ConfigPath != nil {
						configPath = def.ConfigPath()
					}
					break
				}
			}

			// For agents without dir markers (Claude Desktop), check if config parent dir exists.
			if !found && len(def.DirMarkers) == 0 && def.ConfigPath != nil {
				configPath = def.ConfigPath()
				parentDir := filepath.Dir(configPath)
				if _, err := statFunc(parentDir); err == nil {
					found = true
				}
			}

			if found {
				d := DetectedAgent{Def: def, ResolvedConfig: configPath}
				if configPath != "" {
					d.AlreadySetup = isAlreadyConfiguredFile(configPath, def.ServersKey)
				}
				detected = append(detected, d)
			}
		}
	}

	return detected
}

// isAlreadyConfiguredCLI checks if uispec is already in the project-level .mcp.json.
func isAlreadyConfiguredCLI() bool {
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		return false
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}
	// Check under "mcpServers" key.
	if servers, ok := config["mcpServers"].(map[string]any); ok {
		if _, exists := servers["uispec"]; exists {
			return true
		}
	}
	return false
}

// isAlreadyConfiguredFile checks if uispec entry exists in a JSON config file.
func isAlreadyConfiguredFile(configPath, serversKey string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}
	if servers, ok := config[serversKey].(map[string]any); ok {
		if _, exists := servers["uispec"]; exists {
			return true
		}
	}
	return false
}

// uispecServerEntry returns the MCP server config object for uispec.
func uispecServerEntry(extra map[string]string) map[string]any {
	entry := map[string]any{
		"command": "uispec",
		"args":    []any{"serve"},
	}
	for k, v := range extra {
		entry[k] = v
	}
	return entry
}

// mergeServerEntry reads existing JSON (or creates new), adds a "uispec" entry
// under serversKey, and returns the merged JSON bytes.
// Returns nil, nil if uispec is already configured (no-op).
func mergeServerEntry(existing []byte, serversKey string, extra map[string]string) ([]byte, error) {
	config := make(map[string]any)
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &config); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
	}

	servers, ok := config[serversKey].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}

	if _, exists := servers["uispec"]; exists {
		return nil, nil // already configured
	}

	servers["uispec"] = uispecServerEntry(extra)
	config[serversKey] = servers

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// configureCLIAgent runs `<binary> mcp add` with the chosen scope.
func configureCLIAgent(def AgentDef, scope string) error {
	args := []string{"mcp", "add"}
	if scope != "" {
		args = append(args, "--scope", scope)
	}
	args = append(args, "uispec", "--", "uispec", "serve")
	cmd := exec.Command(def.Binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// configureFileAgent reads, merges, and writes the JSON config file.
func configureFileAgent(def AgentDef, configPath string) error {
	// Create parent directory if needed.
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	var existing []byte
	if data, err := os.ReadFile(configPath); err == nil {
		existing = data
	}

	merged, err := mergeServerEntry(existing, def.ServersKey, def.ExtraFields)
	if err != nil {
		return err
	}
	if merged == nil {
		return nil // already configured
	}

	return os.WriteFile(configPath, merged, 0644)
}

// --- Interactive prompts ---

// promptYesNo prints a question and reads Y/n. Returns true for yes (default).
func promptYesNo(r io.Reader, w io.Writer, question string) bool {
	fmt.Fprintf(w, "%s ", question)
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return true // default yes on EOF
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}

// promptScope prints scope options and reads 1/2/3.
// Returns "project", "user", or "" (skip).
func promptScope(r io.Reader, w io.Writer, agentName string) string {
	fmt.Fprintf(w, "\n%s — add uispec MCP server?\n", agentName)
	fmt.Fprintln(w, "  [1] Project scope (shared with team)")
	fmt.Fprintln(w, "  [2] User scope (personal, global)")
	fmt.Fprintln(w, "  [3] Skip")
	fmt.Fprintf(w, "  > ")

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return "project" // default on EOF
	}
	switch strings.TrimSpace(scanner.Text()) {
	case "1", "":
		return "project"
	case "2":
		return "user"
	default:
		return "" // skip
	}
}

// --- Orchestration ---

// runSetup is the entry point for `uispec setup`.
func runSetup(args []string) {
	opts := parseSetupFlags(args)
	executeSetup(os.Stdin, os.Stdout, opts)
}

func parseSetupFlags(args []string) setupOptions {
	var opts setupOptions
	for _, arg := range args {
		if arg == "--auto" {
			opts.auto = true
		}
	}
	return opts
}

// executeSetup contains the testable core logic, parameterized on I/O.
func executeSetup(r io.Reader, w io.Writer, opts setupOptions) {
	detected := detectAgents()
	if len(detected) == 0 {
		fmt.Fprintln(w, "No supported AI agents detected.")
		return
	}

	// Print detection summary.
	fmt.Fprintln(w, "Detected AI agents:")
	for _, d := range detected {
		if d.AlreadySetup {
			fmt.Fprintf(w, "  * %s (already configured)\n", d.Def.DisplayName)
		} else {
			fmt.Fprintf(w, "  * %s\n", d.Def.DisplayName)
		}
	}
	fmt.Fprintln(w)

	// Global confirmation (unless --auto).
	if !opts.auto {
		if !promptYesNo(r, w, "Configure agents? [Y/n]") {
			return
		}
	}

	// Per-agent configuration.
	for _, d := range detected {
		if d.AlreadySetup {
			fmt.Fprintf(w, "\n%s — already configured, skipping\n", d.Def.DisplayName)
			continue
		}
		configureOneAgent(r, w, d, opts)
	}
}

func configureOneAgent(r io.Reader, w io.Writer, d DetectedAgent, opts setupOptions) {
	switch d.Def.Method {
	case "cli":
		scope := "project" // default for --auto
		if !opts.auto && d.Def.NeedsScope {
			scope = promptScope(r, w, d.Def.DisplayName)
			if scope == "" {
				fmt.Fprintf(w, "  skipped\n")
				return
			}
		}
		if err := configureCLIAgent(d.Def, scope); err != nil {
			fmt.Fprintf(w, "  ! %s: failed: %v\n", d.Def.DisplayName, err)
			return
		}
		fmt.Fprintf(w, "  + %s configured (scope: %s)\n", d.Def.DisplayName, scope)

	case "file":
		if !opts.auto {
			if !promptYesNo(r, w, fmt.Sprintf("\n%s — add to %s? [Y/n]", d.Def.DisplayName, d.ResolvedConfig)) {
				fmt.Fprintf(w, "  skipped\n")
				return
			}
		}
		if err := configureFileAgent(d.Def, d.ResolvedConfig); err != nil {
			fmt.Fprintf(w, "  ! %s: failed: %v\n", d.Def.DisplayName, err)
			return
		}
		fmt.Fprintf(w, "  + %s configured (%s)\n", d.Def.DisplayName, d.ResolvedConfig)
	}
}
