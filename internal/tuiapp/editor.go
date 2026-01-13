package tuiapp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type editorResultMsg struct {
	question   string
	entryIndex int
	responses  []string
	changed    bool
	err        error
}

func editEntriesCmd(question string, lines []string, entryIndex int) tea.Cmd {
	original := strings.Join(lines, "\n")
	tmp, err := os.CreateTemp("", "wlog-edit-*.txt")
	if err != nil {
		return func() tea.Msg { return editorResultMsg{question: question, entryIndex: entryIndex, err: err} }
	}

	if original != "" {
		if _, err := tmp.WriteString(original + "\n"); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return func() tea.Msg { return editorResultMsg{question: question, entryIndex: entryIndex, err: err} }
		}
	}
	tmp.Close()

	cmd, cmdErr := buildEditorCommand(tmp.Name())
	if cmdErr != nil {
		os.Remove(tmp.Name())
		return func() tea.Msg { return editorResultMsg{question: question, entryIndex: entryIndex, err: cmdErr} }
	}

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(tmp.Name())
		if err != nil {
			return editorResultMsg{question: question, entryIndex: entryIndex, err: err}
		}
		data, readErr := os.ReadFile(tmp.Name())
		if readErr != nil {
			return editorResultMsg{question: question, entryIndex: entryIndex, err: readErr}
		}
		newContent := normalizeEditorContent(string(data))
		originalContent := normalizeEditorContent(original)
		if newContent == originalContent {
			return editorResultMsg{question: question, entryIndex: entryIndex, responses: lines, changed: false}
		}
		if entryIndex >= 0 {
			return editorResultMsg{question: question, entryIndex: entryIndex, responses: []string{newContent}, changed: true}
		}
		responses := parseEditorLines(newContent)
		return editorResultMsg{question: question, entryIndex: entryIndex, responses: responses, changed: true}
	})
}

func buildEditorCommand(path string) (*exec.Cmd, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{editor}
	}
	parts = append(parts, path)
	if _, err := exec.LookPath(parts[0]); err != nil {
		return nil, fmt.Errorf("unable to launch editor %q: %w", parts[0], err)
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func normalizeEditorContent(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.TrimRight(value, "\n")
	return value
}

func parseEditorLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
