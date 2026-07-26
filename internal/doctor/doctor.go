// Package doctor checks whether the local PDF publishing toolchain is ready
// before a manuscript is rendered.
package doctor

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/aaronshaf/bookset/internal/style"
	"github.com/aaronshaf/bookset/internal/typst"
)

const RequiredTypstVersion = "0.15.1"

type Status string

const (
	Pass Status = "pass"
	Fail Status = "fail"
)

type Check struct {
	Name    string
	Status  Status
	Message string
}

type Report struct {
	Checks []Check
}

func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if check.Status == Fail {
			return false
		}
	}
	return true
}

type runner interface {
	lookPath(string) (string, error)
	output(string, ...string) ([]byte, error)
}

type systemRunner struct{}

func (systemRunner) lookPath(name string) (string, error) { return exec.LookPath(name) }
func (systemRunner) output(path string, args ...string) ([]byte, error) {
	return exec.Command(path, args...).CombinedOutput()
}

// CheckPDF verifies Typst, the Poppler tools used for PDF validation, and the
// font families selected by cfg. It is intentionally read-only.
func CheckPDF(cfg style.Config) Report {
	return checkPDF(systemRunner{}, cfg)
}

func checkPDF(commands runner, cfg style.Config) Report {
	report := Report{}
	typstPath, err := commands.lookPath("typst")
	if err != nil {
		report.add("typst", Fail, "Typst is not on PATH")
	} else if output, versionErr := commands.output(typstPath, "--version"); versionErr != nil {
		report.add("typst", Fail, "could not run Typst --version")
	} else if version := typstVersion(string(output)); version != RequiredTypstVersion {
		report.add("typst", Fail, fmt.Sprintf("requires Typst %s, found %q", RequiredTypstVersion, version))
	} else {
		report.add("typst", Pass, fmt.Sprintf("Typst %s", RequiredTypstVersion))
	}

	for _, name := range []string{"pdftotext", "pdfinfo", "pdffonts"} {
		path, lookupErr := commands.lookPath(name)
		if lookupErr != nil {
			report.add(name, Fail, name+" is not on PATH")
			continue
		}
		if _, versionErr := commands.output(path, "-v"); versionErr != nil {
			report.add(name, Fail, "could not run "+name+" -v")
			continue
		}
		report.add(name, Pass, name+" is available")
	}

	if typstPath == "" {
		return report
	}
	output, fontErr := commands.output(typstPath, typst.FontListArgs(cfg.FontDir)...)
	if fontErr != nil {
		report.add("fonts", Fail, "could not list fonts available to Typst")
		return report
	}
	available := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		if family := strings.TrimSpace(line); family != "" {
			available[family] = true
		}
	}
	missing := make([]string, 0)
	for _, family := range []string{cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont} {
		if family != "" && !available[family] {
			missing = append(missing, family)
		}
	}
	missing = uniqueSorted(missing)
	if len(missing) > 0 {
		report.add("fonts", Fail, "Typst cannot resolve configured font families: "+strings.Join(missing, ", "))
	} else {
		report.add("fonts", Pass, "Typst resolves configured font families: "+strings.Join(uniqueSorted([]string{cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont}), ", "))
	}
	return report
}

func (r *Report) add(name string, status Status, message string) {
	r.Checks = append(r.Checks, Check{Name: name, Status: status, Message: message})
}

func typstVersion(output string) string {
	for _, field := range strings.Fields(output) {
		if strings.Count(field, ".") >= 2 && field[0] >= '0' && field[0] <= '9' {
			return strings.TrimSpace(field)
		}
	}
	return "unknown"
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
