// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bind

import (
	"fmt"
	"go/constant"
	"go/types"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type CSharpGen struct {
	*Generator
	CSharpPkg         string
	CSharpNamespace   string
	CSharpPackageName string
	LibraryName       string
}

var csharpIdentifierRe = regexp.MustCompile(`[^A-Za-z0-9_]`)

var csharpNameReplacer = newNameSanitizer([]string{
	"abstract", "as", "base", "bool", "break", "byte", "case", "catch", "char",
	"checked", "class", "const", "continue", "decimal", "default", "delegate", "do",
	"double", "else", "enum", "event", "explicit", "extern", "false", "finally",
	"fixed", "float", "for", "foreach", "goto", "if", "implicit", "in", "int",
	"interface", "internal", "is", "lock", "long", "namespace", "new", "null",
	"object", "operator", "out", "override", "params", "private", "protected",
	"public", "readonly", "ref", "return", "sbyte", "sealed", "short", "sizeof",
	"stackalloc", "static", "string", "struct", "switch", "this", "throw", "true",
	"try", "typeof", "uint", "ulong", "unchecked", "unsafe", "ushort", "using",
	"virtual", "void", "volatile", "while", "record", "init", "when", "yield", "add",
	"remove", "value", "var", "dynamic",
})

func (g *CSharpGen) Init() {
	g.Generator.Init()
}

func (g *CSharpGen) gobindOpts() string {
	opts := []string{"-lang=csharp"}
	if g.CSharpPkg != "" {
		opts = append(opts, "-cspkg="+g.CSharpPkg)
	}
	if g.CSharpNamespace != "" {
		opts = append(opts, "-csnamespace="+g.CSharpNamespace)
	}
	if g.CSharpPackageName != "" {
		opts = append(opts, "-cspkgname="+g.CSharpPackageName)
	}
	if g.LibraryName != "" {
		opts = append(opts, "-libname="+g.LibraryName)
	}
	return strings.Join(opts, " ")
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

func (g *CSharpGen) rootNamespace() string {
	root := g.CSharpNamespace
	if root == "" {
		root = g.CSharpPkg
	}
	if root == "" {
		root = "Go"
	}
	parts := strings.Split(root, ".")
	for i, part := range parts {
		parts[i] = g.csIdentifier(part)
	}
	return strings.Join(parts, ".")
}

func (g *CSharpGen) csNamespace(pkg *types.Package) string {
	if pkg == nil {
		return g.rootNamespace()
	}
	if g.CSharpNamespace != "" {
		return g.rootNamespace()
	}
	return g.rootNamespace() + "." + g.csIdentifier(upperFirst(pkg.Name()))
}

func (g *CSharpGen) csIdentifier(name string) string {
	if name == "" {
		return name
	}
	name = csharpNameReplacer(name)
	name = csharpIdentifierRe.ReplaceAllString(name, "_")
	if name == "" {
		return "_"
	}
	r, _ := utf8.DecodeRuneInString(name)
	if r != '_' && !unicode.IsLetter(r) {
		name = "_" + name
	}
	return name
}

func (g *CSharpGen) csInterfaceName(obj *types.TypeName) string {
	if obj != nil && obj.Pkg() == nil && obj.Name() == "error" {
		return "Error"
	}
	return g.csIdentifier(obj.Name())
}

func (g *CSharpGen) csProxyInterfaceName(obj *types.TypeName) string {
	if obj != nil && obj.Pkg() == nil && obj.Name() == "error" {
		return "ProxyError"
	}
	return g.csIdentifier("Proxy" + obj.Name())
}

func (g *CSharpGen) csPackageClassName(pkg *types.Package) string {
	if pkg == nil {
		return "Universe"
	}
	if g.CSharpPackageName != "" {
		return g.csIdentifier(g.CSharpPackageName)
	}
	return g.csIdentifier(upperFirst(pkg.Name()))
}

func (g *CSharpGen) csType(t types.Type) string {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.UntypedBool:
			return "bool"
		case types.Int:
			return "long"
		case types.Int8:
			return "sbyte"
		case types.Int16:
			return "short"
		case types.Int32, types.UntypedRune:
			return "int"
		case types.Int64, types.UntypedInt:
			return "long"
		case types.Uint8:
			return "byte"
		case types.Float32:
			return "float"
		case types.Float64, types.UntypedFloat:
			return "double"
		case types.String, types.UntypedString:
			return "string"
		default:
			g.errorf("unsupported basic type: %s", t)
		}
	case *types.Slice:
		elem, ok := t.Elem().(*types.Basic)
		if ok && elem.Kind() == types.Uint8 {
			return "byte[]"
		}
		g.errorf("unsupported slice type: %s", t)
	case *types.Pointer:
		return g.csType(t.Elem())
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() == nil && obj.Name() == "error" {
			return "Error"
		}
		name := g.csIdentifier(obj.Name())
		if obj.Pkg() == nil || obj.Pkg() == g.Pkg {
			return name
		}
		if !g.validPkg(obj.Pkg()) {
			g.errorf("type %s is defined in %s, which is not bound", t, obj.Pkg())
			return name
		}
		return g.csNamespace(obj.Pkg()) + "." + name
	default:
		g.errorf("unsupported type: %s", t)
	}
	return "object"
}

func (g *CSharpGen) csNativeType(t types.Type) string {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.UntypedBool:
			return "byte"
		case types.Int:
			return "long"
		case types.Int8:
			return "sbyte"
		case types.Int16:
			return "short"
		case types.Int32, types.UntypedRune:
			return "int"
		case types.Int64, types.UntypedInt:
			return "long"
		case types.Uint8:
			return "byte"
		case types.Float32:
			return "float"
		case types.Float64, types.UntypedFloat:
			return "double"
		case types.String, types.UntypedString:
			return "NString"
		default:
			g.errorf("unsupported basic type: %s", t)
		}
	case *types.Slice:
		elem, ok := t.Elem().(*types.Basic)
		if ok && elem.Kind() == types.Uint8 {
			return "NByteslice"
		}
		g.errorf("unsupported slice type: %s", t)
	case *types.Pointer:
		return "int"
	case *types.Named:
		return "int"
	default:
		g.errorf("unsupported type: %s", t)
	}
	return "int"
}

func (g *CSharpGen) csReturnStructName(cName string) string {
	return g.csIdentifier("CProxy_" + cName + "_Return")
}

func (g *CSharpGen) csConstValue(o *types.Const) string {
	basic, ok := o.Type().(*types.Basic)
	if !ok {
		return ""
	}
	kind := basic.Kind()
	switch kind {
	case types.Bool, types.UntypedBool:
		if constant.BoolVal(o.Val()) {
			return "true"
		}
		return "false"
	case types.String, types.UntypedString:
		return strconv.Quote(constant.StringVal(o.Val()))
	case types.Float32:
		f, _ := constant.Float64Val(o.Val())
		return strconv.FormatFloat(f, 'g', -1, 32) + "f"
	case types.Float64, types.UntypedFloat:
		f, _ := constant.Float64Val(o.Val())
		return strconv.FormatFloat(f, 'g', -1, 64)
	case types.Int, types.Int64, types.UntypedInt:
		i, _ := constant.Int64Val(o.Val())
		return fmt.Sprintf("%dL", i)
	case types.Int8, types.Int16, types.Int32, types.UntypedRune, types.Uint8:
		i, _ := constant.Int64Val(o.Val())
		return fmt.Sprintf("(%s)%d", g.csType(o.Type()), i)
	default:
		return o.Val().ExactString()
	}
}

func (g *CSharpGen) paramName(params *types.Tuple, pos int) string {
	return g.csIdentifier(basicParamName(params, pos))
}

func (g *CSharpGen) cproxySetterName(pkgPrefix, ifaceName, methodName string) string {
	return fmt.Sprintf("go_seq_set_cproxy%s_%s_%s", pkgPrefix, ifaceName, methodName)
}

func (g *CSharpGen) proxyFuncName(objName, funcName string) string {
	return fmt.Sprintf("proxy%s_%s_%s", g.pkgPrefix, objName, funcName)
}

// isConsSigSupported reports whether the generators can handle a given
// constructor signature. It skips constructors taking a single int32/uint32
// argument since they clash with the proxy constructors that take a refnum.
func (g *CSharpGen) isConsSigSupported(t types.Type) bool {
	if !g.isSigSupported(t) {
		return false
	}
	params := t.(*types.Signature).Params()
	if params.Len() != 1 {
		return true
	}
	if basicType, ok := params.At(0).Type().(*types.Basic); ok {
		switch basicType.Kind() {
		case types.Int32, types.Uint32:
			return false
		}
	}
	return true
}

func (g *CSharpGen) newFuncName(structName string) string {
	if g.Pkg == nil {
		return ""
	}
	return fmt.Sprintf("new_%s_%s", g.Pkg.Name(), structName)
}

func (g *CSharpGen) GenCSharp() error {
	pkgPath := ""
	if g.Pkg != nil {
		pkgPath = g.Pkg.Path()
	}
	g.Printf(csharpPreamble, g.gobindOpts(), pkgPath)
	g.emitUsings()

	rootNamespace := g.rootNamespace()
	g.emitRootNamespace(rootNamespace)
	g.emitPackageNamespace(rootNamespace)

	if len(g.err) > 0 {
		return g.err
	}
	return nil
}

func (g *CSharpGen) emitUsings() {
	g.Printf("using System;\n")
	g.Printf("using System.Collections.Concurrent;\n")
	g.Printf("using System.Collections.Generic;\n")
	g.Printf("using System.Runtime.InteropServices;\n")
	g.Printf("using System.Text;\n\n")
}

func (g *CSharpGen) emitRootNamespace(rootNamespace string) {
	g.Printf("namespace %s {\n", rootNamespace)
	g.Indent()

	g.emitSharedNativeStructs()

	returnStructs := g.collectReturnStructs()
	g.emitReturnStructs(returnStructs)

	g.emitNativeClass()

	if g.Pkg == nil {
		g.genSeqSupport()
	}

	g.Outdent()
	g.Printf("}\n\n")
}

func (g *CSharpGen) emitSharedNativeStructs() {
	if g.Pkg != nil {
		return
	}

	g.Printf("[StructLayout(LayoutKind.Sequential)]\n")
	g.Printf("internal struct NString {\n")
	g.Indent()
	g.Printf("public IntPtr ptr;\n")
	g.Printf("public int len;\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("[StructLayout(LayoutKind.Sequential)]\n")
	g.Printf("internal struct NByteslice {\n")
	g.Indent()
	g.Printf("public IntPtr ptr;\n")
	g.Printf("public int len;\n")
	g.Outdent()
	g.Printf("}\n\n")
}

func (g *CSharpGen) collectReturnStructs() map[string][]types.Type {
	returnStructs := make(map[string][]types.Type)
	collectStruct := func(cName string, res *types.Tuple) {
		if res == nil || res.Len() <= 1 {
			return
		}
		if _, exists := returnStructs[cName]; exists {
			return
		}
		var fields []types.Type
		for i := 0; i < res.Len(); i++ {
			fields = append(fields, res.At(i).Type())
		}
		returnStructs[cName] = fields
	}
	for _, f := range g.funcs {
		collectStruct(g.proxyFuncName("", f.Name()), f.Type().(*types.Signature).Results())
	}
	for _, s := range g.structs {
		methods := exportedMethodSet(types.NewPointer(s.obj.Type()))
		for _, m := range methods {
			collectStruct(g.proxyFuncName(s.obj.Name(), m.Name()), m.Type().(*types.Signature).Results())
		}
	}
	for _, iface := range g.interfaces {
		for _, m := range iface.summary.callable {
			collectStruct(g.proxyFuncName(iface.obj.Name(), m.Name()), m.Type().(*types.Signature).Results())
			collectStruct(fmt.Sprintf("cproxy%s_%s_%s", g.pkgPrefix, iface.obj.Name(), m.Name()), m.Type().(*types.Signature).Results())
		}
	}
	return returnStructs
}

func (g *CSharpGen) emitReturnStructs(returnStructs map[string][]types.Type) {
	var structNames []string
	for name := range returnStructs {
		structNames = append(structNames, name)
	}
	sort.Strings(structNames)
	for _, name := range structNames {
		structName := g.csReturnStructName(name)
		g.Printf("[StructLayout(LayoutKind.Sequential)]\n")
		g.Printf("internal struct %s {\n", structName)
		g.Indent()
		fields := returnStructs[name]
		for i, field := range fields {
			g.Printf("public %s r%d;\n", g.csNativeType(field), i)
		}
		g.Outdent()
		g.Printf("}\n\n")
	}
}

func (g *CSharpGen) emitNativeClass() {
	g.Printf("internal static partial class Native {\n")
	g.Indent()
	if g.Pkg == nil {
		libName := g.LibraryName
		if libName == "" {
			libName = "gojni"
		}
		g.Printf("internal const string LibraryName = %q;\n\n", libName)
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern void go_seq_init();\n\n")
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern void DestroyRef(int refnum);\n\n")
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern void IncGoRef(int refnum);\n\n")
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern IntPtr GoSeqAlloc(int size);\n\n")
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern void GoSeqFree(IntPtr ptr);\n\n")
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern void go_seq_set_inc_ref(IntPtr fn);\n\n")
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern void go_seq_set_dec_ref(IntPtr fn);\n\n")
	}

	for _, v := range g.vars {
		if !g.isSupported(v.Type()) {
			continue
		}
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern void var_set%s_%s(%s v);\n\n", g.pkgPrefix, v.Name(), g.csNativeType(v.Type()))
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern %s var_get%s_%s();\n\n", g.csNativeType(v.Type()), g.pkgPrefix, v.Name())
	}

	for _, f := range g.funcs {
		if !g.isSigSupported(f.Type()) {
			continue
		}
		sig := f.Type().(*types.Signature)
		cName := g.proxyFuncName("", f.Name())
		retType := "void"
		if sig.Results().Len() == 1 {
			retType = g.csNativeType(sig.Results().At(0).Type())
		} else if sig.Results().Len() == 2 {
			retType = g.csReturnStructName(cName)
		}
		g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
		g.Printf("internal static extern %s %s(", retType, cName)
		params := sig.Params()
		for i := 0; i < params.Len(); i++ {
			if i > 0 {
				g.Printf(", ")
			}
			g.Printf("%s %s", g.csNativeType(params.At(i).Type()), g.paramName(params, i))
		}
		g.Printf(");\n\n")
	}

	for _, s := range g.structs {
		for _, m := range exportedMethodSet(types.NewPointer(s.obj.Type())) {
			if !g.isSigSupported(m.Type()) {
				continue
			}
			sig := m.Type().(*types.Signature)
			cName := g.proxyFuncName(s.obj.Name(), m.Name())
			retType := "void"
			if sig.Results().Len() == 1 {
				retType = g.csNativeType(sig.Results().At(0).Type())
			} else if sig.Results().Len() == 2 {
				retType = g.csReturnStructName(cName)
			}
			g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
			g.Printf("internal static extern %s %s(int refnum", retType, cName)
			params := sig.Params()
			for i := 0; i < params.Len(); i++ {
				g.Printf(", %s %s", g.csNativeType(params.At(i).Type()), g.paramName(params, i))
			}
			g.Printf(");\n\n")
		}
	}

	for _, iface := range g.interfaces {
		for _, m := range iface.summary.callable {
			if !g.isSigSupported(m.Type()) {
				continue
			}
			cName := g.cproxySetterName(g.pkgPrefix, iface.obj.Name(), m.Name())
			g.Printf("[DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)]\n")
			g.Printf("internal static extern void %s(IntPtr fn);\n\n", cName)
		}
	}
	g.Outdent()
	g.Printf("}\n\n")
}

func (g *CSharpGen) emitPackageNamespace(rootNamespace string) {
	pkgNamespace := g.csNamespace(g.Pkg)
	g.Printf("namespace %s {\n", pkgNamespace)
	g.Indent()

	pkgClassName := g.csPackageClassName(g.Pkg)
	g.Printf("public static class %s {\n", pkgClassName)
	g.Indent()
	g.Printf("static %s() { %s.Seq.Touch(); }\n\n", pkgClassName, rootNamespace)

	g.emitPackageConstants()
	g.emitPackageVariables(rootNamespace)
	g.emitPackageFunctions(rootNamespace)

	g.Outdent()
	g.Printf("}\n\n")

	for _, s := range g.structs {
		g.genStructClass(rootNamespace, s)
	}

	for _, iface := range g.interfaces {
		g.genInterface(rootNamespace, iface)
	}

	g.Outdent()
	g.Printf("}\n")
}

func (g *CSharpGen) emitPackageConstants() {
	for _, c := range g.constants {
		if _, ok := c.Type().(*types.Basic); !ok || !g.isSupported(c.Type()) {
			continue
		}
		g.Printf("public const %s %s = %s;\n", g.csType(c.Type()), g.csIdentifier(c.Name()), g.csConstValue(c))
	}
	if len(g.constants) > 0 {
		g.Printf("\n")
	}
}

func (g *CSharpGen) emitPackageVariables(rootNamespace string) {
	for _, v := range g.vars {
		if !g.isSupported(v.Type()) {
			continue
		}
		propName := g.csIdentifier(v.Name())
		g.Printf("public static %s %s {\n", g.csType(v.Type()), propName)
		g.Indent()
		g.Printf("get {\n")
		g.Indent()
		g.Printf("var res = %s.Native.var_get%s_%s();\n", rootNamespace, g.pkgPrefix, v.Name())
		g.emitFromNativeReturn("res", v.Type(), true)
		g.Outdent()
		g.Printf("}\n")
		g.Printf("set {\n")
		g.Indent()
		g.emitToNativeParam("value", v.Type(), modeRetained)
		g.Printf("%s.Native.var_set%s_%s(%s);\n", rootNamespace, g.pkgPrefix, v.Name(), g.nativeParamName("value"))
		g.Outdent()
		g.Printf("}\n")
		g.Outdent()
		g.Printf("}\n\n")
	}
}

func (g *CSharpGen) emitPackageFunctions(rootNamespace string) {
	for _, f := range g.funcs {
		if !g.isSigSupported(f.Type()) {
			continue
		}
		sig := f.Type().(*types.Signature)
		params := sig.Params()
		res := sig.Results()

		returnsError := false
		var returnType string
		switch res.Len() {
		case 0:
			returnType = "void"
		case 1:
			if isErrorType(res.At(0).Type()) {
				returnsError = true
				returnType = "void"
			} else {
				returnType = g.csType(res.At(0).Type())
			}
		case 2:
			if isErrorType(res.At(1).Type()) {
				returnsError = true
				returnType = g.csType(res.At(0).Type())
			} else {
				g.errorf("second result must be error: %s", f.Name())
				continue
			}
		default:
			g.errorf("too many result values: %s", f.Name())
			continue
		}

		g.Printf("public static %s %s(", returnType, g.csIdentifier(f.Name()))
		for i := 0; i < params.Len(); i++ {
			if i > 0 {
				g.Printf(", ")
			}
			g.Printf("%s %s", g.csType(params.At(i).Type()), g.paramName(params, i))
		}
		g.Printf(") {\n")
		g.Indent()
		for i := 0; i < params.Len(); i++ {
			g.emitToNativeParam(g.paramName(params, i), params.At(i).Type(), modeTransient)
		}
		cName := g.proxyFuncName("", f.Name())
		if res.Len() > 0 {
			g.Printf("var res = %s.Native.%s(", rootNamespace, cName)
		} else {
			g.Printf("%s.Native.%s(", rootNamespace, cName)
		}
		for i := 0; i < params.Len(); i++ {
			if i > 0 {
				g.Printf(", ")
			}
			g.Printf("%s", g.nativeParamName(g.paramName(params, i)))
		}
		g.Printf(");\n")
		for i := 0; i < params.Len(); i++ {
			if g.shouldFreeAfterCall(params.At(i).Type(), modeTransient) {
				g.emitFreeNativeParam(g.paramName(params, i))
			}
		}

		if res.Len() == 0 {
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if res.Len() == 1 && !returnsError {
			g.emitFromNativeReturn("res", res.At(0).Type(), true)
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if returnsError {
			if res.Len() == 2 {
				if isRefnumType(res.At(0).Type()) {
					g.Printf("if (res.r1 != %s.Seq.NullRefNum) {\n", rootNamespace)
					g.Indent()
					g.Printf("%s.Seq.DestroyRef(res.r0);\n", rootNamespace)
					g.Printf("%s.Seq.ThrowIfError(res.r1);\n", rootNamespace)
					g.Outdent()
					g.Printf("}\n")
				}
				g.emitFromNativeValue("value", "res.r0", res.At(0).Type(), true)
				if !isRefnumType(res.At(0).Type()) {
					g.Printf("%s.Seq.ThrowIfError(res.r1);\n", rootNamespace)
				}
				g.Printf("return value;\n")
			} else {
				g.Printf("%s.Seq.ThrowIfError(res);\n", rootNamespace)
			}
		}

		g.Outdent()
		g.Printf("}\n\n")
	}
}
func (g *CSharpGen) nativeParamName(name string) string {
	return "_" + name
}

func (g *CSharpGen) emitToNativeParam(name string, t types.Type, mode varMode) {
	paramName := g.nativeParamName(name)
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.UntypedBool:
			g.Printf("var %s = %s ? (byte)1 : (byte)0;\n", paramName, name)
		case types.String, types.UntypedString:
			g.Printf("var %s = %s.Seq.StringToNString(%s);\n", paramName, g.rootNamespace(), name)
		default:
			g.Printf("var %s = (%s)%s;\n", paramName, g.csNativeType(t), name)
		}
	case *types.Slice:
		g.Printf("var %s = %s.Seq.BytesToNByteslice(%s);\n", paramName, g.rootNamespace(), name)
	case *types.Pointer, *types.Named:
		if named, ok := indirectNamed(t); ok {
			if iface, ok := named.Underlying().(*types.Interface); ok && makeIfaceSummary(iface).implementable {
				proxyName := g.interfaceProxyQualifiedName(named)
				g.Printf("%s.EnsureRegistered();\n", proxyName)
			}
		}
		g.Printf("var %s = %s.Seq.IncRef(%s);\n", paramName, g.rootNamespace(), name)
	default:
		g.Printf("var %s = (%s)%s;\n", paramName, g.csNativeType(t), name)
	}

	if mode == modeRetained {
		return
	}
}

func (g *CSharpGen) emitEnsureRegisteredForReturn(t types.Type) {
	if named, ok := indirectNamed(t); ok {
		if iface, ok := named.Underlying().(*types.Interface); ok && makeIfaceSummary(iface).implementable {
			g.Printf("%s.EnsureRegistered();\n", g.interfaceProxyQualifiedName(named))
		}
	}
}

func (g *CSharpGen) shouldFreeAfterCall(t types.Type, mode varMode) bool {
	if mode == modeRetained {
		return false
	}
	slice, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := slice.Elem().(*types.Basic)
	if !ok {
		return false
	}
	return basic.Kind() == types.Uint8
}

func (g *CSharpGen) emitFreeNativeParam(name string) {
	paramName := g.nativeParamName(name)
	g.Printf("if (%s.ptr != IntPtr.Zero) { %s.Seq.Free(%s.ptr); }\n", paramName, g.rootNamespace(), paramName)
}

func (g *CSharpGen) emitFromNativeReturn(source string, t types.Type, freeBytes bool) {
	g.Printf("return %s;\n", g.managedFromNativeExpr(source, t, freeBytes))
}

func (g *CSharpGen) emitFromNativeValue(target string, source string, t types.Type, freeBytes bool) {
	g.Printf("var %s = %s;\n", target, g.managedFromNativeExpr(source, t, freeBytes))
}

func (g *CSharpGen) managedFromNativeExpr(source string, t types.Type, freeBytes bool) string {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.UntypedBool:
			return source + " != 0"
		case types.String, types.UntypedString:
			return fmt.Sprintf("%s.Seq.NStringToString(%s)", g.rootNamespace(), source)
		default:
			return fmt.Sprintf("(%s)%s", g.csType(t), source)
		}
	case *types.Slice:
		if freeBytes {
			return fmt.Sprintf("%s.Seq.NBytesliceToBytes(%s, true)", g.rootNamespace(), source)
		}
		return fmt.Sprintf("%s.Seq.NBytesliceToBytes(%s, false)", g.rootNamespace(), source)
	case *types.Pointer, *types.Named:
		if named, ok := indirectNamed(t); ok {
			if _, ok := named.Underlying().(*types.Interface); ok {
				return fmt.Sprintf("%s.FromRefnum(%s)", g.interfaceProxyQualifiedName(named), source)
			}
			return fmt.Sprintf("%s.FromRefnum(%s)", g.namedQualifiedName(named), source)
		}
	}
	return "null"
}

func (g *CSharpGen) namedQualifiedName(named *types.Named) string {
	obj := named.Obj()
	name := g.csIdentifier(obj.Name())
	if obj.Pkg() == nil || obj.Pkg() == g.Pkg {
		return name
	}
	return g.csNamespace(obj.Pkg()) + "." + name
}

func (g *CSharpGen) interfaceProxyQualifiedName(named *types.Named) string {
	obj := named.Obj()
	proxyName := g.csProxyInterfaceName(obj)
	if obj.Pkg() == nil || obj.Pkg() == g.Pkg {
		return proxyName
	}
	return g.csNamespace(obj.Pkg()) + "." + proxyName
}

func indirectNamed(t types.Type) (*types.Named, bool) {
	switch t := t.(type) {
	case *types.Pointer:
		return indirectNamed(t.Elem())
	case *types.Named:
		return t, true
	default:
		return nil, false
	}
}

// isRefnumType returns true if the type is represented by a refnum (Go object reference)
// and would need cleanup if not properly managed.
func isRefnumType(t types.Type) bool {
	switch t := t.(type) {
	case *types.Pointer:
		_, ok := t.Elem().(*types.Named)
		return ok
	case *types.Named:
		switch t.Underlying().(type) {
		case *types.Interface, *types.Struct:
			return true
		}
	}
	return false
}

func (g *CSharpGen) genStructClass(rootNamespace string, s structInfo) {
	name := g.csIdentifier(s.obj.Name())
	g.Printf("public sealed class %s : %s.Seq.IProxy, IDisposable {\n", name, rootNamespace)
	g.Indent()
	g.Printf("private readonly int refnum;\n")
	g.Printf("private int disposed;\n\n")

	g.Printf("internal %s(int refnum) { this.refnum = refnum; }\n\n", name)
	var constructors []*types.Func
	hasDefaultConstructor := false
	for _, f := range g.funcs {
		if t := g.constructorType(f); t != nil && t == s.obj {
			if !g.isConsSigSupported(f.Type()) {
				continue
			}
			constructors = append(constructors, f)
			if f.Type().(*types.Signature).Params().Len() == 0 {
				hasDefaultConstructor = true
			}
		}
	}

	if !hasDefaultConstructor {
		newName := g.newFuncName(s.obj.Name())
		if newName != "" {
			g.Printf("public %s() {\n", name)
			g.Indent()
			g.Printf("refnum = %s.Native.%s();\n", rootNamespace, newName)
			g.Outdent()
			g.Printf("}\n\n")
		}
	}

	for _, f := range constructors {
		g.genConstructorFromFunc(rootNamespace, name, f)
	}

	g.Printf("~%s() { Dispose(false); }\n\n", name)
	g.Printf("public void Dispose() {\n")
	g.Indent()
	g.Printf("Dispose(true);\n")
	g.Printf("GC.SuppressFinalize(this);\n")
	g.Outdent()
	g.Printf("}\n\n")
	g.Printf("private void Dispose(bool disposing) {\n")
	g.Indent()
	g.Printf("if (System.Threading.Interlocked.Exchange(ref disposed, 1) != 0) { return; }\n")
	g.Printf("%s.Seq.DestroyRef(refnum);\n", rootNamespace)
	g.Outdent()
	g.Printf("}\n\n")
	g.Printf("private void ThrowIfDisposed() {\n")
	g.Indent()
	g.Printf("if (disposed != 0) { throw new ObjectDisposedException(GetType().FullName); }\n")
	g.Outdent()
	g.Printf("}\n\n")
	g.Printf("public int Refnum { get { ThrowIfDisposed(); return refnum; } }\n\n")
	g.Printf("public int IncRefnum() { ThrowIfDisposed(); %s.Seq.IncGoRef(refnum, this); return refnum; }\n\n", rootNamespace)

	g.Printf("internal static %s FromRefnum(int refnum) {\n", name)
	g.Indent()
	g.Printf("if (refnum == %s.Seq.NullRefNum) { return null; }\n", rootNamespace)
	g.Printf("return new %s(refnum);\n", name)
	g.Outdent()
	g.Printf("}\n\n")

	// Fields
	for _, f := range exportedFields(s.t) {
		if !g.isSupported(f.Type()) {
			continue
		}
		propName := g.csIdentifier(f.Name())
		g.Printf("public %s %s {\n", g.csType(f.Type()), propName)
		g.Indent()
		g.Printf("get {\n")
		g.Indent()
		g.Printf("ThrowIfDisposed(); %s.Seq.ThrowIfPendingException();\n", rootNamespace)
		g.Printf("%s.Seq.IncGoRef(refnum, this);\n", rootNamespace)
		g.Printf("var res = %s.Native.%s_Get(refnum);\n", rootNamespace, g.proxyFuncName(s.obj.Name(), f.Name()))
		g.emitFromNativeReturn("res", f.Type(), true)
		g.Outdent()
		g.Printf("}\n")
		g.Printf("set {\n")
		g.Indent()
		g.Printf("ThrowIfDisposed(); %s.Seq.ThrowIfPendingException();\n", rootNamespace)
		g.Printf("%s.Seq.IncGoRef(refnum, this);\n", rootNamespace)
		g.emitToNativeParam("value", f.Type(), modeRetained)
		g.Printf("%s.Native.%s_Set(refnum, %s);\n", rootNamespace, g.proxyFuncName(s.obj.Name(), f.Name()), g.nativeParamName("value"))
		g.Outdent()
		g.Printf("}\n")
		g.Outdent()
		g.Printf("}\n\n")
	}

	// Methods
	for _, m := range exportedMethodSet(types.NewPointer(s.obj.Type())) {
		if !g.isSigSupported(m.Type()) {
			continue
		}
		sig := m.Type().(*types.Signature)
		params := sig.Params()
		res := sig.Results()

		returnsError := false
		var returnType string
		switch res.Len() {
		case 0:
			returnType = "void"
		case 1:
			if isErrorType(res.At(0).Type()) {
				returnsError = true
				returnType = "void"
			} else {
				returnType = g.csType(res.At(0).Type())
			}
		case 2:
			if isErrorType(res.At(1).Type()) {
				returnsError = true
				returnType = g.csType(res.At(0).Type())
			} else {
				g.errorf("second result must be error: %s", m.Name())
				continue
			}
		default:
			g.errorf("too many result values: %s", m.Name())
			continue
		}

		g.Printf("public %s %s(", returnType, g.csIdentifier(m.Name()))
		for i := 0; i < params.Len(); i++ {
			if i > 0 {
				g.Printf(", ")
			}
			g.Printf("%s %s", g.csType(params.At(i).Type()), g.paramName(params, i))
		}
		g.Printf(") {\n")
		g.Indent()
		g.Printf("ThrowIfDisposed(); %s.Seq.ThrowIfPendingException();\n", rootNamespace)
		g.Printf("%s.Seq.IncGoRef(refnum, this);\n", rootNamespace)
		for i := 0; i < params.Len(); i++ {
			g.emitToNativeParam(g.paramName(params, i), params.At(i).Type(), modeTransient)
		}
		cName := g.proxyFuncName(s.obj.Name(), m.Name())
		if res.Len() > 0 {
			g.Printf("var res = %s.Native.%s(refnum", rootNamespace, cName)
		} else {
			g.Printf("%s.Native.%s(refnum", rootNamespace, cName)
		}
		for i := 0; i < params.Len(); i++ {
			g.Printf(", %s", g.nativeParamName(g.paramName(params, i)))
		}
		g.Printf(");\n")
		for i := 0; i < params.Len(); i++ {
			if g.shouldFreeAfterCall(params.At(i).Type(), modeTransient) {
				g.emitFreeNativeParam(g.paramName(params, i))
			}
		}

		if res.Len() == 0 {
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if res.Len() == 1 && !returnsError {
			g.emitFromNativeReturn("res", res.At(0).Type(), true)
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if returnsError {
			if res.Len() == 2 {
				if isRefnumType(res.At(0).Type()) {
					g.Printf("if (res.r1 != %s.Seq.NullRefNum) {\n", rootNamespace)
					g.Indent()
					g.Printf("%s.Seq.DestroyRef(res.r0);\n", rootNamespace)
					g.Printf("%s.Seq.ThrowIfError(res.r1);\n", rootNamespace)
					g.Outdent()
					g.Printf("}\n")
				}
				g.emitFromNativeValue("value", "res.r0", res.At(0).Type(), true)
				if !isRefnumType(res.At(0).Type()) {
					g.Printf("%s.Seq.ThrowIfError(res.r1);\n", rootNamespace)
				}
				g.Printf("return value;\n")
			} else {
				g.Printf("%s.Seq.ThrowIfError(res);\n", rootNamespace)
			}
		}

		g.Outdent()
		g.Printf("}\n\n")
	}

	g.Outdent()
	g.Printf("}\n\n")
}

func (g *CSharpGen) genConstructorFromFunc(rootNamespace string, structName string, f *types.Func) {
	sig := f.Type().(*types.Signature)
	params := sig.Params()
	res := sig.Results()

	returnsError := false
	if res.Len() == 2 && isErrorType(res.At(1).Type()) {
		returnsError = true
	}

	g.Printf("public %s(", structName)
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			g.Printf(", ")
		}
		g.Printf("%s %s", g.csType(params.At(i).Type()), g.paramName(params, i))
	}
	g.Printf(") {\n")
	g.Indent()
	for i := 0; i < params.Len(); i++ {
		g.emitToNativeParam(g.paramName(params, i), params.At(i).Type(), modeTransient)
	}
	cName := g.proxyFuncName("", f.Name())
	g.Printf("var res = %s.Native.%s(", rootNamespace, cName)
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			g.Printf(", ")
		}
		g.Printf("%s", g.nativeParamName(g.paramName(params, i)))
	}
	g.Printf(");\n")
	for i := 0; i < params.Len(); i++ {
		if g.shouldFreeAfterCall(params.At(i).Type(), modeTransient) {
			g.emitFreeNativeParam(g.paramName(params, i))
		}
	}
	if returnsError {
		g.Printf("refnum = res.r0;\n")
		g.Printf("if (res.r1 != %s.Seq.NullRefNum) {\n", rootNamespace)
		g.Indent()
		g.Printf("%s.Seq.DestroyRef(refnum);\n", rootNamespace)
		g.Printf("%s.Seq.ThrowIfError(res.r1);\n", rootNamespace)
		g.Outdent()
		g.Printf("}\n")
	} else {
		g.Printf("refnum = res;\n")
	}
	g.Outdent()
	g.Printf("}\n\n")
}

func (g *CSharpGen) genInterface(rootNamespace string, iface interfaceInfo) {
	name := g.csInterfaceName(iface.obj)
	g.Printf("public interface %s {\n", name)
	g.Indent()
	for _, m := range iface.summary.callable {
		if !g.isSigSupported(m.Type()) {
			continue
		}
		sig := m.Type().(*types.Signature)
		params := sig.Params()
		res := sig.Results()
		returnType := "void"
		switch res.Len() {
		case 0:
			returnType = "void"
		case 1:
			if isErrorType(res.At(0).Type()) {
				returnType = "void"
			} else {
				returnType = g.csType(res.At(0).Type())
			}
		case 2:
			if isErrorType(res.At(1).Type()) {
				returnType = g.csType(res.At(0).Type())
			} else {
				g.errorf("second result must be error: %s", m.Name())
				continue
			}
		}
		g.Printf("%s %s(", returnType, g.csIdentifier(m.Name()))
		for i := 0; i < params.Len(); i++ {
			if i > 0 {
				g.Printf(", ")
			}
			g.Printf("%s %s", g.csType(params.At(i).Type()), g.paramName(params, i))
		}
		g.Printf(");\n")
	}
	g.Outdent()
	g.Printf("}\n\n")

	proxyName := g.csProxyInterfaceName(iface.obj)
	g.Printf("internal sealed class %s : %s.Seq.IProxy, %s, IDisposable {\n", proxyName, rootNamespace, name)
	g.Indent()
	g.Printf("private readonly int refnum;\n")
	g.Printf("private int disposed;\n")
	g.Printf("private static int registered;\n\n")

	g.Printf("internal %s(int refnum) { this.refnum = refnum; }\n\n", proxyName)
	g.Printf("~%s() { Dispose(false); }\n\n", proxyName)
	g.Printf("public void Dispose() {\n")
	g.Indent()
	g.Printf("Dispose(true);\n")
	g.Printf("GC.SuppressFinalize(this);\n")
	g.Outdent()
	g.Printf("}\n\n")
	g.Printf("private void Dispose(bool disposing) {\n")
	g.Indent()
	g.Printf("if (System.Threading.Interlocked.Exchange(ref disposed, 1) != 0) { return; }\n")
	g.Printf("%s.Seq.DestroyRef(refnum);\n", rootNamespace)
	g.Outdent()
	g.Printf("}\n\n")
	g.Printf("private void ThrowIfDisposed() {\n")
	g.Indent()
	g.Printf("if (disposed != 0) { throw new ObjectDisposedException(GetType().FullName); }\n")
	g.Outdent()
	g.Printf("}\n\n")
	g.Printf("public int Refnum { get { ThrowIfDisposed(); return refnum; } }\n\n")
	g.Printf("public int IncRefnum() { ThrowIfDisposed(); %s.Seq.IncGoRef(refnum, this); return refnum; }\n\n", rootNamespace)

	g.Printf("internal static %s FromRefnum(int refnum) {\n", name)
	g.Indent()
	g.Printf("if (refnum == %s.Seq.NullRefNum) { return null; }\n", rootNamespace)
	g.Printf("if (refnum < 0) { return new %s(refnum); }\n", proxyName)
	g.Printf("return (%s)%s.Seq.GetRef(refnum);\n", name, rootNamespace)
	g.Outdent()
	g.Printf("}\n\n")

	// Proxy methods calling Go
	for _, m := range iface.summary.callable {
		if !g.isSigSupported(m.Type()) {
			continue
		}
		sig := m.Type().(*types.Signature)
		params := sig.Params()
		res := sig.Results()
		returnsError := false
		returnType := "void"
		switch res.Len() {
		case 0:
			returnType = "void"
		case 1:
			if isErrorType(res.At(0).Type()) {
				returnsError = true
				returnType = "void"
			} else {
				returnType = g.csType(res.At(0).Type())
			}
		case 2:
			if isErrorType(res.At(1).Type()) {
				returnsError = true
				returnType = g.csType(res.At(0).Type())
			} else {
				g.errorf("second result must be error: %s", m.Name())
				continue
			}
		}
		g.Printf("public %s %s(", returnType, g.csIdentifier(m.Name()))
		for i := 0; i < params.Len(); i++ {
			if i > 0 {
				g.Printf(", ")
			}
			g.Printf("%s %s", g.csType(params.At(i).Type()), g.paramName(params, i))
		}
		g.Printf(") {\n")
		g.Indent()
		g.Printf("ThrowIfDisposed(); %s.Seq.ThrowIfPendingException();\n", rootNamespace)
		g.Printf("%s.Seq.IncGoRef(refnum, this);\n", rootNamespace)
		for i := 0; i < params.Len(); i++ {
			g.emitToNativeParam(g.paramName(params, i), params.At(i).Type(), modeTransient)
		}
		cName := g.proxyFuncName(iface.obj.Name(), m.Name())
		if res.Len() > 0 {
			g.Printf("var res = %s.Native.%s(refnum", rootNamespace, cName)
		} else {
			g.Printf("%s.Native.%s(refnum", rootNamespace, cName)
		}
		for i := 0; i < params.Len(); i++ {
			g.Printf(", %s", g.nativeParamName(g.paramName(params, i)))
		}
		g.Printf(");\n")
		for i := 0; i < params.Len(); i++ {
			if g.shouldFreeAfterCall(params.At(i).Type(), modeTransient) {
				g.emitFreeNativeParam(g.paramName(params, i))
			}
		}

		if res.Len() == 0 {
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if res.Len() == 1 && !returnsError {
			g.emitFromNativeReturn("res", res.At(0).Type(), true)
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if returnsError {
			if res.Len() == 2 {
				if isRefnumType(res.At(0).Type()) {
					g.Printf("if (res.r1 != %s.Seq.NullRefNum) {\n", rootNamespace)
					g.Indent()
					g.Printf("%s.Seq.DestroyRef(res.r0);\n", rootNamespace)
					g.Printf("%s.Seq.ThrowIfError(res.r1);\n", rootNamespace)
					g.Outdent()
					g.Printf("}\n")
				}
				g.emitFromNativeValue("value", "res.r0", res.At(0).Type(), true)
				if !isRefnumType(res.At(0).Type()) {
					g.Printf("%s.Seq.ThrowIfError(res.r1);\n", rootNamespace)
				}
				g.Printf("return value;\n")
			} else {
				g.Printf("%s.Seq.ThrowIfError(res);\n", rootNamespace)
			}
		}
		g.Outdent()
		g.Printf("}\n\n")
	}

	// Callback registration
	g.Printf("internal static void EnsureRegistered() {\n")
	g.Indent()
	g.Printf("if (System.Threading.Interlocked.CompareExchange(ref registered, 1, 0) != 0) { return; }\n")
	g.Printf("%s.Seq.Touch();\n", rootNamespace)
	for _, m := range iface.summary.callable {
		if !g.isSigSupported(m.Type()) {
			continue
		}
		cbName := g.csIdentifier("Callback_" + m.Name() + "Handler")
		setterName := g.cproxySetterName(g.pkgPrefix, iface.obj.Name(), m.Name())
		g.Printf("%s.Native.%s(Marshal.GetFunctionPointerForDelegate(%s));\n", rootNamespace, setterName, cbName)
	}
	g.Outdent()
	g.Printf("}\n\n")

	for _, m := range iface.summary.callable {
		if !g.isSigSupported(m.Type()) {
			continue
		}
		sig := m.Type().(*types.Signature)
		params := sig.Params()
		res := sig.Results()
		delegateType := g.csIdentifier("Callback_" + m.Name())
		delegateName := g.csIdentifier("Callback_" + m.Name() + "Handler")
		delegateImpl := g.csIdentifier("Callback_" + m.Name() + "Impl")
		returnType := "void"
		switch res.Len() {
		case 0:
			returnType = "void"
		case 1:
			returnType = g.csNativeType(res.At(0).Type())
		case 2:
			returnType = g.csReturnStructName(fmt.Sprintf("cproxy%s_%s_%s", g.pkgPrefix, iface.obj.Name(), m.Name()))
		}
		g.Printf("[UnmanagedFunctionPointer(CallingConvention.Cdecl)]\n")
		g.Printf("private delegate %s %s(int refnum", returnType, delegateType)
		for i := 0; i < params.Len(); i++ {
			g.Printf(", %s %s", g.csNativeType(params.At(i).Type()), g.paramName(params, i))
		}
		g.Printf(");\n")
		g.Printf("private static readonly %s %s = %s;\n\n", delegateType, delegateName, delegateImpl)

		g.Printf("private static %s %s(int refnum", returnType, delegateImpl)
		for i := 0; i < params.Len(); i++ {
			g.Printf(", %s %s", g.csNativeType(params.At(i).Type()), g.paramName(params, i))
		}
		g.Printf(") {\n")
		g.Indent()
		g.Printf("try {\n")
		g.Indent()
		g.Printf("var instance = (%s)%s.Seq.GetRef(refnum);\n", name, rootNamespace)
		for i := 0; i < params.Len(); i++ {
			paramName := g.paramName(params, i)
			g.emitFromNativeParam(paramName, params.At(i).Type())
		}

		callExpr := fmt.Sprintf("instance.%s(%s)", g.csIdentifier(m.Name()), g.callArgs(params))
		if res.Len() == 0 {
			g.Printf("%s;\n", callExpr)
			g.Outdent()
			g.Printf("} catch (Exception ex) {\n")
			g.Indent()
			g.Printf("%s.Seq.ReportUnhandledException(ex, \"%s.%s\");\n", rootNamespace, iface.obj.Name(), m.Name())
			g.Outdent()
			g.Printf("}\n")
			g.Printf("return;\n")
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		returnsErrorOnly := res.Len() == 1 && isErrorType(res.At(0).Type())
		returnsValueAndError := res.Len() == 2 && isErrorType(res.At(1).Type())

		if returnsErrorOnly {
			g.Printf("%s;\n", callExpr)
			g.Printf("return %s.Seq.NullRefNum;\n", rootNamespace)
			g.Outdent()
			g.Printf("} catch (Exception ex) {\n")
			g.Indent()
			g.Printf("%s.ProxyError.EnsureRegistered();\n", rootNamespace)
			g.Printf("return %s.Seq.IncRef(new %s.GoError(ex));\n", rootNamespace, rootNamespace)
			g.Outdent()
			g.Printf("}\n")
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if res.Len() == 1 {
			g.emitEnsureRegisteredForReturn(res.At(0).Type())
			g.Printf("var result = %s;\n", callExpr)
			g.emitToNativeReturn("result", res.At(0).Type())
			g.Outdent()
			g.Printf("} catch (Exception ex) {\n")
			g.Indent()
			g.Printf("%s.Seq.ReportUnhandledException(ex, \"%s.%s\");\n", rootNamespace, iface.obj.Name(), m.Name())
			g.Printf("return %s;\n", g.defaultNativeReturn(res.At(0).Type()))
			g.Outdent()
			g.Printf("}\n")
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		if returnsValueAndError {
			structName := g.csReturnStructName(fmt.Sprintf("cproxy%s_%s_%s", g.pkgPrefix, iface.obj.Name(), m.Name()))
			g.emitEnsureRegisteredForReturn(res.At(0).Type())
			g.Printf("var value = %s;\n", callExpr)
			g.Printf("var error = %s.Seq.NullRefNum;\n", rootNamespace)
			g.Printf("return new %s { r0 = %s, r1 = error };\n", structName, g.nativeValueExpression("value", res.At(0).Type()))
			g.Outdent()
			g.Printf("} catch (Exception ex) {\n")
			g.Indent()
			g.Printf("%s.ProxyError.EnsureRegistered();\n", rootNamespace)
			g.Printf("var error = %s.Seq.IncRef(new %s.GoError(ex));\n", rootNamespace, rootNamespace)
			g.Printf("return new %s { r0 = %s, r1 = error };\n", structName, g.defaultNativeReturn(res.At(0).Type()))
			g.Outdent()
			g.Printf("}\n")
			g.Outdent()
			g.Printf("}\n\n")
			continue
		}

		g.Outdent()
		g.Printf("}\n\n")
	}

	g.Outdent()
	g.Printf("}\n\n")
}

func (g *CSharpGen) emitFromNativeParam(name string, t types.Type) {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.UntypedBool:
			g.Printf("var %sValue = %s != 0;\n", name, name)
		case types.String, types.UntypedString:
			g.Printf("var %sValue = %s.Seq.NStringToString(%s);\n", name, g.rootNamespace(), name)
		default:
			g.Printf("var %sValue = (%s)%s;\n", name, g.csType(t), name)
		}
	case *types.Slice:
		g.Printf("var %sValue = %s.Seq.NBytesliceToBytes(%s, false);\n", name, g.rootNamespace(), name)
	case *types.Pointer, *types.Named:
		if named, ok := indirectNamed(t); ok {
			if _, ok := named.Underlying().(*types.Interface); ok {
				g.Printf("var %sValue = %s.FromRefnum(%s);\n", name, g.interfaceProxyQualifiedName(named), name)
			} else {
				g.Printf("var %sValue = %s.FromRefnum(%s);\n", name, g.namedQualifiedName(named), name)
			}
			return
		}
		g.Printf("var %sValue = (%s)%s;\n", name, g.csType(t), name)
	default:
		g.Printf("var %sValue = (%s)%s;\n", name, g.csType(t), name)
	}
}

func (g *CSharpGen) callArgs(params *types.Tuple) string {
	var names []string
	for i := 0; i < params.Len(); i++ {
		name := g.paramName(params, i)
		names = append(names, name+"Value")
	}
	return strings.Join(names, ", ")
}

func (g *CSharpGen) nativeValueExpression(name string, t types.Type) string {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.UntypedBool:
			return fmt.Sprintf("%s ? (byte)1 : (byte)0", name)
		case types.String, types.UntypedString:
			return fmt.Sprintf("%s.Seq.StringToNString(%s)", g.rootNamespace(), name)
		default:
			return fmt.Sprintf("(%s)%s", g.csNativeType(t), name)
		}
	case *types.Slice:
		return fmt.Sprintf("%s.Seq.BytesToNByteslice(%s)", g.rootNamespace(), name)
	case *types.Pointer, *types.Named:
		return fmt.Sprintf("%s.Seq.IncRef(%s)", g.rootNamespace(), name)
	default:
		return fmt.Sprintf("(%s)%s", g.csNativeType(t), name)
	}
}

func (g *CSharpGen) emitToNativeReturn(name string, t types.Type) {
	g.Printf("return %s;\n", g.nativeValueExpression(name, t))
}

func (g *CSharpGen) defaultNativeReturn(t types.Type) string {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.UntypedBool:
			return "0"
		case types.String, types.UntypedString:
			return "new NString()"
		default:
			return "0"
		}
	case *types.Slice:
		return "new NByteslice()"
	case *types.Pointer, *types.Named:
		return fmt.Sprintf("%s.Seq.NullRefNum", g.rootNamespace())
	default:
		return "0"
	}
}

func (g *CSharpGen) genSeqSupport() {
	g.Printf("internal static class Seq {\n")
	g.Indent()
	g.Printf("internal const int NullRefNum = 41;\n\n")

	g.Printf("private static readonly RefTracker Tracker = new RefTracker();\n")
	g.Printf("private static readonly RefCallback IncRefCallback = IncRefnum;\n")
	g.Printf("private static readonly RefCallback DecRefCallback = DecRefnum;\n\n")

	g.Printf("static Seq() {\n")
	g.Indent()
	g.Printf("Native.go_seq_init();\n")
	g.Printf("Native.go_seq_set_inc_ref(Marshal.GetFunctionPointerForDelegate(IncRefCallback));\n")
	g.Printf("Native.go_seq_set_dec_ref(Marshal.GetFunctionPointerForDelegate(DecRefCallback));\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal static void Touch() { }\n\n")
	g.Printf("internal static IntPtr Alloc(int size) { return Native.GoSeqAlloc(size); }\n")
	g.Printf("internal static void Free(IntPtr ptr) { if (ptr != IntPtr.Zero) { Native.GoSeqFree(ptr); } }\n\n")

	g.Printf("internal static NString StringToNString(string value) {\n")
	g.Indent()
	g.Printf("if (string.IsNullOrEmpty(value)) { return new NString(); }\n")
	g.Printf("var bytes = Encoding.UTF8.GetBytes(value);\n")
	g.Printf("var ptr = Alloc(bytes.Length);\n")
	g.Printf("Marshal.Copy(bytes, 0, ptr, bytes.Length);\n")
	g.Printf("return new NString { ptr = ptr, len = bytes.Length };\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal static string NStringToString(NString value) {\n")
	g.Indent()
	g.Printf("if (value.ptr == IntPtr.Zero || value.len == 0) { return string.Empty; }\n")
	g.Printf("var result = Marshal.PtrToStringUTF8(value.ptr, value.len);\n")
	g.Printf("Free(value.ptr);\n")
	g.Printf("return result ?? string.Empty;\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal static NByteslice BytesToNByteslice(byte[] value) {\n")
	g.Indent()
	g.Printf("if (value == null || value.Length == 0) { return new NByteslice(); }\n")
	g.Printf("var ptr = Alloc(value.Length);\n")
	g.Printf("Marshal.Copy(value, 0, ptr, value.Length);\n")
	g.Printf("return new NByteslice { ptr = ptr, len = value.Length };\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal static byte[] NBytesliceToBytes(NByteslice value, bool free) {\n")
	g.Indent()
	g.Printf("if (value.ptr == IntPtr.Zero || value.len == 0) { return Array.Empty<byte>(); }\n")
	g.Printf("var result = new byte[value.len];\n")
	g.Printf("Marshal.Copy(value.ptr, result, 0, value.len);\n")
	g.Printf("if (free) { Free(value.ptr); }\n")
	g.Printf("return result;\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal static int IncRef(object value) {\n")
	g.Indent()
	g.Printf("if (value == null) { return NullRefNum; }\n")
	g.Printf("if (value is IProxy proxy) { return proxy.IncRefnum(); }\n")
	g.Printf("return Tracker.Inc(value);\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal static void IncGoRef(int refnum, object keepAlive) {\n")
	g.Indent()
	g.Printf("Native.IncGoRef(refnum);\n")
	g.Printf("GC.KeepAlive(keepAlive);\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal static void DestroyRef(int refnum) { Native.DestroyRef(refnum); }\n\n")
	g.Printf("internal static object GetRef(int refnum) { return Tracker.GetRef(refnum); }\n\n")
	g.Printf("internal static void ThrowIfError(int refnum) {\n")
	g.Indent()
	g.Printf("if (refnum == NullRefNum) { return; }\n")
	g.Printf("var error = ProxyError.FromRefnum(refnum);\n")
	g.Printf("throw new GoException(error);\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("private static readonly ConcurrentQueue<(Exception ex, string method)> pendingExceptions = new ConcurrentQueue<(Exception, string)>();\n\n")
	g.Printf("/// <summary>Returns the most recent unhandled callback exception, if any.</summary>\n")
	g.Printf("public static Exception LastUnhandledException => pendingExceptions.TryPeek(out var p) ? p.ex : null;\n")
	g.Printf("/// <summary>Returns the method name of the most recent unhandled callback exception, if any.</summary>\n")
	g.Printf("public static string LastUnhandledExceptionMethod => pendingExceptions.TryPeek(out var p) ? p.method : null;\n")
	g.Printf("/// <summary>When true, calls Environment.FailFast on unhandled callback exceptions.</summary>\n")
	g.Printf("public static bool FailFastOnUnhandledCallbackException { get; set; }\n")
	g.Printf("public static Action<Exception, string> UnhandledCallbackException { get; set; }\n\n")
	g.Printf("internal static void ReportUnhandledException(Exception ex, string methodName) {\n")
	g.Indent()
	g.Printf("pendingExceptions.Enqueue((ex, methodName));\n")
	g.Printf("var handler = UnhandledCallbackException;\n")
	g.Printf("if (handler != null) { handler(ex, methodName); }\n")
	g.Printf("Console.Error.WriteLine($\"[GoMobile] Unhandled exception in callback {methodName}: {ex}\");\n")
	g.Printf("if (FailFastOnUnhandledCallbackException) { Environment.FailFast($\"Unhandled exception in Go callback {methodName}\", ex); }\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("/// <summary>Throws if a previous callback had an unhandled exception.</summary>\n")
	g.Printf("internal static void ThrowIfPendingException() {\n")
	g.Indent()
	g.Printf("if (pendingExceptions.TryDequeue(out var pending)) {\n")
	g.Indent()
	g.Printf("throw new InvalidOperationException($\"Unhandled exception in previous callback {pending.method}: {pending.ex.Message}\", pending.ex);\n")
	g.Outdent()
	g.Printf("}\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("[UnmanagedFunctionPointer(CallingConvention.Cdecl)]\n")
	g.Printf("private delegate void RefCallback(int refnum);\n\n")

	g.Printf("private static void IncRefnum(int refnum) { Tracker.IncRefnum(refnum); }\n")
	g.Printf("private static void DecRefnum(int refnum) { Tracker.DecRefnum(refnum); }\n\n")

	g.Printf("internal interface IProxy : IDisposable {\n")
	g.Indent()
	g.Printf("int Refnum { get; }\n")
	g.Printf("int IncRefnum();\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("private sealed class RefTracker {\n")
	g.Indent()
	g.Printf("private const int RefOffset = 42;\n")
	g.Printf("private int nextRefnum = RefOffset;\n")
	g.Printf("private readonly Dictionary<int, Ref> refs = new Dictionary<int, Ref>();\n")
	g.Printf("private readonly Dictionary<object, int> objectRefs = new Dictionary<object, int>(ReferenceEqualityComparer.Instance);\n\n")

	g.Printf("internal int Inc(object value) {\n")
	g.Indent()
	g.Printf("lock (refs) {\n")
	g.Indent()
	g.Printf("if (value == null) { return NullRefNum; }\n")
	g.Printf("if (!objectRefs.TryGetValue(value, out var refnum)) {\n")
	g.Indent()
	g.Printf("if (nextRefnum == int.MaxValue) { throw new InvalidOperationException(\"RefTracker: refnum overflow\"); }\n")
	g.Printf("refnum = nextRefnum++;\n")
	g.Printf("objectRefs[value] = refnum;\n")
	g.Outdent()
	g.Printf("}\n")
	g.Printf("if (!refs.TryGetValue(refnum, out var entry)) {\n")
	g.Indent()
	g.Printf("entry = new Ref(value);\n")
	g.Printf("refs[refnum] = entry;\n")
	g.Outdent()
	g.Printf("}\n")
	g.Printf("entry.Inc();\n")
	g.Printf("return refnum;\n")
	g.Outdent()
	g.Printf("}\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal void IncRefnum(int refnum) {\n")
	g.Indent()
	g.Printf("lock (refs) {\n")
	g.Indent()
	g.Printf("if (refnum <= 0 || refnum == NullRefNum) { return; }\n")
	g.Printf("if (!refs.TryGetValue(refnum, out var entry)) {\n")
	g.Indent()
	g.Printf("throw new InvalidOperationException(\"unknown reference\");\n")
	g.Outdent()
	g.Printf("}\n")
	g.Printf("entry.Inc();\n")
	g.Outdent()
	g.Printf("}\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal void DecRefnum(int refnum) {\n")
	g.Indent()
	g.Printf("lock (refs) {\n")
	g.Indent()
	g.Printf("if (refnum <= 0 || refnum == NullRefNum) { return; }\n")
	g.Printf("if (!refs.TryGetValue(refnum, out var entry)) { return; }\n")
	g.Printf("entry.Dec();\n")
	g.Printf("if (entry.Count <= 0) {\n")
	g.Indent()
	g.Printf("refs.Remove(refnum);\n")
	g.Printf("objectRefs.Remove(entry.Value);\n")
	g.Outdent()
	g.Printf("}\n")
	g.Outdent()
	g.Printf("}\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal object GetRef(int refnum) {\n")
	g.Indent()
	g.Printf("if (refnum == NullRefNum) { return null; }\n")
	g.Printf("lock (refs) {\n")
	g.Indent()
	g.Printf("if (!refs.TryGetValue(refnum, out var entry)) { throw new InvalidOperationException(\"unknown reference\"); }\n")
	g.Printf("return entry.Value;\n")
	g.Outdent()
	g.Printf("}\n")
	g.Outdent()
	g.Printf("}\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("private sealed class Ref {\n")
	g.Indent()
	g.Printf("internal Ref(object value) { Value = value; }\n")
	g.Printf("internal object Value { get; }\n")
	g.Printf("internal int Count { get; private set; }\n")
	g.Printf("internal void Inc() { if (Count == int.MaxValue) { throw new InvalidOperationException(\"Ref: refcount overflow\"); } Count++; }\n")
	g.Printf("internal void Dec() { Count--; }\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("private sealed class ReferenceEqualityComparer : IEqualityComparer<object> {\n")
	g.Indent()
	g.Printf("internal static readonly ReferenceEqualityComparer Instance = new ReferenceEqualityComparer();\n")
	g.Printf("public bool Equals(object x, object y) { return ReferenceEquals(x, y); }\n")
	g.Printf("public int GetHashCode(object obj) { return System.Runtime.CompilerServices.RuntimeHelpers.GetHashCode(obj); }\n")
	g.Outdent()
	g.Printf("}\n")

	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("public sealed class GoException : Exception {\n")
	g.Indent()
	g.Printf("public GoException(Error error) : base(error?.Error() ?? \"Go error\") { ErrorValue = error; }\n")
	g.Printf("public Error ErrorValue { get; }\n")
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("internal sealed class GoError : Error {\n")
	g.Indent()
	g.Printf("private readonly string message;\n")
	g.Printf("internal GoError(Exception ex) { message = ex.Message; }\n")
	g.Printf("public string Error() { return message; }\n")
	g.Outdent()
	g.Printf("}\n\n")
}

const csharpPreamble = gobindPreamble + `// C# bindings for Go.
//
//   autogenerated by gobind %[1]s %[2]s

`

const csharpHPreamble = gobindPreamble + `// C function headers for the Go <=> C# bridge.
//
//   autogenerated by gobind %[1]s %[2]s

#ifndef __%[3]s_WINDOWS_H__
#define __%[3]s_WINDOWS_H__

#include "seq.h"

`

const csharpCPreamble = gobindPreamble + `// C functions for the Go <=> C# bridge.
//
//   autogenerated by gobind %[1]s %[2]s

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdatomic.h>
#include "seq.h"
#include "%[3]s_windows.h"

`

func (g *CSharpGen) GenH() error {
	pkgPath := ""
	pkgName := "UNIVERSE"
	if g.Pkg != nil {
		pkgPath = g.Pkg.Path()
		pkgName = strings.ToUpper(g.Pkg.Name())
	}
	g.Printf(csharpHPreamble, g.gobindOpts(), pkgPath, pkgName)
	for _, iface := range g.interfaces {
		if !iface.summary.implementable {
			continue
		}
		for _, m := range iface.summary.callable {
			if !g.isSigSupported(m.Type()) {
				continue
			}
			g.genInterfaceMethodSignature(m, iface.obj.Name(), true, g.paramName)
			g.Printf("\n")
		}
	}
	g.Printf("#endif\n")
	if len(g.err) > 0 {
		return g.err
	}
	return nil
}

func (g *CSharpGen) GenC() error {
	pkgPath := ""
	pkgName := "universe"
	if g.Pkg != nil {
		pkgPath = g.Pkg.Path()
		pkgName = g.Pkg.Name()
	}
	g.Printf(csharpCPreamble, g.gobindOpts(), pkgPath, pkgName)
	for _, iface := range g.interfaces {
		if !iface.summary.implementable {
			continue
		}
		for _, m := range iface.summary.callable {
			if !g.isSigSupported(m.Type()) {
				continue
			}
			g.genCProxyBridge(iface.obj.Name(), m)
		}
	}
	if len(g.err) > 0 {
		return g.err
	}
	return nil
}

func (g *CSharpGen) genCProxyBridge(ifaceName string, m *types.Func) {
	sig := m.Type().(*types.Signature)
	params := sig.Params()
	res := sig.Results()
	cproxyName := fmt.Sprintf("cproxy%s_%s_%s", g.pkgPrefix, ifaceName, m.Name())
	setterName := g.cproxySetterName(g.pkgPrefix, ifaceName, m.Name())

	returnType := "void"
	if res.Len() == 1 {
		returnType = g.cgoType(res.At(0).Type())
	} else if res.Len() == 2 {
		returnType = "cproxy" + g.pkgPrefix + "_" + ifaceName + "_" + m.Name() + "_return"
	}

	g.Printf("typedef %s (*%s_fn)(int32_t refnum", returnType, cproxyName)
	for i := 0; i < params.Len(); i++ {
		g.Printf(", %s %s", g.cgoType(params.At(i).Type()), g.paramName(params, i))
	}
	g.Printf(");\n")
	g.Printf("static _Atomic(%s_fn) %s_callback = NULL;\n\n", cproxyName, cproxyName)

	g.Printf("SEQ_EXPORT void %s(%s_fn fn) {\n", setterName, cproxyName)
	g.Indent()
	g.Printf("atomic_store(&%s_callback, fn);\n", cproxyName)
	g.Outdent()
	g.Printf("}\n\n")

	g.Printf("%s %s(int32_t refnum", returnType, cproxyName)
	for i := 0; i < params.Len(); i++ {
		g.Printf(", %s %s", g.cgoType(params.At(i).Type()), g.paramName(params, i))
	}
	g.Printf(") {\n")
	g.Indent()
	g.Printf("%s_fn fn = atomic_load(&%s_callback);\n", cproxyName, cproxyName)
	g.Printf("if (fn == NULL) {\n")
	g.Indent()
	g.Printf("abort();\n")
	if res.Len() == 0 {
		g.Printf("return;\n")
	} else {
		g.Printf("%s zero;\n", returnType)
		g.Printf("memset(&zero, 0, sizeof(zero));\n")
		g.Printf("return zero;\n")
	}
	g.Outdent()
	g.Printf("}\n")
	if res.Len() == 0 {
		g.Printf("fn(refnum")
	} else {
		g.Printf("return fn(refnum")
	}
	for i := 0; i < params.Len(); i++ {
		g.Printf(", %s", g.paramName(params, i))
	}
	g.Printf(");\n")
	g.Outdent()
	g.Printf("}\n\n")
}
