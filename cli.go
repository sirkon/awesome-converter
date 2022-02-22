package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/posener/complete"
	"github.com/sirkon/jsonexec"
	"github.com/sirkon/message"
	"github.com/willabides/kongplete"

	"gitlab.stageoffice.ru/UCS-COMMON/errors"
)

// cliArgs аргументы утилиты
type cliArgs struct {
	Version  VersionCommand  `cmd:"" help:"Print version and exit."`
	Generate GenerateCommand `cmd:"" help:"Generate conversions."`

	InstallCompletions kongplete.InstallCompletions `cmd:"" help:"Install shell completions."`
}

// RunContext контекст для работы утилиты
type RunContext struct {
	args *cliArgs
}

type structPath struct {
	needLocal bool
	pkgPath   string
	name      string
}

// UnmarshalText для кустомной обработки пути к структуре
func (p *structPath) UnmarshalText(x []byte) error {
	parts := strings.Split(string(x), ":")
	if len(parts) != 2 {
		return errors.Newf("<pkg-path>:<struct name> value required, got '%s'", string(x))
	}

	p.pkgPath = parts[0]
	p.name = parts[1]

	if p.needLocal {
		// проверка, что задан локальный пакет (относительным путём)
		if !strings.HasPrefix(p.pkgPath, "./") {
			return errors.Newf(
				"pkg-path must be set with relative path against current project — must be in the project root — got '%s'",
				p.pkgPath,
			)
		}
	}

	return nil
}

// Predict для реализации complete.Predictor
func (p *structPath) Predict(args complete.Args) []string {
	// здесь два варианта: мы уже начали искать структуру, либо пока только выбираем между пакетами
	parts := strings.Split(args.Last, ":")
	if len(parts) > 2 {
		return nil
	}

	if len(parts) == 1 {
		var pkgs []string
		if p.needLocal {
			pkgs = p.lookForLocalPackages(parts[0])
		} else {
			switch {
			case parts[0] == "":
				pkgs = p.lookForLocalPackages("")
				pkgs = append(pkgs, p.lookForOuterPackages("")...)
			case strings.HasPrefix(parts[0], "./"):
				pkgs = p.lookForLocalPackages(parts[0])
			default:
				pkgs = p.lookForOuterPackages(parts[0])
			}
		}

		switch len(pkgs) {
		case 0:
			return nil
		case 1:
			// в случае если пакет только один, то уже нужно показывать структуры из него
			return p.lookForPackageStructs(pkgs[0])
		default:
			for i, pkg := range pkgs {
				pkgs[i] = pkg + ":"
			}

			return pkgs
		}
	}

	// пакет выбрали, ищем структуры
	structs := p.lookForPackageStructs(parts[0])
	var res []string
	for _, s := range structs {
		if strings.HasPrefix(s, args.Last) {
			res = append(res, s)
		}
	}

	return res
}

// lookForLocalPackages показ путей локальных пакетов проекта
// TODO сделать независимым от нахождения в "локальном" каталоге проекта
func (p *structPath) lookForLocalPackages(prefix string) []string {
	var pkgs []string
	err := filepath.Walk(".", func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".idea":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasPrefix(path, "."+string(os.PathSeparator)) {
			path = "." + string(os.PathSeparator) + path
		}

		if !strings.HasPrefix(path, prefix) {
			return nil
		}

		if strings.HasSuffix(path, ".go") {
			dir, _ := filepath.Split(path)
			dir = strings.TrimRight(dir, string(os.PathSeparator))
			if len(pkgs) == 0 {
				pkgs = append(pkgs, dir)
			} else {
				if pkgs[len(pkgs)-1] != dir {
					pkgs = append(pkgs, dir)
				}
			}
		}

		return nil
	})
	if err != nil {
		message.Fatal(errors.Wrap(err, "look for local packages"))
	}

	return pkgs
}

func (p *structPath) lookForGoPkgsInRoot(root string) ([]string, error) {
	var pkgs []string
	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".idea":
				return filepath.SkipDir
			default:
				return nil
			}
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		dir, _ := filepath.Split(path)
		dir = strings.TrimRight(dir, string(os.PathSeparator))
		pkg := strings.TrimLeft(dir[len(root):], string(os.PathSeparator))

		pkgs = append(pkgs, pkg)
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	return pkgs, nil
}

func (p *structPath) lookForOuterPackages(prefix string) []string {
	var modInfo struct {
		Require []struct {
			Path string
		}
	}
	if err := jsonexec.Run(&modInfo, "go", "mod", "edit", "--json"); err != nil {
		message.Fatal(errors.Wrap(err, "get list of module dependencies"))
	}

	var pkgs []string
	for _, r := range modInfo.Require {
		// это может быть случай когда мы ищем уже внутри модуля, среди его пакетов
		if strings.HasPrefix(prefix, r.Path) {
			// точно, сейчас ищем в директории пакета
			var listInfo struct {
				Dir string
			}
			if err := jsonexec.Run(&listInfo, "go", "list", "--json", "-m", r.Path); err != nil {
				message.Fatal(errors.Wrap(err, "[1] look for package directory of an outer package"))
			}

			rel := strings.TrimLeft(prefix[len(r.Path):], string(os.PathSeparator))
			subpkgs, err := p.lookForGoPkgsInRoot(listInfo.Dir)
			if err != nil {
				message.Fatal(errors.Wrap(err, "[1] look for subpackages of "+r.Path))
			}

			for _, subpkg := range subpkgs {
				if strings.HasPrefix(subpkg, rel) {
					pkgs = append(pkgs, filepath.Join(r.Path, subpkg))
				}
			}

			continue
		}

		if !strings.HasPrefix(r.Path, prefix) {
			continue
		}

		// в модуле могут быть свои пакеты, находим такие
		var listInfo struct {
			Dir string
		}
		if err := jsonexec.Run(&listInfo, "go", "list", "--json", "-m", r.Path); err != nil {
			message.Fatal(errors.Wrap(err, "[1] look for package directory of an outer package"))
		}

		subpkgs, err := p.lookForGoPkgsInRoot(listInfo.Dir)
		if err != nil {
			message.Fatal(errors.Wrap(err, "[2] look for subpackages of "+r.Path))
		}

		for _, subpkg := range subpkgs {
			pkgs = append(pkgs, filepath.Join(r.Path, subpkg))
		}
	}

	sort.Strings(pkgs)
	return pkgs
}

func (p *structPath) lookForPackageStructs(pkg string) []string {
	var listInfo struct {
		Dir string
	}
	if err := jsonexec.Run(&listInfo, "go", "list", "--json", pkg); err != nil {
		message.Fatal(errors.Wrap(err, "look for package directory of "+pkg))
	}

	dirItems, err := os.ReadDir(listInfo.Dir)
	if err != nil {
		message.Fatal(errors.Wrap(err, "look for directory items"))
	}

	var structs []string
	for _, item := range dirItems {
		if item.IsDir() {
			continue
		}

		if !strings.HasSuffix(item.Name(), ".go") {
			continue
		}

		res, err := p.lookForFileStructs(listInfo.Dir, item.Name())
		if err != nil {
			message.Fatal(errors.Wrapf(err, "look for structs in %s", filepath.Join(listInfo.Dir, item.Name())))
		}

		for _, name := range res {
			structs = append(structs, pkg+":"+name)
		}
	}

	return structs
}

func (p *structPath) lookForFileStructs(dir, name string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.AllErrors)
	if err != nil {
		return nil, errors.Wrap(err, "parse file")
	}

	var structs []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch ts := node.(type) {
		case *ast.TypeSpec:
			if _, ok := ts.Type.(*ast.StructType); !ok {
				return true
			}

			structs = append(structs, ts.Name.Name)
			return false
		}

		return true
	})

	return structs, nil
}
