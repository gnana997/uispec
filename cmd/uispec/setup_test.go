package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- JSON merge tests ---

func TestMergeServerEntry_EmptyFile(t *testing.T) {
	out, err := mergeServerEntry(nil, "mcpServers", nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	var config map[string]any
	require.NoError(t, json.Unmarshal(out, &config))

	servers := config["mcpServers"].(map[string]any)
	uispec := servers["uispec"].(map[string]any)
	assert.Equal(t, "uispec", uispec["command"])
	assert.Equal(t, []any{"serve"}, uispec["args"])
}

func TestMergeServerEntry_ExistingServers(t *testing.T) {
	existing := []byte(`{
  "mcpServers": {
    "other-server": {
      "command": "other",
      "args": ["start"]
    }
  }
}`)
	out, err := mergeServerEntry(existing, "mcpServers", nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	var config map[string]any
	require.NoError(t, json.Unmarshal(out, &config))

	servers := config["mcpServers"].(map[string]any)
	// Original server preserved.
	assert.Contains(t, servers, "other-server")
	// New server added.
	assert.Contains(t, servers, "uispec")
}

func TestMergeServerEntry_AlreadyConfigured(t *testing.T) {
	existing := []byte(`{
  "mcpServers": {
    "uispec": {
      "command": "uispec",
      "args": ["serve"]
    }
  }
}`)
	out, err := mergeServerEntry(existing, "mcpServers", nil)
	assert.NoError(t, err)
	assert.Nil(t, out, "should return nil when already configured")
}

func TestMergeServerEntry_VSCodeFormat(t *testing.T) {
	out, err := mergeServerEntry(nil, "servers", map[string]string{"type": "stdio"})
	require.NoError(t, err)
	require.NotNil(t, out)

	var config map[string]any
	require.NoError(t, json.Unmarshal(out, &config))

	servers := config["servers"].(map[string]any)
	uispec := servers["uispec"].(map[string]any)
	assert.Equal(t, "uispec", uispec["command"])
	assert.Equal(t, "stdio", uispec["type"])
}

func TestMergeServerEntry_InvalidJSON(t *testing.T) {
	_, err := mergeServerEntry([]byte("not json"), "mcpServers", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestMergeServerEntry_TrailingNewline(t *testing.T) {
	out, err := mergeServerEntry(nil, "mcpServers", nil)
	require.NoError(t, err)
	assert.True(t, out[len(out)-1] == '\n', "output should end with newline")
}

// --- Prompt tests ---

func TestPromptYesNo_DefaultYes(t *testing.T) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	assert.True(t, promptYesNo(r, w, "Continue?"))
}

func TestPromptYesNo_ExplicitYes(t *testing.T) {
	r := strings.NewReader("y\n")
	w := &bytes.Buffer{}
	assert.True(t, promptYesNo(r, w, "Continue?"))
}

func TestPromptYesNo_ExplicitNo(t *testing.T) {
	r := strings.NewReader("n\n")
	w := &bytes.Buffer{}
	assert.False(t, promptYesNo(r, w, "Continue?"))
}

func TestPromptYesNo_EOF(t *testing.T) {
	r := strings.NewReader("") // EOF
	w := &bytes.Buffer{}
	assert.True(t, promptYesNo(r, w, "Continue?"), "should default to yes on EOF")
}

func TestPromptScope_Project(t *testing.T) {
	r := strings.NewReader("1\n")
	w := &bytes.Buffer{}
	assert.Equal(t, "project", promptScope(r, w, "Claude Code"))
}

func TestPromptScope_User(t *testing.T) {
	r := strings.NewReader("2\n")
	w := &bytes.Buffer{}
	assert.Equal(t, "user", promptScope(r, w, "Claude Code"))
}

func TestPromptScope_Skip(t *testing.T) {
	r := strings.NewReader("3\n")
	w := &bytes.Buffer{}
	assert.Equal(t, "", promptScope(r, w, "Claude Code"))
}

func TestPromptScope_DefaultProject(t *testing.T) {
	r := strings.NewReader("\n") // empty = default
	w := &bytes.Buffer{}
	assert.Equal(t, "project", promptScope(r, w, "Claude Code"))
}

// --- Detection tests ---

func TestDetectAgents_CLIOnPath(t *testing.T) {
	origLookPath := lookPathFunc
	origStat := statFunc
	defer func() {
		lookPathFunc = origLookPath
		statFunc = origStat
	}()

	lookPathFunc = func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", exec.ErrNotFound
	}
	statFunc = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	detected := detectAgents()
	require.Len(t, detected, 1)
	assert.Equal(t, "claude_code", detected[0].Def.ID)
}

func TestDetectAgents_NoneDetected(t *testing.T) {
	origLookPath := lookPathFunc
	origStat := statFunc
	defer func() {
		lookPathFunc = origLookPath
		statFunc = origStat
	}()

	lookPathFunc = func(name string) (string, error) {
		return "", exec.ErrNotFound
	}
	statFunc = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	detected := detectAgents()
	assert.Empty(t, detected)
}

func TestDetectAgents_FileBasedAgent(t *testing.T) {
	origLookPath := lookPathFunc
	origStat := statFunc
	defer func() {
		lookPathFunc = origLookPath
		statFunc = origStat
	}()

	lookPathFunc = func(name string) (string, error) {
		return "", exec.ErrNotFound
	}
	statFunc = func(name string) (os.FileInfo, error) {
		if name == ".vscode" {
			return nil, nil // exists
		}
		return nil, os.ErrNotExist
	}

	detected := detectAgents()
	require.Len(t, detected, 1)
	assert.Equal(t, "vscode_copilot", detected[0].Def.ID)
	assert.Equal(t, filepath.Join(".vscode", "mcp.json"), detected[0].ResolvedConfig)
}

// --- Integration tests ---

func TestExecuteSetup_NoAgents(t *testing.T) {
	origLookPath := lookPathFunc
	origStat := statFunc
	defer func() {
		lookPathFunc = origLookPath
		statFunc = origStat
	}()

	lookPathFunc = func(name string) (string, error) { return "", exec.ErrNotFound }
	statFunc = func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	r := strings.NewReader("")
	w := &bytes.Buffer{}
	executeSetup(r, w, setupOptions{})

	assert.Contains(t, w.String(), "No supported AI agents detected.")
}

func TestExecuteSetup_AutoModeFileAgent(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	// Create .vscode dir to simulate VS Code presence.
	require.NoError(t, os.MkdirAll(".vscode", 0755))

	origLookPath := lookPathFunc
	origStat := statFunc
	defer func() {
		lookPathFunc = origLookPath
		statFunc = origStat
	}()

	lookPathFunc = func(name string) (string, error) { return "", exec.ErrNotFound }
	statFunc = os.Stat // use real stat within temp dir

	r := strings.NewReader("")
	w := &bytes.Buffer{}
	executeSetup(r, w, setupOptions{auto: true})

	// Verify the config file was written.
	data, err := os.ReadFile(filepath.Join(".vscode", "mcp.json"))
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, json.Unmarshal(data, &config))

	servers := config["servers"].(map[string]any)
	uispec := servers["uispec"].(map[string]any)
	assert.Equal(t, "uispec", uispec["command"])
	assert.Equal(t, "stdio", uispec["type"])

	assert.Contains(t, w.String(), "VS Code Copilot configured")
}

func TestConfigureFileAgent_CreatesAndMerges(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "sub", "mcp.json")

	def := AgentDef{
		ServersKey:  "mcpServers",
		ExtraFields: nil,
	}

	require.NoError(t, configureFileAgent(def, configPath))

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, json.Unmarshal(data, &config))

	servers := config["mcpServers"].(map[string]any)
	assert.Contains(t, servers, "uispec")
}

func TestConfigureFileAgent_MergesExisting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")

	existing := []byte(`{"mcpServers": {"other": {"command": "other"}}}`)
	require.NoError(t, os.WriteFile(configPath, existing, 0644))

	def := AgentDef{ServersKey: "mcpServers"}
	require.NoError(t, configureFileAgent(def, configPath))

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, json.Unmarshal(data, &config))

	servers := config["mcpServers"].(map[string]any)
	assert.Contains(t, servers, "other", "original server should be preserved")
	assert.Contains(t, servers, "uispec", "uispec should be added")
}
