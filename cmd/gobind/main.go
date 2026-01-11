// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sagernet/gomobile/internal/importers"
	"github.com/sagernet/gomobile/internal/importers/java"
	"github.com/sagernet/gomobile/internal/importers/objc"
	"golang.org/x/tools/go/packages"
)

var (
	lang                  = flag.String("lang", "", "target languages for bindings, either java, go, objc, or csharp. If empty, all languages are generated.")
	outdir                = flag.String("outdir", "", "result will be written to the directory instead of stdout.")
	javaPkg               = flag.String("javapkg", "", "custom Java package path prefix. Valid only with -lang=java.")
	csharpPkg             = flag.String("cspkg", "", "custom C# namespace prefix. Valid only with -lang=csharp.")
	csharpNamespaceFlag   = flag.String("csnamespace", "", "custom C# namespace. Overrides -cspkg. Valid only with -lang=csharp.")
	csharpPackageNameFlag = flag.String("cspkgname", "", "custom C# package class name. Valid only with -lang=csharp.")
	libname               = flag.String("libname", "gojni", "custom library name. Valid only with -lang=java or -lang=csharp.")
	prefix                = flag.String("prefix", "", "custom Objective-C name prefix. Valid only with -lang=objc.")
	bootclasspath         = flag.String("bootclasspath", "", "Java bootstrap classpath.")
	classpath             = flag.String("classpath", "", "Java classpath.")
	tags                  = flag.String("tags", "", "build tags.")
)

var usage = `The Gobind tool generates Java, Objective-C, and C# language bindings for Go.

For usage details, see doc.go.`

func main() {
	flag.Parse()

	run()
	os.Exit(exitStatus)
}

func run() {
	var langs []string
	if *lang != "" {
		langs = strings.Split(*lang, ",")
	} else {
		langs = []string{"go", "java", "objc", "csharp"}
	}
	if *csharpNamespaceFlag != "" && *csharpPkg != "" {
		log.Fatal("-cspkg and -csnamespace cannot be used together")
	}

	// We need to give appropriate environment variables like CC or CXX so that the returned packages no longer have errors.
	// However, getting such environment variables is difficult or impossible so far.
	// Gomobile can obtain such environment variables in env.go, but this logic assumes some condiitons gobind doesn't assume.
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles |
			packages.NeedImports | packages.NeedDeps |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		BuildFlags: []string{"-tags", strings.Join(strings.Split(*tags, ","), " ")},
	}

	// Call Load twice to warm the cache. There is a known issue that the result of Load
	// depends on build cache state. See golang/go#33687.
	packages.Load(cfg, flag.Args()...)

	allPkg, err := packages.Load(cfg, flag.Args()...)
	if err != nil {
		log.Fatal(err)
	}

	jrefs, err := importers.AnalyzePackages(allPkg, "Java/")
	if err != nil {
		log.Fatal(err)
	}
	orefs, err := importers.AnalyzePackages(allPkg, "ObjC/")
	if err != nil {
		log.Fatal(err)
	}
	var classes []*java.Class
	if len(jrefs.Refs) > 0 {
		jimp := &java.Importer{
			Bootclasspath: *bootclasspath,
			Classpath:     *classpath,
			JavaPkg:       *javaPkg,
		}
		classes, err = jimp.Import(jrefs)
		if err != nil {
			log.Fatal(err)
		}
	}
	var otypes []*objc.Named
	if len(orefs.Refs) > 0 {
		otypes, err = objc.Import(orefs)
		if err != nil {
			log.Fatal(err)
		}
	}

	if len(classes) > 0 || len(otypes) > 0 {
		srcDir := *outdir
		if srcDir == "" {
			srcDir, err = ioutil.TempDir(os.TempDir(), "gobind-")
			if err != nil {
				log.Fatal(err)
			}
			defer os.RemoveAll(srcDir)
		} else {
			srcDir, err = filepath.Abs(srcDir)
			if err != nil {
				log.Fatal(err)
			}
		}
		if len(classes) > 0 {
			if err := genJavaPackages(srcDir, classes, jrefs.Embedders); err != nil {
				log.Fatal(err)
			}
		}
		if len(otypes) > 0 {
			if err := genObjcPackages(srcDir, otypes, orefs.Embedders); err != nil {
				log.Fatal(err)
			}
		}

		// Add a new directory to GOPATH where the file for reverse bindings exist, and recreate allPkg.
		// It is because the current allPkg did not solve imports for reverse bindings.
		var gopath string
		if out, err := exec.Command("go", "env", "GOPATH").Output(); err != nil {
			log.Fatal(err)
		} else {
			gopath = string(bytes.TrimSpace(out))
		}
		if gopath != "" {
			gopath = string(filepath.ListSeparator) + gopath
		}
		gopath = srcDir + gopath
		cfg.Env = append(os.Environ(), "GOPATH="+gopath)
		allPkg, err = packages.Load(cfg, flag.Args()...)
		if err != nil {
			log.Fatal(err)
		}
	}

	typePkgs := make([]*types.Package, len(allPkg))
	astPkgs := make([][]*ast.File, len(allPkg))
	for i, pkg := range allPkg {
		// Ignore pkg.Errors. pkg.Errors can exist when Cgo is used, but this should not affect the result.
		// See the discussion at golang/go#36547.
		typePkgs[i] = pkg.Types
		astPkgs[i] = pkg.Syntax
	}
	for _, l := range langs {
		for i, pkg := range typePkgs {
			genPkg(l, pkg, astPkgs[i], typePkgs, classes, otypes, *libname)
		}
		// Generate the error package and support files
		genPkg(l, nil, nil, typePkgs, classes, otypes, *libname)
	}
}

var exitStatus = 0

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
	exitStatus = 1
}
