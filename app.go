package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"light-ai/internal/eino"
	"light-ai/internal/handler"
	"light-ai/internal/storage"
)

type App struct {
	chatHandler         *handler.ChatHandler
	conversationHandler *handler.ConversationHandler
	settingsHandler     *handler.SettingsHandler
	mcpHandler          *handler.MCPHandler
	providerHandler     *handler.ProviderHandler
	agentHandler        *handler.AgentHandler
	skillHandler        *handler.SkillHandler
	ctx                 context.Context
}

func NewApp() *App {
	chatSvc := eino.NewChatService()
	return &App{
		chatHandler:         handler.NewChatHandler(chatSvc),
		conversationHandler: handler.NewConversationHandler(),
		settingsHandler:     handler.NewSettingsHandler(),
		mcpHandler:          handler.NewMCPHandler(),
		providerHandler:     handler.NewProviderHandler(),
		agentHandler:        handler.NewAgentHandler(),
		skillHandler:        handler.NewSkillHandler(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.chatHandler.SetContext(ctx)
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".wails-chat")
	os.MkdirAll(logDir, 0755)

	// 写文件日志，方便调试
	logFile, err := os.OpenFile(filepath.Join(logDir, "app.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	dbPath := filepath.Join(logDir, "chat.db")
	if err := storage.InitDB(dbPath); err != nil {
		panic("failed to init db: " + err.Error())
	}
	if err := storage.SeedAgents(); err != nil {
		panic("failed to seed agents: " + err.Error())
	}
}
