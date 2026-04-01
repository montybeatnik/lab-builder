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
		"channel.postMessage({ type: 'requestState' });",
		"if (msg.type === 'state' && msg.payload) {",
		"state.steps = Array.isArray(msg.payload.steps) ? msg.payload.steps : [];",
		"byId('closeBtn').addEventListener('click', () => window.close());",
	}
	for _, snippet := range required {
		if !strings.Contains(src, snippet) {
			t.Fatalf("missing popout guide contract snippet: %q", snippet)
		}
	}
}

