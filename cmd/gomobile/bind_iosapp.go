// Copyright 2015 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"
)

func goAppleBind(gobind string, pkgs []*packages.Package, targets []targetInfo) error {
	var name string
	var title string

	if buildO == "" {
		name = pkgs[0].Name
		title = strings.Title(name)
		buildO = title + ".xcframework"
	} else {
		if !strings.HasSuffix(buildO, ".xcframework") {
			return fmt.Errorf("static framework name %q missing .xcframework suffix", buildO)
		}
		base := filepath.Base(buildO)
		name = base[:len(base)-len(".xcframework")]
		title = strings.Title(name)
	}

	if err := removeAll(buildO); err != nil {
		return err
	}

	outDirsForPlatform := map[string]string{}
	for _, t := range targets {
		outDirsForPlatform[t.platform] = filepath.Join(tmpdir, t.platform)
	}

	// Run the gobind command for each platform
	var gobindWG errgroup.Group
	for platform, outDir := range outDirsForPlatform {
		platform := platform
		outDir := outDir
		gobindWG.Go(func() error {
			// Catalyst support requires iOS 13+
			v, _ := strconv.ParseFloat(buildIOSVersion, 64)
			if platform == "maccatalyst" && v < 13.0 {
				return errors.New("catalyst requires -iosversion=13 or higher")
			}

			// Run gobind once per platform to generate the bindings
			cmd := exec.Command(
				gobind,
				"-lang=go,objc",
				"-outdir="+outDir,
			)
			cmd.Env = append(cmd.Env, "GOOS="+platformOS(platform))
			cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
			tags := append(buildTags[:], platformTags(platform)...)
			if platform == "macos" {
				tags = append(tags, buildTagsMacOS...)
			} else {
				tags = append(tags, buildTagsNotMacos...)
			}
			cmd.Args = append(cmd.Args, "-tags="+strings.Join(tags, ","))
			if bindPrefix != "" {
				cmd.Args = append(cmd.Args, "-prefix="+bindPrefix)
			}
			for _, p := range pkgs {
				cmd.Args = append(cmd.Args, p.PkgPath)
			}
			if err := runCmd(cmd); err != nil {
				return err
			}
			return nil
		})
	}
	if err := gobindWG.Wait(); err != nil {
		return err
	}

	modulesUsed, err := areGoModulesUsed()
	if err != nil {
		return err
	}

	// Build archive files.
	var buildWG errgroup.Group
	for _, t := range targets {
		t := t
		buildWG.Go(func() error {
			outDir := outDirsForPlatform[t.platform]
			outSrcDir := filepath.Join(outDir, "src", "gobind")

			if modulesUsed {
				newOutSrcDir, _ := filepath.Abs(filepath.Join(".", "build", t.platform+"-"+t.arch, "Libbox"))
				if !buildN {
					if err := doCopyAll(newOutSrcDir, outSrcDir); err != nil {
						return err
					}
				}
				outSrcDir = newOutSrcDir
				defer os.RemoveAll(outSrcDir)
			}

			// Copy the environment variables to make this function concurrent-safe.
			env := make([]string, len(appleEnv[t.String()]))
			copy(env, appleEnv[t.String()])

			// Add the generated packages to GOPATH for reverse bindings.
			gopath := fmt.Sprintf("GOPATH=%s%c%s", outDir, filepath.ListSeparator, goEnv("GOPATH"))
			env = append(env, gopath)

			// Build platform-specific tags
			tags := append(buildTags[:], platformTags(t.platform)...)
			if t.platform == "macos" {
				tags = append(tags, buildTagsMacOS...)
			} else {
				tags = append(tags, buildTagsNotMacos...)
			}

			if err := goAppleBindArchive(appleArchiveFilepath(name, t), env, outSrcDir, tags); err != nil {
				return fmt.Errorf("%s/%s: %v", t.platform, t.arch, err)
			}

			// Extract and merge external static libraries from CGO LDFLAGS
			pkgPaths := make([]string, len(pkgs))
			for i, p := range pkgs {
				pkgPaths[i] = p.PkgPath
			}
			externalLibraries, err := extractExternalStaticLibraries(env, outSrcDir, pkgPaths, tags)
			if err != nil {
				return fmt.Errorf("failed to extract external libraries for %s/%s: %v", t.platform, t.arch, err)
			}
			if len(externalLibraries) > 0 {
				archivePath := appleArchiveFilepath(name, t)
				mergedPath := archivePath + ".merged"
				if err := mergeStaticLibraries(archivePath, externalLibraries, mergedPath); err != nil {
					return fmt.Errorf("failed to merge static libraries for %s/%s: %v", t.platform, t.arch, err)
				}
				if err := os.Rename(mergedPath, archivePath); err != nil {
					return fmt.Errorf("failed to rename merged library: %v", err)
				}
			}

			return nil
		})
	}
	if err := buildWG.Wait(); err != nil {
		return err
	}

	var frameworkDirs []string
	frameworkArchCount := map[string]int{}
	for _, t := range targets {
		outDir := outDirsForPlatform[t.platform]
		gobindDir := filepath.Join(outDir, "src", "gobind")

		env := appleEnv[t.String()][:]
		sdk := getenv(env, "DARWIN_SDK")

		frameworkDir := filepath.Join(tmpdir, t.platform, sdk, title+".framework")
		frameworkDirs = append(frameworkDirs, frameworkDir)
		frameworkArchCount[frameworkDir] = frameworkArchCount[frameworkDir] + 1

		versionsDir := filepath.Join(frameworkDir, "Versions")
		versionsADir := filepath.Join(versionsDir, "A")
		titlePath := filepath.Join(versionsADir, title)
		if frameworkArchCount[frameworkDir] > 1 {
			// Not the first static lib, attach to a fat library and skip create headers
			fatCmd := exec.Command(
				"xcrun",
				"lipo", appleArchiveFilepath(name, t), titlePath, "-create", "-output", titlePath,
			)
			if err := runCmd(fatCmd); err != nil {
				return err
			}
			continue
		}

		versionsAHeadersDir := filepath.Join(versionsADir, "Headers")
		if err := mkdir(versionsAHeadersDir); err != nil {
			return err
		}
		if err := symlink("A", filepath.Join(versionsDir, "Current")); err != nil {
			return err
		}
		if err := symlink("Versions/Current/Headers", filepath.Join(frameworkDir, "Headers")); err != nil {
			return err
		}
		if err := symlink(filepath.Join("Versions/Current", title), filepath.Join(frameworkDir, title)); err != nil {
			return err
		}

		lipoCmd := exec.Command(
			"xcrun",
			"lipo", appleArchiveFilepath(name, t), "-create", "-o", titlePath,
		)
		if err := runCmd(lipoCmd); err != nil {
			return err
		}

		fileBases := make([]string, len(pkgs)+1)
		for i, pkg := range pkgs {
			fileBases[i] = bindPrefix + strings.Title(pkg.Name)
		}
		fileBases[len(fileBases)-1] = "Universe"

		// Copy header file next to output archive.
		var headerFiles []string
		if len(fileBases) == 1 {
			headerFiles = append(headerFiles, title+".h")
			err := copyFile(
				filepath.Join(versionsAHeadersDir, title+".h"),
				filepath.Join(gobindDir, bindPrefix+title+".objc.h"),
			)
			if err != nil {
				return err
			}
		} else {
			for _, fileBase := range fileBases {
				headerFiles = append(headerFiles, fileBase+".objc.h")
				err := copyFile(
					filepath.Join(versionsAHeadersDir, fileBase+".objc.h"),
					filepath.Join(gobindDir, fileBase+".objc.h"),
				)
				if err != nil {
					return err
				}
			}
			err := copyFile(
				filepath.Join(versionsAHeadersDir, "ref.h"),
				filepath.Join(gobindDir, "ref.h"),
			)
			if err != nil {
				return err
			}
			headerFiles = append(headerFiles, title+".h")
			err = writeFile(filepath.Join(versionsAHeadersDir, title+".h"), func(w io.Writer) error {
				return appleBindHeaderTmpl.Execute(w, map[string]interface{}{
					"pkgs": pkgs, "title": title, "bases": fileBases,
				})
			})
			if err != nil {
				return err
			}
		}

		if err := mkdir(filepath.Join(versionsADir, "Resources")); err != nil {
			return err
		}
		if err := symlink("Versions/Current/Resources", filepath.Join(frameworkDir, "Resources")); err != nil {
			return err
		}
		err = writeFile(filepath.Join(frameworkDir, "Resources", "Info.plist"), func(w io.Writer) error {
			_, err := w.Write([]byte(appleBindInfoPlist))
			return err
		})
		if err != nil {
			return err
		}

		var mmVals = struct {
			Module  string
			Headers []string
		}{
			Module:  title,
			Headers: headerFiles,
		}
		err = writeFile(filepath.Join(versionsADir, "Modules", "module.modulemap"), func(w io.Writer) error {
			return appleModuleMapTmpl.Execute(w, mmVals)
		})
		if err != nil {
			return err
		}
		err = symlink(filepath.Join("Versions/Current/Modules"), filepath.Join(frameworkDir, "Modules"))
		if err != nil {
			return err
		}
	}

	// Finally combine all frameworks to an XCFramework
	xcframeworkArgs := []string{"-create-xcframework"}

	for _, dir := range frameworkDirs {
		// On macOS, a temporary directory starts with /var, which is a symbolic link to /private/var.
		// And in gomobile, a temporary directory is usually used as a working directly.
		// Unfortunately, xcodebuild in Xcode 15 seems to have a bug and might not be able to understand fullpaths with symbolic links.
		// As a workaround, resolve the path with symbolic links by filepath.EvalSymlinks.
		dir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		xcframeworkArgs = append(xcframeworkArgs, "-framework", dir)
	}

	xcframeworkArgs = append(xcframeworkArgs, "-output", buildO)
	cmd := exec.Command("xcodebuild", xcframeworkArgs...)
	err = runCmd(cmd)
	return err
}

const appleBindInfoPlist = `<?xml version="1.0" encoding="UTF-8"?>
    <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
    <plist version="1.0">
      <dict>
      </dict>
    </plist>
`

var appleModuleMapTmpl = template.Must(template.New("iosmmap").Parse(`framework module "{{.Module}}" {
	header "ref.h"
{{range .Headers}}    header "{{.}}"
{{end}}
    export *
}`))

func appleArchiveFilepath(name string, t targetInfo) string {
	return filepath.Join(tmpdir, name+"-"+t.platform+"-"+t.arch+".a")
}

func goAppleBindArchive(out string, env []string, gosrc string, tags []string) error {
	cmd := exec.Command("go", "build", "-buildmode=c-archive", "-o", out)
	if len(tags) > 0 {
		cmd.Args = append(cmd.Args, "-tags="+strings.Join(tags, ","))
	}
	if buildV {
		cmd.Args = append(cmd.Args, "-v")
	}
	if buildX {
		cmd.Args = append(cmd.Args, "-x")
	}
	if buildGcflags != "" {
		cmd.Args = append(cmd.Args, "-gcflags", buildGcflags)
	}
	if buildLdflags != "" {
		cmd.Args = append(cmd.Args, "-ldflags", buildLdflags)
	}
	if buildTrimpath {
		cmd.Args = append(cmd.Args, "-trimpath")
	}
	if buildWork {
		cmd.Args = append(cmd.Args, "-work")
	}
	if !buildVCS {
		cmd.Args = append(cmd.Args, "-buildvcs=false")
	}
	cmd.Args = append(cmd.Args, ".")
	cmd.Dir = gosrc
	cmd.Env = append(os.Environ(), env...)
	return runCmd(cmd)
}

// extractExternalStaticLibraries extracts static library paths from CGO LDFLAGS
// of all dependencies. This is needed because go build -buildmode=c-archive
// does not include external static libraries specified in CGO LDFLAGS.
func extractExternalStaticLibraries(env []string, gosrc string, pkgPaths []string, tags []string) ([]string, error) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{range .CgoLDFLAGS}}{{println .}}{{end}}")
	if len(tags) > 0 {
		cmd.Args = append(cmd.Args, "-tags="+strings.Join(tags, ","))
	}
	cmd.Args = append(cmd.Args, pkgPaths...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = gosrc

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("go list stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("go list start failed: %w", err)
	}

	seen := make(map[string]bool)
	var libraries []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		flag := strings.TrimSpace(scanner.Text())
		if flag == "" {
			continue
		}
		// Only include .a files (static libraries)
		if strings.HasSuffix(flag, ".a") && !seen[flag] {
			seen[flag] = true
			libraries = append(libraries, flag)
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return nil, fmt.Errorf("failed to parse go list output: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		if errMsg := strings.TrimSpace(stderr.String()); errMsg != "" {
			return nil, fmt.Errorf("go list failed: %w: %s", err, errMsg)
		}
		return nil, fmt.Errorf("go list failed: %w", err)
	}

	return libraries, nil
}

// mergeStaticLibraries merges the Go archive with external static libraries
// using libtool. This creates a single archive containing all symbols.
func mergeStaticLibraries(goArchive string, externalLibraries []string, output string) error {
	args := []string{"libtool", "-static", "-o", output, goArchive}
	args = append(args, externalLibraries...)
	cmd := exec.Command("xcrun", args...)
	return runCmd(cmd)
}

var appleBindHeaderTmpl = template.Must(template.New("apple.h").Parse(`
// Objective-C API for talking to the following Go packages
//
{{range .pkgs}}//	{{.PkgPath}}
{{end}}//
// File is generated by gomobile bind. Do not edit.
#ifndef __{{.title}}_FRAMEWORK_H__
#define __{{.title}}_FRAMEWORK_H__

{{range .bases}}#include "{{.}}.objc.h"
{{end}}
#endif
`))
