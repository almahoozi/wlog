package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type BuildInfo struct {
	Commit  string
	Ref     string
	Version string
}

var DefaultQuestions = []string{
	"What did you do yesterday?",
	"What will/did you do today?",
	"Are you blocked with anything?",
}

var lastDaysPattern = regexp.MustCompile(`^last\s+(\d+)\s+days?$`)

func Run(args []string, build BuildInfo) error {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "using default questions: %v\n", err)
	}

	if len(args) == 0 {
		return RunPrompts(cfg.Questions)
	}

	switch args[0] {
	case "view":
		interval := strings.Join(args[1:], " ")
		return RunView(interval, cfg.Questions)
	case "cat":
		interval := strings.Join(args[1:], " ")
		return RunCat(interval, cfg.Questions)
	case "ls":
		return RunLS(args[1:])
	case "help", "-h", "--help":
		fmt.Println(UsageText())
		return nil
	case "version", "-v", "--version":
		fmt.Printf("wlog %s %s %s\n", build.Commit, build.Ref, build.Version)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], UsageText())
	}
}

func UsageText() string {
	return strings.TrimSpace(`wlog - a simple work log

Usage:
  wlog                Run prompts for today's log
  wlog view           Show today's entries
  wlog view <interval>
                      Show entries for a plain-english interval (e.g. "yesterday", "last 3 days", "last week", "this year")
  wlog cat             Print today's entries in list-view format
  wlog cat <interval>
                      Print entries in list-view format for a plain-english interval
  wlog ls              Print the log storage directory path
  wlog ls config       Print the config file path
  wlog help           Show this help message
  wlog version        Show build metadata

Examples:
  wlog
  wlog ls
  wlog ls config
  wlog view yesterday
  wlog view "last 3 days"`)
}

func RunLS(args []string) error {
	if len(args) > 0 && args[0] == "config" {
		path, err := ConfigFilePath()
		if err != nil {
			return err
		}
		if err := EnsureDir(filepath.Dir(path)); err != nil {
			return err
		}
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if err := writeConfig(path, Config{Questions: DefaultQuestions}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	}

	dir, err := DataDir()
	if err != nil {
		return err
	}
	if err := EnsureDir(dir); err != nil {
		return err
	}
	fmt.Println(dir)
	return nil
}

func RunPrompts(questions []string) error {
	if len(questions) == 0 {
		fmt.Println("No questions configured. Update your config file to add some.")
		return nil
	}

	today := DayFloor(time.Now())
	log, err := LoadDayLog(today)
	if err != nil {
		return err
	}

	fmt.Println("Answer the following questions. Press Enter to skip any question.")
	reader := bufio.NewReader(os.Stdin)
	updated := false

	for _, q := range questions {
		fmt.Printf("%s\n> ", q)
		text, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		response := strings.TrimSpace(text)
		if response == "" {
			continue
		}
		if log.Answers == nil {
			log.Answers = make(map[string][]Answer)
		}
		log.Answers[q] = append(log.Answers[q], Answer{
			Time:     time.Now().Format(time.RFC3339),
			Response: response,
		})
		updated = true
	}

	if !updated {
		fmt.Println("No entries recorded today.")
		return nil
	}

	if err := SaveDayLog(today, log); err != nil {
		return err
	}

	fmt.Println("Entries saved.")
	return nil
}

func RunView(interval string, questions []string) error {
	start, end, err := ParseInterval(interval)
	if err != nil {
		return err
	}

	var logs []DayLog
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		entry, err := ReadDayLogIfExists(cursor)
		if err != nil {
			return err
		}
		if entry != nil {
			logs = append(logs, *entry)
		}
	}

	if len(logs) == 0 {
		if interval == "" {
			interval = "today"
		}
		fmt.Printf("No entries found for %s.\n", interval)
		return nil
	}

	for _, day := range logs {
		printDayLog(day, questions)
	}

	return nil
}

func RunCat(interval string, questions []string) error {
	start, end, err := ParseInterval(interval)
	if err != nil {
		return err
	}

	trimmed := strings.ToLower(strings.TrimSpace(interval))
	forceSingleDay := start.Equal(end) && (trimmed == "" || trimmed == "today")
	printed := false

	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		log, err := LoadDayLog(cursor)
		if err != nil {
			return err
		}
		if !forceSingleDay && !dayLogHasEntries(log) {
			continue
		}
		fmt.Print(renderListView(cursor, log, questions))
		printed = true
	}

	if !printed {
		fmt.Printf("No entries found for %s.\n", intervalLabel(interval))
	}

	return nil
}

func dayLogHasEntries(log DayLog) bool {
	for _, answers := range log.Answers {
		if len(answers) > 0 {
			return true
		}
	}
	return false
}

var listIndexRunes = []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'}

func renderListView(day time.Time, log DayLog, base []string) string {
	if log.Answers == nil {
		log.Answers = make(map[string][]Answer)
	}

	var b strings.Builder
	dayLabel := day.Format("Mon 2006-01-02")
	b.WriteString(fmt.Sprintf("%s — %s\n\n", dayLabel, relativeDayLabel(day)))

	ordered := mergeQuestionsForList(base, log)
	if len(ordered) == 0 {
		b.WriteString("No questions configured.\n\n")
		return b.String()
	}

	for idx, q := range ordered {
		answers := log.Answers[q]
		label := "--"
		if idx < len(listIndexRunes) {
			label = string(listIndexRunes[idx])
		}
		countLabel := ""
		if len(answers) > 0 {
			countLabel = fmt.Sprintf(" (%d)", len(answers))
		}
		b.WriteString(fmt.Sprintf("[%s] %s%s\n", label, q, countLabel))
		for _, ans := range answers {
			b.WriteString(fmt.Sprintf("    - [%s] %s\n", DisplayTime(ans.Time), ans.Response))
		}
	}

	b.WriteString("\n")
	return b.String()
}

func mergeQuestionsForList(base []string, log DayLog) []string {
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

func relativeDayLabel(day time.Time) string {
	today := DayFloor(time.Now())
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

func intervalLabel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "today"
	}
	return trimmed
}

func printDayLog(day DayLog, questions []string) {
	fmt.Printf("%s\n", day.Date)

	ordered := OrderQuestions(day.Answers, questions)
	for _, q := range ordered {
		answers := day.Answers[q]
		if len(answers) == 0 {
			continue
		}
		fmt.Printf("  %s\n", q)
		for _, ans := range answers {
			fmt.Printf("    - [%s] %s\n", DisplayTime(ans.Time), ans.Response)
		}
	}

	fmt.Println()
}

func OrderQuestions(answers map[string][]Answer, base []string) []string {
	seen := make(map[string]bool)
	ordered := make([]string, 0, len(answers))
	for _, q := range base {
		if _, ok := answers[q]; ok {
			ordered = append(ordered, q)
			seen[q] = true
		}
	}
	extras := make([]string, 0, len(answers))
	for q := range answers {
		if !seen[q] {
			extras = append(extras, q)
		}
	}
	sort.Strings(extras)
	ordered = append(ordered, extras...)
	return ordered
}

func ParseInterval(raw string) (time.Time, time.Time, error) {
	now := DayFloor(time.Now())
	input := strings.ToLower(strings.TrimSpace(raw))
	if input == "" || input == "today" {
		return now, now, nil
	}
	switch input {
	case "yesterday":
		day := now.AddDate(0, 0, -1)
		return day, day, nil
	case "last week":
		end := StartOfWeek(now).AddDate(0, 0, -1)
		start := end.AddDate(0, 0, -6)
		return start, end, nil
	case "this week":
		start := StartOfWeek(now)
		return start, now, nil
	case "this year":
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		return start, now, nil
	}

	if matches := lastDaysPattern.FindStringSubmatch(input); len(matches) == 2 {
		days, err := strconv.Atoi(matches[1])
		if err != nil || days <= 0 {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid day count in interval %q", raw)
		}
		start := now.AddDate(0, 0, -(days - 1))
		return start, now, nil
	}

	return time.Time{}, time.Time{}, fmt.Errorf("unsupported interval %q", raw)
}

func StartOfWeek(t time.Time) time.Time {
	base := DayFloor(t)
	weekday := int(base.Weekday())
	if weekday == 0 { // Sunday
		weekday = 6
	} else {
		weekday--
	}
	return base.AddDate(0, 0, -weekday)
}

func DayFloor(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func LoadConfig() (Config, error) {
	path, err := ConfigFilePath()
	if err != nil {
		cfg := Config{Questions: DefaultQuestions}
		cfg.ensureDefaults()
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		cfg := Config{Questions: DefaultQuestions}
		cfg.ensureDefaults()
		if err := writeConfig(path, cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err != nil {
		cfg := Config{Questions: DefaultQuestions}
		cfg.ensureDefaults()
		return cfg, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		cfg = Config{Questions: DefaultQuestions}
		cfg.ensureDefaults()
		return cfg, err
	}
	cfg.ensureDefaults()

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err == nil {
		if applyDefaultMarkers(raw) {
			if err := writeConfigMap(path, raw); err != nil {
				return cfg, err
			}
		}
	}

	return cfg, nil
}

func SaveConfig(cfg Config) error {
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	return writeConfig(path, cfg)
}

func writeConfig(path string, cfg Config) error {
	cfg.ensureDefaults()

	raw, err := readConfigMap(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		raw = make(map[string]any)
	}

	applyConfigToMap(raw, cfg)
	applyDefaultMarkers(raw)
	return writeConfigMap(path, raw)
}

func readConfigMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func writeConfigMap(path string, raw map[string]any) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func applyConfigToMap(raw map[string]any, cfg Config) {
	raw["questions"] = append([]string(nil), cfg.Questions...)
	setOptionalBool(raw, "showHints", cfg.ShowHints)
	setOptionalBool(raw, "autoInsertEntries", cfg.AutoInsertEntries)
	setOptionalBool(raw, "defaultListMode", cfg.DefaultListMode)
	setOptionalBool(raw, "autoOpenIndexJump", cfg.AutoOpenIndexJump)
	setOptionalBool(raw, "confirmDelete", cfg.ConfirmDelete)
	setOptionalBool(raw, "continueInsertAfterSave", cfg.ContinueInsertAfterSave)
	setOptionalBool(raw, "confirmEscapeWithText", cfg.ConfirmEscapeWithText)
	setOptionalInt(raw, "statusMessageDurationMs", cfg.StatusMessageDurationMs)
	setOptionalInt(raw, "escapeConfirmTimeoutMs", cfg.EscapeConfirmTimeoutMs)
}

func setOptionalBool(raw map[string]any, key string, value *bool) {
	if value == nil {
		delete(raw, key)
		return
	}
	raw[key] = *value
}

func setOptionalInt(raw map[string]any, key string, value *int) {
	if value == nil {
		delete(raw, key)
		return
	}
	raw[key] = *value
}

func applyDefaultMarkers(raw map[string]any) bool {
	changed := false
	for key, value := range defaultConfigMarkers {
		if current, ok := raw[key]; ok && configValuesEqual(current, value) {
			continue
		}
		raw[key] = value
		changed = true
	}
	return changed
}

func configValuesEqual(a, b any) bool {
	switch av := a.(type) {
	case float64:
		switch bv := b.(type) {
		case float64:
			return av == bv
		case int:
			return av == float64(bv)
		}
	case int:
		switch bv := b.(type) {
		case int:
			return av == bv
		case float64:
			return float64(av) == bv
		}
	}
	return reflect.DeepEqual(a, b)
}

func ConfigFilePath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "wlog", "config.json"), nil
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("AppData"); appData != "" {
			return filepath.Join(appData, "wlog", "config.json"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "wlog", "config.json"), nil
	}
	return filepath.Join(home, ".config", "wlog", "config.json"), nil
}

func DataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "wlog"), nil
	}
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LocalAppData"); localAppData != "" {
			return filepath.Join(localAppData, "wlog"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "wlog"), nil
	}
	return filepath.Join(home, ".local", "share", "wlog"), nil
}

func DayFilePath(date time.Time) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.json", date.Format("2006-01-02"))
	return filepath.Join(dir, name), nil
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func LoadDayLog(date time.Time) (DayLog, error) {
	entry, err := ReadDayLogIfExists(date)
	if err != nil {
		return DayLog{}, err
	}
	if entry == nil {
		return DayLog{
			Date:    date.Format("2006-01-02"),
			Answers: make(map[string][]Answer),
		}, nil
	}
	if entry.Answers == nil {
		entry.Answers = make(map[string][]Answer)
	}
	return *entry, nil
}

func ReadDayLogIfExists(date time.Time) (*DayLog, error) {
	path, err := DayFilePath(date)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var log DayLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	if log.Answers == nil {
		log.Answers = make(map[string][]Answer)
	}
	return &log, nil
}

func SaveDayLog(date time.Time, log DayLog) error {
	path, err := DayFilePath(date)
	if err != nil {
		return err
	}
	log.Date = date.Format("2006-01-02")
	if log.Answers == nil {
		log.Answers = make(map[string][]Answer)
	}
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func DisplayTime(value string) string {
	if value == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Format("15:04")
	}
	return value
}

const (
	defaultShowHints               = true
	defaultAutoInsertEntries       = true
	defaultListMode                = false
	defaultAutoOpenIndexJump       = true
	defaultConfirmDelete           = true
	defaultStatusMessageDurationMs = 1000
	defaultContinueInsertAfterSave = true
	defaultConfirmEscapeWithText   = true
	defaultEscapeConfirmTimeoutMs  = 1000
)

var defaultConfigMarkers = map[string]any{
	"_showHints":               defaultShowHints,
	"_autoInsertEntries":       defaultAutoInsertEntries,
	"_defaultListMode":         defaultListMode,
	"_autoOpenIndexJump":       defaultAutoOpenIndexJump,
	"_confirmDelete":           defaultConfirmDelete,
	"_statusMessageDurationMs": float64(defaultStatusMessageDurationMs),
	"_continueInsertAfterSave": defaultContinueInsertAfterSave,
	"_confirmEscapeWithText":   defaultConfirmEscapeWithText,
	"_escapeConfirmTimeoutMs":  float64(defaultEscapeConfirmTimeoutMs),
}

type Config struct {
	Questions               []string `json:"questions"`
	ShowHints               *bool    `json:"showHints,omitempty"`
	AutoInsertEntries       *bool    `json:"autoInsertEntries,omitempty"`
	DefaultListMode         *bool    `json:"defaultListMode,omitempty"`
	AutoOpenIndexJump       *bool    `json:"autoOpenIndexJump,omitempty"`
	ConfirmDelete           *bool    `json:"confirmDelete,omitempty"`
	ContinueInsertAfterSave *bool    `json:"continueInsertAfterSave,omitempty"`
	ConfirmEscapeWithText   *bool    `json:"confirmEscapeWithText,omitempty"`
	StatusMessageDurationMs *int     `json:"statusMessageDurationMs,omitempty"`
	EscapeConfirmTimeoutMs  *int     `json:"escapeConfirmTimeoutMs,omitempty"`
}

type DayLog struct {
	Date    string              `json:"date"`
	Answers map[string][]Answer `json:"answers"`
}

type Answer struct {
	Time     string `json:"time"`
	Response string `json:"response"`
}

func (cfg *Config) ensureDefaults() {
	if len(cfg.Questions) == 0 {
		cfg.Questions = DefaultQuestions
	}
	if cfg.StatusMessageDurationMs != nil && *cfg.StatusMessageDurationMs <= 0 {
		cfg.StatusMessageDurationMs = nil
	}
	if cfg.EscapeConfirmTimeoutMs != nil && *cfg.EscapeConfirmTimeoutMs <= 0 {
		cfg.EscapeConfirmTimeoutMs = nil
	}
}

func (cfg Config) HintsEnabled() bool {
	if cfg.ShowHints == nil {
		return defaultShowHints
	}
	return *cfg.ShowHints
}

func (cfg Config) AutoInsertEnabled() bool {
	if cfg.AutoInsertEntries == nil {
		return defaultAutoInsertEntries
	}
	return *cfg.AutoInsertEntries
}

func (cfg Config) DefaultListModeEnabled() bool {
	if cfg.DefaultListMode == nil {
		return defaultListMode
	}
	return *cfg.DefaultListMode
}

func (cfg Config) AutoOpenIndexJumpEnabled() bool {
	if cfg.AutoOpenIndexJump == nil {
		return defaultAutoOpenIndexJump
	}
	return *cfg.AutoOpenIndexJump
}

func (cfg Config) ConfirmDeleteEnabled() bool {
	if cfg.ConfirmDelete == nil {
		return defaultConfirmDelete
	}
	return *cfg.ConfirmDelete
}

func (cfg Config) StatusMessageDuration() time.Duration {
	ms := defaultStatusMessageDurationMs
	if cfg.StatusMessageDurationMs != nil && *cfg.StatusMessageDurationMs > 0 {
		ms = *cfg.StatusMessageDurationMs
	}
	return time.Duration(ms) * time.Millisecond
}

func (cfg Config) ContinueInsertAfterSaveEnabled() bool {
	if cfg.ContinueInsertAfterSave == nil {
		return defaultContinueInsertAfterSave
	}
	return *cfg.ContinueInsertAfterSave
}

func (cfg Config) ConfirmEscapeWithTextEnabled() bool {
	if cfg.ConfirmEscapeWithText == nil {
		return defaultConfirmEscapeWithText
	}
	return *cfg.ConfirmEscapeWithText
}

func (cfg Config) EscapeConfirmTimeout() time.Duration {
	ms := defaultEscapeConfirmTimeoutMs
	if cfg.EscapeConfirmTimeoutMs != nil && *cfg.EscapeConfirmTimeoutMs > 0 {
		ms = *cfg.EscapeConfirmTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}
