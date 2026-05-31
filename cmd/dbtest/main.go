package main

import (
	"fmt"
	"os"
	"path/filepath"
	"light-ai/internal/storage"
)

func main() {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".wails-chat", "chat.db")
	if err := storage.InitDB(dbPath); err != nil {
		panic(err)
	}
	p := &storage.LLMProvider{Name: "Test", Type: "openai", APIKey: "sk-x", Enabled: false}
	err := storage.SaveProvider(p)
	fmt.Println("SaveProvider err:", err, "id:", p.ID)
	list, _ := storage.ListProviders()
	fmt.Println("Total providers:", len(list))
	for _, v := range list {
		fmt.Printf("  %s | %s | enabled=%v\n", v.Name, v.Type, v.Enabled)
	}
}
