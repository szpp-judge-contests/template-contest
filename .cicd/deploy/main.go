package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iancoleman/strcase"
	backendv1 "github.com/szpp-dev-team/szpp-judge/proto-gen/go/backend/v1"
	judgev1 "github.com/szpp-dev-team/szpp-judge/proto-gen/go/judge/v1"
	pkgtask "github.com/szpp-judge-contests/template-contest/task"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	systemRoots, err := x509.SystemCertPool()
	if err != nil {
		panic(err)
	}
	conn, err := grpc.Dial(os.Getenv("BACKEND_GRPC_ADDR"), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		RootCAs: systemRoots,
	})))
	defer conn.Close()
	cli := backendv1.NewTaskServiceClient(conn)

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
		task, err := pkgtask.Load(entry.Name())
		if err != nil {
			panic(err)
		}

		testcaseSets := make([]*backendv1.MutationTestcaseSet, 0)
		for name, testcaseSet := range task.Config.TestcaseSets {
			testcaseSets = append(testcaseSets, &backendv1.MutationTestcaseSet{
				Slug:          name,
				ScoreRatio:    int32(testcaseSet.ScoreRatio),
				IsSample:      testcaseSet.IsSample,
				TestcaseSlugs: testcaseSet.List,
			})
		}

		testcases := make([]*backendv1.MutationTestcase, 0)
		for _, testcase := range task.Config.Testcases {
			in, err := os.ReadFile(filepath.Join(task.TestcaseInPath, testcase.Name))
			if err != nil {
				panic(err)
			}
			out, err := os.ReadFile(filepath.Join(task.TestcaseOutPath, testcase.Name))
			if err != nil {
				panic(err)
			}
			testcases = append(testcases, &backendv1.MutationTestcase{
				Slug:        testcase.Name,
				Description: testcase.Description,
				Input:       string(in),
				Output:      string(out),
			})
		}

		mutationTask := &backendv1.MutationTask{
			Title:           task.Config.Title,
			Statement:       string(task.Statement),
			ExecTimeLimit:   int32(task.Config.TimeLimit),
			ExecMemoryLimit: int32(task.Config.MemoryLimit),
			JudgeType:       &judgev1.JudgeType{},
			Difficulty:      backendv1.Difficulty(backendv1.Difficulty_value[strcase.ToScreamingSnake(task.Config.Difficulty)]),
		}
		taskIDkey := fmt.Sprintf("%s_TASK_ID", strings.ToUpper(entry.Name()))
		if taskID, err := strconv.Atoi(os.Getenv(taskIDkey)); err != nil {
			resp, err := cli.CreateTask(context.Background(), &backendv1.CreateTaskRequest{
				Task: mutationTask,
			})
			if err != nil {
				panic(err)
			}
			slog.Info("created", "id", resp.Task.Id)
			os.Setenv("GITHUB_ENV", fmt.Sprintf("%s\n%s=%d", os.Getenv("GITHUB_ENV"), taskIDkey, resp.Task.Id))
		} else {
			resp, err := cli.UpdateTask(context.Background(), &backendv1.UpdateTaskRequest{
				TaskId: int32(taskID),
				Task:   mutationTask,
			})
			if err != nil {
				panic(err)
			}
			slog.Info("updated", "id", resp.Task.Id)
		}
	}
}
