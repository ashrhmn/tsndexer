# tsndexer

A tiny Go CLI to generate `index.ts` / `index.tsx` files for your TypeScript codebase.

## Features

- Recursively scans your project for `.ts` / `.tsx` (skips `.d.ts` and existing `index.*`)
- Enforces a configurable max file-count (default 10000)
- Chooses `.tsx` for an index if **any** `.tsx` is found in its subtree
- Exports all immediate files and subfolders that contain TS(X)
- Supports **ignore patterns** (default: `node_modules`, `.git` etc.)
- Supports `--dry-run`, `--verbose`
- Zero dependencies beyond the Go stdlib

## Install

```bash
# with Go 1.18+ installed:
go install github.com/ashrhmn/tsndexer@latest

# or, clone & build manually:
git clone https://github.com/ashrhmn/tsndexer.git
cd tsndexer
go build -o tsndexer .
```
