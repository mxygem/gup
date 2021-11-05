package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/valyala/fasthttp"
)

const (
	latestURL   string = "https://golang.org/VERSION"
	linuxTar    string = ".linux-amd64.tar.gz"
	downloadURL string = "https://dl.google.com/go/"
)

func main() {
	var wv, n string
	var err error
	var goroot string

	flag.StringVar(&wv, "version", "latest", "version to update to")
	flag.StringVar(&goroot, "goroot", "", "location of goroot")
	flag.Parse()

	if wv == "latest" {
		sc, b, err := fasthttp.Get(nil, latestURL)
		if err != nil || sc >= 400 {
			log.Fatalf("failed to get latest version. code: %d err: %s", sc, err)
		}
		n = string(b) + linuxTar
	} else {
		n = "go" + wv + linuxTar
	}

	td, err := ioutil.TempDir(os.TempDir(), "gup")
	if err != nil {
		log.Fatalf("failed to create new temp dir: %v", err)
	}
	if _, err := os.Stat(td); err != nil {
		if err := os.MkdirAll(td, 0755); err != nil {
			log.Fatal("could not make temp directory: ", err)
		}
	}

	s := spinner.New(
		spinner.CharSets[43],
		100*time.Millisecond,
		spinner.WithHiddenCursor(true),
		spinner.WithFinalMSG("\n"),
	)

	fmt.Printf("downloading go tar from %s\nplease wait\n", downloadURL+n)
	s.Start()
	sc, b, err := fasthttp.Get(nil, downloadURL+n)
	if err != nil || sc >= 400 {
		s.Stop()
		log.Fatalf("failed getting download. code: %d err: %s", sc, err)
	}
	s.Stop()
	fmt.Printf("downloaded %d bytes successfully.\nsaving file to %s\n", len(b), td+"/"+n)
	if err := os.WriteFile(td+"/"+n, b, 0644); err != nil {
		log.Fatalf("failed to tar: %v\n", err)
	}

	tf, err := os.Open(td + "/" + n)
	if err != nil {
		log.Fatalf("could not open tar file: %v", err)
	}

	fmt.Printf("reading tar and saving to: %s\n", td+"/")
	s.Start()
	gzr, err := gzip.NewReader(tf)
	if err != nil {
		log.Fatalf("failed to create new gzip reader: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("failed to read entry in tar: %v", err)
		}
		if header == nil {
			continue
		}

		target := filepath.Join(td+"/", header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					log.Fatalf("failed to make tar directory: %v", err)
				}
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				log.Fatalf("failed to open file: %v", err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				log.Fatalf("failed to write file: %v", err)
			}

			f.Close()
		}
	}
	s.Stop()

	if err := os.RemoveAll(goroot); err != nil {
		log.Fatal("failed to remove files: ", err)
	}
	fmt.Printf("installing new go files to: %q\n", goroot)
	s.Start()
	defer s.Stop()
	if err := os.Mkdir(goroot, 0755); err != nil {
		log.Fatal("failed to recreate goroot")
	}

	err = filepath.Walk(td+"/go", func(path string, info os.FileInfo, err error) error {
		var relPath string = strings.Replace(path, td+"/go", "", 1)
		if relPath == "" {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(goroot+"/", relPath), 0755)
		} else {
			data, err := ioutil.ReadFile(filepath.Join(td+"/go", relPath))
			if err != nil {
				return fmt.Errorf("failed to read file: %v", err)
			}
			if err := ioutil.WriteFile(filepath.Join(goroot+"/", relPath), data, 0777); err != nil {
				return fmt.Errorf("failed to write file: %v", err)
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("filepath walking failed: %v", err)
	}
}
