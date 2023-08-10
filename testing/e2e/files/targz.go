package files

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// TarGzip creates a tar.gz file from a list of files
func TarGzip(dest *os.File, files ...string) error {
	gw := gzip.NewWriter(dest)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		header.Name = file

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
	}

	return nil
}

// Dir returns a list of all files in a directory and subdirectories
func Dir(root string) ([]string, error) {
	ret := []string{}
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		ret = append(ret, path)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking root: %w", err)
	}

	return ret, nil
}
