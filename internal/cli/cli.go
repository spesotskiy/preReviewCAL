package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"prereviewcal/internal/config"
	"prereviewcal/internal/gitdiff"
	"prereviewcal/internal/objectfile"
	"prereviewcal/internal/rules"
)

// Run executes the CLI with the given arguments and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	cmd := args[0]
	switch cmd {
	case "fix", "check":
		return runCommand(cmd, args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %q\n\n", cmd)
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "PreReviewCAL - C/AL (NAV 2017) code cleaner & formatter")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  prereviewcal fix [flags] [file-or-dir ...]")
	fmt.Fprintln(w, "  prereviewcal check [flags] [file-or-dir ...]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --config <path>             Path to .prereviewcal.json configuration file")
	fmt.Fprintln(w, "  --diff                      Print unified diff (with 'fix', dry-run without writing files)")
	fmt.Fprintln(w, "  --symbols-dir <dir>         Directory with NAV Table .txt files for WITH field resolution")
	fmt.Fprintln(w, "  --verbose                   Print all applied fixes in addition to warnings")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Git Diff Scope Flags:")
	fmt.Fprintln(w, "  --staged                    Compare staged changes against HEAD (pre-commit dev mode)")
	fmt.Fprintln(w, "  --git-diff                  Compare working tree against HEAD")
	fmt.Fprintln(w, "  --git-base <ref>            Compare against base ref/commit (e.g. origin/main for pre-review / MR)")
	fmt.Fprintln(w, "  --git-head <ref>            Head ref/commit for CI/CD MR 'check' (with --git-base)")
	fmt.Fprintln(w, "  --repo <dir>                Git repository directory (defaults to target path or CWD)")
	fmt.Fprintln(w, "  --max-old-func-lines <N>    Max body lines of modified old function/trigger to refactor as a whole (0 = unlimited)")
}

func runCommand(cmd string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "path to config JSON")
	showDiff := fs.Bool("diff", false, "print unified diff")
	symbolsDir := fs.String("symbols-dir", "", "directory containing NAV table .txt files")
	verbose := fs.Bool("verbose", false, "print all fixed diagnostics")

	staged := fs.Bool("staged", false, "compare staged changes against HEAD")
	gitDiffFlag := fs.Bool("git-diff", false, "compare working tree against HEAD")
	gitBase := fs.String("git-base", "", "base git ref/commit for diff mode")
	gitHead := fs.String("git-head", "", "head git ref/commit for CI/CD check mode")
	repoDir := fs.String("repo", "", "git repository directory")
	maxOldFuncLines := fs.Int("max-old-func-lines", -1, "max body lines of touched old function/trigger to refactor wholly (0 = unlimited)")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	targets := fs.Args()

	isGitMode := *staged || *gitDiffFlag || *gitBase != "" || *gitHead != ""

	if *gitHead != "" && cmd == "fix" {
		fmt.Fprintln(stderr, "error: --git-head can only be used with 'check' (cannot write fixes into a git commit)")
		return 2
	}
	if *maxOldFuncLines < -1 {
		fmt.Fprintf(stderr, "error: --max-old-func-lines must be >= 0, got %d\n", *maxOldFuncLines)
		return 2
	}

	if !isGitMode && len(targets) == 0 {
		fmt.Fprintln(stderr, "error: at least one file or directory path is required (or use --staged / --git-base / --git-diff)")
		printUsage(stderr)
		return 2
	}

	configSearchDir := "."
	if *repoDir != "" {
		configSearchDir = *repoDir
	} else if len(targets) > 0 {
		configSearchDir = targets[0]
	}

	cfg, err := loadConfig(*configPath, configSearchDir)
	if err != nil {
		fmt.Fprintf(stderr, "error loading config: %v\n", err)
		return 2
	}
	if *maxOldFuncLines >= 0 {
		cfg.Diff.MaxOldFunctionLines = *maxOldFuncLines
	}

	if isGitMode {
		return runGitDiffMode(cmd, cfg, gitRunParams{
			staged:     *staged,
			gitBase:    *gitBase,
			gitHead:    *gitHead,
			repoDir:    *repoDir,
			symbolsDir: *symbolsDir,
			showDiff:   *showDiff,
			verbose:    *verbose,
			targets:    targets,
		}, stdout, stderr)
	}

	return runFullFileMode(cmd, cfg, *symbolsDir, *showDiff, *verbose, targets, stdout, stderr)
}

type gitRunParams struct {
	staged     bool
	gitBase    string
	gitHead    string
	repoDir    string
	symbolsDir string
	showDiff   bool
	verbose    bool
	targets    []string
}

func runGitDiffMode(cmd string, cfg *config.Config, p gitRunParams, stdout, stderr io.Writer) int {
	opts := gitdiff.Options{
		RepoDir: p.repoDir,
		BaseRef: p.gitBase,
		HeadRef: p.gitHead,
	}
	switch {
	case p.gitHead != "":
		opts.Mode = gitdiff.ModeCommitRange
		if opts.BaseRef == "" {
			opts.BaseRef = "HEAD~1"
		}
	case p.staged:
		opts.Mode = gitdiff.ModeStaged
		opts.BaseRef = "HEAD"
	default:
		opts.Mode = gitdiff.ModeWorktree
		if opts.BaseRef == "" {
			opts.BaseRef = "HEAD"
		}
	}

	readStagedBlob := cmd == "check" && opts.Mode == gitdiff.ModeStaged
	targetFiles, repoRoot, err := gitdiff.CollectTargets(opts, p.targets, readStagedBlob)
	if err != nil {
		fmt.Fprintf(stderr, "git diff error: %v\n", err)
		return 2
	}
	if len(targetFiles) == 0 {
		return 0
	}

	symDir := p.symbolsDir
	if symDir == "" {
		symDir = repoRoot
	}
	ws := objectfile.NewWorkspaceIndex(symDir, cfg.FileNaming)

	changedCount := 0
	warningCount := 0
	errorCount := 0

	for _, tf := range targetFiles {
		res, err := rules.ProcessDiff(tf.Base, tf.Current, cfg, ws)
		if err != nil {
			fmt.Fprintf(stderr, "%s: parse error: %v\n", tf.RelPath, err)
			errorCount++
			continue
		}

		for _, d := range res.Diagnostics {
			if !d.Fixed {
				warningCount++
				fmt.Fprintf(stdout, "%s: %s\n", tf.RelPath, d.String())
			} else if p.verbose || cmd == "check" {
				fmt.Fprintf(stdout, "%s: %s\n", tf.RelPath, d.String())
			}
		}

		if res.Changed {
			changedCount++
			if p.showDiff {
				fmt.Fprint(stdout, UnifiedDiff(tf.RelPath, tf.Current, res.Output))
			}
			if cmd == "fix" && !p.showDiff {
				if err := os.WriteFile(tf.DiskPath, res.Output, 0o644); err != nil {
					fmt.Fprintf(stderr, "%s: write error: %v\n", tf.RelPath, err)
					errorCount++
					continue
				}
				fmt.Fprintf(stdout, "fixed %s\n", tf.RelPath)
			} else if cmd == "check" && !p.showDiff {
				fmt.Fprintf(stdout, "needs fix: %s\n", tf.RelPath)
			}
		}
	}

	if errorCount > 0 {
		return 2
	}
	if cmd == "check" && (changedCount > 0 || warningCount > 0) {
		return 1
	}
	return 0
}

func runFullFileMode(cmd string, cfg *config.Config, symbolsDir string, showDiff, verbose bool, targets []string, stdout, stderr io.Writer) int {
	files, err := collectTxtFiles(targets)
	if err != nil {
		fmt.Fprintf(stderr, "error scanning targets: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		fmt.Fprintln(stderr, "no .txt files found")
		return 0
	}

	symDir := symbolsDir
	if symDir == "" {
		info, statErr := os.Stat(targets[0])
		if statErr == nil && info.IsDir() {
			symDir = targets[0]
		} else {
			symDir = filepath.Dir(targets[0])
		}
	}
	ws := objectfile.NewWorkspaceIndex(symDir, cfg.FileNaming)

	changedCount := 0
	warningCount := 0
	errorCount := 0

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: read error: %v\n", path, err)
			errorCount++
			continue
		}

		res, err := rules.Process(data, cfg, ws)
		if err != nil {
			fmt.Fprintf(stderr, "%s: parse error: %v\n", path, err)
			errorCount++
			continue
		}

		for _, d := range res.Diagnostics {
			if !d.Fixed {
				warningCount++
				fmt.Fprintf(stdout, "%s: %s\n", path, d.String())
			} else if verbose {
				fmt.Fprintf(stdout, "%s: %s\n", path, d.String())
			}
		}

		if res.Changed {
			changedCount++
			if showDiff {
				fmt.Fprint(stdout, UnifiedDiff(path, data, res.Output))
			}
			if cmd == "fix" && !showDiff {
				if err := os.WriteFile(path, res.Output, 0o644); err != nil {
					fmt.Fprintf(stderr, "%s: write error: %v\n", path, err)
					errorCount++
					continue
				}
				fmt.Fprintf(stdout, "fixed %s\n", path)
			} else if cmd == "check" && !showDiff {
				fmt.Fprintf(stdout, "needs fix: %s\n", path)
			}
		}
	}

	if errorCount > 0 {
		return 2
	}
	if cmd == "check" && (changedCount > 0 || warningCount > 0) {
		return 1
	}
	return 0
}

func loadConfig(explicit string, firstTarget string) (*config.Config, error) {
	if explicit != "" {
		return config.Load(explicit)
	}
	// Auto-discover .prereviewcal.json from target directory upward, then CWD.
	searchDir := firstTarget
	if info, err := os.Stat(searchDir); err == nil && !info.IsDir() {
		searchDir = filepath.Dir(searchDir)
	}
	if abs, err := filepath.Abs(searchDir); err == nil {
		dir := abs
		for {
			candidate := filepath.Join(dir, ".prereviewcal.json")
			if _, err := os.Stat(candidate); err == nil {
				return config.Load(candidate)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if _, err := os.Stat(".prereviewcal.json"); err == nil {
		return config.Load(".prereviewcal.json")
	}
	return config.Default(), nil
}

func collectTxtFiles(targets []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	for _, t := range targets {
		info, err := os.Stat(t)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !seen[t] {
				seen[t] = true
				files = append(files, t)
			}
			continue
		}
		err = filepath.WalkDir(t, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if strings.EqualFold(filepath.Ext(d.Name()), ".txt") {
				if !seen[path] {
					seen[path] = true
					files = append(files, path)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
