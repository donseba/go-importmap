// Command importmap vendors front-end packages at build time, for an
// application to embed and load with importmap.FromFS, without network
// access at run time.
//
//	importmap vendor [-config importmap.json] [-out assets/vendor] [-timeout 2m] [-local]
//	importmap verify [-out assets/vendor] [-root dir]
//
// vendor downloads the packages of the configuration into the out
// directory, with their licences, and writes importmap.lock.json there with
// each file's source and SHA-384 integrity. With -local it only records the
// integrity of local packages again, without network access, for example
// after a build changed a bundle. verify checks the files against the lock
// without network access, for CI; when local files are outside the out
// directory, -root names a directory that holds both, such as the parent of
// out.
//
// It suits go:generate:
//
//	//go:generate go run github.com/donseba/go-importmap/cmd/importmap vendor -config importmap.json -out assets/vendor
//
// The configuration lists the packages:
//
//	{
//	  "provider": "cdnjs",
//	  "packages": [
//	    {
//	      "name": "htmx.org",
//	      "version": "4.0.0",
//	      "provider": "jsdelivr",
//	      "require": [{"file": "dist/htmx.esm.min.js", "as": "htmx"}]
//	    },
//	    {"name": "lib.js", "url": "https://example.com/lib.js", "as": "lib", "license": "https://example.com/LICENSE"}
//	  ]
//	}
//
// provider is cdnjs (the default), jsdelivr, jsdelivr-esm, unpkg, skypack or
// esmsh. A package with url is that one file, saved under its name. require
// selects files by pattern, as library.Include does, and names them with as;
// raw maps a name to a URL left on its host. license is a file of the
// package or a URL; without it a LICENSE, LICENCE or COPYING file of the
// package is copied when there is one.
//
// A package with "local": true holds files the application builds itself,
// such as a bundle: require lists them by their paths relative to the out
// directory (which may start with "../"), with as to import them by, and
// license is such a path too. They are not downloaded; the lock records
// their integrity, so FromFS and verify check them:
//
//	{"name": "editor", "local": true, "require": [{"file": "../editor/bundle.js"}], "license": "../editor/LICENSES.txt"}
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/donseba/go-importmap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "importmap:", err)

		stop()
		os.Exit(1)
	}
}

// usage is the synopsis of the command, shown with an error in its use.
const usage = `usage:
  importmap vendor [-config importmap.json] [-out assets/vendor] [-timeout 2m] [-local]
  importmap verify [-out assets/vendor] [-root dir]`

// run runs the command of args, writing what it reports to stdout.
func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}

	switch args[0] {
	case "vendor":
		return vendor(ctx, args[1:], stdout)
	case "verify":
		return verify(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

// vendor runs importmap vendor with the flags of args.
func vendor(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("vendor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	configFile := flags.String("config", "importmap.json", "the configuration file")
	out := flags.String("out", "assets/vendor", "the directory for the files and the lock")
	timeout := flags.Duration("timeout", 2*time.Minute, "the time allowed for all downloads")
	localOnly := flags.Bool("local", false, "only record the integrity of local packages again, without network")

	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}

	config, err := readConfig(*configFile)
	if err != nil {
		return err
	}

	im, err := config.importMap()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	var lock *importmap.Lock
	if *localOnly {
		lock, err = im.VendorLocal(*out)
	} else {
		lock, err = im.Vendor(ctx, *out)
	}

	if err != nil {
		return err
	}

	for _, pkg := range lock.Packages {
		license := pkg.License
		if license == "" {
			license = "no licence file found"
		}

		name := pkg.Name
		if pkg.Version != "" {
			name += " " + pkg.Version
		}

		kind := ""
		if pkg.Local {
			kind = "local "
		}

		_, _ = fmt.Fprintf(stdout, "%s: %d %sfiles (%s)\n", name, len(pkg.Files), kind, license)
	}

	return nil
}

// verify runs importmap verify with the flags of args.
func verify(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	out := flags.String("out", "assets/vendor", "the directory with the files and the lock")
	root := flags.String("root", "", "a directory holding out and the local files outside it; out by default")

	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}

	dir := *out
	lock := importmap.LockFile

	if *root != "" {
		rel, err := filepath.Rel(*root, *out)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("-out %s is not below -root %s", *out, *root)
		}

		dir = *root
		lock = path.Join(filepath.ToSlash(rel), importmap.LockFile)
	}

	if _, err := importmap.FromFS(os.DirFS(dir), lock); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "%s matches %s\n", *out, importmap.LockFile)

	return nil
}
