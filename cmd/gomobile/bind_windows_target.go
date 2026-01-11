package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

func goWindowsBind(libName string, gobind string, pkgs []*packages.Package, targets []targetInfo) error {
	if len(targets) != 1 {
		return fmt.Errorf("windows binding requires a single architecture; use -target=windows/386, windows/amd64 or windows/arm64")
	}
	target := targets[0]

	cmd := exec.Command(
		gobind,
		"-lang=go,csharp",
		"-outdir="+tmpdir,
	)
	cmd.Env = append(cmd.Env, "GOOS=windows", "GOARCH="+target.arch, "CGO_ENABLED=1")
	if len(buildTags) > 0 {
		cmd.Args = append(cmd.Args, "-tags="+strings.Join(buildTags, ","))
	}
	if bindCSharpPkg != "" {
		cmd.Args = append(cmd.Args, "-cspkg="+bindCSharpPkg)
	}
	if bindCSharpNamespace != "" {
		cmd.Args = append(cmd.Args, "-csnamespace="+bindCSharpNamespace)
	}
	if bindCSharpPackageName != "" {
		cmd.Args = append(cmd.Args, "-cspkgname="+bindCSharpPackageName)
	}
	if libName != "" {
		cmd.Args = append(cmd.Args, "-libname="+libName)
	}
	for _, p := range pkgs {
		cmd.Args = append(cmd.Args, p.PkgPath)
	}
	if err := runCmd(cmd); err != nil {
		return err
	}

	srcDir := filepath.Join(tmpdir, "src", "gobind")
	modulesUsed, err := areGoModulesUsed()
	if err != nil {
		return err
	}
	if modulesUsed {
		newSrcDir, _ := filepath.Abs(filepath.Join(".", "build", "windows-"+target.arch, "lib"+libName))
		if !buildN {
			if err := doCopyAll(newSrcDir, srcDir); err != nil {
				return err
			}
		}
		srcDir = newSrcDir
		defer os.RemoveAll(srcDir)
	}

	dllName := libName
	if dllName == "" {
		dllName = pkgs[0].Name
	}
	if buildO == "" {
		buildO = dllName + ".dll"
	}
	if !strings.HasSuffix(buildO, ".dll") {
		return fmt.Errorf("output file name %q does not end in '.dll'", buildO)
	}
	if err := mkdir(filepath.Dir(buildO)); err != nil {
		return err
	}

	env := []string{"GOOS=windows", "GOARCH=" + target.arch, "CGO_ENABLED=1"}
	gopath := fmt.Sprintf("GOPATH=%s%c%s", tmpdir, filepath.ListSeparator, goEnv("GOPATH"))
	env = append(env, gopath)

	if err := goBuildAt(
		srcDir,
		".",
		env,
		"-buildmode=c-shared",
		"-o="+buildO,
	); err != nil {
		return err
	}

	csharpDir := filepath.Join(tmpdir, "csharp")
	outDir := strings.TrimSuffix(buildO, ".dll") + "-csharp"
	if err := removeAll(outDir); err != nil {
		return err
	}
	if !buildN {
		if err := doCopyAll(outDir, csharpDir); err != nil {
			return err
		}
	}
	return nil
}
