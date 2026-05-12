// Run with: go run ./tools/mkzip <source-dir> <output.zip>
// Zips the contents of source-dir into output.zip.
// Used at build time to bundle the browser-extension/ folder into FpbxCTC.exe.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mkzip <source-dir> <output.zip>")
		os.Exit(1)
	}
	srcDir := filepath.Clean(os.Args[1])
	outFile := os.Args[2]

	f, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip the icons/ folder — it's generated and will be regenerated on extraction
		rel, _ := filepath.Rel(srcDir, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "icons/") {
			return nil
		}

		zf, err := w.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(zf, src)
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}
}
