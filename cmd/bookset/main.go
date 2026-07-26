package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaronshaf/bookset/internal/artifact"
	"github.com/aaronshaf/bookset/internal/book"
	"github.com/aaronshaf/bookset/internal/config"
	"github.com/aaronshaf/bookset/internal/doctor"
	"github.com/aaronshaf/bookset/internal/epub"
	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/style"
	"github.com/aaronshaf/bookset/internal/typst"
	"github.com/aaronshaf/bookset/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version.Value)
	case "doctor":
		doctorCommand(os.Args[2:])
	case "render":
		render(os.Args[2:])
	case "build":
		build(os.Args[2:])
	case "plan":
		plan(os.Args[2:])
	case "proof":
		proof(os.Args[2:])
	case "validate":
		validate(os.Args[2:])
	case "inspect":
		inspect(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func plan(args []string) {
	flags := flag.NewFlagSet("plan", flag.ExitOnError)
	configPath := flags.String("config", "", "book TOML configuration")
	jsonOutput := flags.Bool("json", false, "write a machine-readable plan")
	flags.Parse(args)
	if *configPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bookset plan --config bookset.toml [--json]")
		os.Exit(2)
	}
	project, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(2)
	}
	manuscript, err := book.Load(project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
	report, err := book.Audit(manuscript)
	if *jsonOutput {
		data, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			fmt.Fprintln(os.Stderr, "bookset:", marshalErr)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		for _, entry := range report.Entries {
			chapter := ""
			if entry.Chapter > 0 {
				chapter = fmt.Sprintf(" chapter=%d", entry.Chapter)
			}
			source := ""
			if entry.Source != "" {
				source = " source=" + entry.Source
			}
			fmt.Printf("%s kind=%s section=%s toc=%t words=%d%s%s title=%q\n", entry.ID, entry.Kind, entry.Section, entry.TOC, entry.BodyWords, chapter, source, entry.Title)
		}
		fmt.Printf("summary: documents=%d chapters=%d front=%d main=%d back=%d\n", report.Summary.Documents, report.Summary.Chapters, report.Summary.Front, report.Summary.Main, report.Summary.Back)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
}

func proof(args []string) {
	flags := flag.NewFlagSet("proof", flag.ExitOnError)
	configPath := flags.String("config", "", "book TOML configuration")
	output := flags.String("output", "", "PDF output path")
	jsonOutput := flags.Bool("json", false, "write a machine-readable proof report")
	flags.Parse(args)
	if *configPath == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bookset proof --config bookset.toml --output book.pdf [--json]")
		os.Exit(2)
	}
	project, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(2)
	}
	manuscript, err := book.Load(project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
	entries, err := typst.ProofDocuments(*output, manuscript.Documents, manuscript.Style)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
	if *jsonOutput {
		data, marshalErr := json.MarshalIndent(struct {
			Entries []typst.ProofEntry `json:"entries"`
		}{entries}, "", "  ")
		if marshalErr != nil {
			fmt.Fprintln(os.Stderr, "bookset:", marshalErr)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	for _, entry := range entries {
		fmt.Printf("%s kind=%s section=%s pages=%d-%d folio=%d title=%q\n", entry.ID, entry.Kind, entry.Section, entry.StartPage, entry.EndPage, entry.Folio, entry.Title)
	}
}

func doctorCommand(args []string) {
	flags := flag.NewFlagSet("doctor", flag.ExitOnError)
	styleName := flags.String("style", "trade", "style whose configured fonts to check")
	configPath := flags.String("config", "", "project or book TOML configuration")
	flags.Parse(args)
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bookset doctor [--style STYLE] [--config bookset.toml]")
		os.Exit(2)
	}
	cfg, err := resolveStyle(*styleName, "en")
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(2)
	}
	if *configPath != "" {
		project, configErr := config.Load(*configPath)
		if configErr != nil {
			fmt.Fprintln(os.Stderr, "bookset:", configErr)
			os.Exit(2)
		}
		if len(project.Chapters) > 0 || len(project.Contents) > 0 {
			manuscript, loadErr := book.Load(project)
			if loadErr != nil {
				fmt.Fprintln(os.Stderr, "bookset:", loadErr)
				os.Exit(2)
			}
			cfg = manuscript.Style
		} else {
			cfg, err = style.ApplyProject(cfg, project)
			if err != nil {
				fmt.Fprintln(os.Stderr, "bookset:", err)
				os.Exit(2)
			}
		}
	}
	report := doctor.CheckPDF(cfg)
	for _, check := range report.Checks {
		fmt.Printf("%s %s: %s\n", check.Status, check.Name, check.Message)
	}
	if !report.Healthy() {
		os.Exit(1)
	}
}

func render(args []string) {
	flags := flag.NewFlagSet("render", flag.ExitOnError)
	format := flags.String("format", "pdf", "output format: pdf or epub")
	output := flags.String("output", "", "output path")
	styleName := flags.String("style", "trade", "layout style: trade, classic-trade, or timeline-trade")
	configPath := flags.String("config", "", "project TOML configuration")
	sheet := flags.String("sheet", "", "proof sheet: letter (keeps the configured trim size inside the sheet)")
	trimMarks := flags.Bool("trim-marks", false, "draw crop marks around the configured trim boundary")
	typstSource := flags.String("typst-source", "", "write generated Typst source to FILE (PDF only)")
	flags.Parse(args)

	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bookset render [flags] input.md")
		os.Exit(2)
	}
	if *format != "pdf" && *format != "epub" {
		fmt.Fprintln(os.Stderr, "bookset: format must be pdf or epub")
		os.Exit(2)
	}

	if *output == "" {
		fmt.Fprintln(os.Stderr, "bookset: --output is required")
		os.Exit(2)
	}
	doc, issues := load(flags.Arg(0))
	if len(issues) > 0 {
		fail(issues)
	}
	var project config.Project
	if *configPath != "" {
		var configErr error
		project, configErr = config.Load(*configPath)
		if configErr != nil {
			fmt.Fprintln(os.Stderr, "bookset:", configErr)
			os.Exit(2)
		}
		if project.Book.Language != "" {
			doc.Language = project.Book.Language
		}
	}
	cfg, err := resolveStyle(*styleName, doc.Language)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(2)
	}
	if *configPath != "" && !project.TemplatesConfigured {
		cfg.TemplateDir = project.Templates.Dir
	}
	if *configPath != "" {
		cfg, err = style.ApplyProject(cfg, project)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bookset:", err)
			os.Exit(2)
		}
	}
	if strings.HasSuffix(*styleName, ".toml") || strings.Contains(*styleName, string(os.PathSeparator)) || *configPath != "" {
		cfg.TemplateRequired = true
		if err = style.ValidateTemplateDir(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "bookset:", err)
			os.Exit(2)
		}
	}
	if *sheet != "" {
		if *sheet != "letter" {
			fmt.Fprintln(os.Stderr, "bookset: --sheet currently supports only letter")
			os.Exit(2)
		}
		cfg.Sheet = *sheet
	}
	if *trimMarks {
		if cfg.Sheet != "letter" {
			fmt.Fprintln(os.Stderr, "bookset: --trim-marks requires --sheet letter")
			os.Exit(2)
		}
		cfg.TrimMarks = true
	}
	if *format == "epub" {
		if *typstSource != "" {
			fmt.Fprintln(os.Stderr, "bookset: --typst-source requires --format pdf")
			os.Exit(2)
		}
		err = epub.Write(*output, doc, cfg)
		if err == nil {
			err = epub.Validate(*output)
		}
	} else {
		err = typst.RenderWithOptions(*output, doc, cfg, typst.RenderOptions{SourcePath: *typstSource})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
}

func build(args []string) {
	flags := flag.NewFlagSet("build", flag.ExitOnError)
	format := flags.String("format", "pdf", "output format: pdf or epub")
	output := flags.String("output", "", "output path")
	configPath := flags.String("config", "", "book TOML configuration")
	typstSource := flags.String("typst-source", "", "write generated Typst source to FILE (PDF only)")
	flags.Parse(args)
	if *configPath == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: bookset build --config bookset.toml --format pdf|epub --output FILE")
		os.Exit(2)
	}
	if *format != "pdf" && *format != "epub" {
		fmt.Fprintln(os.Stderr, "bookset: format must be pdf or epub")
		os.Exit(2)
	}
	project, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(2)
	}
	manuscript, err := book.Load(project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
	if *format == "epub" {
		if *typstSource != "" {
			fmt.Fprintln(os.Stderr, "bookset: --typst-source requires --format pdf")
			os.Exit(2)
		}
		err = epub.WriteBook(*output, manuscript.Documents, manuscript.Style)
		if err == nil {
			err = epub.Validate(*output)
		}
	} else {
		err = typst.RenderDocumentsWithOptions(*output, manuscript.Documents, manuscript.Style, typst.RenderOptions{SourcePath: *typstSource})
	}
	if err == nil {
		if issues := artifact.ValidateDocumentsWithStyle(*output, manuscript.Documents, manuscript.Style); len(issues) > 0 {
			err = fmt.Errorf("artifact validation failed: %s", strings.Join(artifact.SortedMessages(issues), "; "))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
	fmt.Println("built:", *output)
}

func resolveStyle(name, language string) (style.Config, error) {
	if strings.HasSuffix(name, ".toml") || strings.Contains(name, string(os.PathSeparator)) {
		return style.LoadFile(name, language)
	}
	cfg, ok := style.Preset(name, language)
	if !ok {
		return style.Config{}, fmt.Errorf("unknown style %q", name)
	}
	return cfg, nil
}

func validate(args []string) {
	flags := flag.NewFlagSet("validate", flag.ExitOnError)
	artifactPath := flags.String("artifact", "", "rendered PDF or EPUB to validate")
	configPath := flags.String("config", "", "book TOML configuration")
	flags.Parse(args)
	if *configPath != "" {
		project, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bookset:", err)
			os.Exit(2)
		}
		manuscript, err := book.Load(project)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bookset:", err)
			os.Exit(1)
		}
		if *artifactPath == "" {
			fmt.Println("valid:", *configPath)
			return
		}
		if issues := artifact.ValidateDocumentsWithStyle(*artifactPath, manuscript.Documents, manuscript.Style); len(issues) > 0 {
			messages := make([]markdown.Issue, 0, len(issues))
			for _, issue := range issues {
				messages = append(messages, markdown.Issue{Message: issue.Error()})
			}
			fail(messages)
		}
		fmt.Println("valid:", *configPath, "and", *artifactPath)
		return
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bookset validate [--config bookset.toml] [--artifact FILE] input.md")
		os.Exit(2)
	}
	doc, issues := load(flags.Arg(0))
	if len(issues) > 0 {
		fail(issues)
	}
	if *artifactPath != "" {
		artifactIssues := artifact.Validate(*artifactPath, doc)
		if len(artifactIssues) > 0 {
			messages := make([]markdown.Issue, 0, len(artifactIssues))
			for _, issue := range artifactIssues {
				messages = append(messages, markdown.Issue{Message: issue.Error()})
			}
			fail(messages)
		}
		fmt.Println("valid:", flags.Arg(0), "and", *artifactPath)
		return
	}
	fmt.Println("valid:", flags.Arg(0))
}

func inspect(args []string) {
	flags := flag.NewFlagSet("inspect", flag.ExitOnError)
	artifactPath := flags.String("artifact", "", "rendered PDF or EPUB to inspect")
	configPath := flags.String("config", "", "book TOML manifest for complete-book inspection")
	jsonOutput := flags.Bool("json", false, "emit stable JSON for agents and CI")
	strict := flags.Bool("strict", false, "treat inspection warnings as failures")
	flags.Parse(args)
	if *artifactPath != "" {
		if flags.NArg() > 1 || (*configPath != "" && flags.NArg() > 0) {
			fmt.Fprintln(os.Stderr, "usage: bookset inspect --artifact FILE [--config bookset.toml | INPUT.md] [--json] [--strict]")
			os.Exit(2)
		}
		var report artifact.Inspection
		var err error
		if *configPath != "" {
			project, configErr := config.Load(*configPath)
			if configErr != nil {
				fmt.Fprintln(os.Stderr, "bookset:", configErr)
				os.Exit(2)
			}
			manuscript, loadErr := book.Load(project)
			if loadErr != nil {
				fmt.Fprintln(os.Stderr, "bookset:", loadErr)
				os.Exit(1)
			}
			report, err = artifact.InspectArtifactAgainstWithStyle(*artifactPath, manuscript.Documents, manuscript.Style)
			paths := make([]string, 0, len(manuscript.Documents))
			titles := make([]string, 0, len(manuscript.Documents))
			footnotes := 0
			for _, document := range manuscript.Documents {
				if document.SourcePath != "" {
					paths = append(paths, filepath.Clean(document.SourcePath))
				}
				titles = append(titles, document.Title)
				footnotes += len(document.Footnotes)
			}
			first := manuscript.Documents[0]
			title := project.Book.Title
			if title == "" {
				title = first.Title
			}
			report.Source = &artifact.SourceInfo{Path: filepath.Clean(*configPath), Title: title, Author: first.Author, Language: first.Language, Chapters: len(manuscript.Chapters), Footnotes: footnotes, ChapterPaths: paths, ChapterTitles: titles}
		} else if flags.NArg() == 1 {
			doc, issues := load(flags.Arg(0))
			if len(issues) > 0 {
				fail(issues)
			}
			report, err = artifact.InspectArtifactAgainst(*artifactPath, []*markdown.Document{doc})
			report.Source = &artifact.SourceInfo{Path: filepath.Clean(flags.Arg(0)), Title: doc.Title, Author: doc.Author, Language: doc.Language, Chapters: 1, Footnotes: len(doc.Footnotes)}
		} else {
			report, err = artifact.InspectArtifact(*artifactPath)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "bookset:", err)
			os.Exit(1)
		}
		if *jsonOutput {
			data, marshalErr := json.MarshalIndent(report, "", "  ")
			if marshalErr != nil {
				fmt.Fprintln(os.Stderr, "bookset:", marshalErr)
				os.Exit(1)
			}
			fmt.Println(string(data))
		} else {
			fmt.Printf("artifact: %s\nformat: %s\nstatus: %s\nsize_bytes: %d\nsha256: %s\n", report.Artifact.Path, report.Artifact.Format, report.Status, report.Artifact.SizeBytes, report.Artifact.SHA256)
			if report.Source != nil {
				fmt.Printf("source: %s\n", report.Source.Path)
			}
			for _, check := range report.Checks {
				fmt.Printf("%s [%s] %s: %s\n", check.Status, check.Severity, check.Code, check.Message)
			}
			for _, issue := range report.Issues {
				if issue.Chapter > 0 {
					fmt.Printf("fail [%s] chapter=%d %s: %s\n", issue.Severity, issue.Chapter, issue.Code, issue.Message)
				} else {
					fmt.Printf("fail [%s] %s: %s\n", issue.Severity, issue.Code, issue.Message)
				}
			}
		}
		if report.Status == "error" || (*strict && report.Status == "warning") {
			os.Exit(1)
		}
		return
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bookset inspect [--json] input.md | --artifact FILE [--config bookset.toml | INPUT.md] [--json] [--strict]")
		os.Exit(2)
	}
	doc, issues := load(flags.Arg(0))
	if len(issues) > 0 {
		fail(issues)
	}
	if *jsonOutput {
		report := struct {
			Schema    string `json:"schema"`
			Status    string `json:"status"`
			Path      string `json:"path"`
			Title     string `json:"title"`
			Author    string `json:"author"`
			Language  string `json:"language"`
			Blocks    int    `json:"blocks"`
			Footnotes int    `json:"footnotes"`
		}{"bookset.document-inspection.v1", "ok", filepath.Clean(flags.Arg(0)), doc.Title, doc.Author, doc.Language, len(doc.Blocks), len(doc.Footnotes)}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "bookset:", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Printf("title: %s\nauthor: %s\nlanguage: %s\nblocks: %d\nfootnotes: %d\n", doc.Title, doc.Author, doc.Language, len(doc.Blocks), len(doc.Footnotes))
}

func load(path string) (*markdown.Document, []markdown.Issue) {
	source, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		fmt.Fprintln(os.Stderr, "bookset:", err)
		os.Exit(1)
	}
	doc, parseIssues := markdown.Parse(source)
	doc.SourcePath = filepath.Clean(path)
	return doc, markdown.Validate(doc, parseIssues)
}
func fail(issues []markdown.Issue) {
	fmt.Fprintln(os.Stderr, "bookset: validation failed:", markdown.FormatIssues(issues))
	os.Exit(1)
}

func usage() {
	fmt.Println("bookset — deterministic book rendering")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  bookset render --format pdf|epub [--typst-source FILE] --output FILE INPUT.md")
	fmt.Println("  bookset render --format pdf --sheet letter --trim-marks --output FILE INPUT.md")
	fmt.Println("  bookset render --format pdf|epub --style trade|classic-trade|timeline-trade --output FILE INPUT.md")
	fmt.Println("  bookset build --config bookset.toml --format pdf|epub [--typst-source FILE] --output FILE")
	fmt.Println("  bookset plan --config bookset.toml [--json]")
	fmt.Println("  bookset proof --config bookset.toml --output FILE [--json]")
	fmt.Println("  bookset validate [--config bookset.toml] [--artifact FILE] INPUT.md")
	fmt.Println("  bookset inspect [--json] INPUT.md")
	fmt.Println("  bookset inspect --artifact FILE [--config bookset.toml | INPUT.md] [--json] [--strict]")
	fmt.Println("  bookset doctor [--style STYLE] [--config bookset.toml]")
	fmt.Println("  bookset version")
}
