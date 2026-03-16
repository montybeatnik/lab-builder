package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Listen  string
	BaseDir string
}

func DefaultConfig() Config {
	return Config{
		Listen:  ":8080",
		BaseDir: "/home/ubuntu/lab",
	}
}

func (c Config) SanitizeLabPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("lab file required")
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(c.BaseDir, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	baseAbs, _ := filepath.Abs(c.BaseDir)
	if abs != baseAbs && !strings.HasPrefix(abs, baseAbs+string(os.PathSeparator)) {
		return "", errors.New("lab file must be under basedir")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", errors.New("lab file not found")
	}
	if info.IsDir() {
		abs = filepath.Join(abs, "lab.clab.yml")
		info, err = os.Stat(abs)
		if err != nil || info.IsDir() {
			return "", errors.New("lab file not found")
		}
	}
	return abs, nil
}
