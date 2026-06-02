package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	appdb "step-ui/db"
)

const (
	adminConsoleTimeout = 8 * time.Second
	adminConsoleMaxOut  = 16 * 1024
)

type adminConsoleCommand struct {
	ID          string
	Label       string
	Description string
	Name        string
	Args        []string
}

type adminConsoleResult struct {
	CommandLine string
	Output      string
	ExitCode    int
	Duration    string
	TimedOut    bool
	Truncated   bool
	Success     bool
}

func adminConsoleCommands() []adminConsoleCommand {
	return []adminConsoleCommand{
		{ID: "system.date", Label: "Дата и время", Description: "Текущее время внутри контейнера step-ui", Name: "date"},
		{ID: "system.hostname", Label: "Hostname", Description: "Имя контейнера", Name: "hostname"},
		{ID: "system.identity", Label: "Текущий пользователь", Description: "UID/GID процесса приложения", Name: "id"},
		{ID: "system.disk", Label: "Диск", Description: "Свободное место для каталогов приложения и CA", Name: "df", Args: []string{"-h", "/opt/step-ui", "/home/step"}},
		{ID: "system.processes", Label: "Процессы", Description: "Список процессов внутри контейнера", Name: "ps"},
		{ID: "app.files", Label: "Каталоги приложения", Description: "Верхний уровень /opt/step-ui", Name: "ls", Args: []string{"-la", "/opt/step-ui"}},
		{ID: "step.version", Label: "step version", Description: "Версия Smallstep CLI внутри контейнера", Name: "step", Args: []string{"version"}},
		{ID: "step.ca.health", Label: "step-ca health", Description: "Проверка доступности CA из контейнера UI", Name: "step", Args: []string{"ca", "health", "--ca-url", "https://step-ca:9443", "--root", "/home/step/certs/root_ca.crt"}},
		{ID: "openssl.version", Label: "OpenSSL version", Description: "Версия OpenSSL", Name: "openssl", Args: []string{"version", "-a"}},
		{ID: "postgres.ready", Label: "PostgreSQL readiness", Description: "Проверка доступности PostgreSQL", Name: "pg_isready", Args: []string{"-h", "postgres", "-U", "stepui", "-d", "stepui"}},
	}
}

func findAdminConsoleCommand(id string) (adminConsoleCommand, bool) {
	for _, c := range adminConsoleCommands() {
		if c.ID == id {
			return c, true
		}
	}
	return adminConsoleCommand{}, false
}

func (h *Handler) AdminConsoleGet(w http.ResponseWriter, r *http.Request) {
	data := h.adminConsoleData(w, r)
	h.render(w, "admin_console", data)
}

func (h *Handler) AdminConsolePost(w http.ResponseWriter, r *http.Request) {
	if !h.requireCSRF(w, r, "/admin/console") {
		return
	}

	commandID := strings.TrimSpace(r.FormValue("command_id"))
	data := h.adminConsoleData(w, r)
	data["SelectedCommandID"] = commandID

	c, ok := findAdminConsoleCommand(commandID)
	if !ok {
		h.auditSecurity(r, "console.denied command_id="+commandID)
		data["ConsoleError"] = "Команда не входит в allowlist."
		h.render(w, "admin_console", data)
		return
	}

	result := runAdminConsoleCommand(r.Context(), c)
	data["Result"] = result
	h.auditSecurity(r, fmt.Sprintf("console.run id=%s command=%q exit=%d timeout=%t duration=%s",
		c.ID, result.CommandLine, result.ExitCode, result.TimedOut, result.Duration))
	h.render(w, "admin_console", data)
}

func (h *Handler) adminConsoleData(w http.ResponseWriter, r *http.Request) map[string]interface{} {
	data := h.base(w, r, "admin_console")
	data["Commands"] = adminConsoleCommands()
	data["Timeout"] = adminConsoleTimeout.String()
	data["MaxOutputKB"] = adminConsoleMaxOut / 1024

	si := h.sessionInfo(r)
	if u, err := appdb.GetUserByID(h.db, si.UserID); err == nil && u != nil {
		data["TOTPEnabled"] = u.TOTPEnabled
	}
	return data
}

func runAdminConsoleCommand(ctx context.Context, c adminConsoleCommand) adminConsoleResult {
	cctx, cancel := context.WithTimeout(ctx, adminConsoleTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(cctx, c.Name, c.Args...)
	cmd.Dir = "/opt/step-ui"
	out, err := cmd.CombinedOutput()
	duration := time.Since(start).Round(time.Millisecond)

	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	timedOut := cctx.Err() == context.DeadlineExceeded
	if timedOut {
		exitCode = -1
	}

	truncated := false
	if len(out) > adminConsoleMaxOut {
		out = append(out[:adminConsoleMaxOut], []byte("\n\n[output truncated]\n")...)
		truncated = true
	}
	text := strings.TrimRight(string(bytes.ToValidUTF8(out, []byte("?"))), "\r\n")
	if text == "" && err != nil {
		text = err.Error()
	}
	if timedOut {
		text = strings.TrimSpace(text + "\ncommand timed out")
	}

	return adminConsoleResult{
		CommandLine: commandLine(c),
		Output:      text,
		ExitCode:    exitCode,
		Duration:    duration.String(),
		TimedOut:    timedOut,
		Truncated:   truncated,
		Success:     err == nil && !timedOut,
	}
}

func commandLine(c adminConsoleCommand) string {
	parts := append([]string{c.Name}, c.Args...)
	return strings.Join(parts, " ")
}
