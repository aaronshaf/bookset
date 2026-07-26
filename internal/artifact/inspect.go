package artifact

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aaronshaf/bookset/internal/epub"
	"github.com/aaronshaf/bookset/internal/markdown"
)

// Inspection is a stable, machine-readable artifact report. The schema field
// lets agents and CI reject unknown report versions instead of guessing.
type Inspection struct {
	Schema   string            `json:"schema"`
	Status   string            `json:"status"`
	Artifact ArtifactInfo      `json:"artifact"`
	Source   *SourceInfo       `json:"source,omitempty"`
	Checks   []InspectionCheck `json:"checks"`
	Issues   []InspectionIssue `json:"issues"`
}

type SourceInfo struct {
	Path          string   `json:"path"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Language      string   `json:"language"`
	Chapters      int      `json:"chapters"`
	Footnotes     int      `json:"footnotes"`
	ChapterPaths  []string `json:"chapter_paths,omitempty"`
	ChapterTitles []string `json:"chapter_titles,omitempty"`
}

type ArtifactInfo struct {
	Path      string    `json:"path"`
	Format    string    `json:"format"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	PDF       *PDFInfo  `json:"pdf,omitempty"`
	EPUB      *EPUBInfo `json:"epub,omitempty"`
}

type PDFInfo struct {
	Pages    int      `json:"pages,omitempty"`
	PageSize string   `json:"page_size,omitempty"`
	Fonts    []string `json:"fonts,omitempty"`
}

type EPUBInfo struct {
	Entries      []string `json:"entries"`
	ChapterFiles []string `json:"chapter_files"`
}

type InspectionCheck struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

type InspectionIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Chapter  int    `json:"chapter,omitempty"`
}

// InspectArtifact reports artifact facts and available validation checks.
// Missing optional inspection tools are warnings; malformed artifacts and
// invalid required structure are errors.
func InspectArtifact(path string) (Inspection, error) {
	report := Inspection{Schema: "bookset.artifact-inspection.v1", Status: "ok", Issues: []InspectionIssue{}}
	info, err := os.Stat(path)
	if err != nil {
		return report, err
	}
	report.Artifact.Path = filepath.Clean(path)
	report.Artifact.SizeBytes = info.Size()
	if digest, digestErr := fileSHA256(path); digestErr == nil {
		report.Artifact.SHA256 = digest
	} else {
		return report, digestErr
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		report.Artifact.Format = "pdf"
		report.Artifact.PDF = &PDFInfo{}
		inspectPDF(&report)
	case ".epub":
		report.Artifact.Format = "epub"
		report.Artifact.EPUB = &EPUBInfo{}
		inspectEPUB(&report)
	default:
		addCheck(&report, "artifact.format", "error", "unsupported", fmt.Sprintf("unsupported artifact format: %s", filepath.Ext(path)))
	}
	if report.Status == "ok" {
		for _, check := range report.Checks {
			if check.Severity == "warning" {
				report.Status = "warning"
				break
			}
		}
	}
	return report, nil
}

// InspectArtifactAgainst adds source-to-artifact fidelity checks to the
// structural report. It is the preferred entry point for publishability gates.
func InspectArtifactAgainst(path string, docs []*markdown.Document) (Inspection, error) {
	report, err := InspectArtifact(path)
	if err != nil {
		return report, err
	}
	for chapter, doc := range docs {
		for _, issue := range Validate(path, doc) {
			report.Issues = append(report.Issues, InspectionIssue{
				Code: issueCode(issue.Error()), Severity: "error", Message: issue.Error(), Chapter: chapter + 1,
			})
		}
	}
	if len(report.Issues) > 0 {
		report.Status = "error"
	}
	return report, nil
}

func issueCode(message string) string {
	message = strings.ToLower(message)
	switch {
	case strings.Contains(message, "italic"):
		return "fidelity.italic"
	case strings.Contains(message, "bold"):
		return "fidelity.bold"
	case strings.Contains(message, "footnote"):
		return "fidelity.footnote"
	case strings.Contains(message, "manuscript text") || strings.Contains(message, "manuscript body"):
		return "fidelity.text"
	case strings.Contains(message, "could not inspect") || strings.Contains(message, "could not extract"):
		return "inspection.tool-unavailable"
	default:
		return "fidelity.artifact"
	}
}

func inspectPDF(report *Inspection) {
	path := report.Artifact.Path
	info, err := commandOutput("pdfinfo", path)
	if err != nil {
		addCheck(report, "pdf.pdfinfo", "warning", "unavailable", err.Error())
	} else {
		report.Artifact.PDF.Pages = parseInfoInt(info, "Pages")
		report.Artifact.PDF.PageSize = normalizePageSize(parseInfoValue(info, "Page size"))
		if report.Artifact.PDF.Pages == 0 {
			addCheck(report, "pdf.pages", "error", "missing", "PDF metadata did not report a page count")
		} else {
			addCheck(report, "pdf.pages", "info", "pass", fmt.Sprintf("PDF contains %d pages", report.Artifact.PDF.Pages))
		}
		if report.Artifact.PDF.PageSize != "432 x 648 pts" && report.Artifact.PDF.PageSize != "612 x 792 pts" {
			addCheck(report, "pdf.trim", "error", "invalid", fmt.Sprintf("unsupported PDF page size %q", report.Artifact.PDF.PageSize))
		} else {
			addCheck(report, "pdf.trim", "info", "pass", "PDF page size is a supported trim or proof sheet")
		}
	}
	fonts, err := commandOutput("pdffonts", path)
	if err != nil {
		addCheck(report, "pdf.fonts", "warning", "unavailable", err.Error())
	} else {
		report.Artifact.PDF.Fonts = parsePDFFonts(fonts)
		if len(report.Artifact.PDF.Fonts) == 0 {
			addCheck(report, "pdf.fonts", "error", "missing", "PDF contains no embedded font records")
		} else {
			addCheck(report, "pdf.fonts", "info", "pass", fmt.Sprintf("PDF contains %d font records", len(report.Artifact.PDF.Fonts)))
		}
	}
}

func inspectEPUB(report *Inspection) {
	path := report.Artifact.Path
	if err := epub.Validate(path); err != nil {
		addCheck(report, "epub.structure", "error", "invalid", err.Error())
		return
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		addCheck(report, "epub.zip", "error", "invalid", err.Error())
		return
	}
	defer r.Close()
	for _, file := range r.File {
		report.Artifact.EPUB.Entries = append(report.Artifact.EPUB.Entries, file.Name)
		if strings.HasPrefix(file.Name, "OEBPS/content") && strings.HasSuffix(file.Name, ".xhtml") {
			report.Artifact.EPUB.ChapterFiles = append(report.Artifact.EPUB.ChapterFiles, file.Name)
		}
	}
	sort.Strings(report.Artifact.EPUB.Entries)
	sort.Strings(report.Artifact.EPUB.ChapterFiles)
	addCheck(report, "epub.structure", "info", "pass", "EPUB ZIP, container, XML, navigation, and package structure are valid")
	addCheck(report, "epub.chapters", "info", "pass", fmt.Sprintf("EPUB contains %d chapter XHTML documents", len(report.Artifact.EPUB.ChapterFiles)))
}

func addCheck(report *Inspection, code, severity, status, message string) {
	report.Checks = append(report.Checks, InspectionCheck{Code: code, Severity: severity, Status: status, Message: message})
	if severity == "error" {
		report.Status = "error"
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func parseInfoValue(info, key string) string {
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+":"))
		}
	}
	return ""
}

func parseInfoInt(info, key string) int {
	value, _ := strconv.Atoi(parseInfoValue(info, key))
	return value
}

func normalizePageSize(value string) string {
	fields := strings.Fields(value)
	if len(fields) >= 3 && fields[2] == "pts" {
		return strings.Join(fields[:3], " ")
	}
	return value
}

func parsePDFFonts(output string) []string {
	var fonts []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] != "name" && !strings.HasPrefix(fields[0], "-") {
			fonts = append(fonts, fields[0])
		}
	}
	sort.Strings(fonts)
	return fonts
}
