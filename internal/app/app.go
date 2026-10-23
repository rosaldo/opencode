package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"sync"
	"time"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/db"
	"github.com/opencode-ai/opencode/internal/format"
	"github.com/opencode-ai/opencode/internal/history"
	"github.com/opencode-ai/opencode/internal/llm/agent"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/lsp"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/permission"
	"github.com/opencode-ai/opencode/internal/plugin"
	"github.com/opencode-ai/opencode/internal/session"
	"github.com/opencode-ai/opencode/internal/snapshot"
	"github.com/opencode-ai/opencode/internal/tui/theme"
)

type App struct {
	Sessions    session.Service
	Messages    message.Service
	History     history.Service
	Permissions permission.Service
	Snapshots   snapshot.Service

	Agents *AgentManager

	LSPClients map[string]*lsp.Client

	clientsMutex sync.RWMutex

	snapshotsMutex sync.Mutex
	snapshotsByDir map[string]snapshot.Service

	watcherCancelFuncs []context.CancelFunc
	cancelFuncsMutex   sync.Mutex
	watcherWG          sync.WaitGroup
}

func New(ctx context.Context, conn *sql.DB) (*App, error) {
	q := db.New(conn)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	files := history.NewService(q, conn)
	snapshots := snapshot.NewService(config.WorkingDirectory())

	app := &App{
		Sessions:    sessions,
		Messages:    messages,
		History:     files,
		Permissions: permission.NewPermissionService(),
		Snapshots:   snapshots,
		LSPClients:  make(map[string]*lsp.Client),

		snapshotsByDir: make(map[string]snapshot.Service),
	}

	// Initialize theme based on configuration
	app.initTheme()

	// Initialize LSP clients in the background
	go app.initLSPClients(ctx)

	app.Agents = NewAgentManager(ctx, agent.AgentToolDeps{
		Permissions: app.Permissions,
		Sessions:    app.Sessions,
		Messages:    app.Messages,
		History:     app.History,
		LspClients:  app.LSPClients,
		Snapshots:   app.Snapshots,
		SnapshotFor: app.SnapshotFor,
	})

	return app, nil
}

// SnapshotFor returns the snapshot service for dir, creating and caching one
// per directory (Spec 09v). The launch directory reuses the boot service.
func (app *App) SnapshotFor(dir string) snapshot.Service {
	if dir == "" {
		return app.Snapshots
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return app.Snapshots
	}
	if abs == app.SnapshotsDir() {
		return app.Snapshots
	}
	app.snapshotsMutex.Lock()
	defer app.snapshotsMutex.Unlock()
	if svc, ok := app.snapshotsByDir[abs]; ok {
		return svc
	}
	svc := snapshot.NewService(abs)
	app.snapshotsByDir[abs] = svc
	return svc
}

// SnapshotsDir returns the launch directory the boot snapshot service uses.
func (app *App) SnapshotsDir() string {
	abs, err := filepath.Abs(config.WorkingDirectory())
	if err != nil {
		return config.WorkingDirectory()
	}
	return abs
}

// initTheme sets the application theme based on the configuration
func (app *App) initTheme() {
	cfg := config.Get()
	if cfg == nil || cfg.TUI.Theme == "" {
		return // Use default theme
	}

	// Try to set the theme from config
	err := theme.SetTheme(cfg.TUI.Theme)
	if err != nil {
		logging.Warn("Failed to set theme from config, using default theme", "theme", cfg.TUI.Theme, "error", err)
	} else {
		logging.Debug("Set theme from config", "theme", cfg.TUI.Theme)
	}
}

// RunNonInteractive handles the execution flow when a prompt is provided via CLI flag.
func (a *App) RunNonInteractive(ctx context.Context, prompt string, outputFormat string, quiet bool) error {
	logging.Info("Running in non-interactive mode")

	// Start spinner if not in quiet mode
	var spinner *format.Spinner
	if !quiet {
		spinner = format.NewSpinner("Thinking...")
		spinner.Start()
		defer spinner.Stop()
	}

	const maxPromptLengthForTitle = 100
	titlePrefix := "Non-interactive: "
	var titleSuffix string

	if len(prompt) > maxPromptLengthForTitle {
		titleSuffix = prompt[:maxPromptLengthForTitle] + "..."
	} else {
		titleSuffix = prompt
	}
	title := titlePrefix + titleSuffix

	sess, err := a.Sessions.Create(ctx, title)
	if err != nil {
		return fmt.Errorf("failed to create session for non-interactive mode: %w", err)
	}
	logging.Info("Created session for non-interactive run", "session_id", sess.ID)

	// Automatically approve all permission requests for this non-interactive session
	a.Permissions.AutoApproveSession(sess.ID)

	done, err := a.Agents.Run(ctx, sess.ID, prompt)
	if err != nil {
		return fmt.Errorf("failed to start agent processing stream: %w", err)
	}

	result := <-done
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, agent.ErrRequestCancelled) {
			logging.Info("Agent processing cancelled", "session_id", sess.ID)
			return nil
		}
		return fmt.Errorf("agent processing failed: %w", result.Error)
	}

	// Stop spinner before printing output
	if !quiet && spinner != nil {
		spinner.Stop()
	}

	// Get the text content from the response
	content := "No content available"
	if result.Message.Content().String() != "" {
		content = result.Message.Content().String()
	}

	fmt.Println(format.FormatOutput(content, outputFormat))

	logging.Info("Non-interactive run completed", "session_id", sess.ID)

	return nil
}

// Shutdown performs a clean shutdown of the application
func (app *App) Shutdown() {
	// Cancel all watcher goroutines
	app.cancelFuncsMutex.Lock()
	for _, cancel := range app.watcherCancelFuncs {
		cancel()
	}
	app.cancelFuncsMutex.Unlock()
	app.watcherWG.Wait()

	if app.Agents != nil {
		app.Agents.Shutdown()
	}

	// Close every persistent MCP client (Spec 08 lifecycle).
	agent.GetMCPManager().CloseAll()

	// Stop plugin processes (Spec 10 lifecycle).
	if mgr := plugin.GetManager(); mgr != nil {
		mgr.Close()
	}

	// Perform additional cleanup for LSP clients
	app.clientsMutex.RLock()
	clients := make(map[string]*lsp.Client, len(app.LSPClients))
	maps.Copy(clients, app.LSPClients)
	app.clientsMutex.RUnlock()

	for name, client := range clients {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := client.Shutdown(shutdownCtx); err != nil {
			logging.Error("Failed to shutdown LSP client", "name", name, "error", err)
		}
		cancel()
	}
}
