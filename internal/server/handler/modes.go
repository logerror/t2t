package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/logerror/t2t/internal/server/web"
)

const (
	commandLogDir       = "/tmp/t2t/commands"
	defaultCommandLimit = 500
	maxCommandLimit     = 5000
)

var safeIdentifierPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type timeMachineCommandRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	HostTag     string    `json:"host_tag"`
	ClientID    string    `json:"client_id"`
	Command     string    `json:"command"`
	RawCommand  string    `json:"raw_command,omitempty"`
	User        string    `json:"user,omitempty"`
	WorkingDir  string    `json:"working_dir,omitempty"`
	IsDangerous bool      `json:"is_dangerous,omitempty"`
}

type TimeMachineSession struct {
	SessionKey     string `json:"sessionKey"`
	HostTag        string `json:"hostTag"`
	ClientID       string `json:"clientId"`
	CommandCount   int    `json:"commandCount"`
	DangerousCount int    `json:"dangerousCount"`
	FirstTimestamp int64  `json:"firstTimestamp"`
	LastTimestamp  int64  `json:"lastTimestamp"`
	LastCommand    string `json:"lastCommand"`
}

type TimeMachineCommand struct {
	Timestamp  int64  `json:"timestamp"`
	HostTag    string `json:"hostTag"`
	ClientID   string `json:"clientId"`
	Command    string `json:"command"`
	RawCommand string `json:"rawCommand,omitempty"`
	User       string `json:"user,omitempty"`
	WorkingDir string `json:"workingDir,omitempty"`
	Dangerous  bool   `json:"dangerous"`
}

type TimeMachineCommandsOutput struct {
	HostTag   string               `json:"hostTag"`
	ClientID  string               `json:"clientId"`
	Commands  []TimeMachineCommand `json:"commands"`
	Total     int                  `json:"total"`
	Truncated bool                 `json:"truncated"`
}

func ServeTimeMachinePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl := template.Must(template.ParseFS(web.TemplateFiles, "templates/time_machine.html"))
	if err := tmpl.Execute(w, nil); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func ServeWarRoomPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl := template.Must(template.ParseFS(web.TemplateFiles, "templates/war_room.html"))
	if err := tmpl.Execute(w, nil); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func ListTimeMachineSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessions, err := collectTimeMachineSessions()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list sessions: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
	}
}

func GetTimeMachineCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hostTag := r.URL.Query().Get("hostTag")
	clientID := r.URL.Query().Get("clientId")
	if hostTag == "" || clientID == "" {
		http.Error(w, "Missing hostTag or clientId", http.StatusBadRequest)
		return
	}
	if !safeIdentifierPattern.MatchString(hostTag) || !safeIdentifierPattern.MatchString(clientID) {
		http.Error(w, "Invalid hostTag or clientId", http.StatusBadRequest)
		return
	}

	limit := defaultCommandLimit
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		n, err := strconv.Atoi(rawLimit)
		if err != nil || n <= 0 {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
		if n > maxCommandLimit {
			n = maxCommandLimit
		}
		limit = n
	}

	logFile := filepath.Join(commandLogDir, fmt.Sprintf("commands_%s_%s.json", hostTag, clientID))
	commands, total, truncated, err := readTimeMachineCommands(logFile, limit)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			output := TimeMachineCommandsOutput{
				HostTag:   hostTag,
				ClientID:  clientID,
				Commands:  []TimeMachineCommand{},
				Total:     0,
				Truncated: false,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(output)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to read commands: %v", err), http.StatusInternalServerError)
		return
	}

	output := TimeMachineCommandsOutput{
		HostTag:   hostTag,
		ClientID:  clientID,
		Commands:  commands,
		Total:     total,
		Truncated: truncated,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(output); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
	}
}

func collectTimeMachineSessions() ([]TimeMachineSession, error) {
	files, err := filepath.Glob(filepath.Join(commandLogDir, "commands_*.json"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return []TimeMachineSession{}, nil
	}

	sessions := make([]TimeMachineSession, 0, len(files))
	for _, file := range files {
		session, ok, err := summarizeCommandLog(file)
		if err != nil {
			continue
		}
		if !ok {
			continue
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].LastTimestamp == sessions[j].LastTimestamp {
			return sessions[i].SessionKey < sessions[j].SessionKey
		}
		return sessions[i].LastTimestamp > sessions[j].LastTimestamp
	})

	return sessions, nil
}

func summarizeCommandLog(logFile string) (TimeMachineSession, bool, error) {
	file, err := os.Open(logFile)
	if err != nil {
		return TimeMachineSession{}, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 2*1024*1024)

	var (
		record         timeMachineCommandRecord
		hostTag        string
		clientID       string
		firstTimestamp int64
		lastTimestamp  int64
		lastCommand    string
		commandCount   int
		dangerousCount int
	)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}

		if record.HostTag == "" || record.ClientID == "" {
			continue
		}

		ts := record.Timestamp.UnixMilli()
		if ts <= 0 {
			continue
		}

		if hostTag == "" {
			hostTag = record.HostTag
			clientID = record.ClientID
			firstTimestamp = ts
		}

		commandCount++
		lastTimestamp = ts
		lastCommand = record.Command
		if record.IsDangerous {
			dangerousCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return TimeMachineSession{}, false, err
	}
	if commandCount == 0 || hostTag == "" || clientID == "" {
		return TimeMachineSession{}, false, nil
	}

	return TimeMachineSession{
		SessionKey:     fmt.Sprintf("%s-%s", hostTag, clientID),
		HostTag:        hostTag,
		ClientID:       clientID,
		CommandCount:   commandCount,
		DangerousCount: dangerousCount,
		FirstTimestamp: firstTimestamp,
		LastTimestamp:  lastTimestamp,
		LastCommand:    lastCommand,
	}, true, nil
}

func readTimeMachineCommands(logFile string, limit int) ([]TimeMachineCommand, int, bool, error) {
	file, err := os.Open(logFile)
	if err != nil {
		return nil, 0, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 2*1024*1024)

	commands := make([]TimeMachineCommand, 0, limit)
	total := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record timeMachineCommandRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Command == "" || record.HostTag == "" || record.ClientID == "" {
			continue
		}

		total++
		item := TimeMachineCommand{
			Timestamp:  record.Timestamp.UnixMilli(),
			HostTag:    record.HostTag,
			ClientID:   record.ClientID,
			Command:    record.Command,
			RawCommand: record.RawCommand,
			User:       record.User,
			WorkingDir: record.WorkingDir,
			Dangerous:  record.IsDangerous,
		}

		if len(commands) >= limit {
			copy(commands, commands[1:])
			commands[len(commands)-1] = item
			continue
		}
		commands = append(commands, item)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, false, err
	}

	return commands, total, total > len(commands), nil
}
