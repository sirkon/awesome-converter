package generator

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"gitlab.stageoffice.ru/UCS-COMMON/errors"
)

type structDescription struct {
	pkg  string
	name string
}

func (d structDescription) String() string {
	return d.pkg + ":" + d.name
}

// getOrigStructs получение конвертируемых типов
func (g *Generator) getOrigStructs(descrs ...structDescription) (map[string]*types.Named, error) {
	if g.fs == nil {
		g.fs = token.NewFileSet()
	}

	var packageNames []string
	for _, pkg := range descrs {
		packageNames = append(packageNames, pkg.pkg)
	}

	pkgs, err := packages.Load(
		&packages.Config{
			Mode: packages.NeedImports | packages.NeedTypes | packages.NeedName | packages.NeedDeps |
				packages.NeedSyntax | packages.NeedFiles | packages.NeedModule,
			Fset:  g.fs,
			Tests: false,
		},
		packageNames...,
	)
	if err != nil {
		return nil, errors.Wrap(err, "parse package")
	}

	res := map[string]*types.Named{}
	for _, p := range pkgs {
		for _, descr := range descrs {
			if p.PkgPath != descr.pkg {
				continue
			}

			t := p.Types.Scope().Lookup(descr.name)
			if t == nil {
				return nil, errors.Newf("type '%s' not found in '%s'", descr.name, descr.pkg)
			}

			v := t.Type().(*types.Named)

			switch vv := v.Underlying().(type) {
			case *types.Struct:
				res[descr.String()] = v
			default:
				return nil, errors.Newf("can only process %T, but %s is %T", &types.Struct{}, descr.name, vv)
			}
		}
	}

	for _, descr := range descrs {
		if _, ok := res[descr.String()]; !ok {
			return nil, errors.Newf("package '%s' not found, please run 'go mod tidy'", descr.name)
		}
	}

	return res, nil
}
