package generator

import "go/types"

type enumDescription struct {
	orig    types.Type
	values  map[string]*types.Const
	isProto bool
}

// getEnumInfo пытается выяснить, представляет ли данный тип "перечисление" в смысле Go и возвращает
// его значения. Если перечислением тип не является возвращает nil.
// Критерии перечисления:
//   * один из underlying-типов является либо строкой, либо (u)intXXX
//   * в пакете имеются const-выражения с данным типом
// Так же выясняется, является ли данное перечисление "протобуфовым", для этого underying-тип должен быть int32
// и должны иметься мапы int32 -> string и обратно с определённым именем.
func (g *Generator) getEnumInfo(x types.Type) *enumDescription {
	if p, ok := x.(*types.Pointer); ok {
		return g.getEnumInfo(p.Elem())
	}

	t, ok := x.(*types.Named)
	if !ok {
		return nil
	}

	if !canBeEnum(t.Underlying()) {
		return nil
	}

	pkg := t.Obj().Pkg()
	consts := map[string]*types.Const{}
	for _, name := range pkg.Scope().Names() {
		cnst, ok := pkg.Scope().Lookup(name).(*types.Const)
		if !ok {
			continue
		}

		if cnst.Type() != t {
			continue
		}

		// мы нашли константу данного типа, пробуем
		consts[name] = cnst
	}

	if len(consts) == 0 {
		return nil
	}

	// если в скоупе содержатся переменные <name>_name и <name>_value, то это, похоже, перечисление protobuf-а
	isProto := pkg.Scope().Lookup(t.Obj().Name()+"_name") != nil && pkg.Scope().Lookup(t.Obj().Name()+"_value") != nil

	return &enumDescription{
		orig:    t,
		values:  consts,
		isProto: isProto,
	}
}
