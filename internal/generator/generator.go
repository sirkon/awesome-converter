package generator

import (
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"awesome-converter/internal/app"
	"github.com/sirkon/message"

	"gitlab.stageoffice.ru/UCS-COMMON/errors"
	"gitlab.stageoffice.ru/UCS-COMMON/matiss/v2"
)

// New конструктор генератора сущностей
func New(primPkg, primName string, secPkg, secName, method string) (*Generator, error) {
	var g Generator

	prim := structDescription{
		pkg:  primPkg,
		name: primName,
	}
	sec := structDescription{
		pkg:  secPkg,
		name: secName,
	}

	structs, err := g.getOrigStructs(prim, sec)
	if err != nil {
		return nil, errors.Wrap(err, "look for primary and secondary structs definitions")
	}

	g.prim = structs[prim.String()]
	g.sec = structs[sec.String()]
	g.method = method
	return &g, nil
}

// Generator генератор преобразований структур
type Generator struct {
	prim   *types.Named
	sec    *types.Named
	method string

	fs    *token.FileSet
	fqsec int
}

// Generate генерация кода
func (g *Generator) Generate(prj *matiss.Project) error {
	message.Infof("generate conversions between primary %s and secondary %s structures", g.prim, g.sec)

	matches, oos := g.getFieldsMatches(nil)
	missingPrim, missingSec := g.reportMatchingInfo(matches, oos)

	// вычисляем относительный путь пакета с primary-структурой
	pkgName := g.prim.Obj().Pkg()
	relPkg := strings.TrimPrefix(strings.TrimPrefix(pkgName.Path(), prj.Path()), "/")

	// вычисляем имя файла для генерируемой части конвертации
	position := g.fs.Position(g.prim.Obj().Pos())
	_, fileName := filepath.Split(position.Filename)
	fileName = strings.TrimSuffix(fileName, ".go") + "_convgen.go"

	pkg, err := prj.Package(g.prim.Obj().Pkg().Name(), relPkg)
	if err != nil {
		return errors.Wrap(err, "setup package of primary structure")
	}

	r, err := pkg.GoFile(fileName, matiss.GoAutogen(app.Name+" generate"))
	if err != nil {
		return errors.Wrap(err, "setup file to generate conversions in")
	}

	if err := g.generate(r, matches, oos, missingPrim, missingSec); err != nil {
		return errors.Wrap(err, "generate source code")
	}

	return nil
}

// generate генерация кода преобразований структур
// TODO нужно добавить в matiss/v2 поддержку "глобальных" переменных рендерера, что-то вроде
//      func (r *GoRenderer) Var(name string, value any)
//      для последующего использования в генерации под именем $<name>
//      Это позволит решить (потенциальную) проблему расхождения имён ресиверов методов, т.к.
//      на текущий момент это просто x и оно может не совпадать с названиями ресиверов в других методах.
//      Как вариант, глобальные переменные рендерера могут назначаться при его создании как опция рендерера
//      (matiss.Shy, matiss.GoAutogen и т.п.)
func (g *Generator) generate(
	r *matiss.GoRenderer,
	matches []fieldMatchInfo,
	oos []fieldSecondaryOneof,
	primMismatch bool,
	secMismatch bool,
) error {
	var secname string
	if g.sec.Obj().Pkg().Path() != g.prim.Obj().Pkg().Path() {
		r.Imports().Add(g.sec.Obj().Pkg().Path()).Ref("secpkg")
		secname = r.S("$secpkg.$0", g.sec.Obj().Name())
	} else {
		secname = g.sec.Obj().Name()
	}
	primname := g.prim.Obj().Name()
	secunder := strings.ReplaceAll(secname, ".", "_")

	if g.method != "" {
		r.L(`// $0 конвертация $1 в $2`, g.method, primname, secname)
		r.L(`func (x *$0) $1() (*$2, error) {`, primname, g.method, secname)
	} else {
		r.L(`// $0To${1|P} конвертация $0 в $2`, primname, secunder, secname)
		r.L(`func $0To${1|P}(x *$0) (*$2, error) {`, primname, secunder, secname)
	}

	r.L(`    if x == nil {`)
	r.L(`        return nil, nil`)
	r.L(`    }`)
	r.N()
	r.L(`    var res $0`, secname)

	prim := g.prim.Underlying().(*types.Struct)
	oopassed := map[string]struct{}{}
	for i := 0; i < prim.NumFields(); i++ {
		field := prim.Field(i)

		if _, ok := oopassed[field.Name()]; ok {
			// уже может быть пройдено в рамках обработки oneof
			continue
		}

		match, oomatch := g.getPrimFieldConversionDiscrs(field, matches, oos, oopassed)

		r.N()
		switch {
		case match != nil:
			if _, ok := match.descr.(*FieldMatchNoMatch); ok {
				continue
			}

			r.L(`// преобразование поля $0`, match.prim.Name())
			g.convertValue(
				r,
				"res."+match.sec.Name(),
				match.sec.Type(),
				"x."+match.prim.Name(),
				match.prim.Type(),
				match.descr,
				"field "+match.prim.Name(),
				false,
			)

		case oomatch != nil:
			r.Imports().Errors().Ref("errors")

			// поле соответствующее ветви oneof
			var oofields []string
			for _, branch := range oomatch.branches {
				oofields = append(oofields, branch.branch)
			}
			r.L(
				`// проверка корректности полей $0 соответствующих ветвям oneof-а $1 вторичной структуры`,
				strings.Join(oofields, " | "),
				oomatch.sec.Name(),
			)

			r.L(`switch {`)
			for i, b1 := range oomatch.branches[:len(oomatch.branches)-1] {
				for _, b2 := range oomatch.branches[i+1:] {
					r.L(`case x.$0 != nil && x.$1 != nil:`, b1.prim.Name(), b2.prim.Name())
					r.L(
						`    return nil, $errors.New("fields $0 and $1 refer to respective branches of oneof $2 and must not coexist")`,
						b1.branch,
						b2.branch,
						oomatch.sec.Name(),
					)
				}
			}
			r.L(`}`)
			r.L(`// преобразование полей в ветви`)
			r.L(`switch {`)
			for i, b := range oomatch.branches {
				r.L(`case x.$0 != nil:`, b.prim.Name())
				// TODO здесь понадобится шаманство с именами типов ветвей, может добавляться _ в конце
				//      разрешающий конфликты имён.
				r.L(`var branch$0 $1`, b.branch, g.safeBranch(r, b.branch))
				g.convertValue(
					r,
					"branch"+b.branch+"."+b.sec.Name(),
					b.sec.Type(),
					"x."+b.prim.Name(),
					b.prim.Type(),
					b.descr,
					"field "+b.prim.Name()+" into respective oneof branch",
					true,
				)
				r.L(`res.$0 = &branch$1`, oomatch.sec.Name(), b.branch)
				if i < len(oomatch.branches)-1 {
					r.N()
				}
			}
			r.L(`}`)
		}
	}

	if primMismatch {
		r.Imports().Errors().Ref("errors")

		r.N()
		r.L(`// есть несоответствие между полями, зовём ручную процедуру конвертации`)
		r.L(`if err := manual$0To${1|P}(x, &res); err != nil {`, primname, secunder)
		r.L(`    return nil, $errors.Wrap(err, "run user defined conversion")`)
		r.L(`}`)
	}

	r.N()
	r.L(`    return &res, nil`)
	r.L(`}`)

	r.N()
	r.L(`// ${0|P}To$1 конвертация $2 в $1`, secunder, primname, secname)
	r.L(`func ${0|P}To$1(x *$2) (*$1, error) {`, secunder, primname, secname)
	r.L(`    if x == nil {`)
	r.L(`        return nil, nil`)
	r.L(`    }`)
	r.N()
	r.L(`    var res $0`, primname)

	sec := g.sec.Underlying().(*types.Struct)

	for i := 0; i < sec.NumFields(); i++ {
		field := sec.Field(i)
		if field.Name() == "" || !field.Exported() {
			continue
		}

		match, oomatch := g.getSecFieldConversionDiscrs(field, matches, oos)
		switch {
		case match != nil:
			if _, ok := match.descr.(*FieldMatchNoMatch); ok {
				continue
			}

			// некоторые виды descr должны быть преобразованы зеркальным образом для конвертации sec -> prim
			descr := reflectDescr(match.descr)

			r.N()
			r.L(`// преобразование поля $0`, field.Name())
			g.convertValue(
				r,
				"res."+match.prim.Name(),
				match.prim.Type(),
				"x."+match.sec.Name(),
				match.sec.Type(),
				descr,
				"field "+match.sec.Name(),
				false,
			)

		case oomatch != nil:
			r.N()
			r.L(`// преобразование oneof-а $0`, field.Name())
			r.L(`switch v := x.$0.(type) {`, field.Name())
			for _, b := range oomatch.branches {
				r.L(`case *$0:`, g.safeBranch(r, b.branch))
				g.convertValue(
					r,
					"res."+b.prim.Name(),
					b.prim.Type(),
					"v."+b.branch,
					b.sec.Type(),
					reflectDescr(b.descr),
					"branch "+b.branch+" of oneof "+field.Name(),
					false,
				)
			}
			r.L(`}`)
		}
	}

	if secMismatch {
		r.Imports().Errors().Ref("errors")

		r.N()
		r.L(`// есть несоответствие между полями, зовём ручную процедуру конвертации`)
		r.L(`if err := manual${0|P}To$1(x, &res); err != nil {`, secunder, primname)
		r.L(`    return nil, $errors.Wrap(err, "run user defined conversion")`)
		r.L(`}`)
	}

	r.N()
	r.L(`    return &res, nil`)
	r.L(`}`)

	return nil
}

// при генерации метода primary -> secondary привязываемся к порядку полей в primary
// для этого бежим по полям и затем ищем соответствие в регулярных соответствиях matches и в
// соответствиях oneof (oos)
func (g *Generator) getPrimFieldConversionDiscrs(
	primfield *types.Var,
	matches []fieldMatchInfo,
	oos []fieldSecondaryOneof,
	oopassed map[string]struct{},
) (*fieldMatchInfo, *fieldSecondaryOneof) {
	var match *fieldMatchInfo
	for _, m := range matches {
		if m.prim != primfield {
			continue
		}

		m := m
		match = &m
		break
	}

	var oomatch *fieldSecondaryOneof
	if match == nil {
		for _, oo := range oos {
			for _, b := range oo.branches {
				if b.prim != primfield {
					continue
				}
			}

			for _, b := range oo.branches {
				// исключаем поля oneof из дальнейшей обработки, т.к. они все будут охвачены на последующих
				// шагах в рамках генерации
				oopassed[b.prim.Name()] = struct{}{}
			}
			oo := oo
			oomatch = &oo
			break
		}
	}

	return match, oomatch
}

func (g *Generator) getSecFieldConversionDiscrs(
	secfield *types.Var,
	matches []fieldMatchInfo,
	oos []fieldSecondaryOneof,
) (*fieldMatchInfo, *fieldSecondaryOneof) {
	// сначала ищем между соответствиями в регулярных полях
	for _, m := range matches {
		if m.sec == nil {
			break
		}

		if m.sec.Id() == secfield.Id() {
			return &m, nil
		}
	}

	// сейчас в oo-полях
	for _, oo := range oos {
		if oo.sec.Id() == secfield.Id() {
			return nil, &oo
		}
	}

	return nil, nil
}

// typeName возвращает полное имя типа с учётом размещения в разных с primary-типом пакетах
func (g *Generator) typeName(r *matiss.GoRenderer, x types.Type) string {
	switch v := x.(type) {
	case *types.Basic:
		return v.String()
	case *types.Pointer:
		return "*" + g.typeName(r, v.Elem())
	case *types.Map:
		return fmt.Sprintf("map[%s]%s", g.typeName(r, v.Key()), g.typeName(r, v.Elem()))
	case *types.Slice:
		return fmt.Sprintf("[]%s", g.typeName(r, v.Elem()))
	case *types.Named:
		if g.prim.Obj().Pkg().Path() == v.Obj().Pkg().Path() {
			return v.Obj().Name()
		}

		refname := fmt.Sprintf("pkgname%d", g.fqsec)
		g.fqsec++
		r.Imports().Add(v.Obj().Pkg().Path()).Ref(refname)
		return r.S(`$`+refname+".$0", v.Obj().Name())

	default:
		message.Fatalf("type %T is not supported for conversion", x)
		return ""
	}
}

// funcName возвращает полное имя функции преобразования с учётом размещения в разных с primary-типом пакетах
func (g *Generator) funcName(r *matiss.GoRenderer, f *types.Func) string {
	if f.Pkg().Path() == g.prim.Obj().Pkg().Path() {
		return f.Name()
	}

	refname := fmt.Sprintf("fieldpkg%d", g.fqsec)
	g.fqsec++
	r.Imports().Add(f.Pkg().Path()).Ref(refname)
	return r.S("$"+refname+".$0", f.Name())
}

func stripPointers(x types.Type) types.Type {
	switch v := x.(type) {
	case *types.Pointer:
		return v.Elem()
	default:
		return x
	}
}

// safeBranch генерирует корректное имя для структуры содержащей ветвь
func (g *Generator) safeBranch(r *matiss.GoRenderer, branch string) string {
	parent := g.typeName(r, g.sec)
	name := parent + "_" + branch
	pretender := name + "_"

	scope := g.sec.Obj().Pkg().Scope()
	if scope.Lookup(pretender) != nil {
		return pretender
	}

	return name
}

// getFuncFromPkg поиск функции с данными именем в пакете содержащем данный тип
func getFuncFromPkg(t types.Type, fname string) *types.Func {
	t = stripPointers(t)
	n := t.(*types.Named)

	scope := n.Obj().Pkg().Scope()
	return scope.Lookup(fname).(*types.Func)
}

func isPointer(x types.Type) bool {
	_, ok := x.(*types.Pointer)
	return ok
}

func is[T types.Type](x types.Type) bool {
	_, ok := x.(T)
	return ok
}

func reflectDescr(descr FieldMatchDescription) FieldMatchDescription {
	switch v := descr.(type) {
	case *FieldMatchNoMatch:
		return v
	case *FieldMatchDirect:
		return v
	case *FieldMatchConversion:
		return &FieldMatchConversion{
			MethodPrimary:        v.MethodSecondary,
			PrimaryToSecondary:   v.SecondaryToPrimary,
			PrimaryFromSecondary: v.SecondaryFromPrimary,
			MethodSecondary:      v.MethodPrimary,
			SecondaryToPrimary:   v.PrimaryToSecondary,
			SecondaryFromPrimary: v.PrimaryFromSecondary,
		}
	case *FieldMatchEnum:
		return &FieldMatchEnum{
			Primary:   v.Secondary,
			Secondary: v.Primary,
		}
	case *FieldMatchCastable:
		return v
	case *FieldMatchSlice:
		return &FieldMatchSlice{
			Elem: reflectDescr(v.Elem),
		}
	case *FieldMatchMap:
		return &FieldMatchMap{
			Key:  reflectDescr(v.Key),
			Elem: reflectDescr(v.Elem),
		}
	default:
		return nil
	}
}
