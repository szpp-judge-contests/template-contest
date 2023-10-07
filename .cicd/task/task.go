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
	judgev1 "github.com/szpp-dev-team/szpp-judge/proto-gen/go/judge/v1"
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
	Checker      *Checker                `yaml:"checker"`
	Verifier     string                  `yaml:"verifier"`
	Correct      string                  `yaml:"correct"`
	TimeLimit    int                     `yaml:"time_limit"`
	MemoryLimit  int                     `yaml:"memory_limit"`
	Difficulty   string                  `yaml:"difficulty"`
	TestcaseSets map[string]*TestcaseSet `yaml:"testcase_sets"`
	Testcases    []*Testcase             `yaml:"testcases"`
}

type Checker struct {
	Type    string         `yaml:"type"`
	Options map[string]any `yaml:"options"`
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

func (t *Task) JudgeType() (*judgev1.JudgeType, error) {
	var judgeType *judgev1.JudgeType
	switch t.Config.Checker.Type {
	case "normal":
		caseInsensitive := t.Config.Checker.Options["case_insensitive"].(bool)
		judgeType = &judgev1.JudgeType{
			JudgeType: &judgev1.JudgeType_Normal{
				Normal: &judgev1.JudgeTypeNormal{
					CaseInsensitive: &caseInsensitive,
				},
			},
		}
	case "eps":
		ndigits := uint32(t.Config.Checker.Options["ndigits"].(int))
		judgeType = &judgev1.JudgeType{
			JudgeType: &judgev1.JudgeType_Eps{
				Eps: &judgev1.JudgeTypeEPS{
					Ndigits: ndigits,
				},
			},
		}
	case "custom":
		judgeType = &judgev1.JudgeType{
			JudgeType: &judgev1.JudgeType_Custom{
				Custom: &judgev1.JudgeTypeCustom{
					JudgeCodePath: t.Config.Checker.Options["judge_code_path"].(string),
				},
			},
		}
	case "interactive":
		judgeType = &judgev1.JudgeType{
			JudgeType: &judgev1.JudgeType_Interactive{
				Interactive: &judgev1.JudgeTypeInteractive{
					JudgeCodePath: t.Config.Checker.Options["judge_code_path"].(string),
				},
			},
		}
	default:
		return nil, errors.New("unknown checker type")
	}
	return judgeType, nil
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
