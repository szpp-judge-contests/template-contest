package task

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/samber/lo"
	yamlv3 "gopkg.in/yaml.v3"
)

type Task struct {
	Dir             string
	TestcaseInPath  string
	TestcaseOutPath string
	Config          *TaskConfig
	Statement       []byte

	executableVerifier string
	executableCorrect  string
}

func Load(path string) (*Task, error) {
	f, err := os.Open(filepath.Join(path, "task.yaml"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c TaskConfig
	if err := yamlv3.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(path, "statement.md"))
	if err != nil {
		return nil, err
	}
	return &Task{
		Dir:                path,
		TestcaseInPath:     filepath.Join(path, "testcases", "in"),
		TestcaseOutPath:    filepath.Join(path, "testcases", "out"),
		Config:             &c,
		Statement:          b,
		executableVerifier: basename(c.Verifier),
		executableCorrect:  basename(c.Correct),
	}, nil
}

type TaskConfig struct {
	Title        string                  `yaml:"title"`
	Writer       string                  `yaml:"writer"`
	Checker      string                  `yaml:"checker"`
	Verifier     string                  `yaml:"verifier"`
	Correct      string                  `yaml:"correct"`
	TimeLimit    int                     `yaml:"time_limit"`
	MemoryLimit  int                     `yaml:"memory_limit"`
	Difficulty   string                  `yaml:"difficulty"`
	TestcaseSets map[string]*TestcaseSet `yaml:"testcase_sets"`
	Testcases    []*Testcase             `yaml:"testcases"`
}

type TestcaseSet struct {
	Score int      `yaml:"score"`
	List  []string `yaml:"list"`
}

type Testcase struct {
	Name        string  `yaml:"name"`
	Description *string `yaml:"description"`
}

func (t *Task) CompileVerifier() error {
	slog.Info("compiling verifier", "task", t.Dir)
	if err := executeCommand("g++", []string{"-O2", "-std=gnu++17", "-o", t.executableVerifier, "-I", "..", t.Config.Verifier}, withWorkdir(t.Dir)); err != nil {
		slog.Error("failed to compile", "name", t.Config.Verifier, "error", err)
		return err
	}
	return nil
}

func (t *Task) CompileCorrect() error {
	slog.Info("compiling correct", "task", t.Dir)
	if err := executeCommand("g++", []string{"-O2", "-std=gnu++17", "-o", t.executableCorrect, "-I", "..", t.Config.Correct}, withWorkdir(t.Dir)); err != nil {
		slog.Error("failed to compile", "name", t.Config.Correct, "error", err)
		return err
	}
	return nil
}

func (t *Task) VerifyTestcase(testcase string) error {
	slog.Info("verifying testcase", "task", t.Dir, "testcase", testcase)
	in, err := os.Open(filepath.Join(t.TestcaseInPath, testcase+".txt"))
	if err != nil {
		return err
	}
	defer in.Close()
	if err := executeCommand(fmt.Sprintf("./%s", t.executableVerifier), []string{}, withWorkdir(t.Dir), withStdin(in)); err != nil {
		slog.Error("failed to verify testcase", "task", t.Dir, "testcase", testcase, "error", err)
		return err
	}
	f, err := os.OpenFile("user_stdout.txt", os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := executeCommand(fmt.Sprintf("./%s", t.executableCorrect), []string{}, withWorkdir(t.Dir), withStdin(in), withStdout(f)); err != nil {
		slog.Error("failed to execute correct", "task", t.Dir, "testcase", testcase, "error", err)
		return err
	}

	// TODO: check output

	return nil
}

func (t *Task) ListTestcases() ([]string, []string, error) {
	getFilename := func(e os.DirEntry, _ int) string { return e.Name() }
	inEntries, err := os.ReadDir(t.TestcaseInPath)
	if err != nil {
		return nil, nil, err
	}
	outEntries, err := os.ReadDir(t.TestcaseInPath)
	if err != nil {
		return nil, nil, err
	}
	return lo.Map(inEntries, getFilename), lo.Map(outEntries, getFilename), nil
}

func basename(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

func withWorkdir(dir string) func(*exec.Cmd) {
	return func(c *exec.Cmd) {
		c.Dir = dir
	}
}

func withStdin(r io.Reader) func(*exec.Cmd) {
	return func(c *exec.Cmd) {
		c.Stdin = r
	}
}

func withStdout(w io.Writer) func(*exec.Cmd) {
	return func(c *exec.Cmd) {
		c.Stdout = w
	}
}

func executeCommand(command string, args []string, opts ...func(*exec.Cmd)) error {
	cmd := exec.Command(command, args...)
	for _, opt := range opts {
		opt(cmd)
	}
	buf := &bytes.Buffer{}
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		log.Println(err)
		if _, ok := err.(*exec.ExitError); ok {
			return errors.New(buf.String())
		}
		return err
	}
	return nil
}
