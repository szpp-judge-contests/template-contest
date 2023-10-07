package contest

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Contest struct {
	Scores []int `yaml:"scores"`
}

func Load(path string) (*Contest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var contest Contest
	if err := yaml.NewDecoder(f).Decode(&contest); err != nil {
		return nil, err
	}
	return &contest, nil
}
