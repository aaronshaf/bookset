package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/aaronshaf/bookset/internal/style"
)

type fakeRunner struct {
	paths   map[string]string
	outputs map[string]string
}

func (f fakeRunner) lookPath(name string) (string, error) {
	if path := f.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (f fakeRunner) output(path string, args ...string) ([]byte, error) {
	key := path + " " + strings.Join(args, " ")
	if output, ok := f.outputs[key]; ok {
		return []byte(output), nil
	}
	return nil, errors.New("failed")
}

func TestCheckPDFReportsReadyToolchain(t *testing.T) {
	runner := fakeRunner{
		paths: map[string]string{"typst": "/bin/typst", "pdftotext": "/bin/pdftotext", "pdfinfo": "/bin/pdfinfo", "pdffonts": "/bin/pdffonts"},
		outputs: map[string]string{
			"/bin/typst --version": "typst 0.15.1 (abc)\n",
			"/bin/typst fonts":     "Source Serif 4\nSource Sans 3\n",
			"/bin/pdftotext -v":    "24.02.0\n",
			"/bin/pdfinfo -v":      "24.02.0\n",
			"/bin/pdffonts -v":     "24.02.0\n",
		},
	}
	cfg := style.Trade("en")
	report := checkPDF(runner, cfg)
	if !report.Healthy() || len(report.Checks) != 5 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestCheckPDFUsesConfiguredFontDir(t *testing.T) {
	runner := fakeRunner{
		paths: map[string]string{"typst": "/bin/typst", "pdftotext": "/bin/pdftotext", "pdfinfo": "/bin/pdfinfo", "pdffonts": "/bin/pdffonts"},
		outputs: map[string]string{
			"/bin/typst --version":                      "typst 0.15.1\n",
			"/bin/typst fonts --font-path vendor/fonts": "Vendored Serif\nVendored Sans\n",
			"/bin/pdftotext -v":                         "24.02.0\n",
			"/bin/pdfinfo -v":                           "24.02.0\n",
			"/bin/pdffonts -v":                          "24.02.0\n",
		},
	}
	cfg := style.Trade("en")
	cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont = "Vendored Serif", "Vendored Serif", "Vendored Sans"
	cfg.FontDir = "vendor/fonts"
	if report := checkPDF(runner, cfg); !report.Healthy() {
		t.Fatalf("doctor did not use configured font path: %#v", report)
	}
}

func TestCheckPDFReportsEveryProblem(t *testing.T) {
	runner := fakeRunner{
		paths: map[string]string{"typst": "/bin/typst", "pdfinfo": "/bin/pdfinfo"},
		outputs: map[string]string{
			"/bin/typst --version": "typst 0.14.0\n",
			"/bin/typst fonts":     "Source Serif 4\n",
			"/bin/pdfinfo -v":      "24.02.0\n",
		},
	}
	report := checkPDF(runner, style.Trade("en"))
	if report.Healthy() {
		t.Fatal("broken toolchain was reported healthy")
	}
	var messages []string
	for _, check := range report.Checks {
		if check.Status == Fail {
			messages = append(messages, check.Message)
		}
	}
	joined := strings.Join(messages, "\n")
	for _, want := range []string{"requires Typst", "pdftotext is not", "pdffonts is not", "Source Sans 3"} {
		if !strings.Contains(joined, want) {
			t.Errorf("report missing %q: %s", want, joined)
		}
	}
}
