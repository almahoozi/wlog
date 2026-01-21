package tuiapp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/almahoozi/wlog/internal/app"
)

type cfgRowKind int

const (
	cfgRowQuestion cfgRowKind = iota
	cfgRowAddQuestion
	cfgRowBool
	cfgRowInt
)

type configField int

const (
	cfgFieldShowHints configField = iota
	cfgFieldAutoInsert
	cfgFieldContinueInsertAfterSave
	cfgFieldDefaultListMode
	cfgFieldAutoOpenIndex
	cfgFieldConfirmDelete
	cfgFieldConfirmEscapeWithText
	cfgFieldStatusDuration
	cfgFieldEscapeConfirmTimeout
)

type configRow struct {
	kind  cfgRowKind
	index int
	field configField
}

type configValues struct {
	Questions                     []string
	ShowHints                     bool
	ShowHintsCustom               bool
	AutoInsert                    bool
	AutoInsertCustom              bool
	ContinueInsertAfterSave       bool
	ContinueInsertAfterSaveCustom bool
	DefaultListMode               bool
	DefaultListModeCustom         bool
	AutoOpenIndexJump             bool
	AutoOpenIndexCustom           bool
	ConfirmDelete                 bool
	ConfirmDeleteCustom           bool
	ConfirmEscapeWithText         bool
	ConfirmEscapeWithTextCustom   bool
	StatusDuration                int
	StatusDurationSet             bool
	EscapeConfirmTimeout          int
	EscapeConfirmTimeoutSet       bool
}

func newConfigValues(cfg app.Config) configValues {
	values := configValues{
		Questions:                     append([]string(nil), cfg.Questions...),
		ShowHints:                     cfg.HintsEnabled(),
		ShowHintsCustom:               cfg.ShowHints != nil,
		AutoInsert:                    cfg.AutoInsertEnabled(),
		AutoInsertCustom:              cfg.AutoInsertEntries != nil,
		ContinueInsertAfterSave:       cfg.ContinueInsertAfterSaveEnabled(),
		ContinueInsertAfterSaveCustom: cfg.ContinueInsertAfterSave != nil,
		DefaultListMode:               cfg.DefaultListModeEnabled(),
		DefaultListModeCustom:         cfg.DefaultListMode != nil,
		AutoOpenIndexJump:             cfg.AutoOpenIndexJumpEnabled(),
		AutoOpenIndexCustom:           cfg.AutoOpenIndexJump != nil,
		ConfirmDelete:                 cfg.ConfirmDeleteEnabled(),
		ConfirmDeleteCustom:           cfg.ConfirmDelete != nil,
		ConfirmEscapeWithText:         cfg.ConfirmEscapeWithTextEnabled(),
		ConfirmEscapeWithTextCustom:   cfg.ConfirmEscapeWithText != nil,
	}
	resolved := int(cfg.StatusMessageDuration() / time.Millisecond)
	if resolved <= 0 {
		resolved = 2000
	}
	values.StatusDuration = resolved
	if cfg.StatusMessageDurationMs != nil && *cfg.StatusMessageDurationMs > 0 {
		values.StatusDurationSet = true
		values.StatusDuration = *cfg.StatusMessageDurationMs
	}

	escapeResolved := int(cfg.EscapeConfirmTimeout() / time.Millisecond)
	if escapeResolved <= 0 {
		escapeResolved = 1000
	}
	values.EscapeConfirmTimeout = escapeResolved
	if cfg.EscapeConfirmTimeoutMs != nil && *cfg.EscapeConfirmTimeoutMs > 0 {
		values.EscapeConfirmTimeoutSet = true
		values.EscapeConfirmTimeout = *cfg.EscapeConfirmTimeoutMs
	}
	return values
}

func (v configValues) clone() configValues {
	copyVals := v
	copyVals.Questions = append([]string(nil), v.Questions...)
	return copyVals
}

func (v configValues) equal(other configValues) bool {
	if len(v.Questions) != len(other.Questions) {
		return false
	}
	for i := range v.Questions {
		if v.Questions[i] != other.Questions[i] {
			return false
		}
	}
	return v.ShowHints == other.ShowHints &&
		v.ShowHintsCustom == other.ShowHintsCustom &&
		v.AutoInsert == other.AutoInsert &&
		v.AutoInsertCustom == other.AutoInsertCustom &&
		v.ContinueInsertAfterSave == other.ContinueInsertAfterSave &&
		v.ContinueInsertAfterSaveCustom == other.ContinueInsertAfterSaveCustom &&
		v.DefaultListMode == other.DefaultListMode &&
		v.DefaultListModeCustom == other.DefaultListModeCustom &&
		v.AutoOpenIndexJump == other.AutoOpenIndexJump &&
		v.AutoOpenIndexCustom == other.AutoOpenIndexCustom &&
		v.ConfirmDelete == other.ConfirmDelete &&
		v.ConfirmDeleteCustom == other.ConfirmDeleteCustom &&
		v.ConfirmEscapeWithText == other.ConfirmEscapeWithText &&
		v.ConfirmEscapeWithTextCustom == other.ConfirmEscapeWithTextCustom &&
		v.StatusDuration == other.StatusDuration &&
		v.StatusDurationSet == other.StatusDurationSet &&
		v.EscapeConfirmTimeout == other.EscapeConfirmTimeout &&
		v.EscapeConfirmTimeoutSet == other.EscapeConfirmTimeoutSet
}

func (v configValues) toConfig() app.Config {
	cfg := app.Config{Questions: append([]string(nil), v.Questions...)}
	if v.ShowHintsCustom {
		cfg.ShowHints = boolPtr(v.ShowHints)
	}
	if v.AutoInsertCustom {
		cfg.AutoInsertEntries = boolPtr(v.AutoInsert)
	}
	if v.DefaultListModeCustom {
		cfg.DefaultListMode = boolPtr(v.DefaultListMode)
	}
	if v.AutoOpenIndexCustom {
		cfg.AutoOpenIndexJump = boolPtr(v.AutoOpenIndexJump)
	}
	if v.ConfirmDeleteCustom {
		cfg.ConfirmDelete = boolPtr(v.ConfirmDelete)
	}
	if v.ContinueInsertAfterSaveCustom {
		cfg.ContinueInsertAfterSave = boolPtr(v.ContinueInsertAfterSave)
	}
	if v.ConfirmEscapeWithTextCustom {
		cfg.ConfirmEscapeWithText = boolPtr(v.ConfirmEscapeWithText)
	}
	if v.StatusDurationSet {
		cfg.StatusMessageDurationMs = intPtr(v.StatusDuration)
	}
	if v.EscapeConfirmTimeoutSet {
		cfg.EscapeConfirmTimeoutMs = intPtr(v.EscapeConfirmTimeout)
	}
	return cfg
}

func (v configValues) resolvedStatusDuration() int {
	if v.StatusDurationSet && v.StatusDuration > 0 {
		return v.StatusDuration
	}
	if v.StatusDuration > 0 {
		return v.StatusDuration
	}
	return 2000
}

type configModel struct {
	values   configValues
	original configValues
	rows     []configRow
	selected int

	editing      bool
	editingKind  cfgRowKind
	editingIndex int
	editingField configField
	editOriginal string
	input        textinput.Model

	status         string
	statusSeq      int
	statusTimeout  time.Duration
	statusTimerCmd tea.Cmd
	confirmExit    bool

	err    error
	width  int
	height int
}

func newConfigModel(cfg app.Config) *configModel {
	ti := textinput.New()
	ti.CharLimit = 0
	ti.Placeholder = ""
	values := newConfigValues(cfg)
	model := &configModel{
		values:        values,
		original:      values.clone(),
		input:         ti,
		statusTimeout: 2 * time.Second,
		editingIndex:  -1,
	}
	model.rebuildRows()
	return model
}

func (m *configModel) Init() tea.Cmd {
	return nil
}

func (m *configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.editing {
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(20, m.width-4)
	case tea.KeyMsg:
		if cmd := m.handleKey(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case statusTimeoutMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
	case externalOpenResultMsg:
		if msg.kind == openKindConfig {
			m.handleConfigFileResult(msg.err)
		}
	}

	if m.statusTimerCmd != nil {
		cmds = append(cmds, m.statusTimerCmd)
		m.statusTimerCmd = nil
	}

	return m, tea.Batch(cmds...)
}

func (m *configModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if m.editing {
		switch key {
		case "enter":
			m.commitEdit()
		case "esc", "ctrl+c":
			m.cancelEdit()
		}
		return nil
	}

	switch key {
	case "ctrl+c":
		return tea.Quit
	case "q":
		return m.handleQuit()
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "enter", " ":
		return m.activateSelection()
	case "d":
		m.deleteOrDefaultSelection()
	case "w":
		m.saveChanges()
	case "r":
		m.reloadFromDisk()
	case "e":
		return m.openConfigJSON()
	}
	return nil
}

func (m *configModel) handleQuit() tea.Cmd {
	if !m.isDirty() {
		return tea.Quit
	}
	if m.confirmExit {
		return tea.Quit
	}
	m.confirmExit = true
	m.setStatus("Unsaved changes. Press q again to exit without saving.")
	return nil
}

func (m *configModel) moveSelection(delta int) {
	if len(m.rows) == 0 {
		m.selected = 0
		return
	}
	next := m.selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.rows) {
		next = len(m.rows) - 1
	}
	m.selected = next
}

func (m *configModel) activateSelection() tea.Cmd {
	row := m.currentRow()
	if row == nil {
		return nil
	}
	switch row.kind {
	case cfgRowQuestion:
		m.startQuestionEdit(row.index)
	case cfgRowAddQuestion:
		m.values.Questions = append(m.values.Questions, "")
		m.rebuildRows()
		m.selected = row.index
		m.startQuestionEdit(row.index)
	case cfgRowBool:
		m.toggleBool(row.field)
	case cfgRowInt:
		m.startIntEdit(row.field)
	}
	return nil
}

func (m *configModel) deleteOrDefaultSelection() {
	row := m.currentRow()
	if row == nil {
		return
	}
	switch row.kind {
	case cfgRowQuestion:
		m.deleteQuestion(row.index)
	case cfgRowBool:
		m.resetBoolField(row.field)
	case cfgRowInt:
		m.resetIntField(row.field)
	}
}

func (m *configModel) deleteQuestion(idx int) {
	if idx < 0 || idx >= len(m.values.Questions) {
		return
	}
	m.values.Questions = append(m.values.Questions[:idx], m.values.Questions[idx+1:]...)
	m.rebuildRows()
	m.markDirty()
	m.setStatus("Question deleted.")
}

func (m *configModel) resetBoolField(field configField) {
	defaultCfg := app.Config{}
	changed := true
	switch field {
	case cfgFieldShowHints:
		m.values.ShowHints = defaultCfg.HintsEnabled()
		m.values.ShowHintsCustom = false
	case cfgFieldAutoInsert:
		m.values.AutoInsert = defaultCfg.AutoInsertEnabled()
		m.values.AutoInsertCustom = false
	case cfgFieldContinueInsertAfterSave:
		m.values.ContinueInsertAfterSave = defaultCfg.ContinueInsertAfterSaveEnabled()
		m.values.ContinueInsertAfterSaveCustom = false
	case cfgFieldDefaultListMode:
		m.values.DefaultListMode = defaultCfg.DefaultListModeEnabled()
		m.values.DefaultListModeCustom = false
	case cfgFieldAutoOpenIndex:
		m.values.AutoOpenIndexJump = defaultCfg.AutoOpenIndexJumpEnabled()
		m.values.AutoOpenIndexCustom = false
	case cfgFieldConfirmDelete:
		m.values.ConfirmDelete = defaultCfg.ConfirmDeleteEnabled()
		m.values.ConfirmDeleteCustom = false
	case cfgFieldConfirmEscapeWithText:
		m.values.ConfirmEscapeWithText = defaultCfg.ConfirmEscapeWithTextEnabled()
		m.values.ConfirmEscapeWithTextCustom = false
	default:
		changed = false
	}
	if !changed {
		return
	}
	m.markDirty()
	m.setStatus("Option reset to default.")
}

func (m *configModel) resetIntField(field configField) {
	defaultCfg := app.Config{}
	switch field {
	case cfgFieldStatusDuration:
		m.values.StatusDuration = int(defaultCfg.StatusMessageDuration() / time.Millisecond)
		m.values.StatusDurationSet = false
	case cfgFieldEscapeConfirmTimeout:
		m.values.EscapeConfirmTimeout = int(defaultCfg.EscapeConfirmTimeout() / time.Millisecond)
		m.values.EscapeConfirmTimeoutSet = false
	default:
		return
	}
	m.markDirty()
	m.setStatus("Option reset to default.")
}

func (m *configModel) currentRow() *configRow {
	if len(m.rows) == 0 || m.selected < 0 || m.selected >= len(m.rows) {
		return nil
	}
	return &m.rows[m.selected]
}

func (m *configModel) startQuestionEdit(idx int) {
	if idx < 0 || idx >= len(m.values.Questions) {
		return
	}
	m.editing = true
	m.editingKind = cfgRowQuestion
	m.editingIndex = idx
	m.editOriginal = m.values.Questions[idx]
	m.input.Placeholder = "Question"
	m.input.SetValue(m.values.Questions[idx])
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *configModel) startIntEdit(field configField) {
	m.editing = true
	m.editingKind = cfgRowInt
	m.editingField = field
	m.editOriginal = ""
	placeholder := "Milliseconds"
	value := ""
	switch field {
	case cfgFieldStatusDuration:
		placeholder = "Status duration (ms)"
		if m.values.StatusDurationSet {
			value = strconv.Itoa(m.values.StatusDuration)
		}
	case cfgFieldEscapeConfirmTimeout:
		placeholder = "Escape confirm timeout (ms)"
		if m.values.EscapeConfirmTimeoutSet {
			value = strconv.Itoa(m.values.EscapeConfirmTimeout)
		}
	}
	m.input.Placeholder = placeholder
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *configModel) commitEdit() {
	switch m.editingKind {
	case cfgRowQuestion:
		m.commitQuestionEdit()
	case cfgRowInt:
		m.commitIntEdit()
	}
}

func (m *configModel) commitQuestionEdit() {
	if m.editingIndex < 0 || m.editingIndex >= len(m.values.Questions) {
		m.finishEditing()
		return
	}
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		m.values.Questions = append(m.values.Questions[:m.editingIndex], m.values.Questions[m.editingIndex+1:]...)
		if m.selected >= len(m.values.Questions) {
			m.selected = len(m.values.Questions)
		}
	} else {
		m.values.Questions[m.editingIndex] = text
	}
	m.finishEditing()
	m.rebuildRows()
	m.markDirty()
}

func (m *configModel) commitIntEdit() {
	field := m.editingField
	raw := strings.TrimSpace(m.input.Value())
	defaultCfg := app.Config{}
	if raw == "" {
		switch field {
		case cfgFieldStatusDuration:
			m.values.StatusDurationSet = false
			m.values.StatusDuration = int(defaultCfg.StatusMessageDuration() / time.Millisecond)
		case cfgFieldEscapeConfirmTimeout:
			m.values.EscapeConfirmTimeoutSet = false
			m.values.EscapeConfirmTimeout = int(defaultCfg.EscapeConfirmTimeout() / time.Millisecond)
		default:
			m.setStatus("Enter a positive number of milliseconds.")
			return
		}
	} else {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			m.setStatus("Enter a positive number of milliseconds.")
			return
		}
		switch field {
		case cfgFieldStatusDuration:
			m.values.StatusDuration = val
			m.values.StatusDurationSet = true
		case cfgFieldEscapeConfirmTimeout:
			m.values.EscapeConfirmTimeout = val
			m.values.EscapeConfirmTimeoutSet = true
		default:
			m.setStatus("Enter a positive number of milliseconds.")
			return
		}
	}
	if field == cfgFieldStatusDuration {
		m.values.StatusDuration = m.values.resolvedStatusDuration()
	}
	m.finishEditing()
	m.markDirty()
}

func (m *configModel) finishEditing() {
	m.editing = false
	m.editingIndex = -1
	m.input.Blur()
}

func (m *configModel) cancelEdit() {
	if m.editingKind == cfgRowQuestion && m.editingIndex >= 0 && m.editingIndex < len(m.values.Questions) {
		if strings.TrimSpace(m.editOriginal) == "" && strings.TrimSpace(m.values.Questions[m.editingIndex]) == "" {
			m.values.Questions = append(m.values.Questions[:m.editingIndex], m.values.Questions[m.editingIndex+1:]...)
			m.rebuildRows()
		}
	}
	m.finishEditing()
}

func (m *configModel) toggleBool(field configField) {
	switch field {
	case cfgFieldShowHints:
		m.values.ShowHints = !m.values.ShowHints
		m.values.ShowHintsCustom = true
	case cfgFieldAutoInsert:
		m.values.AutoInsert = !m.values.AutoInsert
		m.values.AutoInsertCustom = true
	case cfgFieldContinueInsertAfterSave:
		m.values.ContinueInsertAfterSave = !m.values.ContinueInsertAfterSave
		m.values.ContinueInsertAfterSaveCustom = true
	case cfgFieldDefaultListMode:
		m.values.DefaultListMode = !m.values.DefaultListMode
		m.values.DefaultListModeCustom = true
	case cfgFieldAutoOpenIndex:
		m.values.AutoOpenIndexJump = !m.values.AutoOpenIndexJump
		m.values.AutoOpenIndexCustom = true
	case cfgFieldConfirmDelete:
		m.values.ConfirmDelete = !m.values.ConfirmDelete
		m.values.ConfirmDeleteCustom = true
	case cfgFieldConfirmEscapeWithText:
		m.values.ConfirmEscapeWithText = !m.values.ConfirmEscapeWithText
		m.values.ConfirmEscapeWithTextCustom = true
	}
	m.markDirty()
}

func (m *configModel) markDirty() {
	m.confirmExit = false
	if m.values.equal(m.original) {
		return
	}
}

func (m *configModel) isDirty() bool {
	return !m.values.equal(m.original)
}

func (m *configModel) saveChanges() {
	cfg := m.values.toConfig()
	if err := app.SaveConfig(cfg); err != nil {
		m.err = err
		return
	}
	m.err = nil
	m.original = m.values.clone()
	m.confirmExit = false
	m.setStatus("Config saved.")
}

func (m *configModel) reloadFromDisk() {
	if err := m.loadConfigFromDisk(); err != nil {
		m.err = err
		return
	}
	m.err = nil
	m.setStatus("Changes discarded.")
}

func (m *configModel) loadConfigFromDisk() error {
	cfg, err := app.LoadConfig()
	if err != nil {
		return err
	}
	m.values = newConfigValues(cfg)
	m.original = m.values.clone()
	m.rebuildRows()
	if m.selected >= len(m.rows) {
		m.selected = len(m.rows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.confirmExit = false
	return nil
}

func (m *configModel) openConfigJSON() tea.Cmd {
	if m.editing {
		m.setStatus("Finish editing before opening the config file.")
		return nil
	}
	if m.isDirty() {
		m.setStatus("Save or discard changes before opening the config file.")
		return nil
	}
	path, err := app.ConfigFilePath()
	if err != nil {
		m.err = err
		return nil
	}
	m.setStatus("Opened config file in editor.")
	return openFileInEditorCmd(path, openKindConfig)
}

func (m *configModel) handleConfigFileResult(err error) {
	if err != nil {
		m.err = err
		return
	}
	m.err = nil
	if loadErr := m.loadConfigFromDisk(); loadErr != nil {
		m.err = loadErr
		return
	}
	m.setStatus("Config reloaded from disk.")
}

func (m *configModel) rebuildRows() {
	rows := make([]configRow, 0, len(m.values.Questions)+6)
	for idx := range m.values.Questions {
		rows = append(rows, configRow{kind: cfgRowQuestion, index: idx})
	}
	rows = append(rows, configRow{kind: cfgRowAddQuestion, index: len(m.values.Questions)})
	rows = append(rows, configRow{kind: cfgRowBool, field: cfgFieldShowHints})
	rows = append(rows, configRow{kind: cfgRowBool, field: cfgFieldAutoInsert})
	rows = append(rows, configRow{kind: cfgRowBool, field: cfgFieldContinueInsertAfterSave})
	rows = append(rows, configRow{kind: cfgRowBool, field: cfgFieldDefaultListMode})
	rows = append(rows, configRow{kind: cfgRowBool, field: cfgFieldAutoOpenIndex})
	rows = append(rows, configRow{kind: cfgRowBool, field: cfgFieldConfirmDelete})
	rows = append(rows, configRow{kind: cfgRowBool, field: cfgFieldConfirmEscapeWithText})
	rows = append(rows, configRow{kind: cfgRowInt, field: cfgFieldStatusDuration})
	rows = append(rows, configRow{kind: cfgRowInt, field: cfgFieldEscapeConfirmTimeout})
	m.rows = rows
	if m.selected >= len(rows) {
		m.selected = len(rows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *configModel) View() string {
	var b strings.Builder
	b.WriteString("Configuration")
	if m.isDirty() {
		b.WriteString(" *")
	}
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(fmt.Sprintf("Error: %v\n\n", m.err))
	}

	b.WriteString("Questions:\n")
	for idx, row := range m.rows {
		if row.kind == cfgRowQuestion || row.kind == cfgRowAddQuestion {
			marker := " "
			if idx == m.selected {
				marker = ">"
			}
			if row.kind == cfgRowQuestion {
				label := m.values.Questions[row.index]
				if label == "" {
					label = "(empty)"
				}
				b.WriteString(fmt.Sprintf("%s  [%d] %s\n", marker, row.index+1, label))
			} else {
				b.WriteString(fmt.Sprintf("%s  [+] Add question\n", marker))
			}
		}
	}

	b.WriteString("\nOptions:\n")
	for idx, row := range m.rows {
		if row.kind == cfgRowBool || row.kind == cfgRowInt {
			marker := " "
			if idx == m.selected {
				marker = ">"
			}
			switch row.field {
			case cfgFieldShowHints:
				b.WriteString(fmt.Sprintf("%s  Show hints: %s\n", marker, boolLabel(m.values.ShowHints, !m.values.ShowHintsCustom)))
			case cfgFieldAutoInsert:
				b.WriteString(fmt.Sprintf("%s  Auto-insert entries: %s\n", marker, boolLabel(m.values.AutoInsert, !m.values.AutoInsertCustom)))
			case cfgFieldContinueInsertAfterSave:
				b.WriteString(fmt.Sprintf("%s  Continue after save: %s\n", marker, boolLabel(m.values.ContinueInsertAfterSave, !m.values.ContinueInsertAfterSaveCustom)))
			case cfgFieldDefaultListMode:
				b.WriteString(fmt.Sprintf("%s  Default list mode: %s\n", marker, boolLabel(m.values.DefaultListMode, !m.values.DefaultListModeCustom)))
			case cfgFieldAutoOpenIndex:
				b.WriteString(fmt.Sprintf("%s  Auto-open index jumps: %s\n", marker, boolLabel(m.values.AutoOpenIndexJump, !m.values.AutoOpenIndexCustom)))
			case cfgFieldConfirmDelete:
				b.WriteString(fmt.Sprintf("%s  Confirm deletes: %s\n", marker, boolLabel(m.values.ConfirmDelete, !m.values.ConfirmDeleteCustom)))
			case cfgFieldConfirmEscapeWithText:
				b.WriteString(fmt.Sprintf("%s  Confirm escape with text: %s\n", marker, boolLabel(m.values.ConfirmEscapeWithText, !m.values.ConfirmEscapeWithTextCustom)))
			case cfgFieldStatusDuration:
				label := fmt.Sprintf("%d ms", m.values.resolvedStatusDuration())
				if !m.values.StatusDurationSet {
					label += " (default)"
				}
				b.WriteString(fmt.Sprintf("%s  Status duration: %s\n", marker, label))
			case cfgFieldEscapeConfirmTimeout:
				timeLabel := fmt.Sprintf("%d ms", m.values.EscapeConfirmTimeout)
				if !m.values.EscapeConfirmTimeoutSet {
					timeLabel += " (default)"
				}
				b.WriteString(fmt.Sprintf("%s  Escape confirm timeout: %s\n", marker, timeLabel))
			}
		}
	}

	b.WriteString("\nCommands: Enter edit/toggle • d delete/default • w write • r reload • e edit file • q quit\n")
	if m.editing {
		b.WriteString("\n" + m.input.View() + "\n")
	}
	if m.status != "" {
		b.WriteString("\n" + statusStyle.Render(m.status))
	}
	return b.String()
}

func (m *configModel) setStatus(text string) {
	m.status = text
	m.statusSeq++
	if text == "" || m.statusTimeout <= 0 {
		m.statusTimerCmd = nil
		return
	}
	seq := m.statusSeq
	m.statusTimerCmd = tea.Tick(m.statusTimeout, func(time.Time) tea.Msg {
		return statusTimeoutMsg{seq: seq}
	})
}

func boolLabel(value bool, isDefault bool) string {
	label := fmt.Sprintf("%t", value)
	if isDefault {
		label += " (default)"
	}
	return label
}

func intPtr(v int) *int {
	b := v
	return &b
}

func RunConfigEditor() error {
	cfg, err := app.LoadConfig()
	if err != nil {
		return err
	}
	return runProgram(newConfigModel(cfg))
}
