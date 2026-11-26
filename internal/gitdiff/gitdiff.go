package gitdiff

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Mode represents one of the Git comparison workflows.
type Mode int

const (
	// ModeStaged compares staged index changes against HEAD (pre-commit / development).
	ModeStaged Mode = iota
	// ModeWorktree compares working tree (including commits on branch + local changes) against BaseRef (pre-review).
	ModeWorktree
	// ModeCommitRange compares HeadRef against BaseRef purely from Git objects (CI/CD MR check).
	ModeCommitRange
)

// Options configures how Git base and target contents are resolved.
type Options struct {
	RepoDir string // directory inside git repo (defaults to CWD)
	Mode    Mode
	BaseRef string // e.g. "HEAD", "origin/main", or commit SHA
	HeadRef string // used in ModeCommitRange (e.g. "HEAD" or CI_COMMIT_SHA)
}

// TargetFile holds a file to be processed in Git diff mode along with its base and current bytes.
type TargetFile struct {
	DiskPath string // absolute path on disk (for writing in fix mode / display)
	RelPath  string // repo-relative slash path
	Base     []byte // nil if newly added file
	Current  []byte
}

// RepoRoot returns the absolute root directory of the Git repository containing dir.
func RepoRoot(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

// ResolveBaseCommit computes the merge-base between baseRef and headRef (falling back to baseRef).
func ResolveBaseCommit(repoRoot, baseRef, headRef string) (string, error) {
	if baseRef == "" {
		baseRef = "HEAD"
	}
	if headRef == "" {
		headRef = "HEAD"
	}
	if baseRef == "HEAD" && headRef == "HEAD" {
		out, err := runGit(repoRoot, "rev-parse", "--verify", "HEAD")
		if err != nil {
			// Initial repo with no commits yet.
			return "", nil
		}
		return strings.TrimSpace(string(out)), nil
	}
	// Try merge-base first (ideal for comparing feature branch vs main/origin/main).
	if out, err := runGit(repoRoot, "merge-base", baseRef, headRef); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// Fallback to resolving baseRef directly.
	out, err := runGit(repoRoot, "rev-parse", "--verify", baseRef)
	if err != nil {
		return "", fmt.Errorf("cannot resolve git base %q: %w", baseRef, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CollectTargets discovers changed .txt files (or filters explicitPaths) and loads Base and Current bytes.
// When readStagedBlob is true and Mode == ModeStaged, Current is read from the Git index (":path")
// instead of disk (ideal for 'check --staged').
func CollectTargets(opts Options, explicitPaths []string, readStagedBlob bool) ([]TargetFile, string, error) {
	startDir := opts.RepoDir
	if startDir == "" {
		if len(explicitPaths) > 0 {
			if info, err := os.Stat(explicitPaths[0]); err == nil && info.IsDir() {
				startDir = explicitPaths[0]
			} else {
				startDir = filepath.Dir(explicitPaths[0])
			}
		} else {
			wd, err := os.Getwd()
			if err != nil {
				return nil, "", err
			}
			startDir = wd
		}
	}

	root, err := RepoRoot(startDir)
	if err != nil {
		return nil, "", err
	}

	baseRef := opts.BaseRef
	if baseRef == "" {
		baseRef = "HEAD"
	}
	headRef := opts.HeadRef
	if headRef == "" {
		headRef = "HEAD"
	}

	var baseCommit string
	switch opts.Mode {
	case ModeStaged:
		baseCommit, _ = ResolveBaseCommit(root, "HEAD", "HEAD")
	case ModeWorktree:
		baseCommit, err = ResolveBaseCommit(root, baseRef, "HEAD")
		if err != nil {
			return nil, root, err
		}
	case ModeCommitRange:
		baseCommit, err = ResolveBaseCommit(root, baseRef, headRef)
		if err != nil {
			return nil, root, err
		}
	}

	var relPaths []string
	if len(explicitPaths) > 0 {
		relPaths, err = expandExplicitPaths(root, explicitPaths, opts, baseCommit, headRef)
		if err != nil {
			return nil, root, err
		}
	} else {
		relPaths, err = listChangedTxtFiles(root, opts, baseCommit, headRef)
		if err != nil {
			return nil, root, err
		}
	}

	var targets []TargetFile
	for _, rel := range relPaths {
		if !strings.EqualFold(filepath.Ext(rel), ".txt") {
			continue
		}
		diskPath := filepath.Join(root, filepath.FromSlash(rel))

		var baseBytes []byte
		if baseCommit != "" {
			if b, err := gitShowBlob(root, baseCommit+":"+rel); err == nil {
				baseBytes = b
			}
		}

		var currBytes []byte
		switch opts.Mode {
		case ModeCommitRange:
			currBytes, err = gitShowBlob(root, headRef+":"+rel)
			if err != nil {
				return nil, root, fmt.Errorf("read %s at %s: %w", rel, headRef, err)
			}
		case ModeStaged:
			if readStagedBlob {
				currBytes, err = gitShowBlob(root, ":"+rel)
				if err != nil {
					return nil, root, fmt.Errorf("read staged %s: %w", rel, err)
				}
			} else {
				currBytes, err = os.ReadFile(diskPath)
				if err != nil {
					return nil, root, err
				}
			}
		case ModeWorktree:
			currBytes, err = os.ReadFile(diskPath)
			if err != nil {
				return nil, root, err
			}
		}

		targets = append(targets, TargetFile{
			DiskPath: diskPath,
			RelPath:  rel,
			Base:     baseBytes,
			Current:  currBytes,
		})
	}

	return targets, root, nil
}

func listChangedTxtFiles(root string, opts Options, baseCommit, headRef string) ([]string, error) {
	var args []string
	switch opts.Mode {
	case ModeStaged:
		args = []string{"-c", "core.quotepath=false", "diff", "--cached", "--name-only", "--diff-filter=ACMR"}
	case ModeWorktree:
		if baseCommit == "" {
			args = []string{"-c", "core.quotepath=false", "ls-files", "--others", "--cached", "--exclude-standard"}
		} else {
			args = []string{"-c", "core.quotepath=false", "diff", "--name-only", "--diff-filter=ACMR", baseCommit}
		}
	case ModeCommitRange:
		if baseCommit == "" {
			args = []string{"-c", "core.quotepath=false", "ls-tree", "-r", "--name-only", headRef}
		} else {
			args = []string{"-c", "core.quotepath=false", "diff", "--name-only", "--diff-filter=ACMR", baseCommit, headRef}
		}
	}

	out, err := runGit(root, args...)
	if err != nil {
		return nil, fmt.Errorf("git diff file list: %w", err)
	}
	seen := make(map[string]bool)
	var files []string
	addLines := func(raw []byte) {
		for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
			p := strings.TrimSpace(line)
			if p == "" || !strings.EqualFold(filepath.Ext(p), ".txt") {
				continue
			}
			if !seen[p] {
				seen[p] = true
				files = append(files, p)
			}
		}
	}
	addLines(out)

	// In ModeWorktree also include untracked new .txt files in the working tree.
	if opts.Mode == ModeWorktree && baseCommit != "" {
		if untracked, err := runGit(root, "-c", "core.quotepath=false", "ls-files", "--others", "--exclude-standard"); err == nil {
			addLines(untracked)
		}
	}

	return files, nil
}

func expandExplicitPaths(root string, explicitPaths []string, opts Options, baseCommit, headRef string) ([]string, error) {
	changedSet := make(map[string]bool)
	if changed, err := listChangedTxtFiles(root, opts, baseCommit, headRef); err == nil {
		for _, c := range changed {
			changedSet[strings.ToLower(c)] = true
		}
	}

	seen := make(map[string]bool)
	var out []string
	addRel := func(rel string) {
		rel = filepath.ToSlash(rel)
		if !strings.EqualFold(filepath.Ext(rel), ".txt") {
			return
		}
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}

	for _, p := range explicitPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, err
		}
		info, statErr := os.Stat(abs)
		if statErr == nil && info.IsDir() {
			prefix := filepath.ToSlash(rel)
			if prefix == "." {
				prefix = ""
			} else {
				prefix += "/"
			}
			for _, c := range mustSortedKeys(changedSet, root, opts, baseCommit, headRef) {
				if prefix == "" || strings.HasPrefix(c, prefix) {
					addRel(c)
				}
			}
		} else {
			addRel(rel)
		}
	}
	return out, nil
}

func mustSortedKeys(_ map[string]bool, root string, opts Options, baseCommit, headRef string) []string {
	files, _ := listChangedTxtFiles(root, opts, baseCommit, headRef)
	return files
}

func gitShowBlob(repoRoot, spec string) ([]byte, error) {
	return runGit(repoRoot, "show", spec)
}

func runGit(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
