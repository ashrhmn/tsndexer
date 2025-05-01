package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var createdCount int

func main() {
	// --- CLI flags
	var (
		maxFiles          int
		dryRun, verbose   bool
		showHelp          bool
		ignorePatternsStr string
	)
	flag.IntVar(&maxFiles, "m", 10000, "maximum number of .ts/.tsx files to process")
	flag.IntVar(&maxFiles, "max-files", 10000, "maximum number of .ts/.tsx files to process")
	flag.BoolVar(&dryRun, "n", false, "dry run: preview without writing files")
	flag.BoolVar(&dryRun, "dry-run", false, "dry run: preview without writing files")
	flag.BoolVar(&verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose logging")
	flag.BoolVar(&showHelp, "h", false, "show help")
	flag.BoolVar(&showHelp, "help", false, "show help")
	flag.StringVar(
		&ignorePatternsStr,
		"i",
		"node_modules,.git,dist,build,out,coverage,.cache,.next,.parcel-cache,lib,esm,cjs,.husky,.vscode,.idea", "comma-separated glob patterns or names to ignore",
	)
	flag.StringVar(
		&ignorePatternsStr,
		"ignore",
		"node_modules,.git,dist,build,out,coverage,.cache,.next,.parcel-cache,lib,esm,cjs,.husky,.vscode,.idea", "comma-separated glob patterns or names to ignore",
	)

	// Custom usage
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [directory]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nExamples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s              # process current directory\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s . -m 1000    # set max files to 1000\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s ./src        # process src folder\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -n ./lib     # dry-run\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s . -i \"dist,build\"  # ignore dist & build\n", os.Args[0])
	}

	flag.Parse()
	if showHelp {
		flag.Usage()
		return
	}

	// Parse ignore patterns
	ignorePatterns := parseIgnorePatterns(ignorePatternsStr)
	if verbose {
		fmt.Printf("Ignoring patterns: %v\n", ignorePatterns)
	}

	// Determine root directory
	root := "."
	if args := flag.Args(); len(args) > 1 {
		fmt.Fprintln(os.Stderr, "Error: too many arguments.")
		flag.Usage()
		os.Exit(1)
	} else if len(args) == 1 {
		root = args[0]
	}

	// 1) Count everything first
	totalCount, err := countAllFiles(root, verbose, ignorePatterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning files: %v\n", err)
		os.Exit(1)
	}

	// 2) Enforce limit
	if totalCount > maxFiles {
		fmt.Fprintf(
			os.Stderr,
			"Error: found %d .ts/.tsx files, which exceeds the limit of %d. Use `-m` to raise it.\n",
			totalCount, maxFiles,
		)
		os.Exit(1)
	}
	if verbose {
		fmt.Printf("Found %d .ts/.tsx files (limit %d)\n", totalCount, maxFiles)
	}

	// 3) Generate indexes
	if _, _, err := processDir(root, verbose, dryRun, ignorePatterns); err != nil {
		fmt.Fprintf(os.Stderr, "Error processing directories: %v\n", err)
		os.Exit(1)
	}

	// 4) Summary
	if dryRun {
		fmt.Printf("Dry run: %d index file(s) would have been generated.\n", createdCount)
	} else {
		fmt.Printf("Done. Generated %d index file(s).\n", createdCount)
	}
}

// countAllFiles walks the tree and returns the total number of .ts/.tsx
// (excluding index.* and .d.ts), skipping ignored paths.
func countAllFiles(root string, verbose bool, ignore []string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Skipping %s: %v\n", path, err)
			}
			return nil
		}
		name := d.Name()
		// skip ignored dirs
		if d.IsDir() && matchesIgnore(name, ignore) {
			if verbose {
				fmt.Printf("Skipping ignored dir: %s\n", path)
			}
			return fs.SkipDir
		}
		// skip ignored files
		if !d.IsDir() && matchesIgnore(name, ignore) {
			if verbose {
				fmt.Printf("Skipping ignored file: %s\n", path)
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		l := strings.ToLower(name)
		if l == "index.ts" || l == "index.tsx" || strings.HasSuffix(l, ".d.ts") {
			return nil
		}
		ext := filepath.Ext(l)
		if ext == ".ts" || ext == ".tsx" {
			count++
		}
		return nil
	})
	return count, err
}

// processDir scans one directory (skipping ignored), recurses, then creates an index file if needed.
// Returns hasTs, hasTsx indicating whether this subtree contains .ts/.tsx.
func processDir(dir string, verbose, dryRun bool, ignore []string) (hasTs, hasTsx bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, false, err
	}

	var (
		immediateTs   []string
		immediateTsx  []string
		subdirs       []string
		childWithCode []string
	)

	// 1) scan this folder’s entries
	for _, e := range entries {
		name := e.Name()
		// skip ignored
		if matchesIgnore(name, ignore) {
			if verbose {
				fmt.Printf("Skipping ignored entry: %s/%s\n", dir, name)
			}
			continue
		}
		lower := strings.ToLower(name)
		if e.IsDir() {
			subdirs = append(subdirs, name)
		} else {
			if lower == "index.ts" || lower == "index.tsx" || strings.HasSuffix(lower, ".d.ts") {
				continue
			}
			ext := filepath.Ext(lower)
			base := strings.TrimSuffix(name, ext)
			switch ext {
			case ".ts":
				immediateTs = append(immediateTs, base)
			case ".tsx":
				immediateTsx = append(immediateTsx, base)
			}
		}
	}

	// 2) recurse into subdirectories
	for _, sd := range subdirs {
		p := filepath.Join(dir, sd)
		childTs, childTsx, err := processDir(p, verbose, dryRun, ignore)
		if err != nil {
			return false, false, err
		}
		if childTs || childTsx {
			childWithCode = append(childWithCode, sd)
		}
		hasTs = hasTs || childTs
		hasTsx = hasTsx || childTsx
	}

	// 3) incorporate this folder’s immediate files
	if len(immediateTs) > 0 {
		hasTs = true
	}
	if len(immediateTsx) > 0 {
		hasTsx = true
	}

	// 4) if nothing here or below, skip
	if !hasTs && !hasTsx {
		return false, false, nil
	}

	// 5) pick extension
	ext := ".ts"
	if hasTsx {
		ext = ".tsx"
	}
	idxName := "index" + ext
	idxPath := filepath.Join(dir, idxName)

	// 6) build exports
	var exports []string
	sort.Strings(immediateTs)
	sort.Strings(immediateTsx)
	for _, f := range immediateTs {
		exports = append(exports, fmt.Sprintf(`export * from "./%s";`, f))
	}
	for _, f := range immediateTsx {
		exports = append(exports, fmt.Sprintf(`export * from "./%s";`, f))
	}
	sort.Strings(childWithCode)
	for _, d := range childWithCode {
		exports = append(exports, fmt.Sprintf(`export * from "./%s";`, d))
	}
	content := strings.Join(exports, "\n") + "\n"

	// 7) write or preview
	if verbose {
		fmt.Printf("→ %s\n", idxPath)
	}
	if dryRun {
		fmt.Printf("Would create %s with:\n%s\n", idxPath, content)
		createdCount++
	} else {
		if err := os.WriteFile(idxPath, []byte(content), 0644); err != nil {
			return hasTs, hasTsx, err
		}
		createdCount++
	}

	return hasTs, hasTsx, nil
}

// parseIgnorePatterns splits comma-separated list into slice.
func parseIgnorePatterns(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// matchesIgnore returns true if name matches any ignore glob.
func matchesIgnore(name string, patterns []string) bool {
	for _, pat := range patterns {
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
	}
	return false
}
