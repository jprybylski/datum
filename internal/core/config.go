package core

import (
    "os"

    "gopkg.in/yaml.v3"
    "example.com/pinup/internal/registry"
)

type Config struct {
    Version  int        `yaml:"version"`
    Defaults Defaults   `yaml:"defaults"`
    Datasets []Dataset  `yaml:"datasets"`
}

type Defaults struct {
    Policy string `yaml:"policy"` // fail | update | log
    Algo   string `yaml:"algo"`   // sha256
}

type Dataset struct {
    ID     string           `yaml:"id"`
    Desc   string           `yaml:"desc"`
    Target string           `yaml:"target"`
    Policy string           `yaml:"policy"`
    Source registry.Source  `yaml:"source"`
}

func readConfig(path string) (*Config, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var c Config
    if err := yaml.Unmarshal(b, &c); err != nil {
        return nil, err
    }
    if c.Defaults.Policy == "" {
        c.Defaults.Policy = "fail"
    }
    if c.Defaults.Algo == "" {
        c.Defaults.Algo = "sha256"
    }
    return &c, nil
}
