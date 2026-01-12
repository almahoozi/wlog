package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	commit  = "unknown"
	ref     = "unknown"
	version = "unknown"
)

var defaultQuestions = []string{
	"What did you do yesterday?",
	"What will/did you do today?",
	"Are you blocked with anything?",
}

var lastDaysPattern = regexp.MustCompile(`^last\s+(\d+)\s+days?$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "using default questions: %v\n", err)
	}

	if len(args) == 0 {
		return runPrompts(cfg.Questions)
	}

	switch args[0] {
	case "view":
		interval := strings.Join(args[1:], " ")
		return runView(interval, cfg.Questions)
	case "ls":
		return runLS(args[1:])
	case "help", "-h", "--help":
		fmt.Println(usageText())
		return nil
	case "version", "-v", "--version":
		fmt.Printf("wlog %s %s %s\n", commit, ref, version)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func usageText() string {
	return strings.TrimSpace(`wlog - a simple work log

Usage:
  wlog                Run prompts for today's log
  wlog view           Show today's entries
  wlog view <interval>
                      Show entries for a plain-english interval (e.g. "yesterday", "last 3 days", "last week", "this year")
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

func runLS(args []string) error {
	if len(args) > 0 && args[0] == "config" {
		path, err := configFilePath()
		if err != nil {
			return err
		}
		if err := ensureDir(filepath.Dir(path)); err != nil {
			return err
		}
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if err := saveConfig(path, Config{Questions: defaultQuestions}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	}

	dir, err := dataDir()
	if err != nil {
		return err
	}
	if err := ensureDir(dir); err != nil {
		return err
	}
	fmt.Println(dir)
	return nil
}

func runPrompts(questions []string) error {
	if len(questions) == 0 {
		fmt.Println("No questions configured. Update your config file to add some.")
		return nil
	}

	today := dayFloor(time.Now())
	log, err := loadDayLog(today)
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

	if err := saveDayLog(today, log); err != nil {
		return err
	}

	fmt.Println("Entries saved.")
	return nil
}

func runView(interval string, questions []string) error {
	start, end, err := parseInterval(interval)
	if err != nil {
		return err
	}

	var logs []DayLog
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		entry, err := readDayLogIfExists(cursor)
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

func printDayLog(day DayLog, questions []string) {
	fmt.Printf("%s\n", day.Date)

	ordered := orderQuestions(day.Answers, questions)
	for _, q := range ordered {
		answers := day.Answers[q]
		if len(answers) == 0 {
			continue
		}
		fmt.Printf("  %s\n", q)
		for _, ans := range answers {
			fmt.Printf("    - [%s] %s\n", displayTime(ans.Time), ans.Response)
		}
	}

	fmt.Println()
}

func orderQuestions(answers map[string][]Answer, base []string) []string {
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

func parseInterval(raw string) (time.Time, time.Time, error) {
	now := dayFloor(time.Now())
	input := strings.ToLower(strings.TrimSpace(raw))
	if input == "" || input == "today" {
		return now, now, nil
	}
	switch input {
	case "yesterday":
		day := now.AddDate(0, 0, -1)
		return day, day, nil
	case "last week":
		end := startOfWeek(now).AddDate(0, 0, -1)
		start := end.AddDate(0, 0, -6)
		return start, end, nil
	case "this week":
		start := startOfWeek(now)
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

func startOfWeek(t time.Time) time.Time {
	base := dayFloor(t)
	weekday := int(base.Weekday())
	if weekday == 0 { // Sunday
		weekday = 6
	} else {
		weekday--
	}
	return base.AddDate(0, 0, -weekday)
}

func dayFloor(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func loadConfig() (Config, error) {
	path, err := configFilePath()
	if err != nil {
		return Config{Questions: defaultQuestions}, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		cfg := Config{Questions: defaultQuestions}
		if err := saveConfig(path, cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err != nil {
		return Config{Questions: defaultQuestions}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{Questions: defaultQuestions}, err
	}
	if len(cfg.Questions) == 0 {
		cfg.Questions = defaultQuestions
	}
	return cfg, nil
}

func saveConfig(path string, cfg Config) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func configFilePath() (string, error) {
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

func dataDir() (string, error) {
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

func dayFilePath(date time.Time) (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	if err := ensureDir(dir); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.json", date.Format("2006-01-02"))
	return filepath.Join(dir, name), nil
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func loadDayLog(date time.Time) (DayLog, error) {
	entry, err := readDayLogIfExists(date)
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

func readDayLogIfExists(date time.Time) (*DayLog, error) {
	path, err := dayFilePath(date)
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

func saveDayLog(date time.Time, log DayLog) error {
	path, err := dayFilePath(date)
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

func displayTime(value string) string {
	if value == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Format("15:04")
	}
	return value
}

type Config struct {
	Questions []string `json:"questions"`
}

type DayLog struct {
	Date    string              `json:"date"`
	Answers map[string][]Answer `json:"answers"`
}

type Answer struct {
	Time     string `json:"time"`
	Response string `json:"response"`
}
