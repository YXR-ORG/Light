package main

import (
	"context"
	"os"
	"path/filepath"

	"wails-ai-chat/internal/eino"
	"wails-ai-chat/internal/handler"
	"wails-ai-chat/internal/storage"
)

type App struct {
	chatHandler         *handler.ChatHandler
	conversationHandler *handler.ConversationHandler
	settingsHandler     *handler.SettingsHandler
	ctx                 context.Context
}

func NewApp() *App {
	chatSvc := eino.NewChatService()
	return &App{
		chatHandler:         handler.NewChatHandler(chatSvc),
		conversationHandler: handler.NewConversationHandler(),
		settingsHandler:     handler.NewSettingsHandler(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".wails-chat", "chat.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	if err := storage.InitDB(dbPath); err != nil {
		panic("failed to init db: " + err.Error())
	}
}
