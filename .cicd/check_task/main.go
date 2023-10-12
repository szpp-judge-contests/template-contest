package main

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	pkgtask "github.com/szpp-judge-contests/template-contest/task"
)

func main() {
	rootDir := os.Getenv("TASKS_ROOT")
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".example" {
			continue
		}
		taskDirPath := filepath.Join(rootDir, entry.Name())
		if _, err := os.Stat(filepath.Join(taskDirPath, "task.yaml")); errors.Is(err, os.ErrNotExist) {
			continue
		}

		slog.Info("start checking task", "task", entry.Name())

		task, err := pkgtask.Load(taskDirPath)
		if err != nil {
			slog.Error("failed to load task", "task", entry.Name(), "error", err)
			os.Exit(1)
		}

		if err := task.Check(); err != nil {
			slog.Error("found error", "task", task.Dir, "error", err)
			os.Exit(1)
		}
	}
}
