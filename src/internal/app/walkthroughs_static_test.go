package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkthroughsJS_PopoutGuideContract(t *testing.T) {
	t.Helper()

	candidates := []string{
		filepath.Join("web", "static", "walkthroughs.js"),
		filepath.Join("..", "web", "static", "walkthroughs.js"),
		filepath.Join("..", "..", "web", "static", "walkthroughs.js"),
	}
	var (
		raw []byte
		err error
	)
	for _, path := range candidates {
		raw, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("unable to read walkthroughs.js from %v: %v", candidates, err)
	}
	src := string(raw)

	required := []string{
		"function renderGuidePopupFromParent() {",
		"li.addEventListener('click', () => {",
		"prevBtn.addEventListener('click', () => {",
		"nextBtn.addEventListener('click', () => {",
		"closeBtn.addEventListener('click', () => {",
	}
	for _, snippet := range required {
		if !strings.Contains(src, snippet) {
			t.Fatalf("missing popout guide contract snippet: %q", snippet)
		}
	}
}
