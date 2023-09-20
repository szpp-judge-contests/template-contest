package main

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/samber/lo"
	pkgtask "github.com/szpp-judge-contests/template-contest/task"
)

func main() {
	entries, err := os.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(entry.Name(), "task.yaml")); errors.Is(err, os.ErrNotExist) {
			continue
		}

		slog.Info("start checking task", "task", entry.Name())

		task, err := pkgtask.Load(entry.Name())
		if err != nil {
			slog.Error("failed to load task", "task", task.Dir, "error", err)
			os.Exit(1)
		}

		if err := Check(task); err != nil {
			slog.Error("found error", "task", task.Dir, "error", err)
			os.Exit(1)
		}
	}
}

func Check(task *pkgtask.Task) error {
	inList, outList, err := task.ListTestcases()
	if err != nil {
		slog.Error("failed to load testcases", "error", err)
		os.Exit(1)
	}
	if !isEqualSliceSet(inList, outList) {
		slog.Error("the set of in/ is not equal to the set of out/")
		os.Exit(1)
	}

	if err := task.CompileVerifier(); err != nil {
		return err
	}
	if err := task.CompileCorrect(); err != nil {
		return err
	}
	for _, testcase := range task.Config.Testcases {
		if err := task.VerifyTestcase(testcase.Name); err != nil {
			return err
		}
	}

	return nil
}

func isEqualSliceSet[T comparable](a, b []T) bool {
	transform := func(e T) (T, struct{}) { return e, struct{}{} }
	aSet, bSet := lo.Associate(a, transform), lo.Associate(b, transform)
	if len(aSet) != len(bSet) {
		return false
	}
	for k := range aSet {
		if _, ok := bSet[k]; !ok {
			return false
		}
	}
	return true
}
