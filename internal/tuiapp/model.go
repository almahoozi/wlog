package tuiapp

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/almahoozi/wlog/internal/app"
)

var indexRunes = []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'}

const jkDisableThreshold = 20

var statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
)

type rowKind int

const (
	rowQuestion rowKind = iota
	rowEntry
)

type listRow struct {
	kind       rowKind
	question   string
	entryIndex int
}

type detailState struct {
	question string
	editing  bool
	input    textinput.Model
}

type deleteConfirmState struct {
	question   string
	entryIndex int
}

type statusTimeoutMsg struct {
	seq int
}

type externalOpenKind int

const (
	openKindDay externalOpenKind = iota
	openKindConfig
)

type externalOpenResultMsg struct {
	kind externalOpenKind
	err  error
}

type model struct {
	cfgQuestions []string
	config       app.Config
	day          time.Time
	log          app.DayLog

	questions     []string
	questionIndex map[string]int
	rows          []listRow
	selected      int

	listMode      bool
	disableJKNav  bool
	showHints     bool
	autoInsert    bool
	autoOpenIndex bool
	confirmDelete bool

	view   viewMode
	detail detailState

	deleteConfirm    *deleteConfirmState
	confirmPrompt    string
	showDeletePrompt bool

	status         string
	statusSeq      int
	statusTimeout  time.Duration
	statusTimerCmd tea.Cmd
	err            error

	width  int
	height int
}

func newModel(cfg app.Config) (*model, error) {
	day := app.DayFloor(time.Now())
	log, err := app.LoadDayLog(day)
	if err != nil {
		return nil, err
	}
	if log.Answers == nil {
		log.Answers = make(map[string][]app.Answer)
	}

	showHints := cfg.HintsEnabled()
	autoInsert := cfg.AutoInsertEnabled()
	listModeDefault := cfg.DefaultListModeEnabled()
	autoOpenIndex := cfg.AutoOpenIndexJumpEnabled()
	confirmDelete := cfg.ConfirmDeleteEnabled()
	statusTimeout := cfg.StatusMessageDuration()

	ti := textinput.New()
	ti.Prompt = "→ "
	ti.Placeholder = "Add entry..."
	ti.CharLimit = 0
	ti.Width = 60

	m := &model{
		cfgQuestions:  append([]string(nil), cfg.Questions...),
		config:        cfg,
		day:           day,
		log:           log,
		showHints:     showHints,
		autoInsert:    autoInsert,
		listMode:      listModeDefault,
		autoOpenIndex: autoOpenIndex,
		confirmDelete: confirmDelete,
		statusTimeout: statusTimeout,
		detail: detailState{
			input: ti,
		},
	}
	m.refreshQuestions()
	return m, nil
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.view == viewDetail && m.detail.editing {
		var inputCmd tea.Cmd
		m.detail.input, inputCmd = m.detail.input.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.detail.input.Width = max(20, m.width-4)
	case tea.KeyMsg:
		if cmd := m.handleKey(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case editorResultMsg:
		m.handleEditorResult(msg)
	case statusTimeoutMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
	case externalOpenResultMsg:
		m.handleExternalOpenResult(msg)
	}

	if m.statusTimerCmd != nil {
		cmds = append(cmds, m.statusTimerCmd)
		m.statusTimerCmd = nil
	}

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	var b strings.Builder
	dayLabel := m.day.Format("Mon 2006-01-02")
	b.WriteString(fmt.Sprintf("%s — %s\n\n", dayLabel, relativeDayLabel(m.day)))
	if m.showHints {
		b.WriteString("←/→ change day • space today • q quit • h/? toggle hints\n")
		b.WriteString("Enter/i add entry • e edit • d delete entry • l toggle list • o open day file • numbers/letters jump\n\n")
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("Error: %s\n\n", m.err))
	}

	switch m.view {
	case viewList:
		b.WriteString(m.renderList())
	case viewDetail:
		b.WriteString(m.renderDetail())
	}

	if m.showDeletePrompt {
		b.WriteString("\n" + statusStyle.Render(m.confirmPrompt))
	}

	if m.status != "" {
		b.WriteString("\n" + statusStyle.Render(m.status))
	}

	// NOTE: Need to end with a newline for proper rendering
	return b.String() + "\n"
}

func (m *model) renderList() string {
	var b strings.Builder
	if len(m.questions) == 0 {
		b.WriteString("No questions configured.\n")
		return b.String()
	}

	if m.listMode && m.showHints {
		b.WriteString("List mode: showing entries for all questions.\n\n")
	}

	for i, row := range m.rows {
		marker := " "
		if i == m.selected {
			marker = ">"
		}
		switch row.kind {
		case rowQuestion:
			label := "--"
			if idx, ok := m.questionIndex[row.question]; ok && idx < len(indexRunes) {
				label = string(indexRunes[idx])
			}
			count := len(m.log.Answers[row.question])
			countLabel := ""
			if count > 0 {
				countLabel = fmt.Sprintf(" (%d)", count)
			}
			b.WriteString(fmt.Sprintf("%s [%s] %s%s\n", marker, label, row.question, countLabel))
		case rowEntry:
			answers := m.log.Answers[row.question]
			if row.entryIndex >= 0 && row.entryIndex < len(answers) {
				ans := answers[row.entryIndex]
				b.WriteString(fmt.Sprintf("%s     - [%s] %s\n", marker, app.DisplayTime(ans.Time), ans.Response))
			}
		}
	}

	if m.showHints && len(m.rows) > 0 {
		hint := "Use numbers/letters to jump to a question. Enter on an entry opens the editor. Press d to delete an entry."
		b.WriteString("\n" + hint + "\n")
	}

	return b.String()
}

func (m *model) renderDetail() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n\n", m.detail.question))
	entries := m.log.Answers[m.detail.question]
	if len(entries) == 0 {
		b.WriteString("  No entries yet.\n")
	}
	for i, ans := range entries {
		b.WriteString(fmt.Sprintf("  %d. [%s] %s\n", i+1, app.DisplayTime(ans.Time), ans.Response))
	}

	b.WriteString("\n")
	if m.detail.editing {
		b.WriteString("New entry:\n  ")
		b.WriteString(m.detail.input.View())
		if m.showHints {
			b.WriteString("\n  Enter to save and continue, Esc to cancel.\n")
		} else {
			b.WriteString("\n")
		}
	} else if m.showHints {
		b.WriteString("Press Enter or i to start adding entries, e to edit all entries, Esc to go back.\n")
	}

	return b.String()
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	if m.view == viewDetail && m.detail.editing {
		switch key {
		case "ctrl+c":
			return tea.Quit
		default:
			goto viewHandling
		}
	}

	if key == "ctrl+c" || key == "q" {
		return tea.Quit
	}

	if m.view == viewList && m.deleteConfirm != nil {
		if m.handleDeleteConfirmationKey(key) {
			return nil
		}
	}

	switch key {
	case "h", "?":
		m.toggleHints()
		return nil
	case "esc":
		if m.view == viewList && !m.showHints {
			m.showHints = true
			m.setStatus("Hints temporarily shown.")
			return nil
		}
	case "left":
		m.changeDay(-1)
		return nil
	case "right":
		m.changeDay(1)
		return nil
	case " ":
		m.goToToday()
		return nil
	}

viewHandling:
	switch m.view {
	case viewList:
		return m.handleListKey(msg)
	case viewDetail:
		return m.handleDetailKey(msg)
	}

	return nil
}

func (m *model) handleListKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	switch key {
	case "up":
		m.moveSelection(-1)
	case "down":
		m.moveSelection(1)
	case "j":
		if m.disableJKNav {
			m.jumpToIndex('j')
		} else {
			m.moveSelection(1)
		}
	case "k":
		if m.disableJKNav {
			m.jumpToIndex('k')
		} else {
			m.moveSelection(-1)
		}
	case "enter":
		return m.activateSelection()
	case "i":
		if row := m.currentRow(); row != nil {
			m.openDetail(row.question, true)
		}
	case "e":
		if row := m.currentRow(); row != nil {
			if row.kind == rowEntry {
				return m.openEntryEditor(row.question, row.entryIndex)
			}
			return m.openQuestionEditor(row.question)
		}
	case "d":
		m.handleDeleteEntryRequest()
	case "l":
		m.toggleListMode()
	case "o":
		return m.openDayJSON()
	default:
		if len(key) == 1 {
			r := []rune(key)[0]
			if unicode.IsLetter(r) {
				r = unicode.ToLower(r)
			}
			if (r == 'j' || r == 'k') && !m.disableJKNav {
				return nil
			}
			if m.jumpToIndex(r) && m.autoOpenIndex {
				return m.activateSelection()
			}
		}
	}

	return nil
}

func (m *model) handleDeleteEntryRequest() {
	if !m.listMode {
		m.setStatus("Enable list mode to delete entries.")
		return
	}
	row := m.currentRow()
	if row == nil || row.kind != rowEntry {
		m.setStatus("Select an entry to delete.")
		return
	}
	m.initiateEntryDelete(row.question, row.entryIndex)
}

func (m *model) initiateEntryDelete(question string, entryIndex int) {
	entries := m.log.Answers[question]
	if entryIndex < 0 || entryIndex >= len(entries) {
		m.setStatus("Entry not found.")
		return
	}
	if m.confirmDelete {
		m.deleteConfirm = &deleteConfirmState{question: question, entryIndex: entryIndex}
		m.confirmPrompt = "Delete this entry? (y/n)"
		m.showDeletePrompt = true
		return
	}
	m.performDeleteEntry(question, entryIndex)
}

func (m *model) handleDeleteConfirmationKey(key string) bool {
	if m.deleteConfirm == nil {
		return false
	}
	switch key {
	case "y", "Y":
		pending := m.deleteConfirm
		m.deleteConfirm = nil
		m.confirmPrompt = ""
		m.showDeletePrompt = false
		m.performDeleteEntry(pending.question, pending.entryIndex)
	case "n", "N", "esc":
		m.deleteConfirm = nil
		m.confirmPrompt = ""
		m.showDeletePrompt = false
		m.setStatus("Delete canceled.")
	default:
		m.setStatus("Confirm delete with y or n.")
	}
	return true
}

func (m *model) performDeleteEntry(question string, idx int) {
	entries := m.log.Answers[question]
	if idx < 0 || idx >= len(entries) {
		m.setStatus("Entry not found.")
		return
	}
	entries = append(entries[:idx], entries[idx+1:]...)
	if len(entries) == 0 {
		delete(m.log.Answers, question)
	} else {
		m.log.Answers[question] = entries
	}
	if err := app.SaveDayLog(m.day, m.log); err != nil {
		m.err = err
		m.setStatus("Failed to delete entry.")
		return
	}
	m.err = nil
	m.confirmPrompt = ""
	m.showDeletePrompt = false
	m.setStatus("Entry deleted.")
	m.refreshQuestions()
	m.selectQuestionByName(question)
}

func (m *model) openDayJSON() tea.Cmd {
	if m.log.Answers == nil {
		m.log.Answers = make(map[string][]app.Answer)
	}
	if err := app.SaveDayLog(m.day, m.log); err != nil {
		m.err = err
		return nil
	}
	path, err := app.DayFilePath(m.day)
	if err != nil {
		m.err = err
		return nil
	}
	m.setStatus("Opened day file in editor.")
	return openFileInEditorCmd(path, openKindDay)
}

func (m *model) handleDetailKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	switch key {
	case "esc":
		if m.detail.editing {
			m.detail.editing = false
			m.detail.input.Blur()
			m.detail.input.SetValue("")
			m.setStatus("Insert canceled.")
		} else {
			m.view = viewList
			m.detail.question = ""
		}
	case "-":
		if !m.detail.editing {
			m.view = viewList
			m.detail.question = ""
		}
	case "enter":
		if m.detail.editing {
			m.saveInlineEntry()
		} else {
			m.startEditing()
		}
	case "i":
		if !m.detail.editing {
			m.startEditing()
		}
	case "e":
		if !m.detail.editing {
			return m.openQuestionEditor(m.detail.question)
		}
	}
	return nil
}

func (m *model) activateSelection() tea.Cmd {
	row := m.currentRow()
	if row == nil {
		return nil
	}
	if row.kind == rowEntry {
		return m.openEntryEditor(row.question, row.entryIndex)
	}
	m.openDetail(row.question, m.autoInsert)
	return nil
}

func (m *model) openDetail(question string, startEditing bool) {
	m.view = viewDetail
	m.deleteConfirm = nil
	m.confirmPrompt = ""
	m.showDeletePrompt = false
	m.detail.question = question
	if startEditing {
		m.startEditing()
	} else {
		m.detail.editing = false
		m.detail.input.Blur()
		m.detail.input.SetValue("")
	}
}

func (m *model) startEditing() {
	m.detail.editing = true
	m.detail.input.SetValue("")
	m.detail.input.CursorEnd()
	m.detail.input.Focus()
	m.setStatus("Adding entries...")
}

func (m *model) saveInlineEntry() {
	text := strings.TrimSpace(m.detail.input.Value())
	if text == "" {
		m.setStatus("Entry discarded (empty).")
		return
	}
	if m.log.Answers == nil {
		m.log.Answers = make(map[string][]app.Answer)
	}
	entry := app.Answer{Time: time.Now().Format(time.RFC3339), Response: text}
	m.log.Answers[m.detail.question] = append(m.log.Answers[m.detail.question], entry)
	if err := app.SaveDayLog(m.day, m.log); err != nil {
		m.err = err
		return
	}
	m.err = nil
	m.detail.input.SetValue("")
	m.setStatus("Entry saved.")
	m.refreshQuestions()
}

func (m *model) openQuestionEditor(question string) tea.Cmd {
	lines := responsesForQuestion(m.log.Answers[question])
	return editEntriesCmd(question, lines, -1)
}

func (m *model) openEntryEditor(question string, idx int) tea.Cmd {
	answers := m.log.Answers[question]
	if idx < 0 || idx >= len(answers) {
		return nil
	}
	return editEntriesCmd(question, []string{answers[idx].Response}, idx)
}

func openFileInEditorCmd(path string, kind externalOpenKind) tea.Cmd {
	cmd, err := buildEditorCommand(path)
	if err != nil {
		return func() tea.Msg { return externalOpenResultMsg{kind: kind, err: err} }
	}
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return externalOpenResultMsg{kind: kind, err: execErr}
	})
}

func (m *model) handleEditorResult(msg editorResultMsg) {
	if msg.err != nil {
		m.err = msg.err
		return
	}
	if !msg.changed {
		m.setStatus("No changes saved.")
		return
	}

	if msg.entryIndex >= 0 {
		m.applySingleEntryEdit(msg.question, msg.entryIndex, msg.responses)
	} else {
		m.applyQuestionEdit(msg.question, msg.responses)
	}
}

func (m *model) handleExternalOpenResult(msg externalOpenResultMsg) {
	if msg.err != nil {
		m.err = msg.err
		return
	}
	m.err = nil
	switch msg.kind {
	case openKindDay:
		m.refreshCurrentDayFromDisk()
		m.setStatus("Day file reloaded.")
	}
}

func (m *model) applyQuestionEdit(question string, responses []string) {
	existing := m.log.Answers[question]
	updated := rebuildAnswers(existing, responses)
	if len(updated) == 0 {
		delete(m.log.Answers, question)
	} else {
		m.log.Answers[question] = updated
	}
	if err := app.SaveDayLog(m.day, m.log); err != nil {
		m.err = err
		return
	}
	m.err = nil
	m.setStatus("Entries updated.")
	m.refreshQuestions()
}

func (m *model) applySingleEntryEdit(question string, idx int, responses []string) {
	answers := m.log.Answers[question]
	if idx < 0 || idx >= len(answers) {
		return
	}
	if len(responses) == 0 {
		answers = append(answers[:idx], answers[idx+1:]...)
	} else {
		answers[idx].Response = responses[0]
	}
	if len(answers) == 0 {
		delete(m.log.Answers, question)
	} else {
		m.log.Answers[question] = answers
	}
	if err := app.SaveDayLog(m.day, m.log); err != nil {
		m.err = err
		return
	}
	m.err = nil
	m.setStatus("Entry updated.")
	m.refreshQuestions()
}

func (m *model) setStatus(text string) {
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

func (m *model) moveSelection(delta int) {
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

func (m *model) jumpToIndex(r rune) bool {
	idx, ok := runeToIndex(r)
	if !ok {
		return false
	}
	if idx < 0 || idx >= len(m.questions) {
		return false
	}
	rowIdx := m.rowIndexForQuestion(idx)
	if rowIdx < 0 {
		return false
	}
	m.selected = rowIdx
	return true
}

func (m *model) selectQuestionByIndex(idx int) {
	if idx < 0 || idx >= len(m.questions) {
		return
	}
	rowIdx := m.rowIndexForQuestion(idx)
	if rowIdx >= 0 {
		m.selected = rowIdx
	}
}

func (m *model) selectQuestionByName(question string) {
	if idx, ok := m.questionIndex[question]; ok {
		m.selectQuestionByIndex(idx)
	}
}

func (m *model) rowIndexForQuestion(idx int) int {
	if idx < 0 || idx >= len(m.questions) {
		return -1
	}
	if !m.listMode {
		return idx
	}
	offset := 0
	for i := 0; i < len(m.questions); i++ {
		if i == idx {
			return offset
		}
		offset++
		offset += len(m.log.Answers[m.questions[i]])
	}
	return -1
}

func (m *model) currentRow() *listRow {
	if len(m.rows) == 0 || m.selected < 0 || m.selected >= len(m.rows) {
		return nil
	}
	return &m.rows[m.selected]
}

func (m *model) toggleListMode() {
	var currentQuestion string
	if row := m.currentRow(); row != nil {
		currentQuestion = row.question
	}
	m.listMode = !m.listMode
	m.refreshQuestions()
	if currentQuestion != "" {
		if idx, ok := m.questionIndex[currentQuestion]; ok {
			if rowIdx := m.rowIndexForQuestion(idx); rowIdx >= 0 {
				m.selected = rowIdx
			}
		}
	}
}

func (m *model) toggleHints() {
	m.showHints = !m.showHints
	if m.config.ShowHints == nil {
		m.config.ShowHints = boolPtr(m.showHints)
	} else {
		*m.config.ShowHints = m.showHints
	}
	if err := app.SaveConfig(m.config); err != nil {
		m.err = err
		return
	}
	m.err = nil
	if m.showHints {
		m.setStatus("Hints enabled.")
	} else {
		m.setStatus("Hints hidden.")
	}
}

func (m *model) refreshQuestions() {
	m.deleteConfirm = nil
	m.confirmPrompt = ""
	m.showDeletePrompt = false
	m.questions = mergeQuestions(m.cfgQuestions, m.log)
	m.questionIndex = make(map[string]int, len(m.questions))
	for i, q := range m.questions {
		m.questionIndex[q] = i
	}
	m.disableJKNav = len(m.questions) >= jkDisableThreshold
	m.rebuildRows()
	if len(m.rows) == 0 {
		m.selected = 0
	} else if m.selected >= len(m.rows) {
		m.selected = len(m.rows) - 1
	}
}

func (m *model) rebuildRows() {
	rows := make([]listRow, 0, len(m.questions))
	for _, q := range m.questions {
		rows = append(rows, listRow{kind: rowQuestion, question: q})
		if m.listMode {
			for idx := range m.log.Answers[q] {
				rows = append(rows, listRow{kind: rowEntry, question: q, entryIndex: idx})
			}
		}
	}
	m.rows = rows
}

func (m *model) changeDay(delta int) {
	m.day = m.day.AddDate(0, 0, delta)
	m.reloadDay()
}

func (m *model) goToToday() {
	today := app.DayFloor(time.Now())
	if !today.Equal(m.day) {
		m.day = today
		m.reloadDay()
	}
}

func (m *model) reloadDay() {
	log, err := app.LoadDayLog(m.day)
	if err != nil {
		m.err = err
		return
	}
	m.err = nil
	if log.Answers == nil {
		log.Answers = make(map[string][]app.Answer)
	}
	m.log = log
	m.view = viewList
	m.detail.editing = false
	m.detail.input.Blur()
	m.detail.input.SetValue("")
	m.selected = 0
	m.refreshQuestions()
	m.setStatus(fmt.Sprintf("Viewing %s", m.day.Format("2006-01-02")))
}

func (m *model) refreshCurrentDayFromDisk() {
	log, err := app.LoadDayLog(m.day)
	if err != nil {
		m.err = err
		return
	}
	m.err = nil
	if log.Answers == nil {
		log.Answers = make(map[string][]app.Answer)
	}
	m.log = log
	m.refreshQuestions()
}

func mergeQuestions(base []string, log app.DayLog) []string {
	seen := make(map[string]bool)
	list := make([]string, 0, len(base)+len(log.Answers))
	for _, q := range base {
		list = append(list, q)
		seen[q] = true
	}
	var extras []string
	for q, answers := range log.Answers {
		if len(answers) == 0 {
			continue
		}
		if !seen[q] {
			extras = append(extras, q)
			seen[q] = true
		}
	}
	sort.Strings(extras)
	list = append(list, extras...)
	return list
}

func runeToIndex(r rune) (int, bool) {
	for idx, candidate := range indexRunes {
		if candidate == r {
			return idx, true
		}
	}
	return 0, false
}

func relativeDayLabel(day time.Time) string {
	today := app.DayFloor(time.Now())
	switch {
	case day.Equal(today):
		return "Today"
	case day.Equal(today.AddDate(0, 0, -1)):
		return "Yesterday"
	case day.Equal(today.AddDate(0, 0, 1)):
		return "Tomorrow"
	}
	delta := int(day.Sub(today).Hours() / 24)
	if delta > 0 {
		return fmt.Sprintf("In %d days", delta)
	}
	return fmt.Sprintf("%d days ago", -delta)
}

func responsesForQuestion(entries []app.Answer) []string {
	lines := make([]string, 0, len(entries))
	for _, ans := range entries {
		lines = append(lines, ans.Response)
	}
	return lines
}

func rebuildAnswers(existing []app.Answer, responses []string) []app.Answer {
	if len(responses) == 0 {
		return nil
	}
	pool := make(map[string][]string)
	for _, ans := range existing {
		pool[ans.Response] = append(pool[ans.Response], ans.Time)
	}
	var result []app.Answer
	for _, resp := range responses {
		resp = strings.TrimSpace(resp)
		if resp == "" {
			continue
		}
		timestamp := time.Now().Format(time.RFC3339)
		if times := pool[resp]; len(times) > 0 {
			timestamp = times[0]
			pool[resp] = times[1:]
		}
		result = append(result, app.Answer{Time: timestamp, Response: resp})
	}
	return result
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
