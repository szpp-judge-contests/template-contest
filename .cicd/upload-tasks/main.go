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
	"github.com/samber/lo"
	backendv1 "github.com/szpp-dev-team/szpp-judge/proto-gen/go/backend/v1"
	"github.com/szpp-judge-contests/template-contest/contest"
	pkgtask "github.com/szpp-judge-contests/template-contest/task"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func main() {
	ctx := context.Background()

	systemRoots, err := x509.SystemCertPool()
	if err != nil {
		panic(err)
	}
	conn, err := grpc.Dial(os.Getenv("BACKEND_GRPC_ADDR"), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		RootCAs: systemRoots,
	})))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	authCli := backendv1.NewAuthServiceClient(conn)
	loginResp, err := authCli.Login(ctx, &backendv1.LoginRequest{
		Username: os.Getenv("BACKEND_USERNAME"),
		Password: os.Getenv("BACKEND_PASSWORD"),
	})
	if err != nil {
		panic(err)
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("Authorization", loginResp.AccessToken))

	cli := backendv1.NewTaskServiceClient(conn)
	contestCli := backendv1.NewContestServiceClient(conn)

	rootDir := os.Getenv("TASKS_ROOT")
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		panic(err)
	}

	tasks := make([]*backendv1.Task, 0)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".example" {
			continue
		}
		taskPath := filepath.Join(rootDir, entry.Name())
		if _, err := os.Stat(filepath.Join(taskPath, "task.yaml")); errors.Is(err, os.ErrNotExist) {
			continue
		}
		task, err := pkgtask.Load(taskPath)
		if err != nil {
			panic(err)
		}
		if err := task.Check(); err != nil {
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
			in, err := os.ReadFile(filepath.Join(task.TestcaseInPath, testcase.Name+".txt"))
			if err != nil {
				panic(err)
			}
			out, err := os.ReadFile(filepath.Join(task.TestcaseOutPath, testcase.Name+".txt"))
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

		checker, err := os.ReadFile(filepath.Join(taskPath, task.Config.Checker))
		if err != nil {
			panic(err)
		}

		mutationTask := &backendv1.MutationTask{
			Title:           task.Config.Title,
			Statement:       string(task.Statement),
			ExecTimeLimit:   int32(task.Config.TimeLimit),
			ExecMemoryLimit: int32(task.Config.MemoryLimit),
			JudgeType:       nil,
			Difficulty:      backendv1.Difficulty(backendv1.Difficulty_value[strcase.ToScreamingSnake(task.Config.Difficulty)]),
			Checker:         string(checker),
		}
		taskIDkey := fmt.Sprintf("%s_TASK_ID", strings.ToUpper(entry.Name()))
		taskID, err := strconv.Atoi(os.Getenv(taskIDkey))
		if err != nil {
			resp, err := cli.CreateTask(ctx, &backendv1.CreateTaskRequest{
				Task: mutationTask,
			})
			if err != nil {
				panic(err)
			}
			taskID = int(resp.Task.Id)
			tasks = append(tasks, resp.Task)
			slog.Info("created", "id", resp.Task.Id)
			os.Setenv("GITHUB_ENV", fmt.Sprintf("%s\n%s=%d", os.Getenv("GITHUB_ENV"), taskIDkey, resp.Task.Id))
		} else {
			resp, err := cli.UpdateTask(ctx, &backendv1.UpdateTaskRequest{
				TaskId: int32(taskID),
				Task:   mutationTask,
			})
			if err != nil {
				panic(err)
			}
			tasks = append(tasks, resp.Task)
			slog.Info("updated", "id", resp.Task.Id)
		}

		// テストケースを sync する
		_, err = cli.SyncTestcaseSets(ctx, &backendv1.SyncTestcaseSetsRequest{
			TaskId:       int32(taskID),
			TestcaseSets: testcaseSets,
			Testcases:    testcases,
		})
		if err != nil {
			panic(err)
		}
		slog.Info("synced", "task_id", taskID)
	}

	c, err := contest.Load(filepath.Join(rootDir, "contest.yaml"))
	if err != nil {
		panic(err)
	}

	// コンテストと紐付けする
	resp, err := contestCli.SyncContestTasks(ctx, &backendv1.SyncContestTasksRequest{
		ContestSlug: os.Getenv("CONTEST_SLUG"),
		Tasks: lo.Map(tasks, func(task *backendv1.Task, i int) *backendv1.SyncContestTasksRequest_Task {
			return &backendv1.SyncContestTasksRequest_Task{
				Id:    task.Id,
				Score: int32(c.Scores[i]),
			}
		}),
	})
	if err != nil {
		panic(err)
	}
	slog.Info("contest tasks were synced", slog.Any("resp", resp))
}
