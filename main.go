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

var (
	createdCount int
)

func main() {
	// -- CLI flags
	var (
		maxFiles          int
		dryRun, verbose   bool
		showHelp          bool
		ignorePatternsStr string
	)
	flag.IntVar(&maxFiles, "m", 300, "maximum number of .ts/.tsx files to process")
	flag.IntVar(&maxFiles, "max-files", 300, "maximum number of .ts/.tsx files to process")
	flag.BoolVar(&dryRun, "n", false, "dry run: show what would be done without writing files")
	flag.BoolVar(&dryRun, "dry-run", false, "dry run: show what would be done without writing files")
	flag.BoolVar(&verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose logging")
	flag.BoolVar(&showHelp, "h", false, "show help")
	flag.BoolVar(&showHelp, "help", false, "show help")
	flag.StringVar(&ignorePatternsStr, "i", "node_modules,.git", "comma-separated list of glob patterns or names to ignore")
	flag.StringVar(&ignorePatternsStr, "ignore", "node_modules,.git", "comma-separated list of glob patterns or names to ignore")

	// Custom usage
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [directory]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nExamples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s              # process current directory\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s . -m 1000    # set max files to 1000\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s ./src        # process src folder\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -n ./lib     # preview changes without writing files\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s . -i \"dist,build\"  # ignore dist and build folders\n", os.Args[0])
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

	// Determine target dir
	args := flag.Args()
	var root string
	if len(args) == 0 {
		root = "."
	} else if len(args) == 1 {
		root = args[0]
	} else {
		fmt.Fprintln(os.Stderr, "Error: too many arguments.")
		flag.Usage()
		os.Exit(1)
	}

	// 1) Count .ts/.tsx files (skip index and .d.ts), skipping ignored paths, and enforce limit
	count, err := findCount(root, maxFiles, verbose, ignorePatterns)
	if err != nil {
		if strings.Contains(err.Error(), "max file count exceeded") {
			fmt.Fprintf(os.Stderr, "Error: %s. Use `-m` to raise the limit.\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Error scanning files: %v\n", err)
		}
		os.Exit(1)
	}
	if verbose {
		fmt.Printf("Found %d .ts/.tsx files (limit %d)\n", count, maxFiles)
	}

	// 2) Recursively generate index files (skipping ignored)
	_, _, err = processDir(root, verbose, dryRun, ignorePatterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing directory: %v\n", err)
		os.Exit(1)
	}

	// 3) Summary
	if dryRun {
		fmt.Printf("Dry run complete: %d index file(s) would have been generated.\n", createdCount)
	} else {
		fmt.Printf("Done. Generated %d index file(s).\n", createdCount)
	}
}

// parseIgnorePatterns splits the comma-separated list.
func parseIgnorePatterns(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// matchesIgnore returns true if name matches any of the glob patterns.
func matchesIgnore(name string, patterns []string) bool {
	for _, pat := range patterns {
		if match, _ := filepath.Match(pat, name); match {
			return true
		}
	}
	return false
}

// findCount walks the tree and counts .ts/.tsx (excluding index.* and .d.ts), skipping ignored paths.
// Returns an error if count > max.
func findCount(root string, max int, verbose bool, ignore []string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Skipping %s: %v\n", path, err)
			}
			return nil
		}
		name := d.Name()

		// Skip ignored dirs
		if d.IsDir() && matchesIgnore(name, ignore) {
			if verbose {
				fmt.Printf("Skipping ignored dir: %s\n", path)
			}
			return fs.SkipDir
		}
		// Skip ignored files
		if !d.IsDir() && matchesIgnore(name, ignore) {
			if verbose {
				fmt.Printf("Skipping ignored file: %s\n", path)
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}
		lower := strings.ToLower(name)
		if lower == "index.ts" || lower == "index.tsx" || strings.HasSuffix(lower, ".d.ts") {
			return nil
		}
		ext := filepath.Ext(lower)
		if ext == ".ts" || ext == ".tsx" {
			count++
			if count > max {
				return fmt.Errorf("max file count exceeded: %d > %d", count, max)
			}
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

		// Skip ignored
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

	// 4) if nothing here or below, no index needed
	if !hasTs && !hasTsx {
		return false, false, nil
	}

	// 5) decide extension for index file
	ext := ".ts"
	if hasTsx {
		ext = ".tsx"
	}
	idxName := "index" + ext
	idxPath := filepath.Join(dir, idxName)

	// 6) build the export lines, sorted
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
