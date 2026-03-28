package app

import (
	"fmt"
	"os"
	"path/filepath"
)

func resolveAssetPath(path string) (string, error) {
	candidates := []string{
		path,
		filepath.Join("src", path),
	}
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err != nil || !info.IsDir() {
			continue
		}
		return c, nil
	}
	return "", fmt.Errorf("asset path not found: %s (tried %q)", path, candidates)
}

