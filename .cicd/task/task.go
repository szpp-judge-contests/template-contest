package task

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	executableChecker  string
	executableWrong    *string
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
	var executableWrong *string
	if c.Wrong != nil {
		executableWrong = lo.ToPtr(basename(*c.Wrong))
	}
	return &Task{
		Dir:                path,
		TestcaseInPath:     filepath.Join(path, "testcases", "in"),
		TestcaseOutPath:    filepath.Join(path, "testcases", "out"),
		Config:             &c,
		Statement:          b,
		executableVerifier: basename(c.Verifier),
		executableCorrect:  basename(c.Correct),
		executableChecker:  basename(c.Checker),
		executableWrong:    executableWrong,
	}, nil
}

type TaskConfig struct {
	Title        string                  `yaml:"title"`
	Writer       string                  `yaml:"writer"`
	Checker      string                  `yaml:"checker"`
	Verifier     string                  `yaml:"verifier"`
	Correct      string                  `yaml:"correct"`
	Wrong        *string                 `yaml:"wrong"`
	TimeLimit    int                     `yaml:"time_limit"`
	MemoryLimit  int                     `yaml:"memory_limit"`
	Difficulty   string                  `yaml:"difficulty"`
	TestcaseSets map[string]*TestcaseSet `yaml:"testcase_sets"`
	Testcases    []*Testcase             `yaml:"testcases"`
}

type TestcaseSet struct {
	ScoreRatio int      `yaml:"score_ratio"`
	List       []string `yaml:"list"`
	IsSample   bool     `yaml:"is_sample"`
}

type Testcase struct {
	Name        string  `yaml:"name"`
	Description *string `yaml:"description"`
}

func (t *Task) Check() error {
	inList, outList, err := t.ListTestcases()
	if err != nil {
		slog.Error("failed to load testcases", "error", err)
		return err
	}
	if !isEqualSliceSet(inList, outList) {
		slog.Error("the set of in/ is not equal to the set of out/")
		return err
	}

	if err := t.CompileVerifier(); err != nil {
		return err
	}
	if err := t.CompileCorrect(); err != nil {
		return err
	}
	if err := t.CompileChecker(); err != nil {
		return err
	}
	if t.Config.Wrong != nil {
		if err := t.CompileWrong(); err != nil {
			return err
		}
	}
	for _, testcase := range t.Config.Testcases {
		if err := t.VerifyTestcase(testcase.Name); err != nil {
			return err
		}
	}
	if err := t.VerifyTestcaseSets(); err != nil {
		return err
	}

	return nil
}

func (t *Task) CompileVerifier() error {
	slog.Info("compiling verifier", "task", t.Dir)
	if _, err := executeCommand("g++", []string{"-O2", "-std=gnu++17", "-o", t.executableVerifier, "-I", "..", t.Config.Verifier}, withWorkdir(t.Dir)); err != nil {
		slog.Error("failed to compile", "name", t.Config.Verifier, "error", err)
		return err
	}
	return nil
}

func (t *Task) CompileCorrect() error {
	slog.Info("compiling correct", "task", t.Dir)
	if _, err := executeCommand("g++", []string{"-O2", "-std=gnu++17", "-o", t.executableCorrect, t.Config.Correct}, withWorkdir(t.Dir)); err != nil {
		slog.Error("failed to compile", "name", t.Config.Correct, "error", err)
		return err
	}
	return nil
}

func (t *Task) CompileChecker() error {
	slog.Info("compiling checker", "task", t.Dir)
	if _, err := executeCommand("g++", []string{"-O2", "-std=gnu++17", "-o", t.executableChecker, "-I", "..", t.Config.Checker}, withWorkdir(t.Dir)); err != nil {
		slog.Error("failed to compile", "name", t.Config.Checker, "error", err)
		return err
	}
	return nil
}

func (t *Task) CompileWrong() error {
	slog.Info("compiling wrong", "task", t.Dir)
	if _, err := executeCommand("g++", []string{"-O2", "-std=gnu++17", "-o", *t.executableWrong, "-I", "..", *t.Config.Wrong}, withWorkdir(t.Dir)); err != nil {
		slog.Error("failed to compile", "name", t.Config.Wrong, "error", err)
		return err
	}
	return nil
}

func (t *Task) VerifyTestcaseSets() error {
	scoreRatioSum := 0
	for _, ts := range t.Config.TestcaseSets {
		scoreRatioSum += ts.ScoreRatio
	}
	if scoreRatioSum != 100 {
		return fmt.Errorf("score ratio sum is not 100: %d", scoreRatioSum)
	}
	return nil
}

func (t *Task) VerifyTestcase(testcase string) error {
	slog.Info("verifying testcase", "task", t.Dir, "testcase", testcase)

	inPath := filepath.Join(t.TestcaseInPath, testcase+".txt")
	outPath := filepath.Join(t.TestcaseOutPath, testcase+".txt")

	b, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}

	ok, err := executeCommand(fmt.Sprintf("./%s", t.executableVerifier), []string{}, withWorkdir(t.Dir), withStdin(bytes.NewReader(b)))
	if err != nil {
		return err
	}
	if !ok {
		slog.Error("failed to verify testcase", "task", t.Dir, "testcase", testcase)
		return errors.New("verification failed")
	}

	ok, err = t.executeAndCheckCode(t.executableCorrect, inPath, outPath)
	if err != nil {
		return err
	}
	if !ok {
		slog.Error("failed to check testcase(correct pattern)", "task", t.Dir, "testcase", testcase)
		return errors.New("verification failed")
	}

	if t.executableWrong != nil {
		ok, err := t.executeAndCheckCode(*t.executableWrong, inPath, outPath)
		if err != nil {
			return err
		}
		if ok {
			slog.Error("failed to check testcase(wrong pattern)", "task", t.Dir, "testcase", testcase)
			return errors.New("verification failed")
		}
	}

	return nil
}

func (t *Task) executeAndCheckCode(executable, inPath, outPath string) (bool, error) {
	b, err := os.ReadFile(inPath)
	if err != nil {
		return false, err
	}
	f, err := os.OpenFile(filepath.Join(t.Dir, "user_stdout.txt"), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return false, err
	}
	ok, err := executeCommand(fmt.Sprintf("./%s", executable), []string{}, withWorkdir(t.Dir), withStdin(bytes.NewReader(b)), withStdout(f))
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	defer f.Close()
	return executeCommand(fmt.Sprintf("./%s", t.executableChecker), []string{
		inPath,
		outPath,
		"user_stdout.txt",
	}, withWorkdir(t.Dir))
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

func executeCommand(command string, args []string, opts ...func(*exec.Cmd)) (bool, error) {
	cmd := exec.Command(command, args...)
	for _, opt := range opts {
		opt(cmd)
	}
	buf := &bytes.Buffer{}
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
